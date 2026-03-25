package sqlr

import "github.com/gosoline-project/sqlc"

// QueryBuilderUpdate configures optional behavior for Repository.Update and
// RepositoryTx.Update, augmenting or overriding any schema-level relationship
// sync defaults.
type QueryBuilderUpdate struct {
	syncAllAssociations bool
	associationOptions  associationSyncOptions
	disableAutoUpdates  bool
	preloads            []preloadEntry
}

// NewQueryBuilderUpdate creates a new QueryBuilderUpdate instance.
func NewQueryBuilderUpdate() *QueryBuilderUpdate {
	return &QueryBuilderUpdate{}
}

// SyncAllAssociations enables full synchronization of explicitly-present
// associations during Update. Without this option, Update only persists the base
// entity row unless SyncAssociation is used. Many-to-many relations still use
// link-only synchronization by default; use SyncMany2many to opt a
// many-to-many path into recursive related-row synchronization.
func (u *QueryBuilderUpdate) SyncAllAssociations() *QueryBuilderUpdate {
	u.syncAllAssociations = true

	return u
}

// SyncAssociation restricts association synchronization to the provided
// relation paths. Paths may reference direct relations such as "Posts" or
// nested relations such as "Posts.Comments".
func (u *QueryBuilderUpdate) SyncAssociation(paths ...string) *QueryBuilderUpdate {
	u.associationOptions.addSyncPaths(paths...)

	return u
}

// SyncMany2many opts the provided many-to-many relation paths into
// full entity synchronization during Update. By default, Update only
// reconciles many-to-many join-table membership for existing related rows while
// still inserting new related rows that have no primary key.
func (u *QueryBuilderUpdate) SyncMany2many(paths ...string) *QueryBuilderUpdate {
	u.associationOptions.addFullSyncMany2manyPaths(paths...)

	return u
}

// OmitAssociation excludes the provided relation paths from association
// synchronization. Omitting a direct relation such as "Posts" also omits its
// nested descendants.
func (u *QueryBuilderUpdate) OmitAssociation(paths ...string) *QueryBuilderUpdate {
	u.associationOptions.addOmitPaths(paths...)

	return u
}

// DisableAutoUpdates disables sqlr-managed primary key and timestamp mutations
// for Update. The entity graph is persisted using the values already present on
// the entities instead of auto-populated timestamps.
func (u *QueryBuilderUpdate) DisableAutoUpdates() *QueryBuilderUpdate {
	u.disableAutoUpdates = true

	return u
}

// Preload requests post-update loading for the named relation. After the update
// succeeds, sqlr reloads the entity by primary key and hydrates the requested
// relations using the same preload machinery as Read and Query. Nested paths and
// optional conditions are supported.
func (u *QueryBuilderUpdate) Preload(relation string, conditions ...*sqlc.SqlerWhere) *QueryBuilderUpdate {
	appendPreload(&u.preloads, relation, conditions)

	return u
}

func (u *QueryBuilderUpdate) shouldSyncAllAssociations() bool {
	return u != nil && u.syncAllAssociations
}

func (u *QueryBuilderUpdate) hasPreloads() bool {
	return u != nil && len(u.preloads) > 0
}

func (u *QueryBuilderUpdate) toQueryBuilderRead() *QueryBuilderRead {
	if u == nil {
		return NewQueryBuilderRead()
	}

	return &QueryBuilderRead{preloads: u.preloads}
}

func (u *QueryBuilderUpdate) mutationOptions() mutationOptions {
	if u == nil {
		return mutationOptions{}
	}

	return mutationOptions{
		disableAutoUpdates: u.disableAutoUpdates,
	}
}
