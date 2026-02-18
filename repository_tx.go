package sqlr

import (
	"fmt"

	"github.com/gosoline-project/sqlc"
)

var _ RepositoryTx[int64, Entitier[int64]] = (*repositoryTx[int64, Entitier[int64]])(nil)

type RepositoryTx[K KeyTypes, E Entitier[K]] interface {
	Create(ttx TTx, entity *E) error
	// Read loads one entity by primary key. Optional functions receive a
	// QueryBuilderRead to configure joins and preloads for eager-loading
	// related entities. Schema auto-preloads are always applied.
	Read(ttx TTx, id K, opts ...func(qb *QueryBuilderRead)) (*E, error)
	Query(ttx TTx, opts ...func(qb *QueryBuilderSelect)) ([]E, error)
	Update(ttx TTx, entity *E) (*E, error)
	Delete(ttx TTx, id K) error
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

	if common, err = newRepositoryCommon[K, E](client, settings); err != nil {
		return nil, fmt.Errorf("failed to create repository common: %w", err)
	}

	return &repositoryTx[K, E]{
		repositoryCommon: common,
		client:           client,
	}, nil
}

type repositoryTx[K KeyTypes, E Entitier[K]] struct {
	repositoryCommon[K, E]
	client sqlc.Client
}

func (t *repositoryTx[K, E]) Create(ttx TTx, entity *E) error {
	if !t.hasAssociationsToSave(entity) {
		return t.createEntity(ttx, ttx, entity, &ttx)
	}

	return t.createEntityWithAssociations(ttx, ttx, entity, &ttx)
}

func (t *repositoryTx[K, E]) Read(ttx TTx, id K, opts ...func(qb *QueryBuilderRead)) (*E, error) {
	qbr := NewQueryBuilderRead()
	for _, opt := range opts {
		opt(qbr)
	}

	return t.readEntityWithOpts(ttx, ttx, id, qbr, &ttx)
}

func (t *repositoryTx[K, E]) Query(ttx TTx, opts ...func(qb *QueryBuilderSelect)) ([]E, error) {
	qb := NewQueryBuilderSelect()
	for _, opt := range opts {
		opt(qb)
	}

	return t.queryEntities(ttx, ttx, qb, &ttx)
}

func (t *repositoryTx[K, E]) Update(ttx TTx, entity *E) (*E, error) {
	return t.updateEntity(ttx, ttx, entity, &ttx)
}

func (t *repositoryTx[K, E]) Delete(ttx TTx, id K) error {
	return t.deleteEntity(ttx, ttx, id, &ttx)
}

func (t *repositoryTx[K, E]) Close() error {
	return t.statementCache.Close()
}
