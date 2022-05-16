package hydra

import (
	"encoding/json"
	"math/big"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
)

// HydraRPC is an interface to JSON-RPC bitcoind service.
type HydraRPC struct {
	*btc.BitcoinRPC
	minFeeRate *big.Int // satoshi per kb
}

type resEstimateSmartFee struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		GasPrice  common.JSONNumber `json:"gasPrice"`
		BytePrice common.JSONNumber `json:"bytePrice"`
	} `json:"result"`
}
type cmdEstimateSmartFee struct {
	Method string `json:"method"`
}

// NewHydraRPC returns new HydraRPC instance.
func NewHydraRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &HydraRPC{
		b.(*btc.BitcoinRPC),
		big.NewInt(400000),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateSmartFee = true

	return s, nil
}

// Initialize initializes HydraRPC instance.
func (b *HydraRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewHydraParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// GetTransactionForMempool returns a transaction by the transaction ID
// It could be optimized for mempool, i.e. without block time and confirmations
func (b *HydraRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return b.GetTransaction(txid)
}

// EstimateSmartFee returns fee estimation
func (b *HydraRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {

	res := resEstimateSmartFee{}
	req := cmdEstimateSmartFee{Method: "getoracleinfo"}
	err := b.Call(&req, &res)

	var r big.Int
	if err != nil {
		return r, nil
	}
	if res.Error != nil {
		return r, res.Error
	}
	n, err := res.Result.BytePrice.Int64()
	if err != nil {
		return r, err
	}
	n *= 1000
	r.SetInt64(n)

	return r, nil
}
