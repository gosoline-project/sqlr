package sqlr

type associationSyncOptions struct {
	syncPaths []string
	omitPaths []string
}

func (o *associationSyncOptions) addSyncPaths(paths ...string) {
	o.syncPaths = append(o.syncPaths, paths...)
}

func (o *associationSyncOptions) addOmitPaths(paths ...string) {
	o.omitPaths = append(o.omitPaths, paths...)
}
