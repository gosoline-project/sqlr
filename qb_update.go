package sqlr

// QueryBuilderUpdate configures optional behavior for Repository.Update and
// RepositoryTx.Update.
type QueryBuilderUpdate struct {
	syncAllAssociations bool
	associationOptions  associationSyncOptions
	disableAutoUpdates  bool
}

// NewQueryBuilderUpdate creates a new QueryBuilderUpdate instance.
func NewQueryBuilderUpdate() *QueryBuilderUpdate {
	return &QueryBuilderUpdate{}
}

// SyncAllAssociations enables full synchronization of explicitly-present
// associations during Update. Without this option, Update only persists the base
// entity row unless SyncAssociation is used.
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

func (u *QueryBuilderUpdate) shouldSyncAllAssociations() bool {
	return u != nil && u.syncAllAssociations
}

func (u *QueryBuilderUpdate) shouldSyncAssociations() bool {
	return u != nil && (u.syncAllAssociations || len(u.associationOptions.syncPaths) > 0)
}

func (u *QueryBuilderUpdate) mutationOptions() mutationOptions {
	if u == nil {
		return mutationOptions{}
	}

	return mutationOptions{
		disableAutoUpdates: u.disableAutoUpdates,
	}
}
