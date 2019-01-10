package xzc

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"
)

type ZcoinRPC struct {
	*btc.BitcoinRPC
}

func NewZcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	base, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	zcoin := &ZcoinRPC{
		BitcoinRPC: base.(*btc.BitcoinRPC),
	}

	return zcoin, nil
}
