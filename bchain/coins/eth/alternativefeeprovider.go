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
