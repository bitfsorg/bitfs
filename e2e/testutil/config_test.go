//go:build e2e

package testutil

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Unsetenv("BITFS_E2E_NETWORK")
	os.Unsetenv("BITFS_E2E_RPC_URL")
	os.Unsetenv("BITFS_E2E_RPC_USER")
	os.Unsetenv("BITFS_E2E_RPC_PASS")
	os.Unsetenv("BITFS_E2E_FAUCET_URL")
	os.Unsetenv("BITFS_E2E_FUND_WIF")
	os.Unsetenv("BITFS_E2E_CONFIRM_TIMEOUT")

	cfg := LoadConfig()
	assert.Equal(t, "regtest", cfg.Network)
	assert.Equal(t, "http://localhost:18332", cfg.RPCURL)
	assert.Equal(t, "bitfs", cfg.RPCUser)
	assert.Equal(t, "bitfs", cfg.RPCPass)
	assert.Equal(t, "", cfg.FaucetURL)
	assert.Equal(t, "", cfg.FundWIF)
	assert.Equal(t, 30*time.Second, cfg.ConfirmTimeout)
}

func TestLoadConfig_Testnet(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	cfg := LoadConfig()
	assert.Equal(t, "testnet", cfg.Network)
	assert.Equal(t, "http://localhost:18333", cfg.RPCURL)
	assert.Equal(t, 30*time.Minute, cfg.ConfirmTimeout)
}

func TestLoadConfig_Mainnet(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "mainnet")
	cfg := LoadConfig()
	assert.Equal(t, "mainnet", cfg.Network)
	assert.Equal(t, "http://localhost:8332", cfg.RPCURL)
	assert.Equal(t, 60*time.Minute, cfg.ConfirmTimeout)
}

func TestLoadConfig_CustomOverrides(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	t.Setenv("BITFS_E2E_RPC_URL", "http://remote:9999")
	t.Setenv("BITFS_E2E_CONFIRM_TIMEOUT", "5m")
	t.Setenv("BITFS_E2E_FUND_WIF", "L1abc...")
	cfg := LoadConfig()
	assert.Equal(t, "http://remote:9999", cfg.RPCURL)
	assert.Equal(t, 5*time.Minute, cfg.ConfirmTimeout)
	assert.Equal(t, "L1abc...", cfg.FundWIF)
}
