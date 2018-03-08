package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"time"
)

type ZCashRPC struct {
	*btc.BitcoinRPC
}

func NewZCashRPC(url string, user string, password string, timeout time.Duration, parse bool) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(url, user, password, timeout, parse)
	if err != nil {
		return nil, err
	}
	z := &ZCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	return z, nil
}
