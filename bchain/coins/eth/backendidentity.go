package eth

import (
	"context"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

const (
	// net_version and web3_clientVersion answer with values that change only when the
	// backend is restarted onto a new build, so they are refreshed on this period instead
	// of on every GetChainInfo - which serves /, /api/ and /api/v2/ on every request.
	backendIdentityTTL = 60 * time.Second
	// Past this age the cached identity stops standing in for a live backend: callers turn
	// a GetChainInfo error into backendError and inSync=false, and on EVM chains these two
	// calls are the only part of GetChainInfo that touches the backend at all.
	backendIdentityMaxAge = 5 * time.Minute
)

// backendIdentity holds the near-immutable backend facts GetChainInfo reports.
type backendIdentity struct {
	chainID       uint64
	clientVersion string
	fetchedAt     time.Time
}

// backendIdentityCache serves backendIdentity from a snapshot refreshed at most once per
// backendIdentityTTL, so a burst of API requests costs no backend round trips.
type backendIdentityCache struct {
	mu          sync.Mutex
	snapshot    *backendIdentity
	fetching    bool
	lastAttempt time.Time
	lastErr     error
}

// get returns the cached identity, refreshing it through fetch when stale. now is passed in
// so the staleness rules can be tested without sleeping.
func (c *backendIdentityCache) get(now time.Time, fetch func() (*backendIdentity, error)) (*backendIdentity, error) {
	c.mu.Lock()
	snap := c.snapshot
	warm := snap != nil
	if warm && now.Sub(snap.fetchedAt) < backendIdentityTTL {
		c.mu.Unlock()
		return snap, nil
	}
	// One caller at a time talks to the backend and the rest keep serving the previous
	// snapshot, so no request ever queues behind an RPC that may sit until the timeout.
	// Suppressing retries for a whole TTL caps an unreachable backend at one call a minute.
	if warm && (c.fetching || now.Sub(c.lastAttempt) < backendIdentityTTL) {
		lastErr := c.lastErr
		c.mu.Unlock()
		return serveStaleIdentity(snap, now, lastErr)
	}
	c.fetching = true
	c.lastAttempt = now
	c.mu.Unlock()

	fresh, err := fetch()

	c.mu.Lock()
	c.fetching = false
	c.lastErr = err
	if err != nil {
		c.mu.Unlock()
		// A cold cache has nothing to fall back on, so the failure propagates as before.
		if !warm {
			return nil, err
		}
		return serveStaleIdentity(snap, now, err)
	}
	fresh.fetchedAt = now
	// A changed chain id means the endpoint is answering for a different chain than the one
	// Initialize validated - the whole index is then being built against the wrong backend.
	if warm && snap.chainID != fresh.chainID {
		glog.Errorf("backend chain id changed from %d to %d, the RPC endpoint answers for a different chain", snap.chainID, fresh.chainID)
	}
	c.snapshot = fresh
	c.mu.Unlock()
	return fresh, nil
}

// serveStaleIdentity keeps a cached identity usable through a short backend outage but
// reports the error once the snapshot is too old to be evidence the node is reachable.
func serveStaleIdentity(snap *backendIdentity, now time.Time, lastErr error) (*backendIdentity, error) {
	age := now.Sub(snap.fetchedAt)
	if age <= backendIdentityMaxAge {
		return snap, nil
	}
	if lastErr == nil {
		return nil, errors.Errorf("backend identity not refreshed for %v", age.Truncate(time.Second))
	}
	return nil, errors.Annotatef(lastErr, "backend identity not refreshed for %v", age.Truncate(time.Second))
}

// getBackendIdentity returns the backend chain id and client version, from cache when fresh.
func (b *EthereumRPC) getBackendIdentity() (*backendIdentity, error) {
	return b.identity.get(time.Now(), b.fetchBackendIdentity)
}

func (b *EthereumRPC) fetchBackendIdentity() (*backendIdentity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	netStart := time.Now()
	id, err := b.Client.NetworkID(ctx)
	b.observeSyncRPCLatency("net_version", netStart, err)
	if err != nil {
		return nil, err
	}
	var ver string
	web3Start := time.Now()
	err = b.RPC.CallContext(ctx, &ver, "web3_clientVersion")
	b.observeSyncRPCLatency("web3_clientVersion", web3Start, err)
	if err != nil {
		return nil, err
	}
	return &backendIdentity{chainID: id.Uint64(), clientVersion: ver}, nil
}
