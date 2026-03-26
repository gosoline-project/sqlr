package sqlr

import "github.com/gosoline-project/sqlc"

// QueryBuilderCreate configures optional association synchronization behavior
// for Repository.Create and RepositoryTx.Create, augmenting or overriding any
// schema-level relationship sync defaults. It can also request post-create
// preloading for relations that should be rehydrated after the write succeeds.
type QueryBuilderCreate struct {
	omitAllAssociations bool
	associationOptions  associationSyncOptions
	disableAutoUpdates  bool
	preloads            []preloadEntry
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

// OmitAllAssociations disables association persistence during Create so only
// the root row is inserted.
func (c *QueryBuilderCreate) OmitAllAssociations() *QueryBuilderCreate {
	c.omitAllAssociations = true

	return c
}

// OmitAssociation excludes the provided relation paths from association
// persistence. Omitting a direct relation such as "Posts" also omits its
// nested descendants.
func (c *QueryBuilderCreate) OmitAssociation(paths ...string) *QueryBuilderCreate {
	c.associationOptions.addOmitPaths(paths...)

	return c
}

// Preload requests post-create loading for the named relation. After the create
// succeeds, sqlr reloads the entity by primary key and hydrates the requested
// relations using the same preload machinery as Read and Query. Nested paths and
// optional conditions are supported.
func (c *QueryBuilderCreate) Preload(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderCreate {
	appendPreload(&c.preloads, relation, conditions)

	return c
}

func (c *QueryBuilderCreate) shouldOmitAllAssociations() bool {
	return c != nil && c.omitAllAssociations
}

func (c *QueryBuilderCreate) hasPreloads() bool {
	return c != nil && len(c.preloads) > 0
}

func (c *QueryBuilderCreate) toQueryBuilderRead() *QueryBuilderRead {
	if c == nil {
		return NewQueryBuilderRead()
	}

	return &QueryBuilderRead{preloads: c.preloads}
}

// DisableAutoUpdates disables sqlr-managed primary key and timestamp mutations
// for Create. The entity graph is persisted using the values already present on
// the entities instead of auto-generated IDs or auto-populated timestamps.
func (c *QueryBuilderCreate) DisableAutoUpdates() *QueryBuilderCreate {
	c.disableAutoUpdates = true

	return c
}

func (c *QueryBuilderCreate) mutationOptions() mutationOptions {
	if c == nil {
		return mutationOptions{}
	}

	return mutationOptions{
		disableAutoUpdates: c.disableAutoUpdates,
	}
}
