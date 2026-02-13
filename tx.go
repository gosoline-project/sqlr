package sqlr

import (
	"github.com/gosoline-project/sqlc"
)

// TTx wraps a sqlc.Tx, providing both a context (since sqlc.Tx embeds
// context.Context) and the underlying querier for executing SQL within a
// transaction.
type TTx struct {
	sqlc.Tx
}

// NewTx creates a new TTx wrapping the given sqlc.Tx.
func NewTx(tx sqlc.Tx) TTx {
	return TTx{
		Tx: tx,
	}
}
