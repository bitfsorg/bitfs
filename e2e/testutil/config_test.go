//go:build e2e

package testutil

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var configEnvKeys = []string{
	"BITFS_E2E_NETWORK",
	"BITFS_E2E_PROVIDER",
	"BITFS_E2E_RPC_URL",
	"BITFS_E2E_RPC_USER",
	"BITFS_E2E_RPC_PASS",
	"BITFS_E2E_WOC_BASE_URL",
	"BITFS_E2E_WOC_API_KEY",
	"BITFS_E2E_ARC_BASE_URL",
	"BITFS_E2E_ARC_BASE_URLS",
	"BITFS_E2E_ARC_API_KEY",
	"BITFS_E2E_ARC_CALLBACK_URL",
	"BITFS_E2E_ARC_CALLBACK_TOKEN",
	"BITFS_E2E_ARC_WAIT_FOR",
	"BITFS_E2E_BHS_BASE_URL",
	"BITFS_E2E_BHS_API_KEY",
	"BITFS_E2E_FEE_RATE_SAT_PER_KB",
	"BITFS_E2E_FUND_WIF",
	"BITFS_E2E_FUND_AMOUNT",
	"BITFS_E2E_CONFIRM_TIMEOUT",
}

var configEnvNetworks = []string{"MAINNET", "TESTNET", "REGTEST"}

func clearConfigEnv() {
	for _, k := range configEnvKeys {
		_ = os.Unsetenv(k)
		for _, net := range configEnvNetworks {
			_ = os.Unsetenv(k + "_" + net)
		}
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	clearConfigEnv()

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
	assert.Equal(t, uint64(0), cfg.FeeRateSatPerKB)
	assert.Equal(t, "", cfg.FundWIF)
	assert.InDelta(t, defaultFundAmountBSV, cfg.FundAmount, 1e-12)
	assert.Equal(t, 30*time.Second, cfg.ConfirmTimeout)
}

func TestLoadConfig_Testnet(t *testing.T) {
	clearConfigEnv()
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
	clearConfigEnv()
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
	clearConfigEnv()
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
	t.Setenv("BITFS_E2E_FEE_RATE_SAT_PER_KB", "250")
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
	assert.Equal(t, uint64(250), cfg.FeeRateSatPerKB)
	assert.Equal(t, 5*time.Minute, cfg.ConfirmTimeout)
	assert.Equal(t, "L1abc...", cfg.FundWIF)
	assert.InDelta(t, 0.0025, cfg.FundAmount, 1e-12)
}

func TestLoadConfig_ARCBaseURLsOverride(t *testing.T) {
	clearConfigEnv()
	t.Setenv("BITFS_E2E_NETWORK", "mainnet")
	t.Setenv("BITFS_E2E_ARC_BASE_URLS", " https://arc.gorillapool.io/v1, https://arc.taal.com/v1 ; https://arc.gorillapool.io/v1 ")

	cfg := LoadConfig()
	assert.Equal(t, "https://arc.gorillapool.io/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://arc.gorillapool.io/v1", "https://arc.taal.com/v1"}, cfg.ARCBaseURLs)
}

func TestLoadConfig_BHSKeyOverride(t *testing.T) {
	clearConfigEnv()
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	t.Setenv("BITFS_E2E_ARC_API_KEY", "arc_key")
	t.Setenv("BITFS_E2E_BHS_API_KEY", "bhs_key")

	cfg := LoadConfig()
	assert.Equal(t, "bhs_key", cfg.BHSAPIKey)
}

func TestLoadConfig_NetworkSpecificOverrides(t *testing.T) {
	clearConfigEnv()
	t.Setenv("BITFS_E2E_NETWORK", "mainnet")

	// Generic fallbacks.
	t.Setenv("BITFS_E2E_ARC_BASE_URL", "https://testnet.arc.gorillapool.io/v1")
	t.Setenv("BITFS_E2E_ARC_BASE_URLS", "https://arc.gorillapool.io/v1")
	t.Setenv("BITFS_E2E_ARC_API_KEY", "generic_arc_key")
	t.Setenv("BITFS_E2E_WOC_API_KEY", "generic_woc_key")
	t.Setenv("BITFS_E2E_FEE_RATE_SAT_PER_KB", "150")
	t.Setenv("BITFS_E2E_FUND_WIF", "Lgeneric...")
	t.Setenv("BITFS_E2E_FUND_AMOUNT", "0.00006")
	t.Setenv("BITFS_E2E_CONFIRM_TIMEOUT", "60m")

	// mainnet-specific values should win.
	t.Setenv("BITFS_E2E_ARC_BASE_URL_MAINNET", "https://arc.taal.com/v1")
	t.Setenv("BITFS_E2E_ARC_BASE_URLS_MAINNET", "https://arc.taal.com/v1,https://arc.gorillapool.io/v1")
	t.Setenv("BITFS_E2E_ARC_API_KEY_MAINNET", "mainnet_arc_key")
	t.Setenv("BITFS_E2E_WOC_API_KEY_MAINNET", "mainnet_woc_key")
	t.Setenv("BITFS_E2E_FEE_RATE_SAT_PER_KB_MAINNET", "333")
	t.Setenv("BITFS_E2E_FUND_WIF_MAINNET", "Lmainnet...")
	t.Setenv("BITFS_E2E_FUND_AMOUNT_MAINNET", "0.00001")
	t.Setenv("BITFS_E2E_CONFIRM_TIMEOUT_MAINNET", "10m")

	cfg := LoadConfig()
	assert.Equal(t, "https://arc.taal.com/v1", cfg.ARCBaseURL)
	assert.Equal(t, []string{"https://arc.taal.com/v1", "https://arc.gorillapool.io/v1"}, cfg.ARCBaseURLs)
	assert.Equal(t, "mainnet_arc_key", cfg.ARCAPIKey)
	assert.Equal(t, "mainnet_woc_key", cfg.WOCAPIKey)
	assert.Equal(t, uint64(333), cfg.FeeRateSatPerKB)
	assert.Equal(t, "Lmainnet...", cfg.FundWIF)
	assert.InDelta(t, 0.00001, cfg.FundAmount, 1e-12)
	assert.Equal(t, 10*time.Minute, cfg.ConfirmTimeout)
}

func TestLoadConfig_InvalidFeeRateIgnored(t *testing.T) {
	clearConfigEnv()
	t.Setenv("BITFS_E2E_NETWORK", "testnet")
	t.Setenv("BITFS_E2E_FEE_RATE_SAT_PER_KB", "abc")

	cfg := LoadConfig()
	assert.Equal(t, uint64(0), cfg.FeeRateSatPerKB)
}
