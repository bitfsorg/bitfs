package daemon

import (
	"encoding/json"
	"errors"
	"net/http"
)

// handleAdminReload reloads wallet state from disk.
func (d *Daemon) handleAdminReload(w http.ResponseWriter, r *http.Request) {
	if err := d.ReloadWalletState(); err != nil {
		if errors.Is(err, ErrWalletReloadUnsupported) {
			writeJSONError(w, http.StatusNotImplemented, "NOT_SUPPORTED", "wallet state reload not supported")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "RELOAD_FAILED", "failed to reload wallet state")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"reloaded": true})
}
