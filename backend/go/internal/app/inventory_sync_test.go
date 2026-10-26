package app_test

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/app"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

func TestExactSyncWakesWiredMediaFactsAfterCommit(t *testing.T) {
	dir := t.TempDir()
	repo, err := sqlite.NewRepository(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	cover := filepath.Join(dir, "cover.png")
	file, err := os.Create(cover)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 32, 24))); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cover)
	if err != nil {
		t.Fatal(err)
	}
	pass := app.WiredMediaFacts(repo, dir)
	changed, unsubscribe := pass.Subscribe()
	defer unsubscribe()
	<-changed
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		pass.Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	if err = repo.SyncObservedInventory(filepath.ToSlash(dir), nil, []inventory.InventoryFile{
		{Path: filepath.ToSlash(cover), Size: info.Size(), Mtime: info.ModTime().Unix()},
	}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("exact sync did not wake the wired media facts pass")
	}
	entry, found, err := repo.EntryByPath(filepath.ToSlash(cover))
	if err != nil || !found || entry.Width == nil || entry.Height == nil || *entry.Width != 32 || *entry.Height != 24 {
		t.Fatalf("measured cover = %+v, found = %v, err = %v", entry, found, err)
	}
}
