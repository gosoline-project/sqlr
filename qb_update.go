package sqlr

// QueryBuilderUpdate configures optional behavior for Repository.Update and
// RepositoryTx.Update.
type QueryBuilderUpdate struct {
	syncAssociations bool
}

// NewQueryBuilderUpdate creates a new QueryBuilderUpdate instance.
func NewQueryBuilderUpdate() *QueryBuilderUpdate {
	return &QueryBuilderUpdate{}
}

// SyncAssociations enables full synchronization of explicitly-present
// associations during Update. Without this option, Update only persists the base
// entity row.
func (u *QueryBuilderUpdate) SyncAssociations() *QueryBuilderUpdate {
	u.syncAssociations = true

	return u
}

func (u *QueryBuilderUpdate) shouldSyncAssociations() bool {
	return u != nil && u.syncAssociations
}
