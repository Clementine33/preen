package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
)

// ErrIncompatibleDatabase marks legacy, future, or wrong-mode databases.
var ErrIncompatibleDatabase = errors.New("incompatible database")

//go:embed schema.sql
var developmentSchema string

//go:embed migrations/*.sql
var releaseMigrations embed.FS

// NewRepository opens a release database and applies pending migrations.
func NewRepository(dbPath string) (*Repository, error) {
	migrations, err := fs.Sub(releaseMigrations, "migrations")
	if err != nil {
		return nil, err
	}
	return openRepository(dbPath, false, migrations)
}

// NewDevelopmentRepository initializes empty development databases only.
// Existing tables are left as edited; rebuild after changing schema.sql.
func NewDevelopmentRepository(dbPath string) (*Repository, error) {
	return openRepository(dbPath, true, nil)
}

func openRepository(dbPath string, development bool, migrations fs.FS) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath+connectionPragmas)
	if err != nil {
		return nil, err
	}
	if err := prepareSchema(db, dbPath, development, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func prepareSchema(db *sql.DB, dbPath string, development bool, migrations fs.FS) error {
	tables, err := userTables(db)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		if err := initializeEmptyFile(db); err != nil {
			return err
		}
	}
	if len(tables) == 0 && development {
		return initSchema(db)
	}
	if len(tables) > 0 {
		mode := "release"
		if development {
			mode = "development"
		}
		// goose's zero-version table survives a failed initial transaction.
		onlyGoose := len(tables) == 1
		_, hasGoose := tables["goose_db_version"]
		if development || !onlyGoose || !hasGoose {
			if err := checkDatabaseMode(db, dbPath, mode); err != nil {
				return err
			}
		}
		if !development && !hasGoose {
			return fmt.Errorf(
				"%w: %s has no migration history; use a new ONSEI_DATA_DIR",
				ErrIncompatibleDatabase,
				dbPath,
			)
		}
	}
	if development {
		return nil
	}
	return migrateRelease(db, dbPath, migrations)
}

func initializeEmptyFile(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err = conn.ExecContext(ctx, "PRAGMA auto_vacuum = INCREMENTAL"); err != nil {
		return err
	}
	// Persist the empty file's mode before migrations can use other connections.
	_, err = conn.ExecContext(ctx, "VACUUM")
	return err
}

func checkDatabaseMode(db *sql.DB, dbPath, expected string) error {
	var mode string
	if err := db.QueryRow("SELECT value FROM schema_meta WHERE key = 'database_mode'").Scan(&mode); err != nil {
		return fmt.Errorf(
			"%w: %s has no database mode (legacy database); use a new ONSEI_DATA_DIR or rebuild .dev_data: %w",
			ErrIncompatibleDatabase,
			dbPath,
			err,
		)
	}
	if mode != expected {
		return fmt.Errorf("%w: %s is %q, expected %q; development and release databases are separate",
			ErrIncompatibleDatabase, dbPath, mode, expected)
	}
	return nil
}

func userTables(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = struct{}{}
	}
	return tables, rows.Err()
}

func initSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(developmentSchema); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO schema_meta (key, value) VALUES ('database_mode', 'development')"); err != nil {
		return err
	}
	return tx.Commit()
}
