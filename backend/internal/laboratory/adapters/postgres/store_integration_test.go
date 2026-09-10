//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"sync"
	"testing"
)

func TestLaboratoryIsASingleton(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	name := "Renamed"
	if _, err := store.Laboratory(ctx); !errors.Is(err, application.ErrNotInitialized) {
		t.Fatalf("read before setup: %v", err)
	}
	if _, err := store.Update(ctx, application.Change{Name: &name}); !errors.Is(err, application.ErrNotInitialized) {
		t.Fatalf("update before setup: %v", err)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	created := 0
	start := make(chan struct{})
	for i := range 8 {
		wg.Go(func() {
			<-start
			lab, err := domain.NewLaboratory("Lab", "", []string{"UTC", "Europe/Istanbul"}[i%2])
			if err != nil {
				t.Error(err)
				return
			}
			ok, err := store.Create(ctx, lab)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if ok {
				created++
			}
		})
	}
	close(start)
	wg.Wait()
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO misko.laboratory (singleton, name, timezone) VALUES (false, 'Second', 'UTC')"); err == nil {
		t.Fatal("database accepted a second laboratory")
	}
	code := "HHS-1"
	lab, err := store.Update(ctx, application.Change{Code: &code})
	if err != nil || lab.Code != code || lab.Name != "Lab" || lab.UpdatedAt.Before(lab.CreatedAt) {
		t.Fatalf("set code: %+v %v", lab, err)
	}
	empty := ""
	if lab, err = store.Update(ctx, application.Change{Code: &empty, Name: &name}); err != nil || lab.Code != "" || lab.Name != name {
		t.Fatalf("clear code: %+v %v", lab, err)
	}
	var null bool
	if err := pool.QueryRow(ctx, "SELECT code IS NULL FROM misko.laboratory").Scan(&null); err != nil || !null {
		t.Fatalf("cleared code stored as %v %v", null, err)
	}
	if read, err := store.Laboratory(ctx); err != nil || read != lab {
		t.Fatalf("read back: %+v %v", read, err)
	}
}
