package eth

import (
	"sync"
	"time"

	"github.com/trezor/blockbook/bchain"
)

type alternativeFeeProvider struct {
	eip1559Fees *bchain.Eip1559Fees
	lastSync    time.Time
	chain       bchain.BlockChain
	mux         sync.Mutex
}

type alternativeFeeProviderInterface interface {
	GetEip1559Fees() (*bchain.Eip1559Fees, error)
}

func (p *alternativeFeeProvider) GetEip1559Fees() (*bchain.Eip1559Fees, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.eip1559Fees, nil
}
