package eth

import (
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

const (
	// net_version and web3_clientVersion answer with values that change only when the
	// backend is restarted onto a new build, so they are refreshed on this period instead
	// of on every GetChainInfo - which serves /, /api/ and /api/v2/ on every request.
	backendIdentityTTL = 60 * time.Second
	// Once refreshes have been failing for this long the cached identity stops standing in
	// for a live backend: callers turn a GetChainInfo error into backendError and
	// inSync=false, and on EVM chains these two calls are the only part of GetChainInfo
	// that touches the backend at all.
	backendIdentityMaxFailing = 5 * time.Minute
)

// backendIdentity holds the near-immutable backend facts GetChainInfo reports.
type backendIdentity struct {
	chainID       uint64
	clientVersion string
}

// getBackendIdentity returns the backend chain id and client version, from cache when fresh.
func (b *EthereumRPC) getBackendIdentity() (*backendIdentity, error) {
	return identityFromCache(b.identity.Get(time.Now()))
}

// identityFromCache maps the cache state onto GetChainInfo's contract: no identity at all, or
// refreshes failing for longer than backendIdentityMaxFailing, report the backend as broken;
// a shorter blip keeps serving the cached identity so a transient refresh error does not flip
// the whole instance out of sync over a cosmetic version string.
func identityFromCache(id *backendIdentity, failingFor time.Duration, lastErr error) (*backendIdentity, error) {
	if id == nil {
		if lastErr == nil {
			// only possible while the very first fetch is still in flight
			return nil, errors.New("backend identity not fetched yet")
		}
		return nil, lastErr
	}
	if lastErr != nil && failingFor > backendIdentityMaxFailing {
		return nil, errors.Annotatef(lastErr, "backend identity refresh failing for %v", failingFor.Truncate(time.Second))
	}
	return id, nil
}

func (b *EthereumRPC) fetchBackendIdentity() (backendIdentity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	netStart := time.Now()
	id, err := b.Client.NetworkID(ctx)
	b.observeSyncRPCLatency("net_version", netStart, err)
	if err != nil {
		return backendIdentity{}, err
	}
	if err := b.validateBackendChainID(id.Uint64()); err != nil {
		return backendIdentity{}, err
	}
	var ver string
	web3Start := time.Now()
	err = b.RPC.CallContext(ctx, &ver, "web3_clientVersion")
	b.observeSyncRPCLatency("web3_clientVersion", web3Start, err)
	if err != nil {
		return backendIdentity{}, err
	}
	return backendIdentity{chainID: id.Uint64(), clientVersion: ver}, nil
}

// validateBackendChainID latches the first chain id the backend reports and refuses any later
// flip: an endpoint answering for a different chain would have the whole index built against
// the wrong backend, so the refresh keeps failing instead of adopting the new id as healthy.
func (b *EthereumRPC) validateBackendChainID(id uint64) error {
	if b.validatedChainID.CompareAndSwap(0, id) {
		return nil
	}
	if latched := b.validatedChainID.Load(); latched != id {
		err := errors.Errorf("backend chain id changed from %d to %d, the RPC endpoint answers for a different chain", latched, id)
		glog.Error(err)
		return err
	}
	return nil
}
