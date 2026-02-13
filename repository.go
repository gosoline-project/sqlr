package sqlr

import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

var _ Repository[int64, Entitier[int64]] = (*repository[int64, Entitier[int64]])(nil)

// Repository provides CRUD and query operations for an entity type.
// Create/Update/Delete only persist the base entity table and are not
// relationship-aware (no cascade writes/deletes). Read and Query are
// relation-aware for loading: both support joins for direct HasOne/HasMany/
// BelongsTo relations and preloads for HasOne/HasMany/BelongsTo/ManyToMany;
// both execute schema auto-preloads (db tag option "preload"), including nested
// preload paths. Read uses QueryBuilderRead (restricted to relation-loading
// methods) while Query uses QueryBuilderSelect (full query capabilities).
type Repository[K KeyTypes, E Entitier[K]] interface {
	// Create inserts the base entity row only; related entities are ignored.
	Create(ctx context.Context, entity *E) error
	// Read loads one entity by primary key. Optional functions receive a
	// QueryBuilderRead to configure joins and preloads for eager-loading
	// related entities. Schema auto-preloads are always applied.
	Read(ctx context.Context, id K, opts ...func(qb *QueryBuilderRead)) (*E, error)
	// Query loads entities by applying the given option functions to a fresh
	// QueryBuilderSelect. Each option receives the builder and may call Where,
	// Limit, OrderBy, Preload, LeftJoin, etc. When no options are provided the
	// query selects all rows (with auto-preloads still applied).
	Query(ctx context.Context, opts ...func(qb *QueryBuilderSelect)) ([]E, error)
	// Update updates the base entity row only; related entities are ignored.
	Update(ctx context.Context, entity *E) (*E, error)
	// Delete removes the base entity row only; related entities are not cascaded.
	Delete(ctx context.Context, id K) error
	// Close releases resources held by the repository, including prepared statements
	// when PreparedStatements is enabled. Returns the first error encountered, if any.
	Close() error
}

// NewRepository creates a non-transactional Repository using a sqlc client
// resolved from the given gosoline configuration.
func NewRepository[K KeyTypes, E Entitier[K]](ctx context.Context, config cfg.Config, logger log.Logger, name string) (Repository[K, E], error) {
	return NewRepositoryWithSettings[K, E](ctx, config, logger, name, DefaultSettings())
}

// NewRepositoryWithSettings creates a non-transactional Repository with custom
// settings using a sqlc client resolved from the given gosoline configuration.
func NewRepositoryWithSettings[K KeyTypes, E Entitier[K]](ctx context.Context, config cfg.Config, logger log.Logger, name string, settings Settings) (Repository[K, E], error) {
	var err error
	var client sqlc.Client

	if client, err = sqlc.ProvideClient(ctx, config, logger, name); err != nil {
		return nil, fmt.Errorf("failed to initialize sqlc client: %w", err)
	}

	return NewRepositoryWithInterfaces[K, E](client, settings)
}

// NewRepositoryWithInterfaces creates a non-transactional Repository backed by
// the provided sqlc client.
func NewRepositoryWithInterfaces[K KeyTypes, E Entitier[K]](client sqlc.Client, settings Settings) (Repository[K, E], error) {
	var err error
	var common repositoryCommon[K, E]

	if common, err = newRepositoryCommon[K, E](client, settings); err != nil {
		return nil, fmt.Errorf("failed to create repository common: %w", err)
	}

	return &repository[K, E]{
		repositoryCommon: common,
		client:           client,
	}, nil
}

type repository[K KeyTypes, E Entitier[K]] struct {
	repositoryCommon[K, E]
	client sqlc.Client
}

func (r *repository[K, E]) Create(ctx context.Context, entity *E) error {
	return r.createEntity(r.client, ctx, entity, nil)
}

func (r *repository[K, E]) Read(ctx context.Context, id K, opts ...func(qb *QueryBuilderRead)) (*E, error) {
	qbr := NewQueryBuilderRead()
	for _, opt := range opts {
		opt(qbr)
	}

	return r.readEntityWithOpts(r.client, ctx, id, qbr, nil)
}

func (r *repository[K, E]) Query(ctx context.Context, opts ...func(qb *QueryBuilderSelect)) ([]E, error) {
	qb := NewQueryBuilderSelect()
	for _, opt := range opts {
		opt(qb)
	}

	return r.queryEntities(r.client, ctx, qb, nil)
}

func (r *repository[K, E]) Update(ctx context.Context, entity *E) (*E, error) {
	return r.updateEntity(r.client, ctx, entity, nil)
}

func (r *repository[K, E]) Delete(ctx context.Context, id K) error {
	return r.deleteEntity(r.client, ctx, id, nil)
}

func (r *repository[K, E]) Close() error {
	return r.statementCache.Close()
}
