//go:build e2e

package testutil

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// fallbackFeeRateSatPerKB is the hard fallback used when no override/policy is available.
	fallbackFeeRateSatPerKB = uint64(100)
	policyCacheTTL          = 5 * time.Minute
	policyRequestTimeout    = 3 * time.Second
	arcFeeTooLowStatusCode  = 465
)

const (
	feeSourceOverride = "override"
	feeSourcePolicy   = "arc_policy"
	feeSourceFallback = "fallback"
)

type policyCacheEntry struct {
	feeRateSatPerKB uint64
	policyTimestamp time.Time
}

type feeRateResolution struct {
	FeeRateSatPerKB uint64
	Source          string
	ARCEndpoint     string
	PolicyTimestamp time.Time
}

func (r feeRateResolution) withRetryFlag(retryOn465 bool) string {
	retry := 0
	if retryOn465 {
		retry = 1
	}
	policyTS := ""
	if !r.PolicyTimestamp.IsZero() {
		policyTS = r.PolicyTimestamp.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("fee_rate_sat_per_kb=%d fee_source=%s arc_endpoint=%s policy_timestamp=%s retry_on_465=%d",
		r.FeeRateSatPerKB, r.Source, r.ARCEndpoint, policyTS, retry)
}

func satPerKBFromMiningFee(satoshis, bytes uint64) (uint64, error) {
	if satoshis == 0 {
		return 0, fmt.Errorf("invalid policy miningFee: satoshis must be > 0")
	}
	if bytes == 0 {
		return 0, fmt.Errorf("invalid policy miningFee: bytes must be > 0")
	}

	// ceil(satoshis*1000/bytes) with overflow protection.
	if satoshis > math.MaxUint64/1000 {
		return 0, fmt.Errorf("invalid policy miningFee: satoshis overflow")
	}
	numerator := satoshis * 1000
	rate := (numerator + bytes - 1) / bytes
	if rate == 0 {
		return 1, nil
	}
	return rate, nil
}

func parsePolicyResponse(resp *arcPolicyResponse) (uint64, error) {
	if resp == nil || resp.Policy == nil || resp.Policy.MiningFee == nil {
		return 0, fmt.Errorf("invalid ARC policy response: missing policy.miningFee")
	}
	if resp.Policy.MiningFee.Satoshis == nil || resp.Policy.MiningFee.Bytes == nil {
		return 0, fmt.Errorf("invalid ARC policy response: missing policy.miningFee.satoshis/bytes")
	}
	return satPerKBFromMiningFee(*resp.Policy.MiningFee.Satoshis, *resp.Policy.MiningFee.Bytes)
}

func (c *arcClient) resolveFeeRate(ctx context.Context, cfg *Config, refresh bool) (feeRateResolution, error) {
	if cfg != nil && cfg.FeeRateSatPerKB > 0 {
		return feeRateResolution{
			FeeRateSatPerKB: cfg.FeeRateSatPerKB,
			Source:          feeSourceOverride,
			ARCEndpoint:     c.activeBaseURL(),
		}, nil
	}

	endpoint := c.activeBaseURL()
	if endpoint == "" {
		return feeRateResolution{
			FeeRateSatPerKB: fallbackFeeRateSatPerKB,
			Source:          feeSourceFallback,
			ARCEndpoint:     endpoint,
		}, fmt.Errorf("no ARC endpoint configured")
	}

	if !refresh {
		if entry, ok := c.getCachedPolicy(endpoint); ok {
			return feeRateResolution{
				FeeRateSatPerKB: entry.feeRateSatPerKB,
				Source:          feeSourcePolicy,
				ARCEndpoint:     endpoint,
				PolicyTimestamp: entry.policyTimestamp,
			}, nil
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, policyRequestTimeout)
	defer cancel()

	policyResp, err := c.policy(reqCtx)
	if err != nil {
		return feeRateResolution{
			FeeRateSatPerKB: fallbackFeeRateSatPerKB,
			Source:          feeSourceFallback,
			ARCEndpoint:     c.activeBaseURL(),
		}, err
	}

	feeRate, err := parsePolicyResponse(policyResp)
	if err != nil {
		return feeRateResolution{
			FeeRateSatPerKB: fallbackFeeRateSatPerKB,
			Source:          feeSourceFallback,
			ARCEndpoint:     c.activeBaseURL(),
		}, err
	}

	endpoint = c.activeBaseURL()
	now := c.nowFn()()
	c.putCachedPolicy(endpoint, policyCacheEntry{
		feeRateSatPerKB: feeRate,
		policyTimestamp: now,
	})

	return feeRateResolution{
		FeeRateSatPerKB: feeRate,
		Source:          feeSourcePolicy,
		ARCEndpoint:     endpoint,
		PolicyTimestamp: now,
	}, nil
}

func (c *arcClient) getCachedPolicy(endpoint string) (policyCacheEntry, bool) {
	cacheKey := c.policyCacheKey(endpoint)
	if cacheKey == "" {
		return policyCacheEntry{}, false
	}

	c.policyMu.RLock()
	entry, ok := c.policyCache[cacheKey]
	c.policyMu.RUnlock()
	if !ok {
		return policyCacheEntry{}, false
	}
	if c.nowFn()().Sub(entry.policyTimestamp) > policyCacheTTL {
		return policyCacheEntry{}, false
	}
	return entry, true
}

func (c *arcClient) putCachedPolicy(endpoint string, entry policyCacheEntry) {
	cacheKey := c.policyCacheKey(endpoint)
	if cacheKey == "" {
		return
	}
	c.policyMu.Lock()
	if c.policyCache == nil {
		c.policyCache = make(map[string]policyCacheEntry)
	}
	c.policyCache[cacheKey] = entry
	c.policyMu.Unlock()
}

func (c *arcClient) policyCacheKey(endpoint string) string {
	normalizedEndpoint := strings.TrimSpace(strings.TrimRight(endpoint, "/"))
	if normalizedEndpoint == "" {
		return ""
	}
	network := strings.TrimSpace(strings.ToLower(c.network))
	return network + "|" + strings.ToLower(normalizedEndpoint)
}

func (c *arcClient) nowFn() func() time.Time {
	if c == nil || c.now == nil {
		return time.Now
	}
	return c.now
}

func isARCFeeTooLow(err error) bool {
	var reqErr *arcRequestError
	if !errors.As(err, &reqErr) {
		return false
	}
	return reqErr.StatusCode == arcFeeTooLowStatusCode
}
