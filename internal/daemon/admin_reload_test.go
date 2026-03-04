package daemon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noReloadWallet struct {
	base *mockWallet
}

func newNoReloadWallet(t *testing.T) *noReloadWallet {
	t.Helper()
	return &noReloadWallet{base: newMockWallet(t)}
}

func (w *noReloadWallet) DeriveNodePubKey(vaultIndex uint32, filePath []uint32, hardened []bool) (*ec.PublicKey, error) {
	return w.base.DeriveNodePubKey(vaultIndex, filePath, hardened)
}

func (w *noReloadWallet) DeriveNodeKeyPair(pnode []byte) (*ec.PrivateKey, *ec.PublicKey, error) {
	return w.base.DeriveNodeKeyPair(pnode)
}

func (w *noReloadWallet) GetSellerKeyPair() (*ec.PrivateKey, *ec.PublicKey, error) {
	return w.base.GetSellerKeyPair()
}

func (w *noReloadWallet) GetVaultPubKey(alias string) (string, error) {
	return w.base.GetVaultPubKey(alias)
}

func TestHandleAdminReload_Success(t *testing.T) {
	d, wallet, _, _ := newTestDaemonWithAdmin(t)

	req := httptest.NewRequest("POST", "/_bitfs/admin/reload", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp := httptest.NewRecorder()
	d.Handler().ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "\"reloaded\":true")
	assert.Equal(t, 1, wallet.reloadN)
}

func TestHandleAdminReload_ReloadFailure(t *testing.T) {
	d, wallet, _, _ := newTestDaemonWithAdmin(t)
	wallet.reloadErr = errors.New("boom")

	req := httptest.NewRequest("POST", "/_bitfs/admin/reload", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp := httptest.NewRecorder()
	d.Handler().ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	assert.Contains(t, resp.Body.String(), "RELOAD_FAILED")
	assert.Equal(t, 1, wallet.reloadN)
}

func TestHandleAdminReload_Unauthorized(t *testing.T) {
	d, _, _, _ := newTestDaemonWithAdmin(t)

	req := httptest.NewRequest("POST", "/_bitfs/admin/reload", nil)
	resp := httptest.NewRecorder()
	d.Handler().ServeHTTP(resp, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	assert.Contains(t, resp.Body.String(), "UNAUTHORIZED")
}

func TestHandleAdminReload_NoAdminToken(t *testing.T) {
	d, _, _, _ := newTestDaemon(t)

	req := httptest.NewRequest("POST", "/_bitfs/admin/reload", nil)
	resp := httptest.NewRecorder()
	d.Handler().ServeHTTP(resp, req)

	assert.Equal(t, http.StatusForbidden, resp.Code)
	assert.Contains(t, resp.Body.String(), "NO_ADMIN_TOKEN")
}

func TestHandleAdminReload_UnsupportedWallet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.RateLimit.RPM = 0
	cfg.Security.AdminToken = testAdminToken

	wallet := newNoReloadWallet(t)
	d, err := New(cfg, wallet, newMockStore(), nil)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/_bitfs/admin/reload", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp := httptest.NewRecorder()
	d.Handler().ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotImplemented, resp.Code)
	assert.Contains(t, resp.Body.String(), "NOT_SUPPORTED")
}
