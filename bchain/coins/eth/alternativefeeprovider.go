package eth

import (
	"net/http"
	"sync"
	"time"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// feeProviderParams is the shared config for the EVM alternative fee providers
// (Infura and 1inch). The two were field-for-field identical and had already
// drifted once, so they share one struct to keep parsing and validation in sync.
type feeProviderParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
	// StaleSeconds is how long a cached estimate stays usable after the last
	// successful refresh before it is considered stale (falls back to node
	// estimation). It is independent of the poll cadence. Optional; defaults to
	// defaultFeeStaleSeconds when unset.
	StaleSeconds int `json:"staleSeconds"`
}

// feeHTTPClient is shared by the fee pollers. The timeout bounds each poll well
// under the poll cadence so a blackholed connection (accepted, headers never
// sent) can't block the single poller goroutine forever and silently freeze
// fee refreshes; http.DefaultClient has no timeout.
var feeHTTPClient = &http.Client{Timeout: 5 * time.Second}

type alternativeFeeProvider struct {
	eip1559Fees       *bchain.Eip1559Fees
	lastSync          time.Time
	staleSyncDuration time.Duration
	chain             bchain.BlockChain
	mux               sync.Mutex
	metrics           *common.Metrics
	name              string
}

func (p *alternativeFeeProvider) observeRequest(status string) {
	if p.metrics == nil || p.metrics.AlternativeFeeProviderRequests == nil {
		return
	}
	p.metrics.AlternativeFeeProviderRequests.With(common.Labels{"provider": p.name, "status": status}).Inc()
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
// shorter than the poll cadence — otherwise a config like {periodSeconds:60,
// staleSeconds:30} would flap provider→node once per poll on a healthy provider,
// a trap the old periodSeconds*30 formula made structurally impossible.
func feeStaleDuration(periodSeconds, staleSeconds int) time.Duration {
	if staleSeconds <= 0 {
		staleSeconds = defaultFeeStaleSeconds
	}
	if staleSeconds < periodSeconds {
		staleSeconds = periodSeconds
	}
	return time.Duration(staleSeconds) * time.Second
}

func (p *alternativeFeeProvider) GetEip1559Fees() (*bchain.Eip1559Fees, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	// Treat an unset staleSyncDuration as the default rather than 0, so a future
	// provider that forgets to initialize it degrades to the 10-minute window
	// instead of a silent no-op (lastSync.Add(0) is always in the past).
	stale := p.staleSyncDuration
	if stale <= 0 {
		stale = defaultFeeStaleSeconds * time.Second
	}
	if p.lastSync.Add(stale).Before(time.Now()) {
		return nil, nil
	}
	return p.eip1559Fees, nil
}
