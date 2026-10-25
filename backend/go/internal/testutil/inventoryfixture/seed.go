// Package inventoryfixture seeds stored inventory facts for tests without exposing its SQL shape.
package inventoryfixture

import (
	"database/sql"
	"path"
	"strings"
	"testing"
)

// Row describes a path and its optional stored facts. RootPath defaults to its parent.
type Row struct {
	Path, RootPath    string
	IsDir, IsWorkRoot bool
	Size, Mtime       int64
	Format            string
	ContentRev        int
	Bitrate           *int32
	Width, Height     *int
	DurationSec       *float64
	CreatedAt         int64
}

// SeedInventory inserts rows and missing ancestor directories without replacing existing ancestors.
func SeedInventory(t testing.TB, repo interface{ DB() *sql.DB }, rows ...Row) {
	t.Helper()
	tx, err := repo.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, row := range rows {
		parent, name := split(row.Path)
		if row.RootPath == "" {
			row.RootPath = parent
		}
		for dir := parent; dir != row.RootPath && strings.HasPrefix(dir, strings.TrimSuffix(row.RootPath, "/")+"/"); {
			ancestor, basename := split(dir)
			if _, err = tx.Exec(
				`INSERT INTO entries(path,root_path,parent_path,name,is_dir) VALUES(?,?,?,?,1) ON CONFLICT(path) DO NOTHING`,
				dir,
				row.RootPath,
				ancestor,
				basename,
			); err != nil {
				t.Fatal(err)
			}
			if ancestor == dir {
				break
			}
			dir = ancestor
		}
		if row.ContentRev == 0 {
			row.ContentRev = 1
		}
		_, err = tx.Exec(
			`INSERT INTO entries(path,root_path,parent_path,name,is_dir,is_work_root,size,mtime,format,content_rev,bitrate,width,height,duration_sec,created_at)
			VALUES(?,?,?,?,?,?,?,?,NULLIF(?,''),?,?,?,?,?,COALESCE(NULLIF(?,0),unixepoch()))
			ON CONFLICT(path) DO UPDATE SET root_path=excluded.root_path,parent_path=excluded.parent_path,name=excluded.name,is_dir=excluded.is_dir,is_work_root=excluded.is_work_root,size=excluded.size,mtime=excluded.mtime,format=excluded.format,content_rev=excluded.content_rev,bitrate=excluded.bitrate,width=excluded.width,height=excluded.height,duration_sec=excluded.duration_sec,created_at=excluded.created_at`,
			row.Path,
			row.RootPath,
			parent,
			name,
			row.IsDir,
			row.IsWorkRoot,
			row.Size,
			row.Mtime,
			row.Format,
			row.ContentRev,
			row.Bitrate,
			row.Width,
			row.Height,
			row.DurationSec,
			row.CreatedAt,
		)
		if err != nil {
			t.Fatalf("seed inventory %s: %v", row.Path, err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func split(value string) (string, string) {
	before, _, ok := strings.CutLast(value, "/")
	if !ok {
		return ".", value
	}
	parent := before
	if parent == "" || strings.HasSuffix(parent, ":") {
		parent += "/"
	}
	return parent, path.Base(value)
}

// ClearBitrates makes a copied inventory require probing without changing its other stored facts.
func ClearBitrates(t testing.TB, repo interface{ DB() *sql.DB }) {
	t.Helper()
	if _, err := repo.DB().Exec("UPDATE entries SET bitrate = NULL WHERE is_dir = 0"); err != nil {
		t.Fatal(err)
	}
}
