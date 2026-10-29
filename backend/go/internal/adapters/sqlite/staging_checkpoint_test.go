//nolint:testpackage // Driver instrumentation pauses a committed transaction without changing production hooks.
package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"
)

func pauseAfterFirstStagingChunk(repo *Repository, checkpoint func()) error {
	original := repo.db
	wrapped := stagingCheckpointDriver{Driver: original.Driver(), checkpoint: sync.OnceFunc(checkpoint)}
	repo.db = sql.OpenDB(repositoryConnector{driver: wrapped, dsn: repo.dbPath + connectionPragmas})
	return original.Close()
}

type stagingCheckpointDriver struct {
	driver.Driver

	checkpoint func()
}

func (d stagingCheckpointDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := d.Driver.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &stagingCheckpointConn{Conn: conn, checkpoint: d.checkpoint}, nil
}

type stagingCheckpointConn struct {
	driver.Conn

	checkpoint  func()
	transaction *stagingCheckpointTx
}

func (c *stagingCheckpointConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, fmt.Errorf("staging checkpoint requires ConnBeginTx")
	}
	tx, err := beginner.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	c.transaction = &stagingCheckpointTx{Tx: tx, conn: c}
	return c.transaction, nil
}

func (c *stagingCheckpointConn) ExecContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	executor, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, fmt.Errorf("staging checkpoint requires ExecerContext")
	}
	result, err := executor.ExecContext(ctx, query, args)
	if err == nil && c.transaction != nil && strings.Contains(query, "INSERT INTO plan_roots") {
		c.transaction.roots = true
	}
	return result, err
}

type stagingCheckpointTx struct {
	driver.Tx

	conn  *stagingCheckpointConn
	roots bool
}

func (tx *stagingCheckpointTx) Commit() error {
	err := tx.Tx.Commit()
	tx.conn.transaction = nil
	if err == nil && tx.roots {
		tx.conn.checkpoint()
	}
	return err
}

func (tx *stagingCheckpointTx) Rollback() error {
	err := tx.Tx.Rollback()
	tx.conn.transaction = nil
	return err
}
