package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"

	"github.com/pressly/goose/v3"
)

func migrateRelease(db *sql.DB, dbPath string, migrations fs.FS) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return err
	}
	ctx := context.Background()
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return err
	}
	if current > target {
		return fmt.Errorf(
			"%w: %s is migration %d, this build supports %d",
			ErrIncompatibleDatabase,
			dbPath,
			current,
			target,
		)
	}
	pending, err := provider.HasPending(ctx)
	if err != nil || !pending {
		return err
	}
	if current > 0 {
		backup, err := backupDatabase(db, dbPath)
		if err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
		log.Printf("database backup before migration %d -> %d: %s", current, target, backup)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate %s to %d: %w", dbPath, target, err)
	}
	return nil
}
