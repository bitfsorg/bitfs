package daemon

// walletStateReloader can reload wallet state from persistent storage.
type walletStateReloader interface {
	ReloadState() error
}

// ReloadWalletState triggers an in-process wallet state reload when supported.
func (d *Daemon) ReloadWalletState() error {
	reloader, ok := d.wallet.(walletStateReloader)
	if !ok {
		return ErrWalletReloadUnsupported
	}
	return reloader.ReloadState()
}
