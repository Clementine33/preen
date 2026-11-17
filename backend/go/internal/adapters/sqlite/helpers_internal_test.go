package sqlite

import (
	"os"
	"testing"
)

// Same-package helpers for the white-box tests that remain here; the external
// test package keeps its own copies.

func newTestRepository(t *testing.T) *Repository {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "onsei-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	repo, err := NewRepository(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	return repo
}

// seedWork inserts a collected catalog fixture.
func seedWork(t *testing.T, repo *Repository, id string) {
	t.Helper()
	const stamp = "2026-09-27T10:00:00.000000000Z"
	if _, err := repo.DB().Exec(
		"INSERT INTO works (id, title, collected_at, updated_at) VALUES (?, ?, ?, ?)",
		id, "title of "+id, stamp, stamp,
	); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}
