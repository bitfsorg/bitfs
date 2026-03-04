//go:build e2e

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type wocClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

const wocMaxRetries = 5

type wocChainInfo struct {
	Blocks        int64  `json:"blocks"`
	Headers       int64  `json:"headers"`
	BestBlockHash string `json:"bestblockhash"`
}

type wocUnspent struct {
	Height int64  `json:"height"`
	TxPos  uint32 `json:"tx_pos"`
	TxHash string `json:"tx_hash"`
	Value  uint64 `json:"value"` // satoshis
}

type wocUnspentAllResponse struct {
	Address       string       `json:"address"`
	Script        string       `json:"script"`
	Result        []wocUnspent `json:"result"`
	NextPageToken string       `json:"nextPageToken"`
	NextPage      string       `json:"nextPage"`
}

type wocScriptPubKey struct {
	Hex       string   `json:"hex"`
	Addresses []string `json:"addresses"`
}

type wocVout struct {
	Value        float64         `json:"value"`
	N            uint32          `json:"n"`
	ScriptPubKey wocScriptPubKey `json:"scriptPubKey"`
}

type wocTxInfo struct {
	TxID          string    `json:"txid"`
	BlockHash     string    `json:"blockhash"`
	Confirmations int64     `json:"confirmations"`
	BlockHeight   uint64    `json:"blockheight"`
	Vout          []wocVout `json:"vout"`
}

type wocBlockInfo struct {
	Hash              string   `json:"hash"`
	Confirmations     int64    `json:"confirmations"`
	Height            int64    `json:"height"`
	Version           int64    `json:"version"`
	MerkleRoot        string   `json:"merkleroot"`
	Time              int64    `json:"time"`
	Bits              string   `json:"bits"`
	Nonce             uint64   `json:"nonce"`
	PreviousBlockHash string   `json:"previousblockhash"`
	Tx                []string `json:"tx"`
}

type wocProofTSC struct {
	Index  uint32   `json:"index"`
	TxOrID string   `json:"txOrId"`
	Target string   `json:"target"` // block hash
	Nodes  []string `json:"nodes"`
}

func newWOCClient(baseURL, apiKey string) *wocClient {
	return &wocClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *wocClient) applyAuthHeaders(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	// Accept both documented and legacy header styles for compatibility.
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("woc-api-key", c.apiKey)
}

func (c *wocClient) getJSON(ctx context.Context, path string, out interface{}) error {
	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		c.applyAuthHeaders(req)
		resp, err := c.client.Do(req)
		if err != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, "")); sleepErr == nil {
					continue
				}
			}
			return fmt.Errorf("GET %s: %w", path, err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
					continue
				}
			}
			return fmt.Errorf("read response body: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decode WOC response for %s: %w", path, err)
			}
			return nil
		}

		if isRetryableWOCStatus(resp.StatusCode) && attempt < wocMaxRetries {
			if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
				continue
			}
		}
		return fmt.Errorf("WOC GET %s HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("WOC GET %s failed after retries", path)
}

func (c *wocClient) getText(ctx context.Context, path string) (string, error) {
	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		c.applyAuthHeaders(req)
		resp, err := c.client.Do(req)
		if err != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, "")); sleepErr == nil {
					continue
				}
			}
			return "", fmt.Errorf("GET %s: %w", path, err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
					continue
				}
			}
			return "", fmt.Errorf("read response body: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return strings.TrimSpace(string(body)), nil
		}

		if isRetryableWOCStatus(resp.StatusCode) && attempt < wocMaxRetries {
			if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
				continue
			}
		}
		return "", fmt.Errorf("WOC GET %s HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return "", fmt.Errorf("WOC GET %s failed after retries", path)
}

func (c *wocClient) postJSON(ctx context.Context, path string, payload interface{}) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode request body: %w", err)
	}
	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.applyAuthHeaders(req)

		resp, err := c.client.Do(req)
		if err != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, "")); sleepErr == nil {
					continue
				}
			}
			return "", fmt.Errorf("POST %s: %w", path, err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
					continue
				}
			}
			return "", fmt.Errorf("read response body: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return strings.TrimSpace(string(respBody)), nil
		}

		if isRetryableWOCStatus(resp.StatusCode) && attempt < wocMaxRetries {
			if sleepErr := sleepWithContext(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); sleepErr == nil {
				continue
			}
		}
		return "", fmt.Errorf("WOC POST %s HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return "", fmt.Errorf("WOC POST %s failed after retries", path)
}

func isRetryableWOCStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s > 0 {
			return time.Duration(s) * time.Second
		}
	}

	// Exponential backoff: 500ms, 1s, 2s, 4s, 8s, ...
	d := 500 * time.Millisecond * time.Duration(1<<attempt)
	if d > 15*time.Second {
		return 15 * time.Second
	}
	return d
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *wocClient) chainInfo(ctx context.Context) (*wocChainInfo, error) {
	var info wocChainInfo
	if err := c.getJSON(ctx, "/chain/info", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *wocClient) listUnspent(ctx context.Context, address string) ([]wocUnspent, error) {
	const pageLimit = 10000
	const maxPages = 20

	out := make([]wocUnspent, 0, 32)
	token := ""

	for page := 0; page < maxPages; page++ {
		path := fmt.Sprintf("/address/%s/unspent/all?limit=%d", address, pageLimit)
		if token != "" {
			path += "&token=" + url.QueryEscape(token)
		}

		var resp wocUnspentAllResponse
		if err := c.getJSON(ctx, path, &resp); err != nil {
			return nil, err
		}

		if len(resp.Result) > 0 {
			out = append(out, resp.Result...)
		}

		next := strings.TrimSpace(resp.NextPageToken)
		if next == "" {
			next = strings.TrimSpace(resp.NextPage)
		}
		if next == "" {
			return out, nil
		}
		token = next
	}

	return nil, fmt.Errorf("WOC list unspent(%s) exceeded max pages (%d) with token=%q", address, maxPages, token)
}

func (c *wocClient) txHex(ctx context.Context, txid string) (string, error) {
	path := "/tx/" + txid + "/hex"
	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		raw, err := c.getText(ctx, path)
		if err != nil {
			// tx/hash may surface confirmation slightly earlier than tx/hex indexing.
			// Treat 404 on tx hex as transient and retry.
			if strings.Contains(err.Error(), "HTTP 404") && attempt < wocMaxRetries {
				if sleepErr := sleepWithContext(ctx, retryDelay(attempt, "")); sleepErr == nil {
					continue
				}
			}
			return "", err
		}

		// Some endpoints may return quoted JSON strings.
		if strings.HasPrefix(raw, "\"") {
			var unquoted string
			if err := json.Unmarshal([]byte(raw), &unquoted); err == nil {
				raw = unquoted
			}
		}
		return strings.TrimSpace(raw), nil
	}
	return "", fmt.Errorf("WOC tx hex(%s) failed after retries", txid)
}

func (c *wocClient) txInfo(ctx context.Context, txid string) (*wocTxInfo, error) {
	var out wocTxInfo
	if err := c.getJSON(ctx, "/tx/hash/"+txid, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *wocClient) blockByHash(ctx context.Context, hash string) (*wocBlockInfo, error) {
	var out wocBlockInfo
	if err := c.getJSON(ctx, "/block/hash/"+hash, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *wocClient) blockByHeight(ctx context.Context, height int64) (*wocBlockInfo, error) {
	var out wocBlockInfo
	if err := c.getJSON(ctx, "/block/height/"+strconv.FormatInt(height, 10), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *wocClient) tscProof(ctx context.Context, txid string) (*wocProofTSC, error) {
	var out []wocProofTSC
	if err := c.getJSON(ctx, "/tx/"+txid+"/proof/tsc", &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty TSC proof response for %s", txid)
	}
	return &out[0], nil
}

func (c *wocClient) broadcastRawTx(ctx context.Context, rawTxHex string) (string, error) {
	resp, err := c.postJSON(ctx, "/tx/raw", map[string]string{"txhex": rawTxHex})
	if err != nil {
		return "", err
	}

	// Typical response is a JSON-quoted txid string.
	if strings.HasPrefix(resp, "\"") {
		var s string
		if err := json.Unmarshal([]byte(resp), &s); err == nil {
			resp = s
		}
	}

	resp = strings.TrimSpace(resp)
	if is64Hex(resp) {
		return resp, nil
	}

	// Fallback: try {"txid":"..."} shape.
	var obj struct {
		TxID string `json:"txid"`
	}
	if err := json.Unmarshal([]byte(resp), &obj); err == nil && is64Hex(obj.TxID) {
		return obj.TxID, nil
	}

	return "", fmt.Errorf("unexpected WOC broadcast response: %s", resp)
}

func is64Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
