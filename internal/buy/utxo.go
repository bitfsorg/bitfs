package buy

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/bitfsorg/libbitfs-go/network"
	"github.com/bitfsorg/libbitfs-go/payment"
)

// ErrInsufficientBalance is returned when available UTXOs cannot cover the
// requested amount plus estimated transaction fees.
var ErrInsufficientBalance = errors.New("buyer: insufficient balance")

// EstimateFee estimates the transaction fee in satoshis for a standard
// P2PKH transaction. Each input contributes ~148 bytes (41 bytes outpoint +
// ~107 bytes unlocking script), each output ~34 bytes, plus 10 bytes overhead.
func EstimateFee(nInputs, nOutputs int, feeRate uint64) uint64 {
	size := uint64(nInputs*148 + nOutputs*34 + 10)
	return size * feeRate
}

// htlcScriptSizeEstimate is a conservative estimate of HTLC locking script size.
// Covers both with-InvoiceID (200 bytes) and without (180 bytes) variants.
const htlcScriptSizeEstimate = 200

// EstimateHTLCFee estimates the fee for an HTLC funding transaction.
// Layout: nInputs x P2PKH inputs + 1 HTLC output + 1 P2PKH change output.
func EstimateHTLCFee(nInputs int, feeRate uint64) uint64 {
	inputSize := uint64(nInputs * 148)
	htlcOutput := uint64(8 + 1 + htlcScriptSizeEstimate) // satoshis + varint + script
	changeOutput := uint64(34)                             // P2PKH
	overhead := uint64(10)
	return (inputSize + htlcOutput + changeOutput + overhead) * feeRate
}

// SelectUTXOs queries the blockchain service for UTXOs belonging to address
// and selects the minimum set needed to cover amount + estimated fee using a
// greedy largest-first algorithm. Returns HTLCUTXO slices ready for use in
// HTLC funding transactions.
func SelectUTXOs(ctx context.Context, svc network.BlockchainService, address string, amount, feeRate uint64) ([]*payment.HTLCUTXO, error) {
	netUTXOs, err := svc.ListUnspent(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("buyer: query UTXOs: %w", err)
	}

	// Sort descending by amount (greedy: pick largest first).
	sort.Slice(netUTXOs, func(i, j int) bool {
		return netUTXOs[i].Amount > netUTXOs[j].Amount
	})

	var selected []*payment.HTLCUTXO
	var totalInput uint64

	for _, u := range netUTXOs {
		txid, decErr := hex.DecodeString(u.TxID)
		if decErr != nil {
			continue
		}
		if len(txid) != 32 {
			continue
		}
		scriptPK, decErr := hex.DecodeString(u.ScriptPubKey)
		if decErr != nil {
			continue
		}

		selected = append(selected, &payment.HTLCUTXO{
			TxID:         txid,
			Vout:         u.Vout,
			Amount:       u.Amount,
			ScriptPubKey: scriptPK,
		})
		totalInput += u.Amount

		// Estimate fee for HTLC funding tx (1 HTLC output + 1 change output).
		fee := EstimateHTLCFee(len(selected), feeRate)
		if totalInput >= amount+fee {
			return selected, nil
		}
	}

	// Could not cover amount + fee with all available UTXOs.
	var totalAvailable uint64
	for _, u := range netUTXOs {
		totalAvailable += u.Amount
	}
	return nil, fmt.Errorf("%w: need %d sat, have %d sat",
		ErrInsufficientBalance,
		amount+EstimateHTLCFee(len(netUTXOs), feeRate),
		totalAvailable)
}
