package sqlr

type associationSyncOptions struct {
	syncPaths               []string
	omitPaths               []string
	fullSyncManyToManyPaths []string
}

func (o *associationSyncOptions) addSyncPaths(paths ...string) {
	o.syncPaths = append(o.syncPaths, paths...)
}

func (o *associationSyncOptions) addOmitPaths(paths ...string) {
	o.omitPaths = append(o.omitPaths, paths...)
}

func (o *associationSyncOptions) addFullSyncManyToManyPaths(paths ...string) {
	o.fullSyncManyToManyPaths = append(o.fullSyncManyToManyPaths, paths...)
}
