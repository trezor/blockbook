//go:build unittest

package eth

import (
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
)

func TestIdentityFromCache(t *testing.T) {
	id := &backendIdentity{chainID: 1, clientVersion: "Geth/v1.16"}

	if got, err := identityFromCache(id, 0, nil); err != nil || got != id {
		t.Fatalf("healthy cache = %v, %v", got, err)
	}
	// a refresh blip must not flip the instance out of sync over a cosmetic version string
	if got, err := identityFromCache(id, backendIdentityMaxFailing, errors.New("timeout")); err != nil || got != id {
		t.Fatalf("short outage = %v, %v, want the cached identity", got, err)
	}
	_, err := identityFromCache(id, backendIdentityMaxFailing+time.Second, errors.New("dial tcp: refused"))
	if err == nil || !strings.Contains(err.Error(), "failing for") || !strings.Contains(err.Error(), "dial tcp: refused") {
		t.Fatalf("long outage error %v should carry the duration and the backend error", err)
	}
	// a cold cache has nothing to fall back on, so the fetch error propagates as before
	if _, err := identityFromCache(nil, 0, errors.New("dial tcp: refused")); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("cold failure = %v", err)
	}
	if _, err := identityFromCache(nil, 0, nil); err == nil {
		t.Fatal("cold cache with the first fetch still in flight must report an error, not nil identity")
	}
}

func TestValidateBackendChainID(t *testing.T) {
	b := &EthereumRPC{}
	if err := b.validateBackendChainID(56); err != nil {
		t.Fatal(err)
	}
	if err := b.validateBackendChainID(56); err != nil {
		t.Fatal(err)
	}
	err := b.validateBackendChainID(1)
	if err == nil || !strings.Contains(err.Error(), "56") || !strings.Contains(err.Error(), "1") {
		t.Fatalf("chain id flip = %v, want an error naming both ids", err)
	}
	// the latch keeps the validated id: the flip is refused, not adopted
	if err := b.validateBackendChainID(56); err != nil {
		t.Fatalf("original id refused after a flip: %v", err)
	}
}
