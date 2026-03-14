package sqlr

// QueryBuilderUpdate configures optional behavior for Repository.Update and
// RepositoryTx.Update.
type QueryBuilderUpdate struct {
	syncAllAssociations bool
	associationOptions  associationSyncOptions
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

func (u *QueryBuilderUpdate) shouldSyncAllAssociations() bool {
	return u != nil && u.syncAllAssociations
}

func (u *QueryBuilderUpdate) shouldSyncAssociations() bool {
	return u != nil && (u.syncAllAssociations || len(u.associationOptions.syncPaths) > 0)
}
