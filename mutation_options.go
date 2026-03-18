package sqlr

type mutationOptions struct {
	disableAutoUpdates bool
}

func (o mutationOptions) autoUpdatesDisabled() bool {
	return o.disableAutoUpdates
}
