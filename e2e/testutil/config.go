//go:build e2e

package testutil

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds e2e test configuration from environment variables.
type Config struct {
	Network          string        // "regtest", "testnet", "mainnet"
	Provider         string        // "rpc", "woc", or "arc" (live networks)
	RPCURL           string        // RPC endpoint URL
	RPCUser          string        // RPC username
	RPCPass          string        // RPC password
	WOCBaseURL       string        // Whatsonchain API base URL
	WOCAPIKey        string        // Whatsonchain API key (optional)
	ARCBaseURL       string        // ARC API primary base URL
	ARCBaseURLs      []string      // Ordered ARC API base URLs (primary first)
	ARCAPIKey        string        // ARC API key (optional)
	ARCCallbackURL   string        // ARC callback URL (optional)
	ARCCallbackToken string        // ARC callback token (optional)
	ARCWaitFor       string        // ARC X-WaitFor value (optional)
	BHSBaseURL       string        // BHS API base URL (optional)
	BHSAPIKey        string        // BHS API key (optional; defaults to ARC key when empty)
	FeeRateSatPerKB  uint64        // Optional manual fee-rate override in sat/KB (0 = auto)
	FundWIF          string        // Pre-funded wallet WIF private key
	FundAmount       float64       // Funding amount per Fund() call, in BSV
	ConfirmTimeout   time.Duration // Timeout waiting for confirmations/proof readiness
}

const defaultFundAmountBSV = 0.00006

var networkDefaults = map[string]struct {
	rpcURL         string
	confirmTimeout time.Duration
}{
	"regtest": {"http://localhost:18332", 30 * time.Second},
	"testnet": {"http://localhost:18333", 30 * time.Minute},
	"mainnet": {"http://localhost:8332", 60 * time.Minute},
}

func LoadConfig() *Config {
	network := strings.ToLower(strings.TrimSpace(envOr("BITFS_E2E_NETWORK", "regtest")))
	if network == "" {
		network = "regtest"
	}
	providerDefault := "rpc"
	if network != "regtest" {
		providerDefault = "arc"
	}
	provider := strings.ToLower(strings.TrimSpace(envOrNetwork("BITFS_E2E_PROVIDER", providerDefault, network)))

	defaults, ok := networkDefaults[network]
	if !ok {
		defaults = networkDefaults["regtest"]
	}

	rpcURL := envOrNetwork("BITFS_E2E_RPC_URL", defaults.rpcURL, network)
	confirmTimeout := defaults.confirmTimeout
	fundAmount := defaultFundAmountBSV
	arcBaseURL, arcBaseURLSet := lookupEnvNetwork("BITFS_E2E_ARC_BASE_URL", network)
	arcBaseURL = strings.TrimSpace(arcBaseURL)
	if arcBaseURL == "" {
		arcBaseURL = defaultARCBaseURL(network)
	}
	arcBaseURLs := parseUniqueURLList(envNetwork("BITFS_E2E_ARC_BASE_URLS", network))
	if len(arcBaseURLs) == 0 && arcBaseURL != "" {
		arcBaseURLs = []string{arcBaseURL}
		// Default mainnet ARC failover only when user did not pin ARC_BASE_URL.
		if network == "mainnet" && !arcBaseURLSet {
			arcBaseURLs = appendUniqueURL(arcBaseURLs, "https://arc.taal.com/v1")
		}
	}
	if len(arcBaseURLs) > 0 {
		arcBaseURL = arcBaseURLs[0]
	}

	if v := envNetwork("BITFS_E2E_CONFIRM_TIMEOUT", network); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			confirmTimeout = d
		}
	}
	if v := envNetwork("BITFS_E2E_FUND_AMOUNT", network); v != "" {
		if a, err := strconv.ParseFloat(v, 64); err == nil && a > 0 {
			fundAmount = a
		}
	}
	var feeRateSatPerKB uint64
	if v := strings.TrimSpace(envNetwork("BITFS_E2E_FEE_RATE_SAT_PER_KB", network)); v != "" {
		if rate, err := strconv.ParseUint(v, 10, 64); err == nil && rate > 0 {
			feeRateSatPerKB = rate
		}
	}
	bhsAPIKey := envNetwork("BITFS_E2E_BHS_API_KEY", network)
	if bhsAPIKey == "" {
		bhsAPIKey = envNetwork("BITFS_E2E_ARC_API_KEY", network)
	}

	return &Config{
		Network:          network,
		Provider:         provider,
		RPCURL:           rpcURL,
		RPCUser:          envOrNetwork("BITFS_E2E_RPC_USER", "bitfs", network),
		RPCPass:          envOrNetwork("BITFS_E2E_RPC_PASS", "bitfs", network),
		WOCBaseURL:       envOrNetwork("BITFS_E2E_WOC_BASE_URL", defaultWOCBaseURL(network), network),
		WOCAPIKey:        envNetwork("BITFS_E2E_WOC_API_KEY", network),
		ARCBaseURL:       arcBaseURL,
		ARCBaseURLs:      arcBaseURLs,
		ARCAPIKey:        envNetwork("BITFS_E2E_ARC_API_KEY", network),
		ARCCallbackURL:   envNetwork("BITFS_E2E_ARC_CALLBACK_URL", network),
		ARCCallbackToken: envNetwork("BITFS_E2E_ARC_CALLBACK_TOKEN", network),
		ARCWaitFor:       envNetwork("BITFS_E2E_ARC_WAIT_FOR", network),
		BHSBaseURL:       envNetwork("BITFS_E2E_BHS_BASE_URL", network),
		BHSAPIKey:        bhsAPIKey,
		FeeRateSatPerKB:  feeRateSatPerKB,
		FundWIF:          envNetwork("BITFS_E2E_FUND_WIF", network),
		FundAmount:       fundAmount,
		ConfirmTimeout:   confirmTimeout,
	}
}

func parseUniqueURLList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = appendUniqueURL(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendUniqueURL(dst []string, u string) []string {
	normalized := strings.TrimSpace(strings.TrimRight(u, "/"))
	if normalized == "" {
		return dst
	}
	for _, existing := range dst {
		if strings.EqualFold(strings.TrimSpace(strings.TrimRight(existing, "/")), normalized) {
			return dst
		}
	}
	return append(dst, normalized)
}

func (c *Config) IsMainnet() bool { return c.Network == "mainnet" }
func (c *Config) IsRegtest() bool { return c.Network == "regtest" }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrNetwork(key, fallback, network string) string {
	if v := envNetwork(key, network); v != "" {
		return v
	}
	return fallback
}

func envNetwork(key, network string) string {
	if v, ok := lookupEnvNetwork(key, network); ok {
		return v
	}
	return ""
}

func lookupEnvNetwork(key, network string) (string, bool) {
	suffix := networkSuffix(network)
	if suffix != "" {
		if v, ok := os.LookupEnv(key + "_" + suffix); ok {
			return v, true
		}
	}
	return os.LookupEnv(key)
}

func networkSuffix(network string) string {
	n := strings.ToUpper(strings.TrimSpace(network))
	return n
}

func defaultWOCBaseURL(network string) string {
	switch network {
	case "mainnet":
		return "https://api.whatsonchain.com/v1/bsv/main"
	case "testnet":
		return "https://api.whatsonchain.com/v1/bsv/test"
	default:
		return ""
	}
}

func defaultARCBaseURL(network string) string {
	switch network {
	case "mainnet":
		return "https://arc.gorillapool.io/v1"
	case "testnet":
		return "https://testnet.arc.gorillapool.io/v1"
	default:
		return ""
	}
}
