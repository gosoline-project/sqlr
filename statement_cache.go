package sqlr

import (
	"context"
	"fmt"
	"sync"

	"github.com/gosoline-project/sqlc"
)

type statementCache struct {
	sync.Mutex

	enabled bool
	client  sqlc.Client
	cache   map[string]*sqlc.Stmt
}

func newStatementCache(client sqlc.Client, enabled bool) *statementCache {
	return &statementCache{
		enabled: enabled,
		client:  client,
		cache:   make(map[string]*sqlc.Stmt),
	}
}

func (c *statementCache) Exec(ctx context.Context, sqler sqlc.Sqler, ttx *TTx) (rows *sqlc.Rows, result sqlc.Result, err error) {
	return c.do(ctx, sqler, ttx,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			res, err := c.client.Exec(ctx, sql, args...)

			return nil, res, err
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			res, err := stmt.ExecContext(ctx, args...)

			return nil, res, err
		},
	)
}

func (c *statementCache) Get(ctx context.Context, sqler sqlc.Sqler, ttx *TTx, dest any) error {
	_, _, err := c.do(ctx, sqler, ttx,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, c.client.Get(ctx, dest, sql, args...)
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, stmt.GetContext(ctx, dest, args...)
		},
	)

	return err
}

func (c *statementCache) Query(ctx context.Context, sqler sqlc.Sqler, ttx *TTx) (*sqlc.Rows, error) {
	rows, _, err := c.do(ctx, sqler, ttx,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			rows, err := c.client.Query(ctx, sql, args...)

			return rows, nil, err
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			rows, err := stmt.QueryxContext(ctx, args...)

			return rows, nil, err
		},
	)

	return rows, err
}

func (c *statementCache) Select(ctx context.Context, sqler sqlc.Sqler, ttx *TTx, dest any) error {
	_, _, err := c.do(ctx, sqler, ttx,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, c.client.Select(ctx, dest, sql, args...)
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, stmt.SelectContext(ctx, dest, args...)
		},
	)

	return err
}

func (c *statementCache) do(
	ctx context.Context,
	sqler sqlc.Sqler,
	ttx *TTx,
	doDirect func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error),
	doPrepared func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error),
) (rows *sqlc.Rows, result sqlc.Result, err error) {
	var sql string
	var args []any

	if sql, args, err = sqler.ToSql(); err != nil {
		return nil, nil, fmt.Errorf("failed to convert sqler to SQL: %w", err)
	}

	if !c.enabled {
		return doDirect(sql, args)
	}

	c.Lock()
	if _, ok := c.cache[sql]; !ok {
		if c.cache[sql], err = c.client.Prepare(ctx, sql); err != nil {
			c.Unlock()

			return nil, nil, fmt.Errorf("failed to prepare statement: %w", err)
		}
	}

	stmt := c.cache[sql]
	c.Unlock()

	if ttx != nil {
		stmt = ttx.SqlTx().StmtxContext(ctx, c.cache[sql])
	}

	return doPrepared(stmt, args)
}

func (c *statementCache) Close() error {
	c.Lock()
	defer c.Unlock()

	for _, stmt := range c.cache {
		if err := stmt.Close(); err != nil {
			return fmt.Errorf("failed to close prepared statement: %w", err)
		}
	}

	return nil
}
