package sqlr

import (
	"context"
	"database/sql"

	"github.com/gosoline-project/sqlc"
	"gorm.io/gorm"
)

type wrapper interface {
	gorm.ConnPool
	gorm.TxBeginner
}

type connPoolWrapper struct {
	client sqlc.Client
}

func newConnPoolWrapper(client sqlc.Client) wrapper {
	return &connPoolWrapper{
		client: client,
	}
}

func (c *connPoolWrapper) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	var err error
	var tx sqlc.Tx

	if tx, err = c.client.BeginTx(ctx, opts); err != nil {
		return nil, err
	}

	return tx.SqlTx().Tx, nil
}

func (c *connPoolWrapper) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.client.Exec(ctx, query, args...)
}

func (c *connPoolWrapper) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	var err error
	var stmt *sqlc.Stmt

	if stmt, err = c.client.Prepare(ctx, query); err != nil {
		return nil, err
	}

	return stmt.Stmt, nil
}

func (c *connPoolWrapper) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var err error
	var rows *sqlc.Rows

	if rows, err = c.client.Query(ctx, query, args...); err != nil {
		return nil, err
	}

	return rows.Rows, err
}

func (c *connPoolWrapper) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.client.QueryRow(ctx, query, args...)
}
