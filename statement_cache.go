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

func (c *statementCache) Exec(ctx context.Context, sqler sqlc.Sqler, q sqlc.Querier) (rows *sqlc.Rows, result sqlc.Result, err error) {
	return c.do(ctx, sqler, q,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			res, err := q.Exec(ctx, sql, args...)

			return nil, res, err
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			res, err := stmt.ExecContext(ctx, args...)

			return nil, res, err
		},
	)
}

func (c *statementCache) Get(ctx context.Context, sqler sqlc.Sqler, q sqlc.Querier, dest any) error {
	_, _, err := c.do(ctx, sqler, q,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, q.Get(ctx, dest, sql, args...)
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, stmt.GetContext(ctx, dest, args...)
		},
	)

	return err
}

func (c *statementCache) Query(ctx context.Context, sqler sqlc.Sqler, q sqlc.Querier) (*sqlc.Rows, error) {
	rows, _, err := c.do(ctx, sqler, q,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			rows, err := q.Query(ctx, sql, args...)

			return rows, nil, err
		},
		func(stmt *sqlc.Stmt, args []any) (*sqlc.Rows, sqlc.Result, error) {
			rows, err := stmt.QueryContext(ctx, args...)

			return rows, nil, err
		},
	)

	return rows, err
}

func (c *statementCache) Select(ctx context.Context, sqler sqlc.Sqler, q sqlc.Querier, dest any) error {
	_, _, err := c.do(ctx, sqler, q,
		func(sql string, args []any) (*sqlc.Rows, sqlc.Result, error) {
			return nil, nil, q.Select(ctx, dest, sql, args...)
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
	q sqlc.Querier,
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

	var stmt *sqlc.Stmt
	var ok bool

	c.Lock()
	if stmt, ok = c.cache[sql]; !ok {
		if stmt, err = c.client.Prepare(ctx, sql); err != nil {
			c.Unlock()

			return nil, nil, fmt.Errorf("failed to prepare statement: %w", err)
		}

		c.cache[sql] = stmt
	}
	c.Unlock()

	if tx, ok := q.(sqlc.Tx); ok {
		stmt = stmt.WithTx(ctx, tx.SQLTx())
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
