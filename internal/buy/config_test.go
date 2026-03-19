package buy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 32-byte test private key (hex).
const testKeyHex = "0000000000000000000000000000000000000000000000000000000000000001"

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "buyer.conf")
	require.NoError(t, os.WriteFile(confPath, []byte("wallet_key = "+testKeyHex+"\nnetwork = regtest\nfee_rate_sat_per_kb = 250\n"), 0600))

	cfg, err := LoadConfig(LoadConfigOpts{DataDir: dir})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
	assert.Equal(t, "regtest", cfg.Network)
	assert.Equal(t, uint64(250), cfg.FeeRateSatPerKB)
}

func TestLoadConfig_CLIFlagOverridesFile(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "buyer.conf")
	require.NoError(t, os.WriteFile(confPath, []byte("wallet_key = ffff\nnetwork = mainnet\n"), 0600))

	cfg, err := LoadConfig(LoadConfigOpts{DataDir: dir, WalletKeyFlag: testKeyHex})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
}

func TestLoadConfig_EnvVarOverridesFile(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "buyer.conf")
	require.NoError(t, os.WriteFile(confPath, []byte("wallet_key = ffff\n"), 0600))

	cfg, err := LoadConfig(LoadConfigOpts{DataDir: dir, Env: map[string]string{"BITFS_WALLET_KEY": testKeyHex}})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
}

func TestLoadConfig_NoConfig(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadConfig(LoadConfigOpts{DataDir: dir})
	assert.ErrorIs(t, err, ErrNoBuyerConfig)
}

func TestLoadConfig_ManualUTXO(t *testing.T) {
	cfg, err := LoadConfig(LoadConfigOpts{
		WalletKeyFlag: testKeyHex,
		UTXOFlag:      "aabbccddaabbccddaabbccddaabbccddaabbccddaabbccddaabbccddaabbccdd:0:10000",
	})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
	assert.Len(t, cfg.ManualUTXOs, 1)
	assert.Equal(t, uint64(10000), cfg.ManualUTXOs[0].Amount)
}

func TestLoadConfig_InvalidKey(t *testing.T) {
	_, err := LoadConfig(LoadConfigOpts{WalletKeyFlag: "not-hex"})
	assert.Error(t, err)
}

func TestResolveWalletKey_Literal(t *testing.T) {
	got, err := resolveWalletKey(testKeyHex)
	require.NoError(t, err)
	assert.Equal(t, testKeyHex, got)
}

func TestResolveWalletKey_FromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.hex")
	require.NoError(t, os.WriteFile(keyFile, []byte("  "+testKeyHex+"\n"), 0600))

	got, err := resolveWalletKey("@" + keyFile)
	require.NoError(t, err)
	assert.Equal(t, testKeyHex, got, "should trim whitespace from file contents")
}

func TestResolveWalletKey_EmptyFilePath(t *testing.T) {
	_, err := resolveWalletKey("@")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty file path")
}

func TestResolveWalletKey_MissingFile(t *testing.T) {
	_, err := resolveWalletKey("@/nonexistent/path/key.hex")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read wallet key file")
}

func TestResolveWalletKey_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "empty.hex")
	require.NoError(t, os.WriteFile(keyFile, []byte("   \n  "), 0600))

	_, err := resolveWalletKey("@" + keyFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is empty")
}

func TestLoadConfig_CLIFlagFromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.hex")
	require.NoError(t, os.WriteFile(keyFile, []byte(testKeyHex+"\n"), 0600))

	cfg, err := LoadConfig(LoadConfigOpts{WalletKeyFlag: "@" + keyFile})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
}

func TestLoadConfig_EnvVarFromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.hex")
	require.NoError(t, os.WriteFile(keyFile, []byte(testKeyHex+"\n"), 0600))

	cfg, err := LoadConfig(LoadConfigOpts{
		DataDir: dir,
		Env:     map[string]string{"BITFS_WALLET_KEY": "@" + keyFile},
	})
	require.NoError(t, err)
	assert.NotNil(t, cfg.PrivKey)
}

func TestLoadConfig_FeeRateFromEnv(t *testing.T) {
	cfg, err := LoadConfig(LoadConfigOpts{
		WalletKeyFlag: testKeyHex,
		Env:           map[string]string{"BITFS_BUY_FEE_RATE_SAT_PER_KB": "333"},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(333), cfg.FeeRateSatPerKB)
}

func TestLoadConfig_FeeRateFlagOverridesEnv(t *testing.T) {
	cfg, err := LoadConfig(LoadConfigOpts{
		WalletKeyFlag: testKeyHex,
		FeeRateFlag:   "444",
		Env:           map[string]string{"BITFS_BUY_FEE_RATE_SAT_PER_KB": "333"},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(444), cfg.FeeRateSatPerKB)
}

func TestLoadConfig_InvalidFeeRateIgnored(t *testing.T) {
	cfg, err := LoadConfig(LoadConfigOpts{
		WalletKeyFlag: testKeyHex,
		Env:           map[string]string{"BITFS_BUY_FEE_RATE_SAT_PER_KB": "invalid"},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), cfg.FeeRateSatPerKB)
}
