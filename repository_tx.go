package sqlr

var _ RepositoryTx[int64, Entitier[int64]] = (*repositoryTx[int64, Entitier[int64]])(nil)

type RepositoryTx[K KeyTypes, E Entitier[K]] interface {
	Create(ttx TTx, entity *E) error
	Read(ttx TTx, id K) (*E, error)
	Update(ttx TTx, entity *E) (*E, error)
	Delete(ttx TTx, id K) error
}

type repositoryTx[K KeyTypes, E Entitier[K]] struct {
}

func (t repositoryTx[K, E]) Create(ttx TTx, entity *E) error {
	return createEntity[K, E](ttx.db, ttx, entity)
}

func (t repositoryTx[K, E]) Read(ttx TTx, id K) (*E, error) {
	return readEntity[K, E](ttx.db, ttx, id)
}

func (r *repositoryTx[K, E]) Query(ttx TTx, qb *QueryBuilderSelect) ([]E, error) {
	return queryEntities[K, E](ttx.db, ttx, qb)
}

func (t repositoryTx[K, E]) Update(ttx TTx, entity *E) (*E, error) {
	return updateEntity[K, E](ttx.db, ttx, entity)
}

func (t repositoryTx[K, E]) Delete(ttx TTx, id K) error {
	return deleteEntity[K, E](ttx.db, ttx, id)
}
