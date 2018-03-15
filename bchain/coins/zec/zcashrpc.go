package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"encoding/json"
)

type ZCashRPC struct {
	*btc.BitcoinRPC
}

func NewZCashRPC(config json.RawMessage, pushHandler func(*bchain.MQMessage), metrics *common.Metrics) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler, metrics)
	if err != nil {
		return nil, err
	}
	z := &ZCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	return z, nil
}
