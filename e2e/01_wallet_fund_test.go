//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/bitfsorg/bitfs/e2e/testutil"
	"github.com/bitfsorg/libbitfs-go/wallet"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/stretchr/testify/require"
)

// TestWalletFund verifies that HD wallet key derivation produces real addresses
// that can receive regtest coins. It creates a wallet, derives a fee key,
// funds the derived address via the regtest node, and confirms the UTXO exists.
func TestWalletFund(t *testing.T) {
	node := testutil.NewTestNode(t)
	ctx := context.Background()

	// Step 1: Create a real HD wallet.
	mnemonic, err := wallet.GenerateMnemonic(wallet.Mnemonic12Words)
	require.NoError(t, err)
	t.Logf("Generated mnemonic: %s", mnemonic)

	seed, err := wallet.SeedFromMnemonic(mnemonic, "")
	require.NoError(t, err)

	w, err := wallet.NewWallet(seed, testutil.NetworkConfigFor(node.Network()))
	require.NoError(t, err)

	// Step 2: Derive a fee key (m/44'/236'/0'/0/0).
	feeKey, err := w.DeriveFeeKey(0, 0)
	require.NoError(t, err)
	t.Logf("Fee key path: %s", feeKey.Path)

	// Step 3: Convert to BSV address.
	addr, err := script.NewAddressFromPublicKey(feeKey.PublicKey, node.Network() == "mainnet")
	require.NoError(t, err)
	t.Logf("Fee address: %s", addr.AddressString)

	// Step 4: Import address so the node can track UTXOs for it.
	err = node.ImportAddress(ctx, addr.AddressString)
	require.NoError(t, err, "import address")

	// Step 5: Fund the address via node.Fund (handles mining on regtest,
	// faucet/WIF on live networks).
	_, err = node.Fund(ctx, addr.AddressString, 0.01)
	require.NoError(t, err, "fund address")

	// Step 6: Verify the UTXO exists.
	utxos, err := node.ListUnspent(ctx, addr.AddressString)
	require.NoError(t, err)
	require.NotEmpty(t, utxos, "should have at least one UTXO")
	require.InDelta(t, 0.01, utxos[0].Amount, 0.0001)

	// Step 7: Log the UTXO details.
	t.Logf("UTXO: %s:%d = %.8f BSV", utxos[0].TxID, utxos[0].Vout, utxos[0].Amount)
}
