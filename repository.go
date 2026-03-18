package sqlr

import (
	"context"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

var _ Repository[int64, Entitier[int64]] = (*repository[int64, Entitier[int64]])(nil)

// Repository provides CRUD and query operations for an entity type. Create
// synchronizes populated relationship fields together with the base entity row
// and can limit or omit association paths via QueryBuilderCreate. Update
// persists the base entity row by default and can explicitly synchronize
// associations via QueryBuilderUpdate. Delete only removes the base entity row. Read and Query are relation-aware for loading: both support joins for direct
// HasOne/HasMany/BelongsTo relations and preloads for
// HasOne/HasMany/BelongsTo/ManyToMany; both execute schema auto-preloads (db
// tag option "preload"), including nested preload paths. Read uses
// QueryBuilderRead (restricted to relation-loading methods) while Query uses
// QueryBuilderSelect (full query capabilities).
type Repository[K KeyTypes, E Entitier[K]] interface {
	// Create inserts the entity row and synchronizes populated associations.
	// Optional functions receive a QueryBuilderCreate to restrict or omit
	// association synchronization for this call.
	Create(ctx context.Context, entity *E, opts ...func(qb *QueryBuilderCreate)) error
	// Read loads one entity by primary key. Optional functions receive a
	// QueryBuilderRead to configure joins and preloads for eager-loading
	// related entities. Schema auto-preloads are always applied.
	Read(ctx context.Context, id K, opts ...func(qb *QueryBuilderRead)) (*E, error)
	// Query loads entities by applying the given option functions to a fresh
	// QueryBuilderSelect. Each option receives the builder and may call Where,
	// Limit, OrderBy, Preload, LeftJoin, etc. When no options are provided the
	// query selects all rows (with auto-preloads still applied).
	Query(ctx context.Context, opts ...func(qb *QueryBuilderSelect)) ([]E, error)
	// Update updates the base entity row. Optional functions receive a
	// QueryBuilderUpdate to enable or restrict association synchronization for
	// this call. Many-to-many updates reconcile join-table membership by default;
	// related many-to-many rows are only updated when explicitly opted in per path.
	Update(ctx context.Context, entity *E, opts ...func(qb *QueryBuilderUpdate)) (*E, error)
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

func (r *repository[K, E]) Create(ctx context.Context, entity *E, opts ...func(qb *QueryBuilderCreate)) error {
	if _, err := requireEntityValue(entity); err != nil {
		return err
	}

	qb := applyOptions(NewQueryBuilderCreate(), opts)
	mutationOptions := qb.mutationOptions()

	policy, err := newCreateAssociationSyncPolicy(r.schema, qb)
	if err != nil {
		return err
	}

	journal := newMutationJournal()

	if !r.hasAssociationsToSave(entity, policy) {
		err = r.createEntity(r.client, ctx, entity, journal, mutationOptions)
		if err != nil {
			journal.restore()
		}

		return err
	}

	err = r.client.WithTx(ctx, func(tx sqlc.Tx) error {
		associationCtx := newAssociationCreateContext(r.statementCache, tx, policy, journal, mutationOptions)

		return r.createEntityWithAssociations(ctx, associationCtx, entity)
	})
	if err != nil {
		journal.restore()
	}

	return err
}

func (r *repository[K, E]) Read(ctx context.Context, id K, opts ...func(qb *QueryBuilderRead)) (*E, error) {
	qbr := applyOptions(NewQueryBuilderRead(), opts)

	return r.readEntityWithOpts(r.client, ctx, id, qbr)
}

func (r *repository[K, E]) Query(ctx context.Context, opts ...func(qb *QueryBuilderSelect)) ([]E, error) {
	qb := applyOptions(NewQueryBuilderSelect(), opts)

	return r.queryEntities(r.client, ctx, qb)
}

func (r *repository[K, E]) Update(ctx context.Context, entity *E, opts ...func(qb *QueryBuilderUpdate)) (*E, error) {
	if _, err := requireEntityValue(entity); err != nil {
		return nil, err
	}

	qb := applyOptions(NewQueryBuilderUpdate(), opts)
	mutationOptions := qb.mutationOptions()

	var err error
	policy, err := newUpdateAssociationSyncPolicy(r.schema, qb)
	if err != nil {
		return nil, err
	}

	journal := newMutationJournal()

	if !qb.shouldSyncAssociations() {
		updated, err := r.updateEntity(r.client, ctx, entity, journal, mutationOptions)
		if err != nil {
			journal.restore()

			return nil, err
		}

		return updated, nil
	}

	var updated *E

	err = r.client.WithTx(ctx, func(tx sqlc.Tx) error {
		associationCtx := newAssociationSyncContext(r.statementCache, tx, policy, journal, mutationOptions)
		updated, err = r.updateEntityWithAssociations(ctx, associationCtx, entity)

		return err
	})
	if err != nil {
		journal.restore()

		return nil, err
	}

	return updated, nil
}

func (r *repository[K, E]) Delete(ctx context.Context, id K) error {
	return r.deleteEntity(r.client, ctx, id)
}

func (r *repository[K, E]) Close() error {
	return r.statementCache.Close()
}
