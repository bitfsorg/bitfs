//go:build e2e

package testutil

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	libtx "github.com/bitfsorg/libbitfs-go/tx"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	feemodel "github.com/bsv-blockchain/go-sdk/transaction/fee_model"
	"github.com/bsv-blockchain/go-sdk/transaction/template/p2pkh"
)

// fundFromWIF imports a pre-funded WIF private key into the node's wallet
// and sends the specified amount to the target address.
// Returns (txid, rawTxHex).
func fundFromWIF(ctx context.Context, rpc *RPCClient, wif, addr string, amount float64) (string, string, error) {
	// Import the WIF private key without rescanning (we assume it's already known
	// or has been imported before; rescan=false keeps it fast).
	if err := rpc.Call(ctx, "importprivkey", []interface{}{wif, "", false}, nil); err != nil {
		return "", "", fmt.Errorf("importprivkey: %w", err)
	}

	// Send the requested amount to the target address.
	var txid string
	if err := rpc.Call(ctx, "sendtoaddress", []interface{}{addr, amount}, &txid); err != nil {
		return "", "", fmt.Errorf("sendtoaddress(%s, %f): %w", addr, amount, err)
	}

	var rawHex string
	if err := rpc.Call(ctx, "getrawtransaction", []interface{}{txid, false}, &rawHex); err != nil {
		return "", "", fmt.Errorf("getrawtransaction(%s): %w", txid, err)
	}

	return txid, rawHex, nil
}

// fundFromWIFWOC funds addr by signing a transaction locally with the given WIF
// and broadcasting it via Whatsonchain.
// Returns (txid, rawTxHex).
func fundFromWIFWOC(ctx context.Context, woc *wocClient, cfg *Config, wif, addr string, amount float64) (string, string, error) {
	priv, err := ec.PrivateKeyFromWif(wif)
	if err != nil {
		return "", "", fmt.Errorf("parse WIF: %w", err)
	}

	sourceAddr, err := script.NewAddressFromPublicKey(priv.PubKey(), cfg.IsMainnet())
	if err != nil {
		return "", "", fmt.Errorf("derive source address: %w", err)
	}

	unspent, err := woc.listUnspent(ctx, sourceAddr.AddressString)
	if err != nil {
		return "", "", fmt.Errorf("list unspent on source address %s: %w", sourceAddr.AddressString, err)
	}
	if len(unspent) == 0 {
		return "", "", fmt.Errorf("no UTXOs available on funding WIF address %s", sourceAddr.AddressString)
	}

	// Use larger UTXOs first to minimize input count and fee.
	sort.Slice(unspent, func(i, j int) bool { return unspent[i].Value > unspent[j].Value })

	amountSat := uint64(math.Round(amount * 1e8))
	if amountSat == 0 {
		return "", "", fmt.Errorf("invalid funding amount %.8f", amount)
	}

	selected := make([]wocUnspent, 0, len(unspent))
	var lastErr error
	for _, u := range unspent {
		if u.Value == 0 {
			continue
		}
		selected = append(selected, u)
		rawHex, err := buildFundingRawTxFromWIF(selected, priv, sourceAddr.AddressString, addr, amountSat)
		if err != nil {
			if errors.Is(err, transaction.ErrInsufficientInputs) {
				lastErr = err
				continue
			}
			return "", "", err
		}
		txid, err := woc.broadcastRawTx(ctx, rawHex)
		if err != nil {
			return "", "", fmt.Errorf("broadcast funding tx: %w", err)
		}
		return txid, rawHex, nil
	}

	if lastErr == nil {
		lastErr = transaction.ErrInsufficientInputs
	}
	return "", "", fmt.Errorf("insufficient funds in WIF address %s: %w", sourceAddr.AddressString, lastErr)
}

// fundFromWIFARC funds addr by signing a transaction locally with the given WIF,
// selecting source UTXOs via WoC, and broadcasting through ARC.
// Returns (txid, rawTxHex).
func fundFromWIFARC(ctx context.Context, arc *arcClient, woc *wocClient, cfg *Config, wif, addr string, amount float64) (string, string, error) {
	priv, err := ec.PrivateKeyFromWif(wif)
	if err != nil {
		return "", "", fmt.Errorf("parse WIF: %w", err)
	}

	sourceAddr, err := script.NewAddressFromPublicKey(priv.PubKey(), cfg.IsMainnet())
	if err != nil {
		return "", "", fmt.Errorf("derive source address: %w", err)
	}

	unspent, err := woc.listUnspent(ctx, sourceAddr.AddressString)
	if err != nil {
		return "", "", fmt.Errorf("list unspent on source address %s: %w", sourceAddr.AddressString, err)
	}
	if len(unspent) == 0 {
		return "", "", fmt.Errorf("no UTXOs available on funding WIF address %s", sourceAddr.AddressString)
	}

	sort.Slice(unspent, func(i, j int) bool { return unspent[i].Value > unspent[j].Value })

	amountSat := uint64(math.Round(amount * 1e8))
	if amountSat == 0 {
		return "", "", fmt.Errorf("invalid funding amount %.8f", amount)
	}

	selected := make([]wocUnspent, 0, len(unspent))
	var lastErr error
	for _, u := range unspent {
		if u.Value == 0 {
			continue
		}
		selected = append(selected, u)
		rawHex, err := buildFundingRawTxFromWIF(selected, priv, sourceAddr.AddressString, addr, amountSat)
		if err != nil {
			if errors.Is(err, transaction.ErrInsufficientInputs) {
				lastErr = err
				continue
			}
			return "", "", err
		}
		status, err := arc.broadcastRawTx(ctx, rawHex, cfg)
		if err != nil {
			return "", "", fmt.Errorf("broadcast funding tx via ARC: %w", err)
		}
		if isARCRejectedStatus(status.txStatusValue()) {
			return "", "", fmt.Errorf("broadcast funding tx via ARC rejected: status=%s title=%q detail=%q extra=%q",
				status.txStatusValue(), status.Title, arcStatusDetail(status), arcStatusExtraInfo(status))
		}
		txid := status.txIDValue()
		if !is64Hex(txid) {
			return "", "", fmt.Errorf("broadcast funding tx via ARC: missing txid (status=%d title=%q)", status.Status, status.Title)
		}
		return txid, rawHex, nil
	}

	if lastErr == nil {
		lastErr = transaction.ErrInsufficientInputs
	}
	return "", "", fmt.Errorf("insufficient funds in WIF address %s: %w", sourceAddr.AddressString, lastErr)
}

func fundingUTXOFromRawTx(rawHex, destAddr, expectedTxID string) (*UTXO, error) {
	rawHex = strings.TrimSpace(rawHex)
	if rawHex == "" {
		return nil, fmt.Errorf("empty funding raw transaction")
	}

	tx, err := transaction.NewTransactionFromHex(rawHex)
	if err != nil {
		return nil, fmt.Errorf("decode funding raw tx: %w", err)
	}
	txid := tx.TxID().String()
	if is64Hex(expectedTxID) && !strings.EqualFold(txid, expectedTxID) {
		return nil, fmt.Errorf("funding txid mismatch: expected=%s got=%s", expectedTxID, txid)
	}

	destScriptHex, err := p2pkhScriptHexForAddress(destAddr)
	if err != nil {
		return nil, err
	}

	for i, out := range tx.Outputs {
		if out == nil || out.LockingScript == nil {
			continue
		}
		lockHex := hex.EncodeToString(*out.LockingScript)
		if strings.EqualFold(lockHex, destScriptHex) {
			return &UTXO{
				TxID:          txid,
				Vout:          uint32(i),
				Address:       destAddr,
				ScriptPubKey:  lockHex,
				Amount:        float64(out.Satoshis) / 1e8,
				Confirmations: 0,
			}, nil
		}
	}

	return nil, fmt.Errorf("funding output for %s not found in tx %s", destAddr, txid)
}

func p2pkhScriptHexForAddress(addr string) (string, error) {
	a, err := script.NewAddressFromString(addr)
	if err != nil {
		return "", fmt.Errorf("decode address %s: %w", addr, err)
	}
	b := make([]byte, 0, 25)
	b = append(b, script.OpDUP, script.OpHASH160, script.OpDATA20)
	b = append(b, a.PublicKeyHash...)
	b = append(b, script.OpEQUALVERIFY, script.OpCHECKSIG)
	return hex.EncodeToString(b), nil
}

func buildFundingRawTxFromWIF(selected []wocUnspent, priv *ec.PrivateKey, sourceAddr, destAddr string, amountSat uint64) (string, error) {
	tx := transaction.NewTransaction()

	lockScriptBytes, err := libtx.BuildP2PKHScript(priv.PubKey())
	if err != nil {
		return "", fmt.Errorf("build P2PKH script: %w", err)
	}
	lockScriptHex := hex.EncodeToString(lockScriptBytes)

	unlocker, err := p2pkh.Unlock(priv, nil)
	if err != nil {
		return "", fmt.Errorf("create unlocker: %w", err)
	}

	for _, u := range selected {
		if err := tx.AddInputFrom(u.TxHash, u.TxPos, lockScriptHex, u.Value, unlocker); err != nil {
			return "", fmt.Errorf("add input %s:%d: %w", u.TxHash, u.TxPos, err)
		}
	}

	if err := tx.PayToAddress(destAddr, amountSat); err != nil {
		return "", fmt.Errorf("add destination output: %w", err)
	}
	if err := tx.PayToAddress(sourceAddr, 0); err != nil {
		return "", fmt.Errorf("add change output: %w", err)
	}
	tx.Outputs[len(tx.Outputs)-1].Change = true

	// 1000 sat/KB = 1 sat/byte.
	if err := tx.Fee(&feemodel.SatoshisPerKilobyte{Satoshis: 1000}, transaction.ChangeDistributionEqual); err != nil {
		return "", err
	}

	if err := tx.Sign(); err != nil {
		return "", fmt.Errorf("sign funding tx: %w", err)
	}

	return tx.String(), nil
}
