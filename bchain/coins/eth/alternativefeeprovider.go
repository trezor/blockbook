package eth

import (
	"net/http"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"golang.org/x/sync/singleflight"
)

// feeProviderParams is the shared config for the EVM alternative fee providers
// (Infura and 1inch). The two were field-for-field identical and had already
// drifted once, so they share one struct to keep parsing and validation in sync.
type feeProviderParams struct {
	URL string `json:"url"`
	// PeriodSeconds is the cache TTL: the provider is contacted at most once per
	// period, and only when a client requests an estimate (issue #1746). The name
	// predates on-demand fetching, when it was the poll cadence.
	PeriodSeconds int `json:"periodSeconds"`
	// StaleSeconds is how long a cached estimate stays usable after the last
	// successful refresh before it is considered stale (falls back to node
	// estimation). It is independent of the cache TTL. Optional; defaults to
	// defaultFeeStaleSeconds when unset.
	StaleSeconds int `json:"staleSeconds"`
}

// feeHTTPClient bounds each on-demand fetch so a blackholed connection (accepted,
// headers never sent) can't hold the coalesced estimateFee callers for more than
// a few seconds; http.DefaultClient has no timeout.
var feeHTTPClient = &http.Client{Timeout: 5 * time.Second}

// feeFetchRetryDelay is the floor for pacing upstream retries after a failed
// fetch (the effective delay is max of this and the ttl, so the documented
// one-request-per-periodSeconds cap holds during outages too). Without it an
// outage would be retried at client-request rate — hammering a possibly throttled
// provider and adding fetch latency to every request instead of one per delay.
const feeFetchRetryDelay = 5 * time.Second

type alternativeFeeProvider struct {
	eip1559Fees       *bchain.Eip1559Fees
	lastSync          time.Time
	lastFailure       time.Time
	ttl               time.Duration
	staleSyncDuration time.Duration
	fetch             func() (*bchain.Eip1559Fees, error)
	chain             bchain.BlockChain
	mux               sync.Mutex
	sf                singleflight.Group
	metrics           *common.Metrics
	name              string
}

func (p *alternativeFeeProvider) observeRequest(status string) {
	if p.metrics == nil || p.metrics.AlternativeFeeProviderRequests == nil {
		return
	}
	p.metrics.AlternativeFeeProviderRequests.With(common.Labels{"provider": p.name, "status": status}).Inc()
}

func (p *alternativeFeeProvider) observeCache(result string) {
	if p.metrics == nil || p.metrics.AlternativeFeeProviderCache == nil {
		return
	}
	p.metrics.AlternativeFeeProviderCache.With(common.Labels{"provider": p.name, "result": result}).Inc()
}

// observeSync records a successful refresh of the cached fees: it advances lastSync (which the read
// path uses for its staleness check) and exports the timestamp so cache age can be plotted as
// time() - metric. Both must use the same instant, so the caller passes it in. Exporting a timestamp
// rather than an age keeps the plotted age rising when a provider wedges, since it is only written
// here on success. Callers hold p.mux.
func (p *alternativeFeeProvider) observeSync(t time.Time) {
	p.lastSync = t
	if p.metrics == nil || p.metrics.AlternativeFeeProviderLastSync == nil {
		return
	}
	p.metrics.AlternativeFeeProviderLastSync.With(common.Labels{"provider": p.name}).Set(float64(t.Unix()))
}

type alternativeFeeProviderInterface interface {
	GetEip1559Fees() (*bchain.Eip1559Fees, error)
}

// defaultFeeStaleSeconds is the cached-estimate stale window (10 minutes) used
// by the EVM alternative fee providers when the config omits "staleSeconds".
const defaultFeeStaleSeconds = 600

// feeStaleDuration returns the stale window for cached estimates: the configured
// staleSeconds (falling back to the 10-minute default when unset or non-positive),
// clamped up to periodSeconds. The clamp guarantees the window can never be
// shorter than the cache TTL — otherwise a config like {periodSeconds:60,
// staleSeconds:30} would flap provider→node on a healthy provider, a trap the
// old periodSeconds*30 formula made structurally impossible.
func feeStaleDuration(periodSeconds, staleSeconds int) time.Duration {
	if staleSeconds <= 0 {
		staleSeconds = defaultFeeStaleSeconds
	}
	if staleSeconds < periodSeconds {
		staleSeconds = periodSeconds
	}
	return time.Duration(staleSeconds) * time.Second
}

// freshDuration treats an unset ttl as one second rather than 0, so a provider
// that forgets to initialize it still caches a just-fetched value long enough
// to serve it instead of refetching on every request.
func (p *alternativeFeeProvider) freshDuration() time.Duration {
	if p.ttl <= 0 {
		return time.Second
	}
	return p.ttl
}

// staleDuration treats an unset staleSyncDuration as the default rather than 0,
// so a future provider that forgets to initialize it degrades to the 10-minute
// window instead of a silent no-op (lastSync.Add(0) is always in the past).
func (p *alternativeFeeProvider) staleDuration() time.Duration {
	if p.staleSyncDuration <= 0 {
		return defaultFeeStaleSeconds * time.Second
	}
	return p.staleSyncDuration
}

// cachedWithin returns a copy of the cached fees when the last successful fetch
// is within d, else nil. The copy keeps the cache safe from per-request mutation
// by callers (e.g. fee jitter).
func (p *alternativeFeeProvider) cachedWithin(d time.Duration) *bchain.Eip1559Fees {
	p.mux.Lock()
	defer p.mux.Unlock()
	if p.lastSync.Add(d).Before(time.Now()) {
		return nil
	}
	return p.eip1559Fees.Copy()
}

// refresh runs one upstream fetch and stores the result; called only inside the
// singleflight group so at most one fetch is in flight. Errors are logged, not
// returned — the read path falls back to stale/on-chain data.
func (p *alternativeFeeProvider) refresh() {
	retryDelay := p.freshDuration()
	if retryDelay < feeFetchRetryDelay {
		retryDelay = feeFetchRetryDelay
	}
	p.mux.Lock()
	// a caller queued behind a just-completed flight must not refetch, and a
	// failed fetch is retried at most once per ttl (with a feeFetchRetryDelay floor)
	if p.fetch == nil || time.Since(p.lastSync) < p.freshDuration() || time.Since(p.lastFailure) < retryDelay {
		p.mux.Unlock()
		return
	}
	p.mux.Unlock()
	// fetch outside the lock; bounded by feeHTTPClient's timeout
	fees, err := p.fetch()
	p.mux.Lock()
	defer p.mux.Unlock()
	if err != nil {
		glog.Errorf("%s alternative fee provider fetch: %v", p.name, err)
		p.lastFailure = time.Now()
		return
	}
	p.lastFailure = time.Time{}
	p.observeSync(time.Now())
	p.eip1559Fees = fees
}

// GetEip1559Fees serves fees from the cache when fresh and otherwise fetches from
// the provider on demand, coalescing concurrent callers into a single upstream
// request. It never returns an error: on a failed fetch it serves the last value
// within the stale window, and past that returns nil so the caller falls through
// to on-chain estimation.
func (p *alternativeFeeProvider) GetEip1559Fees() (*bchain.Eip1559Fees, error) {
	if fees := p.cachedWithin(p.freshDuration()); fees != nil {
		p.observeCache("fresh")
		return fees, nil
	}
	// singleflight's shared flag is true for the leader too whenever anyone
	// waited on it, so mark the leader from inside the closure instead
	fetched := false
	p.sf.Do("fees", func() (interface{}, error) {
		fetched = true
		p.refresh()
		return nil, nil
	})
	if fees := p.cachedWithin(p.freshDuration()); fees != nil {
		if fetched {
			p.observeCache("fetched")
		} else {
			p.observeCache("coalesced")
		}
		return fees, nil
	}
	if fees := p.cachedWithin(p.staleDuration()); fees != nil {
		p.observeCache("stale")
		return fees, nil
	}
	p.observeCache("miss")
	return nil, nil
}

// warmUp primes the cache once at startup so the first client does not pay the
// fetch latency and a broken URL/key surfaces in the log right away.
func (p *alternativeFeeProvider) warmUp() {
	p.sf.Do("fees", func() (interface{}, error) {
		p.refresh()
		return nil, nil
	})
}
