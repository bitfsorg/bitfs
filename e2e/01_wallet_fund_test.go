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
	fundAmount := testutil.LoadConfig().FundAmount

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
	// WIF funding on live networks).
	fundedUTXO, err := node.Fund(ctx, addr.AddressString, fundAmount)
	require.NoError(t, err, "fund address")
	require.NotNil(t, fundedUTXO, "funded UTXO should not be nil")

	// Step 6: Prefer node.Fund result, then optionally verify via listunspent.
	utxo := fundedUTXO
	utxos, err := node.ListUnspent(ctx, addr.AddressString)
	require.NoError(t, err)
	if len(utxos) > 0 {
		utxo = &utxos[0]
	} else {
		t.Logf("listunspent not yet updated on %s; using funded UTXO directly", node.Network())
	}
	require.InDelta(t, fundAmount, utxo.Amount, 0.0001)

	// Step 7: Log the UTXO details.
	t.Logf("UTXO: %s:%d = %.8f BSV", utxo.TxID, utxo.Vout, utxo.Amount)
}
