//go:build e2e

package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestARCClientFailoverToSecondary(t *testing.T) {
	var primaryHits atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":"primary unavailable"}`))
	}))
	defer primary.Close()

	var secondaryHits atomic.Int32
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryHits.Add(1)
		require.Equal(t, "/health", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"healthy":true}`))
	}))
	defer secondary.Close()

	c := newARCClient([]string{primary.URL, secondary.URL}, "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	health, err := c.health(ctx)
	require.NoError(t, err)
	require.NotNil(t, health)
	require.True(t, health.Healthy)
	require.Equal(t, int32(1), primaryHits.Load())
	require.Equal(t, int32(1), secondaryHits.Load())

	// Active endpoint should switch to secondary after successful failover.
	health, err = c.health(ctx)
	require.NoError(t, err)
	require.NotNil(t, health)
	require.Equal(t, int32(1), primaryHits.Load())
	require.Equal(t, int32(2), secondaryHits.Load())
}

func TestARCClientNoFailoverOnRequestError(t *testing.T) {
	var primaryHits atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"bad request"}`))
	}))
	defer primary.Close()

	var secondaryHits atomic.Int32
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryHits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200,"txid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer secondary.Close()

	c := newARCClient([]string{primary.URL, secondary.URL}, "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := c.txStatus(ctx, "deadbeef")
	require.Error(t, err)
	require.Equal(t, int32(1), primaryHits.Load())
	require.Equal(t, int32(0), secondaryHits.Load())
}
