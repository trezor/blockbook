package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"time"
)

type ZCashRPC struct {
	*btc.BitcoinRPC
}

func NewZCashRPC(url string, user string, password string, timeout time.Duration, parse bool, metrics *common.Metrics) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(url, user, password, timeout, parse, metrics)
	if err != nil {
		return nil, err
	}
	z := &ZCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	return z, nil
}
