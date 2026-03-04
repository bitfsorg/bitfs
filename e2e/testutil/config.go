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
	network := envOr("BITFS_E2E_NETWORK", "regtest")
	providerDefault := "rpc"
	if network != "regtest" {
		providerDefault = "arc"
	}
	provider := strings.ToLower(envOr("BITFS_E2E_PROVIDER", providerDefault))

	defaults, ok := networkDefaults[network]
	if !ok {
		defaults = networkDefaults["regtest"]
	}

	rpcURL := envOr("BITFS_E2E_RPC_URL", defaults.rpcURL)
	confirmTimeout := defaults.confirmTimeout
	fundAmount := defaultFundAmountBSV
	arcBaseURL, arcBaseURLSet := os.LookupEnv("BITFS_E2E_ARC_BASE_URL")
	arcBaseURL = strings.TrimSpace(arcBaseURL)
	if arcBaseURL == "" {
		arcBaseURL = defaultARCBaseURL(network)
	}
	arcBaseURLs := parseUniqueURLList(os.Getenv("BITFS_E2E_ARC_BASE_URLS"))
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

	if v := os.Getenv("BITFS_E2E_CONFIRM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			confirmTimeout = d
		}
	}
	if v := os.Getenv("BITFS_E2E_FUND_AMOUNT"); v != "" {
		if a, err := strconv.ParseFloat(v, 64); err == nil && a > 0 {
			fundAmount = a
		}
	}
	bhsAPIKey := os.Getenv("BITFS_E2E_BHS_API_KEY")
	if bhsAPIKey == "" {
		bhsAPIKey = os.Getenv("BITFS_E2E_ARC_API_KEY")
	}

	return &Config{
		Network:          network,
		Provider:         provider,
		RPCURL:           rpcURL,
		RPCUser:          envOr("BITFS_E2E_RPC_USER", "bitfs"),
		RPCPass:          envOr("BITFS_E2E_RPC_PASS", "bitfs"),
		WOCBaseURL:       envOr("BITFS_E2E_WOC_BASE_URL", defaultWOCBaseURL(network)),
		WOCAPIKey:        os.Getenv("BITFS_E2E_WOC_API_KEY"),
		ARCBaseURL:       arcBaseURL,
		ARCBaseURLs:      arcBaseURLs,
		ARCAPIKey:        os.Getenv("BITFS_E2E_ARC_API_KEY"),
		ARCCallbackURL:   os.Getenv("BITFS_E2E_ARC_CALLBACK_URL"),
		ARCCallbackToken: os.Getenv("BITFS_E2E_ARC_CALLBACK_TOKEN"),
		ARCWaitFor:       os.Getenv("BITFS_E2E_ARC_WAIT_FOR"),
		BHSBaseURL:       os.Getenv("BITFS_E2E_BHS_BASE_URL"),
		BHSAPIKey:        bhsAPIKey,
		FundWIF:          os.Getenv("BITFS_E2E_FUND_WIF"),
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
