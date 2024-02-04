package btc

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

type alternativeFeeProviderFee struct {
	blocks   int
	feePerKB int
}

type alternativeFeeProvider struct {
	fees     []alternativeFeeProviderFee
	lastSync time.Time
	chain    bchain.BlockChain
	mux      sync.Mutex
}

type alternativeFeeProviderInterface interface {
	compareToDefault()
	estimateFee(blocks int) (big.Int, error)
}

func (p *alternativeFeeProvider) compareToDefault() {
	output := ""
	for _, fee := range p.fees {
		conservative, err := p.chain.(*BitcoinRPC).blockchainEstimateSmartFee(fee.blocks, true)
		if err != nil {
			glog.Error(err)
			return
		}
		economical, err := p.chain.(*BitcoinRPC).blockchainEstimateSmartFee(fee.blocks, false)
		if err != nil {
			glog.Error(err)
			return
		}
		output += fmt.Sprintf("Blocks %d: alternative %d, conservative %s, economical %s\n", fee.blocks, fee.feePerKB, conservative.String(), economical.String())
	}
	glog.Info("alternativeFeeProviderCompareToDefault\n", output)
}

func (p *alternativeFeeProvider) estimateFee(blocks int) (big.Int, error) {
	var r big.Int
	p.mux.Lock()
	defer p.mux.Unlock()
	if len(p.fees) == 0 {
		return r, errors.New("alternativeFeeProvider: no fees")
	}
	if p.lastSync.Before(time.Now().Add(time.Duration(-10) * time.Minute)) {
		return r, errors.Errorf("alternativeFeeProvider: Missing recent value, last sync at %v", p.lastSync)
	}
	for i := range p.fees {
		if p.fees[i].blocks >= blocks {
			r = *big.NewInt(int64(p.fees[i].feePerKB))
			return r, nil
		}
	}
	// use the last value as fallback
	r = *big.NewInt(int64(p.fees[len(p.fees)-1].feePerKB))
	return r, nil
}
