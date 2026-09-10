package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownDrainsInFlightRequest(t *testing.T) {
	entered, release, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "finished")
	}))
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, server, time.Second, func() { close(draining) }) }()
	response := make(chan error, 1)
	go func() {
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://" + listener.Addr().String())
		if e == nil {
			defer r.Body.Close()
			var body []byte
			body, e = io.ReadAll(r.Body)
			if string(body) != "finished" {
				e = io.ErrUnexpectedEOF
			}
		}
		response <- e
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter")
	}
	cancel()
	select {
	case <-draining:
	case <-time.After(3 * time.Second):
		t.Fatal("drain not announced")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before request completed: %v", err)
	default:
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownDeadlineClosesRequests(t *testing.T) {
	entered, requestDone := make(chan struct{}), make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(requestDone) }))
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, server, 20*time.Millisecond, func() {}) }()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://" + listener.Addr().String())
		if e == nil {
			r.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected shutdown deadline error")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request context not cancelled")
	}
	<-clientDone
}
