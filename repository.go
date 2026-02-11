package sqlr

import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var _ Repository[int64, Entitier[int64]] = (*repository[int64, Entitier[int64]])(nil)

type Repository[K KeyTypes, E Entitier[K]] interface {
	Create(ctx context.Context, entity *E) error
	Read(ctx context.Context, id K) (*E, error)
	Query(ctx context.Context, qb *QueryBuilderSelect) ([]E, error)
	Update(ctx context.Context, entity *E) (*E, error)
	Delete(ctx context.Context, id K) error
}

func NewRepository[K KeyTypes, E Entitier[K]](ctx context.Context, config cfg.Config, logger log.Logger, name string) (Repository[K, E], error) {
	var err error
	var client sqlc.Client
	var db *gorm.DB

	if client, err = sqlc.ProvideClient(ctx, config, logger, name); err != nil {
		return nil, fmt.Errorf("failed to initialize sqlc client: %w", err)
	}

	dialector := mysql.New(mysql.Config{
		Conn: newConnPoolWrapper(client),
	})

	if db, err = gorm.Open(dialector, &gorm.Config{}); err != nil {
		return nil, fmt.Errorf("failed to initialize gorm db: %w", err)
	}

	return &repository[K, E]{
		db: db,
	}, nil
}

func NewRepositoryWithInterfaces[K KeyTypes, E Entitier[K]](db *gorm.DB) Repository[K, E] {
	return &repository[K, E]{
		db: db,
	}
}

type repository[K KeyTypes, E Entitier[K]] struct {
	db *gorm.DB
}

func (r *repository[K, E]) Create(ctx context.Context, entity *E) error {
	return createEntity[K, E](r.db, ctx, entity)
}

func (r *repository[K, E]) Read(ctx context.Context, id K) (*E, error) {
	return readEntity[K, E](r.db, ctx, id)
}

func (r *repository[K, E]) Query(ctx context.Context, qb *QueryBuilderSelect) ([]E, error) {
	return queryEntities[K, E](r.db, ctx, qb)
}

func (r *repository[K, E]) Update(ctx context.Context, entity *E) (*E, error) {
	return updateEntity[K, E](r.db, ctx, entity)
}

func (r *repository[K, E]) Delete(ctx context.Context, id K) error {
	return deleteEntity[K, E](r.db, ctx, id)
}
