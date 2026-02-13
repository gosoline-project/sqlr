package sqlr

import (
	"fmt"

	"gorm.io/gorm"
)

var _ RepositoryTx[int64, Entitier[int64]] = (*repositoryTx[int64, Entitier[int64]])(nil)

type RepositoryTx[K KeyTypes, E Entitier[K]] interface {
	Create(ttx TTx, entity *E) error
	Read(ttx TTx, id K) (*E, error)
	Query(ttx TTx, qb *QueryBuilderSelect) ([]E, error)
	Update(ttx TTx, entity *E) (*E, error)
	Delete(ttx TTx, id K) error
}

func NewRepositoryTx[K KeyTypes, E Entitier[K]](db *gorm.DB) (RepositoryTx[K, E], error) {
	var err error
	var common repositoryCommon[K, E]

	if common, err = newRepositoryCommon[K, E](db); err != nil {
		return nil, fmt.Errorf("failed to create repository common: %w", err)
	}

	return &repositoryTx[K, E]{
		repositoryCommon: common,
	}, nil
}

type repositoryTx[K KeyTypes, E Entitier[K]] struct {
	repositoryCommon[K, E]
}

func (t *repositoryTx[K, E]) Create(ttx TTx, entity *E) error {
	return t.createEntity(ttx.db, ttx, entity)
}

func (t *repositoryTx[K, E]) Read(ttx TTx, id K) (*E, error) {
	return t.readEntity(ttx.db, ttx, id)
}

func (t *repositoryTx[K, E]) Query(ttx TTx, qb *QueryBuilderSelect) ([]E, error) {
	return t.queryEntities(ttx.db, ttx, qb)
}

func (t *repositoryTx[K, E]) Update(ttx TTx, entity *E) (*E, error) {
	return t.updateEntity(ttx.db, ttx, entity)
}

func (t *repositoryTx[K, E]) Delete(ttx TTx, id K) error {
	return t.deleteEntity(ttx.db, ttx, id)
}
