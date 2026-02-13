package sqlr

import (
	"context"
	"time"

	"github.com/justtrackio/gosoline/pkg/log"
	"gorm.io/gorm/logger"
)

var _ logger.Interface = &gosoLogger{}

type gosoLogger struct {
	logger log.Logger
	level  logger.LogLevel
}

func (g *gosoLogger) LogMode(level logger.LogLevel) logger.Interface {
	g.level = level

	return g
}

func (g *gosoLogger) Info(ctx context.Context, msg string, data ...any) {
	if g.level < logger.Info {
		return
	}

	g.logger.Info(ctx, msg, data...)
}

func (g *gosoLogger) Warn(ctx context.Context, msg string, data ...any) {
	if g.level < logger.Warn {
		return
	}

	g.logger.Warn(ctx, msg, data...)
}

func (g *gosoLogger) Error(ctx context.Context, msg string, data ...any) {
	if g.level < logger.Error {
		return
	}

	// log errors as warning here and let the caller decide how to handle them
	g.logger.Warn(ctx, msg, data...)
}

func (g *gosoLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	// Trace logging is intentionally not implemented.
	// To implement, check g.level and call fc() to get SQL details, then log appropriately.
	if g.level <= logger.Silent {
		return
	}
}
