package sqlr

// QueryBuilderCreate configures optional association synchronization behavior
// for Repository.Create and RepositoryTx.Create.
type QueryBuilderCreate struct {
	associationOptions associationSyncOptions
}

// NewQueryBuilderCreate creates a new QueryBuilderCreate instance.
func NewQueryBuilderCreate() *QueryBuilderCreate {
	return &QueryBuilderCreate{}
}

// SyncAssociation restricts association persistence to the provided relation
// paths. Paths may reference direct relations such as "Posts" or nested
// relations such as "Posts.Comments".
func (c *QueryBuilderCreate) SyncAssociation(paths ...string) *QueryBuilderCreate {
	c.associationOptions.addSyncPaths(paths...)

	return c
}

// OmitAssociation excludes the provided relation paths from association
// persistence. Omitting a direct relation such as "Posts" also omits its
// nested descendants.
func (c *QueryBuilderCreate) OmitAssociation(paths ...string) *QueryBuilderCreate {
	c.associationOptions.addOmitPaths(paths...)

	return c
}
