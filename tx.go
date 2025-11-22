package sqlr

import (
	"context"

	"gorm.io/gorm"
)

type TTx struct {
	context.Context
	db *gorm.DB
}

func NewTx(ctx context.Context, db *gorm.DB) TTx {
	return TTx{
		Context: ctx,
		db:      db,
	}
}
