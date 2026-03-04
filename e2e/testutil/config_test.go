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
	os.Unsetenv("BITFS_E2E_PROVIDER")
	os.Unsetenv("BITFS_E2E_RPC_URL")
	os.Unsetenv("BITFS_E2E_RPC_USER")
	os.Unsetenv("BITFS_E2E_RPC_PASS")
	os.Unsetenv("BITFS_E2E_WOC_BASE_URL")
	os.Unsetenv("BITFS_E2E_WOC_API_KEY")
	os.Unsetenv("BITFS_E2E_ARC_BASE_URL")
	os.Unsetenv("BITFS_E2E_ARC_BASE_URLS")
	os.Unsetenv("BITFS_E2E_ARC_API_KEY")
	os.Unsetenv("BITFS_E2E_ARC_CALLBACK_URL")
	os.Unsetenv("BITFS_E2E_ARC_CALLBACK_TOKEN")
	os.Unsetenv("BITFS_E2E_ARC_WAIT_FOR")
	os.Unsetenv("BITFS_E2E_BHS_BASE_URL")
	os.Unsetenv("BITFS_E2E_BHS_API_KEY")
	os.Unsetenv("BITFS_E2E_FUND_WIF")
	os.Unsetenv("BITFS_E2E_FUND_AMOUNT")
	os.Unsetenv("BITFS_E2E_CONFIRM_TIMEOUT")

	cfg := LoadConfig()
	assert.Equal(t, "regtest", cfg.Network)
	assert.Equal(t, "rpc", cfg.Provider)
	assert.Equal(t, "http://localhost:18332", cfg.RPCURL)
	assert.Equal(t, "bitfs", cfg.RPCUser)
	assert.Equal(t, "bitfs", cfg.RPCPass)
	assert.Equal(t, "", cfg.WOCBaseURL)
	assert.Equal(t, "", cfg.WOCAPIKey)
	assert.Equal(t, "", cfg.ARCBaseURL)
	assert.Empty(t, cfg.ARCBaseURLs)
	assert.Equal(t, "", cfg.ARCAPIKey)
	assert.Equal(t, "", cfg.BHSBaseURL)
	assert.Equal(t, "", cfg.BHSAPIKey)
	assert.Equal(t, "", cfg.FundWIF)
	assert.InDelta(t, defaultFundAmountBSV, cfg.FundAmount, 1e-12)
	assert.Equal(t, 30*time.Second, cfg.ConfirmTimeout)
}

func TestLoadConfig_Testnet(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	cfg := LoadConfig()
	assert.Equal(t, "testnet", cfg.Network)
	assert.Equal(t, "arc", cfg.Provider)
	assert.Equal(t, "http://localhost:18333", cfg.RPCURL)
	assert.Equal(t, "https://api.whatsonchain.com/v1/bsv/test", cfg.WOCBaseURL)
	assert.Equal(t, "https://testnet.arc.gorillapool.io/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://testnet.arc.gorillapool.io/v1"}, cfg.ARCBaseURLs)
	assert.Equal(t, 30*time.Minute, cfg.ConfirmTimeout)
}

func TestLoadConfig_Mainnet(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "mainnet")
	cfg := LoadConfig()
	assert.Equal(t, "mainnet", cfg.Network)
	assert.Equal(t, "arc", cfg.Provider)
	assert.Equal(t, "http://localhost:8332", cfg.RPCURL)
	assert.Equal(t, "https://api.whatsonchain.com/v1/bsv/main", cfg.WOCBaseURL)
	assert.Equal(t, "https://arc.gorillapool.io/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://arc.gorillapool.io/v1", "https://arc.taal.com/v1"}, cfg.ARCBaseURLs)
	assert.Equal(t, 60*time.Minute, cfg.ConfirmTimeout)
}

func TestLoadConfig_CustomOverrides(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	t.Setenv("BITFS_E2E_PROVIDER", "arc")
	t.Setenv("BITFS_E2E_RPC_URL", "http://remote:9999")
	t.Setenv("BITFS_E2E_WOC_BASE_URL", "https://example.test/woc")
	t.Setenv("BITFS_E2E_WOC_API_KEY", "testnet_demo_key")
	t.Setenv("BITFS_E2E_ARC_BASE_URL", "https://example.test/arc/v1")
	t.Setenv("BITFS_E2E_ARC_API_KEY", "arc_demo_key")
	t.Setenv("BITFS_E2E_ARC_CALLBACK_URL", "https://callback.test/arc")
	t.Setenv("BITFS_E2E_ARC_CALLBACK_TOKEN", "callback_token")
	t.Setenv("BITFS_E2E_ARC_WAIT_FOR", "SEEN_ON_NETWORK")
	t.Setenv("BITFS_E2E_BHS_BASE_URL", "https://example.test/bhs")
	t.Setenv("BITFS_E2E_CONFIRM_TIMEOUT", "5m")
	t.Setenv("BITFS_E2E_FUND_WIF", "L1abc...")
	t.Setenv("BITFS_E2E_FUND_AMOUNT", "0.0025")
	cfg := LoadConfig()
	assert.Equal(t, "arc", cfg.Provider)
	assert.Equal(t, "http://remote:9999", cfg.RPCURL)
	assert.Equal(t, "https://example.test/woc", cfg.WOCBaseURL)
	assert.Equal(t, "testnet_demo_key", cfg.WOCAPIKey)
	assert.Equal(t, "https://example.test/arc/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://example.test/arc/v1"}, cfg.ARCBaseURLs)
	assert.Equal(t, "arc_demo_key", cfg.ARCAPIKey)
	assert.Equal(t, "https://callback.test/arc", cfg.ARCCallbackURL)
	assert.Equal(t, "callback_token", cfg.ARCCallbackToken)
	assert.Equal(t, "SEEN_ON_NETWORK", cfg.ARCWaitFor)
	assert.Equal(t, "https://example.test/bhs", cfg.BHSBaseURL)
	assert.Equal(t, "arc_demo_key", cfg.BHSAPIKey)
	assert.Equal(t, 5*time.Minute, cfg.ConfirmTimeout)
	assert.Equal(t, "L1abc...", cfg.FundWIF)
	assert.InDelta(t, 0.0025, cfg.FundAmount, 1e-12)
}

func TestLoadConfig_ARCBaseURLsOverride(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "mainnet")
	t.Setenv("BITFS_E2E_ARC_BASE_URLS", " https://arc.gorillapool.io/v1, https://arc.taal.com/v1 ; https://arc.gorillapool.io/v1 ")

	cfg := LoadConfig()
	assert.Equal(t, "https://arc.gorillapool.io/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://arc.gorillapool.io/v1", "https://arc.taal.com/v1"}, cfg.ARCBaseURLs)
}

func TestLoadConfig_BHSKeyOverride(t *testing.T) {
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	t.Setenv("BITFS_E2E_ARC_API_KEY", "arc_key")
	t.Setenv("BITFS_E2E_BHS_API_KEY", "bhs_key")

	cfg := LoadConfig()
	assert.Equal(t, "bhs_key", cfg.BHSAPIKey)
}
