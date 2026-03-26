package sqlr

import (
	"fmt"

	"github.com/gosoline-project/sqlc"
)

var _ RepositoryTx[int64, Entitier[int64]] = (*repositoryTx[int64, Entitier[int64]])(nil)

type RepositoryTx[K KeyTypes, E Entitier[K]] interface {
	// Create inserts the entity row and synchronizes populated associations.
	// Optional functions receive a QueryBuilderCreate to restrict, omit, or disable
	// association synchronization for this call, request post-create preloads, and
	// augment any schema-level defaults declared via relationship sqlr tags.
	Create(ttx TTx, entity *E, opts ...func(qb *QueryBuilderCreate)) error
	// Read loads one entity by primary key. Optional functions receive a
	// QueryBuilderRead to configure joins and preloads for eager-loading
	// related entities. Schema auto-preloads are always applied.
	Read(ttx TTx, id K, opts ...func(qb *QueryBuilderRead)) (*E, error)
	Query(ttx TTx, opts ...func(qb *QueryBuilderSelect)) ([]E, error)
	// Update updates the base entity row. Optional functions receive a
	// QueryBuilderUpdate to enable or restrict association synchronization for
	// this call, in addition to any schema-level defaults declared via
	// relationship sqlr tags. Many-to-many updates reconcile join-table membership
	// by default; related many-to-many rows are only updated when explicitly
	// opted in per path. When association sync is active and the schema defines
	// auto-preloads, Update reloads the entity before returning so preload-tagged
	// relations are hydrated.
	Update(ttx TTx, entity *E, opts ...func(qb *QueryBuilderUpdate)) (*E, error)
	// Delete removes the base entity row and, by default, cascades owned
	// associations. Optional functions receive a QueryBuilderDelete to restrict
	// or disable owned-association cleanup for this call. HasOne and HasMany
	// relations are recursively deleted, while ManyToMany relations only delete
	// join-table rows.
	Delete(ttx TTx, id K, opts ...func(qb *QueryBuilderDelete)) error
	// Close releases resources held by the repository, including prepared statements
	// when PreparedStatements is enabled. Returns the first error encountered, if any.
	Close() error
}

// NewRepositoryTx creates a transactional Repository with default settings.
// Note: prepared statements require a client, use NewRepositoryTxWithSettings
// to enable them.
func NewRepositoryTx[K KeyTypes, E Entitier[K]]() (RepositoryTx[K, E], error) {
	return NewRepositoryTxWithSettings[K, E](nil, DefaultSettings())
}

// NewRepositoryTxWithSettings creates a transactional Repository with custom
// settings. The client is used to prepare statements when PreparedStatements
// is enabled; it must be the same connection that transactions are opened from.
func NewRepositoryTxWithSettings[K KeyTypes, E Entitier[K]](client sqlc.Client, settings Settings) (RepositoryTx[K, E], error) {
	var err error
	var common repositoryCommon[K, E]

	if settings.PreparedStatements && client == nil {
		return nil, fmt.Errorf("prepared statements require a client")
	}

	if common, err = newRepositoryCommon[K, E](client, settings); err != nil {
		return nil, fmt.Errorf("failed to create repository common: %w", err)
	}

	return &repositoryTx[K, E]{
		repositoryCommon: common,
	}, nil
}

type repositoryTx[K KeyTypes, E Entitier[K]] struct {
	repositoryCommon[K, E]
}

func (t *repositoryTx[K, E]) Create(ttx TTx, entity *E, opts ...func(qb *QueryBuilderCreate)) error {
	if _, err := requireEntityValue(entity); err != nil {
		return err
	}

	qb := applyOptions(NewQueryBuilderCreate(), opts)
	mutationOptions := qb.mutationOptions()
	if err := t.validateCreatePreloads(qb); err != nil {
		return err
	}

	policy, err := newCreateAssociationSyncPolicy(t.schema, qb)
	if err != nil {
		return err
	}

	journal := newMutationJournal()
	hasAssociations := t.hasAssociationsToSave(entity, policy)

	if !hasAssociations {
		err = t.createEntity(ttx, ttx, entity, journal, mutationOptions)
		if err != nil {
			journal.restore()

			return err
		}

		err = t.rehydrateCreatedEntity(ttx, ttx, entity, qb, policy, false)
		if err != nil {
			journal.restore()
		}

		return err
	}

	associationCtx := newAssociationCreateContext(t.statementCache, ttx, policy, journal, mutationOptions)
	err = t.createEntityWithAssociations(ttx, associationCtx, entity)
	if err == nil {
		err = t.rehydrateCreatedEntity(ttx, ttx, entity, qb, policy, true)
	}
	if err != nil {
		journal.restore()
	}

	return err
}

func (t *repositoryTx[K, E]) Read(ttx TTx, id K, opts ...func(qb *QueryBuilderRead)) (*E, error) {
	qbr := applyOptions(NewQueryBuilderRead(), opts)

	return t.readEntityWithOpts(ttx, ttx, id, qbr)
}

func (t *repositoryTx[K, E]) Query(ttx TTx, opts ...func(qb *QueryBuilderSelect)) ([]E, error) {
	qb := applyOptions(NewQueryBuilderSelect(), opts)

	return t.queryEntities(ttx, ttx, qb)
}

func (t *repositoryTx[K, E]) Update(ttx TTx, entity *E, opts ...func(qb *QueryBuilderUpdate)) (*E, error) {
	if _, err := requireEntityValue(entity); err != nil {
		return nil, err
	}

	qb := applyOptions(NewQueryBuilderUpdate(), opts)
	mutationOptions := qb.mutationOptions()
	if err := t.validateUpdatePreloads(qb); err != nil {
		return nil, err
	}

	policy, err := newUpdateAssociationSyncPolicy(t.schema, qb)
	if err != nil {
		return nil, err
	}

	journal := newMutationJournal()

	if policy == nil || !policy.shouldSyncRootAssociations() {
		updated, err := t.updateEntity(ttx, ttx, entity, journal, mutationOptions)
		if err != nil {
			journal.restore()

			return nil, err
		}

		updated, err = t.rehydrateUpdatedEntity(ttx, ttx, updated, qb, policy)
		if err != nil {
			journal.restore()

			return nil, err
		}

		return updated, nil
	}

	associationCtx := newAssociationSyncContext(t.statementCache, ttx, policy, journal, mutationOptions)
	updated, err := t.updateEntityWithAssociations(ttx, associationCtx, entity)
	if err != nil {
		journal.restore()

		return nil, err
	}

	updated, err = t.rehydrateUpdatedEntity(ttx, ttx, updated, qb, policy)
	if err != nil {
		journal.restore()

		return nil, err
	}

	return updated, nil
}

func (t *repositoryTx[K, E]) Delete(ttx TTx, id K, opts ...func(qb *QueryBuilderDelete)) error {
	qb := applyOptions(NewQueryBuilderDelete(), opts)

	policy, err := newDeleteAssociationSyncPolicy(t.schema, qb)
	if err != nil {
		return err
	}

	if policy == nil || !policy.shouldSyncRootAssociations() || !t.hasAssociationsToDelete(policy) {
		return t.deleteEntity(ttx, ttx, id)
	}

	associationCtx := newAssociationSyncContext(t.statementCache, ttx, policy, nil, mutationOptions{})

	return t.deleteEntityWithAssociations(ttx, associationCtx, id)
}

func (t *repositoryTx[K, E]) Close() error {
	return t.statementCache.Close()
}
