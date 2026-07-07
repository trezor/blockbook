package eth

import (
	"sync"
	"time"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

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

func (p *alternativeFeeProvider) GetEip1559Fees() (*bchain.Eip1559Fees, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	if p.lastSync.Add(p.staleSyncDuration).Before(time.Now()) {
		return nil, nil
	}
	return p.eip1559Fees, nil
}
