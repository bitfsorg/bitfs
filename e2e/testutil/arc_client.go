//go:build e2e

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type arcClient struct {
	baseURLs []string
	apiKey   string
	client   *http.Client

	activeMu  sync.RWMutex
	activeIdx int
}

type arcHealth struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason"`
}

type arcTxStatus struct {
	Status      int     `json:"status"`
	Title       string  `json:"title"`
	TxID        *string `json:"txid"`
	TxStatus    *string `json:"txStatus"`
	MerklePath  *string `json:"merklePath"`
	BlockHash   *string `json:"blockHash"`
	BlockHeight *uint64 `json:"blockHeight"`
	ExtraInfo   *string `json:"extraInfo"`
	Detail      *string `json:"detail"`
}

type arcRequestError struct {
	Method     string
	Path       string
	BaseURL    string
	StatusCode int
	Body       string
	RetryAfter string
	Cause      error
}

func (e *arcRequestError) Error() string {
	if e == nil {
		return "unknown ARC error"
	}
	if e.Cause != nil {
		return fmt.Sprintf("ARC %s %s via %s: %v", e.Method, e.Path, e.BaseURL, e.Cause)
	}
	return fmt.Sprintf("ARC %s %s via %s HTTP %d: %s", e.Method, e.Path, e.BaseURL, e.StatusCode, e.Body)
}

func (e *arcRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newARCClient(baseURLs []string, apiKey string) *arcClient {
	normalized := make([]string, 0, len(baseURLs))
	for _, baseURL := range baseURLs {
		baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
		if baseURL == "" {
			continue
		}
		already := false
		for _, existing := range normalized {
			if strings.EqualFold(existing, baseURL) {
				already = true
				break
			}
		}
		if !already {
			normalized = append(normalized, baseURL)
		}
	}

	return &arcClient{
		baseURLs: normalized,
		apiKey:   strings.TrimSpace(apiKey),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *arcClient) applyAuthHeaders(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	token := c.apiKey
	req.Header.Set("Api-Key", token)
	req.Header.Set("X-Api-Key", token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		req.Header.Set("Authorization", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c *arcClient) endpointOrder() []string {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()

	if len(c.baseURLs) == 0 {
		return nil
	}
	if c.activeIdx < 0 || c.activeIdx >= len(c.baseURLs) {
		out := make([]string, len(c.baseURLs))
		copy(out, c.baseURLs)
		return out
	}

	out := make([]string, 0, len(c.baseURLs))
	out = append(out, c.baseURLs[c.activeIdx])
	for i, u := range c.baseURLs {
		if i == c.activeIdx {
			continue
		}
		out = append(out, u)
	}
	return out
}

func (c *arcClient) markActive(baseURL string) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	for i, u := range c.baseURLs {
		if strings.EqualFold(u, baseURL) {
			c.activeIdx = i
			return
		}
	}
}

func isRetryableARCStatus(code int) bool {
	return code == http.StatusRequestTimeout || isRetryableWOCStatus(code)
}

func (c *arcClient) doJSON(ctx context.Context, method, path string, body []byte, contentType string, headers map[string]string, out interface{}) error {
	if len(c.baseURLs) == 0 {
		return fmt.Errorf("ARC client has no base URLs configured")
	}

	for attempt := 0; attempt <= wocMaxRetries; attempt++ {
		endpoints := c.endpointOrder()
		var lastErr error
		retryAfter := ""

		for _, baseURL := range endpoints {
			err := c.doJSONOnce(ctx, baseURL, method, path, body, contentType, headers, out)
			if err == nil {
				c.markActive(baseURL)
				return nil
			}

			lastErr = err
			arcErr, ok := err.(*arcRequestError)
			if !ok {
				// Unknown local error; try next endpoint.
				continue
			}
			if arcErr.RetryAfter != "" && retryAfter == "" {
				retryAfter = arcErr.RetryAfter
			}
			// 4xx (except 408/429) are request errors and should fail fast.
			if arcErr.StatusCode > 0 && !isRetryableARCStatus(arcErr.StatusCode) {
				return err
			}
		}

		if attempt < wocMaxRetries {
			if sleepErr := sleepWithContext(ctx, retryDelay(attempt, retryAfter)); sleepErr == nil {
				continue
			}
		}

		if lastErr != nil {
			return lastErr
		}
	}
	return fmt.Errorf("ARC %s %s failed after retries", method, path)
}

func (c *arcClient) doJSONOnce(ctx context.Context, baseURL, method, path string, body []byte, contentType string, headers map[string]string, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return &arcRequestError{
			Method:  method,
			Path:    path,
			BaseURL: baseURL,
			Cause:   fmt.Errorf("create request: %w", err),
		}
	}
	c.applyAuthHeaders(req)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &arcRequestError{
			Method:  method,
			Path:    path,
			BaseURL: baseURL,
			Cause:   err,
		}
	}

	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return &arcRequestError{
			Method:  method,
			Path:    path,
			BaseURL: baseURL,
			Cause:   fmt.Errorf("read response body: %w", readErr),
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return &arcRequestError{
				Method:  method,
				Path:    path,
				BaseURL: baseURL,
				Cause:   fmt.Errorf("decode ARC response: %w", err),
			}
		}
		return nil
	}

	return &arcRequestError{
		Method:     method,
		Path:       path,
		BaseURL:    baseURL,
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(respBody)),
		RetryAfter: strings.TrimSpace(resp.Header.Get("Retry-After")),
	}
}

func (c *arcClient) health(ctx context.Context) (*arcHealth, error) {
	var out arcHealth
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *arcClient) txStatus(ctx context.Context, txid string) (*arcTxStatus, error) {
	var out arcTxStatus
	if err := c.doJSON(ctx, http.MethodGet, "/tx/"+txid, nil, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *arcClient) broadcastRawTx(ctx context.Context, rawTxHex string, cfg *Config) (*arcTxStatus, error) {
	headers := map[string]string{
		"X-CallbackUrl":   cfg.ARCCallbackURL,
		"X-CallbackToken": cfg.ARCCallbackToken,
	}
	waitFor := strings.TrimSpace(cfg.ARCWaitFor)
	if waitFor == "" {
		waitFor = "SEEN_ON_NETWORK"
	}
	headers["X-WaitFor"] = waitFor

	var out arcTxStatus
	if err := c.doJSON(ctx, http.MethodPost, "/tx", []byte(rawTxHex), "text/plain", headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *arcTxStatus) txStatusValue() string {
	if s == nil || s.TxStatus == nil {
		return ""
	}
	return strings.TrimSpace(*s.TxStatus)
}

func (s *arcTxStatus) txIDValue() string {
	if s == nil || s.TxID == nil {
		return ""
	}
	return strings.TrimSpace(*s.TxID)
}

func (s *arcTxStatus) blockHashValue() string {
	if s == nil || s.BlockHash == nil {
		return ""
	}
	return strings.TrimSpace(*s.BlockHash)
}

func (s *arcTxStatus) blockHeightValue() uint64 {
	if s == nil || s.BlockHeight == nil {
		return 0
	}
	return *s.BlockHeight
}

func (s *arcTxStatus) merklePathValue() string {
	if s == nil || s.MerklePath == nil {
		return ""
	}
	return strings.TrimSpace(*s.MerklePath)
}
