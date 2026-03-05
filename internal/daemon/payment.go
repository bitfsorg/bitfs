package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitfsorg/libbitfs-go/method42"
	"github.com/bitfsorg/libbitfs-go/payment"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
)

// invoiceSnapshot holds a read-only snapshot of InvoiceRecord fields,
// captured under invoicesMu to prevent data races when fields are read
// outside the lock scope. [Audit fix C-1, H-5, M-1]
type invoiceSnapshot struct {
	ID           string
	TotalPrice   uint64
	CapsuleHash  string
	PricePerKB   uint64
	FileSize     uint64
	PaymentAddr  string
	SellerPubKey string
	Paid         bool
	CapsuleNonce []byte
	HTLCScript   []byte
	Capsule      []byte
	Expiry       time.Time
	NodePNode    []byte
	KeyHash      []byte
	FileTxID     []byte
}

// InvoiceRecord tracks a pending or completed content purchase.
type InvoiceRecord struct {
	ID           string    `json:"invoice_id"`
	TotalPrice   uint64    `json:"total_price"`
	NodePNode    []byte    `json:"-"`
	KeyHash      []byte    `json:"-"`
	FileTxID     []byte    `json:"-"` // 32-byte file transaction ID (binds capsule hash to file identity)
	PricePerKB   uint64    `json:"price_per_kb"`
	FileSize     uint64    `json:"file_size"`
	PaymentAddr  string    `json:"payment_addr"`
	SellerPubKey string    `json:"seller_pubkey"` // Hex-encoded compressed seller pubkey (for HTLC 2-of-2 multisig)
	CapsuleHash  string    `json:"capsule_hash"`
	HTLCScript   []byte    `json:"-"`                       // Precomputed HTLC script for verification
	Capsule      []byte    `json:"capsule,omitempty"`       // ECDH capsule for buyer (persisted for crash recovery)
	CapsuleNonce []byte    `json:"capsule_nonce,omitempty"` // Per-invoice nonce for capsule unlinkability
	Expiry       time.Time `json:"expiry"`
	Paid         bool      `json:"paid"`
}

// DefaultInvoiceExpiry is the default invoice time-to-live.
const DefaultInvoiceExpiry = 1 * time.Hour

// maxHTLCBodySize is the maximum size of an HTLC transaction body (1 MB).
const maxHTLCBodySize = 1 << 20

const (
	// invoiceEvictionInterval is how often the background eviction loop runs.
	invoiceEvictionInterval = 5 * time.Minute

	// maxInvoiceAge is the maximum age for any invoice (paid or unpaid) before
	// it becomes eligible for eviction. Paid invoices are kept for this duration
	// past their expiry as a grace period for crash recovery and auditing.
	maxInvoiceAge = 30 * time.Minute

	// maxInvoices is the hard cap on total invoice count. When reached, eviction
	// runs eagerly; if still at cap, new invoice creation is rejected with 503.
	maxInvoices = 10000
)

// servePaidContent returns 402 Payment Required for paid content,
// generating and storing an invoice for the purchase flow.
// Uses libbitfs/payment for invoice creation, price calculation, and HTTP headers.
//
// Capsule computation is deferred to handleGetBuyInfo, where the buyer
// provides their public key. This is required because the capsule is
// XOR-masked with a buyer-specific mask derived from ECDH(D_node, P_buyer).
func (d *Daemon) servePaidContent(w http.ResponseWriter, node *NodeInfo) {
	sellerPriv, _, err := d.wallet.GetSellerKeyPair()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "WALLET_ERROR", "Failed to get seller key pair")
		return
	}

	// Derive payment address from seller's public key (proper P2PKH address).
	sellerAddr, err := script.NewAddressFromPublicKey(sellerPriv.PubKey(), d.config.Mainnet)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "ADDR_ERROR", "Failed to derive payment address")
		return
	}
	paymentAddr := sellerAddr.AddressString

	// Determine invoice TTL in seconds.
	ttlSeconds := int64(DefaultInvoiceExpiry / time.Second)
	if d.config.Payment.InvoiceExpiry > 0 {
		ttlSeconds = d.config.Payment.InvoiceExpiry
	}

	// Create invoice without capsule hash (deferred until buyer identifies themselves).
	inv, err := payment.NewInvoice(node.PricePerKB, node.FileSize, paymentAddr, nil, ttlSeconds)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INVOICE_CREATE_FAILED", "Failed to create invoice")
		return
	}

	// Convert payment.Invoice to daemon's InvoiceRecord for internal state management.
	sellerPubKeyHex := hex.EncodeToString(sellerPriv.PubKey().Compressed())
	record := &InvoiceRecord{
		ID:           inv.ID,
		TotalPrice:   inv.Price,
		NodePNode:    node.PNode,
		KeyHash:      node.KeyHash,
		FileTxID:     node.FileTxID,
		PricePerKB:   inv.PricePerKB,
		FileSize:     inv.FileSize,
		PaymentAddr:  inv.PaymentAddr,
		SellerPubKey: sellerPubKeyHex,
		Expiry:       time.Unix(inv.Expiry, 0),
		Paid:         false,
	}

	// Enforce invoice cap: eagerly evict if at capacity.
	d.invoicesMu.RLock()
	atCap := len(d.invoices) >= maxInvoices
	d.invoicesMu.RUnlock()

	if atCap {
		d.evictExpiredInvoices()
		// Re-check after eviction.
		d.invoicesMu.RLock()
		stillAtCap := len(d.invoices) >= maxInvoices
		d.invoicesMu.RUnlock()
		if stillAtCap {
			writeJSONError(w, http.StatusServiceUnavailable, "INVOICE_LIMIT",
				"Too many active invoices, please try again later")
			return
		}
	}

	// Store the invoice.
	d.invoicesMu.Lock()
	d.invoices[inv.ID] = record
	d.invoicesMu.Unlock()

	// Set payment HTTP headers and return 402 status via libbitfs/payment.
	w.Header().Set("Content-Type", "application/json")
	payment.SetPaymentHeaders(w, payment.PaymentHeadersFromInvoice(inv))

	// Return JSON body with invoice details.
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":        "payment required",
		"invoice_id":   inv.ID,
		"total_price":  inv.Price,
		"price_per_kb": inv.PricePerKB,
		"file_size":    inv.FileSize,
		"payment_addr": inv.PaymentAddr,
	})
}

// handleGetBuyInfo handles GET /_bitfs/buy/{txid} and returns buy information
// for a previously generated invoice.
//
// The buyer must provide their public key via the "buyer_pubkey" query parameter
// (hex-encoded compressed 33-byte key). On first call with a valid buyer_pubkey,
// the server computes the XOR-masked capsule using:
//
//	capsule = aes_key XOR HKDF(ECDH(D_node, P_buyer).x, key_hash, "bitfs-buyer-mask")
//
// and stores the capsule + capsule_hash in the invoice for the HTLC flow.
func (d *Daemon) handleGetBuyInfo(w http.ResponseWriter, r *http.Request) {
	txid := r.PathValue("txid")
	if txid == "" {
		writeJSONError(w, http.StatusBadRequest, "MISSING_TXID", "Invoice ID is required")
		return
	}

	// Snapshot all needed fields under lock to prevent data race (C-1, H-5, M-1).
	buyerPubHex := r.URL.Query().Get("buyer_pubkey")

	d.invoicesMu.Lock()
	invoice, ok := d.invoices[txid]
	if !ok {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Invoice not found")
		return
	}

	// Check if the invoice has expired (under lock to avoid TOCTOU — M-3).
	if time.Now().After(invoice.Expiry) {
		delete(d.invoices, txid)
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusNotFound, "EXPIRED", "Invoice has expired")
		return
	}

	// Compute capsule on demand when buyer provides their pubkey.
	if buyerPubHex != "" && len(invoice.NodePNode) > 0 && len(invoice.Capsule) == 0 {
		if capsuleErr := d.computeInvoiceCapsule(invoice, buyerPubHex); capsuleErr != nil {
			d.invoicesMu.Unlock()
			log.Printf("[buy] ERROR: capsule computation failed for invoice %s: %v", invoice.ID, capsuleErr)
			writeJSONError(w, http.StatusInternalServerError, "CAPSULE_FAILED",
				"Cannot compute payment capsule")
			return
		}
	}

	// Snapshot all fields before releasing lock.
	snap := invoiceSnapshot{
		ID:           invoice.ID,
		TotalPrice:   invoice.TotalPrice,
		CapsuleHash:  invoice.CapsuleHash,
		PricePerKB:   invoice.PricePerKB,
		FileSize:     invoice.FileSize,
		PaymentAddr:  invoice.PaymentAddr,
		SellerPubKey: invoice.SellerPubKey,
		Paid:         invoice.Paid,
	}
	if len(invoice.CapsuleNonce) > 0 {
		snap.CapsuleNonce = make([]byte, len(invoice.CapsuleNonce))
		copy(snap.CapsuleNonce, invoice.CapsuleNonce)
	}
	d.invoicesMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	buyInfoResp := map[string]interface{}{
		"invoice_id":    snap.ID,
		"total_price":   snap.TotalPrice,
		"capsule_hash":  snap.CapsuleHash,
		"price_per_kb":  snap.PricePerKB,
		"file_size":     snap.FileSize,
		"payment_addr":  snap.PaymentAddr,
		"seller_pubkey": snap.SellerPubKey,
		"paid":          snap.Paid,
	}
	if len(snap.CapsuleNonce) > 0 {
		buyInfoResp["capsule_nonce"] = hex.EncodeToString(snap.CapsuleNonce)
	}
	_ = json.NewEncoder(w).Encode(buyInfoResp)
}

// computeInvoiceCapsule computes the XOR-masked capsule and HTLC script for an invoice.
// Must be called while holding d.invoicesMu write lock.
func (d *Daemon) computeInvoiceCapsule(invoice *InvoiceRecord, buyerPubHex string) error {
	buyerPubBytes, err := hex.DecodeString(buyerPubHex)
	if err != nil || len(buyerPubBytes) != 33 {
		return fmt.Errorf("invalid buyer pubkey hex")
	}
	buyerPub, err := ec.PublicKeyFromBytes(buyerPubBytes)
	if err != nil {
		return fmt.Errorf("invalid buyer public key: %w", err)
	}
	nodePriv, nodePub, err := d.wallet.DeriveNodeKeyPair(invoice.NodePNode)
	if err != nil {
		return fmt.Errorf("derive node key pair: %w", err)
	}

	// Use invoice ID bytes as capsule nonce for per-purchase unlinkability.
	invoiceIDBytes, err := hex.DecodeString(invoice.ID)
	if err != nil {
		return fmt.Errorf("decode invoice ID hex: %w", err)
	}
	capsule, err := method42.ComputeCapsuleWithNonce(nodePriv, nodePub, buyerPub, invoice.KeyHash, invoiceIDBytes)
	if err != nil {
		return fmt.Errorf("compute capsule: %w", err)
	}

	capsuleHash, err := method42.ComputeCapsuleHash(invoice.FileTxID, capsule)
	if err != nil {
		return fmt.Errorf("compute capsule hash: %w", err)
	}

	// Build HTLC script for payment verification.
	sellerPriv, _, err := d.wallet.GetSellerKeyPair()
	if err != nil {
		return fmt.Errorf("get seller key pair: %w", err)
	}
	sellerPKH := sellerPriv.PubKey().Hash()
	htlcScript, err := payment.BuildHTLC(&payment.HTLCParams{
		BuyerPubKey:  buyerPubBytes,
		SellerPubKey: sellerPriv.PubKey().Compressed(),
		SellerAddr:   sellerPKH,
		CapsuleHash:  capsuleHash,
		Amount:       invoice.TotalPrice,
		Timeout:      payment.DefaultHTLCTimeout,
		InvoiceID:    invoiceIDBytes,
	})
	if err != nil {
		return fmt.Errorf("build HTLC script: %w", err)
	}

	invoice.HTLCScript = htlcScript
	invoice.Capsule = capsule
	invoice.CapsuleNonce = invoiceIDBytes
	invoice.CapsuleHash = hex.EncodeToString(capsuleHash)
	return nil
}

// handleSubmitHTLC handles POST /_bitfs/buy/{txid} and accepts an HTLC
// transaction in exchange for the encrypted content capsule.
func (d *Daemon) handleSubmitHTLC(w http.ResponseWriter, r *http.Request) {
	txid := r.PathValue("txid")
	if txid == "" {
		writeJSONError(w, http.StatusBadRequest, "MISSING_TXID", "Invoice ID is required")
		return
	}

	// Read body BEFORE acquiring lock (I/O should not hold locks).
	defer func() { _ = r.Body.Close() }()
	htlcBody, err := io.ReadAll(io.LimitReader(r.Body, maxHTLCBodySize))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body")
		return
	}
	if len(htlcBody) == 0 {
		writeJSONError(w, http.StatusBadRequest, "EMPTY_TX", "HTLC transaction body is required")
		return
	}

	// Single write lock for the entire check-verify-set sequence to prevent TOCTOU.
	// Snapshot all needed fields under lock to prevent data race (C-1, H-5, H-8).
	d.invoicesMu.Lock()
	invoice, ok := d.invoices[txid]
	if !ok {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Invoice not found")
		return
	}
	if time.Now().After(invoice.Expiry) {
		delete(d.invoices, txid)
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusNotFound, "EXPIRED", "Invoice has expired")
		return
	}
	if invoice.Paid {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusConflict, "ALREADY_PAID", "Invoice has already been paid")
		return
	}
	// Mark paid immediately to prevent concurrent claims (optimistic lock).
	invoice.Paid = true

	// Snapshot fields needed for verification and response (outside lock).
	snap := invoiceSnapshot{
		ID:         invoice.ID,
		TotalPrice: invoice.TotalPrice,
		HTLCScript: make([]byte, len(invoice.HTLCScript)),
	}
	copy(snap.HTLCScript, invoice.HTLCScript)
	if len(invoice.Capsule) > 0 {
		snap.Capsule = make([]byte, len(invoice.Capsule))
		copy(snap.Capsule, invoice.Capsule)
	}
	if len(invoice.CapsuleNonce) > 0 {
		snap.CapsuleNonce = make([]byte, len(invoice.CapsuleNonce))
		copy(snap.CapsuleNonce, invoice.CapsuleNonce)
	}
	d.invoicesMu.Unlock()

	// Verify the payment transaction (outside lock — crypto/parsing is CPU-bound, not lock-worthy).
	// On any failure below, rollback the Paid flag.
	// Rollback uses a single lock acquisition covering both usedTxIDs and invoice.Paid
	// to prevent inconsistent state windows (H-8).
	rollbackPaid := func() {
		d.invoicesMu.Lock()
		invoice.Paid = false
		d.invoicesMu.Unlock()
	}

	if len(snap.HTLCScript) == 0 {
		rollbackPaid()
		writeJSONError(w, http.StatusInternalServerError, "NO_HTLC_SCRIPT",
			"Invoice missing HTLC script — cannot verify payment")
		return
	}

	// HTLC path: verify the funding tx has a matching HTLC output.
	if _, err := payment.VerifyHTLCFunding(htlcBody, snap.HTLCScript, snap.TotalPrice); err != nil {
		rollbackPaid()
		log.Printf("[buy] ERROR: HTLC verification failed for invoice %s: %v", snap.ID, err)
		writeJSONError(w, http.StatusBadRequest, "PAYMENT_INVALID",
			"HTLC verification failed")
		return
	}

	// Replay protection: ensure the same transaction is not used for multiple invoices.
	submittedTx, parseErr := transaction.NewTransactionFromBytes(htlcBody)
	if parseErr != nil {
		rollbackPaid()
		writeJSONError(w, http.StatusBadRequest, "PAYMENT_INVALID", "Cannot parse transaction")
		return
	}
	submittedTxID := submittedTx.TxID().String()

	d.usedTxIDsMu.Lock()
	if existingInvoice, used := d.usedTxIDs[submittedTxID]; used {
		d.usedTxIDsMu.Unlock()
		rollbackPaid()
		log.Printf("[buy] WARN: tx %s reused (original invoice %s, attempted invoice %s)", submittedTxID, existingInvoice, snap.ID)
		writeJSONError(w, http.StatusConflict, "TX_REUSED",
			"Transaction already used for another invoice")
		return
	}
	d.usedTxIDs[submittedTxID] = snap.ID
	d.usedTxIDsMu.Unlock()

	// Broadcast the payment transaction to the blockchain before revealing capsule.
	if d.chain == nil {
		// Atomic rollback: both usedTxIDs and paid flag under their respective locks.
		d.usedTxIDsMu.Lock()
		delete(d.usedTxIDs, submittedTxID)
		d.usedTxIDsMu.Unlock()
		rollbackPaid()
		writeJSONError(w, http.StatusServiceUnavailable, "NO_CHAIN",
			"Blockchain service not configured; cannot verify payment broadcast")
		return
	}
	txHex := hex.EncodeToString(htlcBody)
	_, broadcastErr := d.chain.BroadcastTx(r.Context(), txHex)
	if broadcastErr != nil {
		// Rollback replay tracking and paid flag on broadcast failure.
		d.usedTxIDsMu.Lock()
		delete(d.usedTxIDs, submittedTxID)
		d.usedTxIDsMu.Unlock()
		rollbackPaid()
		log.Printf("[buy] ERROR: broadcast failed for invoice %s: %v", snap.ID, broadcastErr)
		writeJSONError(w, http.StatusBadRequest, "BROADCAST_FAILED",
			"Payment transaction rejected")
		return
	}

	// After broadcast succeeds, payment is final regardless of HTTP response delivery.
	// Do NOT rollback invoice.Paid after this point — the tx is on-chain.

	// Persist paid invoice before sending response (crash recovery).
	// Marshal invoice under lock (M-2).
	d.invoicesMu.RLock()
	_ = d.persistInvoice(invoice)
	d.invoicesMu.RUnlock()

	// Return the capsule (ECDH shared secret) using snapshotted values.
	if len(snap.Capsule) == 0 {
		// Payment is on-chain but capsule was never computed (server-side bug).
		// Do NOT rollback paid or usedTxIDs; the payment already happened.
		writeJSONError(w, http.StatusInternalServerError, "NO_CAPSULE", "No capsule computed for this invoice")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"invoice_id": snap.ID,
		"capsule":    hex.EncodeToString(snap.Capsule),
		"paid":       true,
	}
	// Include capsule nonce so the buyer can derive the matching buyer_mask.
	if len(snap.CapsuleNonce) > 0 {
		resp["capsule_nonce"] = hex.EncodeToString(snap.CapsuleNonce)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePayInvoice handles POST /_bitfs/pay/{invoice_id}.
// Accepts bandwidth payment for a previously issued invoice.
// Body must contain a raw transaction hex (JSON or plain) for verification.
func (d *Daemon) handlePayInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID := r.PathValue("invoice_id")
	if invoiceID == "" {
		writeJSONError(w, http.StatusBadRequest, "MISSING_ID", "Missing invoice_id")
		return
	}

	// Read body BEFORE acquiring lock (I/O should not hold locks).
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxHTLCBodySize))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body")
		return
	}

	// Check invoice state under lock first and snapshot needed fields (C-1, H-5, H-8).
	d.invoicesMu.Lock()
	invoice, ok := d.invoices[invoiceID]
	if !ok {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Invoice not found")
		return
	}
	if invoice.Paid {
		d.invoicesMu.Unlock()
		// Idempotent: already paid.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "paid", "invoice_id": invoiceID,
		})
		return
	}
	if time.Now().After(invoice.Expiry) {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusGone, "EXPIRED", "Invoice has expired")
		return
	}

	// Require non-empty body for new payments.
	if len(body) == 0 {
		d.invoicesMu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "EMPTY_BODY", "Payment transaction body is required")
		return
	}

	// Mark paid immediately to prevent concurrent claims (optimistic lock).
	invoice.Paid = true

	// Snapshot all fields needed for verification outside lock.
	paySnap := invoiceSnapshot{
		ID:          invoice.ID,
		TotalPrice:  invoice.TotalPrice,
		PaymentAddr: invoice.PaymentAddr,
		Expiry:      invoice.Expiry,
	}
	d.invoicesMu.Unlock()

	// Parse body as JSON { "raw_tx": "<hex>" } or raw hex string.
	var rawTxHex string
	var req struct {
		RawTx string `json:"raw_tx"`
	}
	if json.Unmarshal(body, &req) == nil && req.RawTx != "" {
		rawTxHex = req.RawTx
	} else {
		rawTxHex = strings.TrimSpace(string(body))
	}

	// Rollback helper for any failure after marking paid.
	rollbackPaid := func() {
		d.invoicesMu.Lock()
		invoice.Paid = false
		d.invoicesMu.Unlock()
	}

	rawTx, err := hex.DecodeString(rawTxHex)
	if err != nil {
		rollbackPaid()
		writeJSONError(w, http.StatusBadRequest, "INVALID_TX", "Invalid transaction hex")
		return
	}

	// Verify P2PKH payment to seller address using snapshotted values.
	proof := &payment.PaymentProof{RawTx: rawTx}
	payInv := &payment.Invoice{
		ID:          paySnap.ID,
		Price:       paySnap.TotalPrice,
		PaymentAddr: paySnap.PaymentAddr,
		Expiry:      paySnap.Expiry.Unix(),
	}
	if _, err := payment.VerifyPayment(proof, payInv); err != nil {
		rollbackPaid()
		log.Printf("[pay] ERROR: payment verification failed for invoice %s: %v", paySnap.ID, err)
		writeJSONError(w, http.StatusBadRequest, "VERIFICATION_FAILED", "Payment verification failed")
		return
	}

	// Replay protection.
	parsedTx, parseErr := transaction.NewTransactionFromBytes(rawTx)
	if parseErr != nil {
		rollbackPaid()
		writeJSONError(w, http.StatusBadRequest, "INVALID_TX", "Cannot parse transaction")
		return
	}
	submittedTxID := parsedTx.TxID().String()

	d.usedTxIDsMu.Lock()
	if _, used := d.usedTxIDs[submittedTxID]; used {
		d.usedTxIDsMu.Unlock()
		rollbackPaid()
		writeJSONError(w, http.StatusConflict, "REPLAY", "Transaction already used")
		return
	}
	d.usedTxIDs[submittedTxID] = invoiceID
	d.usedTxIDsMu.Unlock()

	// Broadcast the payment transaction to the blockchain.
	if d.chain != nil {
		txHex := hex.EncodeToString(rawTx)
		_, broadcastErr := d.chain.BroadcastTx(r.Context(), txHex)
		if broadcastErr != nil {
			d.usedTxIDsMu.Lock()
			delete(d.usedTxIDs, submittedTxID)
			d.usedTxIDsMu.Unlock()
			rollbackPaid()
			log.Printf("[pay] ERROR: broadcast failed for invoice %s: %v", paySnap.ID, broadcastErr)
			writeJSONError(w, http.StatusBadRequest, "BROADCAST_FAILED",
				"Payment transaction rejected")
			return
		}
	}

	// After broadcast succeeds, payment is final regardless of HTTP response delivery.
	// Do NOT rollback invoice.Paid after this point.
	// Persist under read lock (M-2).
	d.invoicesMu.RLock()
	_ = d.persistInvoice(invoice)
	d.invoicesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "paid", "invoice_id": invoiceID,
	})
}

// handleSales handles GET /_bitfs/sales and returns sales (invoice) records,
// optionally filtered by payment status.
//
// Query parameters:
//   - status: "all" (default), "paid", or "pending"
//   - limit:  maximum number of records to return (default 50)
func (d *Daemon) handleSales(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	type saleRecord struct {
		InvoiceID string `json:"invoice_id"`
		Price     uint64 `json:"price"`
		KeyHash   string `json:"key_hash"`
		Timestamp int64  `json:"timestamp"`
		Paid      bool   `json:"paid"`
	}

	d.invoicesMu.RLock()
	records := make([]saleRecord, 0, len(d.invoices))
	for _, inv := range d.invoices {
		switch status {
		case "paid":
			if !inv.Paid {
				continue
			}
		case "pending":
			if inv.Paid {
				continue
			}
		} // "all" includes everything

		keyHashHex := ""
		if len(inv.KeyHash) > 0 {
			keyHashHex = hex.EncodeToString(inv.KeyHash)
		}

		records = append(records, saleRecord{
			InvoiceID: inv.ID,
			Price:     inv.TotalPrice,
			KeyHash:   keyHashHex,
			Timestamp: inv.Expiry.Unix(),
			Paid:      inv.Paid,
		})

		if len(records) >= limit {
			break
		}
	}
	d.invoicesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}

// evictExpiredInvoices removes stale invoices and their associated usedTxIDs entries.
//
// Eviction rules:
//   - Unpaid invoices past their Expiry are removed immediately.
//   - Paid invoices are removed only after maxInvoiceAge past their Expiry,
//     giving a grace period for crash recovery and audit queries.
//
// Lock ordering: invoicesMu first, then usedTxIDsMu (never reversed).
// Returns the number of evicted invoices.
func (d *Daemon) evictExpiredInvoices() int {
	now := time.Now()

	// Phase 1: collect eviction candidates under invoicesMu.
	d.invoicesMu.Lock()
	var evictedIDs []string
	var paidEvictedIDs []string // track paid invoices separately for usedTxIDs cleanup
	for id, inv := range d.invoices {
		if !inv.Paid && now.After(inv.Expiry) {
			// Unpaid + expired: evict immediately.
			evictedIDs = append(evictedIDs, id)
			delete(d.invoices, id)
		} else if inv.Paid && now.Sub(inv.Expiry) > maxInvoiceAge {
			// Paid but well past expiry grace period: evict.
			evictedIDs = append(evictedIDs, id)
			paidEvictedIDs = append(paidEvictedIDs, id)
			delete(d.invoices, id)
		}
	}
	d.invoicesMu.Unlock()

	// Phase 2: clean usedTxIDs for evicted paid invoices.
	if len(paidEvictedIDs) > 0 {
		// Build a set for O(1) lookup.
		evictedSet := make(map[string]struct{}, len(paidEvictedIDs))
		for _, id := range paidEvictedIDs {
			evictedSet[id] = struct{}{}
		}
		d.usedTxIDsMu.Lock()
		for txid, invoiceID := range d.usedTxIDs {
			if _, ok := evictedSet[invoiceID]; ok {
				delete(d.usedTxIDs, txid)
			}
		}
		d.usedTxIDsMu.Unlock()
	}

	return len(evictedIDs)
}

// startInvoiceEviction runs a background goroutine that periodically evicts
// stale invoices. It stops when ctx is canceled.
func (d *Daemon) startInvoiceEviction(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(invoiceEvictionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.evictExpiredInvoices()
			case <-ctx.Done():
				return
			}
		}
	}()
}
