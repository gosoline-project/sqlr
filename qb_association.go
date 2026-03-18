package sqlr

type associationSyncOptions struct {
	syncPaths              []string
	omitPaths              []string
	fullSyncMany2manyPaths []string
}

func (o *associationSyncOptions) addSyncPaths(paths ...string) {
	o.syncPaths = append(o.syncPaths, paths...)
}

func (o *associationSyncOptions) addOmitPaths(paths ...string) {
	o.omitPaths = append(o.omitPaths, paths...)
}

func (o *associationSyncOptions) addFullSyncMany2manyPaths(paths ...string) {
	o.fullSyncMany2manyPaths = append(o.fullSyncMany2manyPaths, paths...)
}
