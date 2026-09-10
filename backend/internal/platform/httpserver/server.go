package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func New(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}

// Serve stops accepting traffic before draining in-flight requests. Request contexts
// remain live during the grace period; a deadline forces all connections closed.
func Serve(ctx context.Context, listener net.Listener, server *http.Server, timeout time.Duration, beginDrain func()) error {
	if err := ctx.Err(); err != nil {
		_ = listener.Close()
		return err
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		beginDrain()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			_ = server.Close()
		}
		serveErr := <-done
		if err != nil {
			return fmt.Errorf("drain HTTP: %w", err)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", serveErr)
		}
		return nil
	}
}
