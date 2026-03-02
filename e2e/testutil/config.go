//go:build e2e

package testutil

import (
	"os"
	"time"
)

// Config holds e2e test configuration from environment variables.
type Config struct {
	Network        string        // "regtest", "testnet", "mainnet"
	RPCURL         string        // RPC endpoint URL
	RPCUser        string        // RPC username
	RPCPass        string        // RPC password
	FaucetURL      string        // Faucet API URL (testnet only)
	FundWIF        string        // Pre-funded wallet WIF private key
	ConfirmTimeout time.Duration // Timeout waiting for confirmations
}

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

	defaults, ok := networkDefaults[network]
	if !ok {
		defaults = networkDefaults["regtest"]
	}

	rpcURL := envOr("BITFS_E2E_RPC_URL", defaults.rpcURL)
	confirmTimeout := defaults.confirmTimeout

	if v := os.Getenv("BITFS_E2E_CONFIRM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			confirmTimeout = d
		}
	}

	return &Config{
		Network:        network,
		RPCURL:         rpcURL,
		RPCUser:        envOr("BITFS_E2E_RPC_USER", "bitfs"),
		RPCPass:        envOr("BITFS_E2E_RPC_PASS", "bitfs"),
		FaucetURL:      os.Getenv("BITFS_E2E_FAUCET_URL"),
		FundWIF:        os.Getenv("BITFS_E2E_FUND_WIF"),
		ConfirmTimeout: confirmTimeout,
	}
}

func (c *Config) IsMainnet() bool { return c.Network == "mainnet" }
func (c *Config) IsRegtest() bool { return c.Network == "regtest" }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
