package sqlr

// QueryBuilderDelete configures optional association cleanup behavior for
// Repository.Delete and RepositoryTx.Delete. Delete cascades owned associations
// by default; the builder can restrict selected relation paths, omit specific
// paths, or disable association cleanup entirely.
type QueryBuilderDelete struct {
	omitAllAssociations bool
	associationOptions  associationSyncOptions
}

// NewQueryBuilderDelete creates a new QueryBuilderDelete instance.
func NewQueryBuilderDelete() *QueryBuilderDelete {
	return &QueryBuilderDelete{}
}

// OmitAllAssociations disables owned-association cleanup during Delete so only
// the root row is deleted.
func (d *QueryBuilderDelete) OmitAllAssociations() *QueryBuilderDelete {
	d.omitAllAssociations = true

	return d
}

// SyncAssociation restricts association synchronization to the provided
// relation paths. Paths may reference direct relations such as "Posts" or
// nested relations such as "Posts.Comments".
func (d *QueryBuilderDelete) SyncAssociation(paths ...string) *QueryBuilderDelete {
	d.associationOptions.addSyncPaths(paths...)

	return d
}

// OmitAssociation excludes the provided relation paths from association
// synchronization. Omitting a direct relation such as "Posts" also omits its
// nested descendants.
func (d *QueryBuilderDelete) OmitAssociation(paths ...string) *QueryBuilderDelete {
	d.associationOptions.addOmitPaths(paths...)

	return d
}

func (d *QueryBuilderDelete) shouldOmitAllAssociations() bool {
	return d != nil && d.omitAllAssociations
}
