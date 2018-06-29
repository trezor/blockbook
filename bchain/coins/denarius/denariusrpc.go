package denarius

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// DenariusRPC is an interface to JSON-RPC bitcoind service.
type DenariusRPC struct {
	*btc.BitcoinRPC
}

// NewDenariusRPC returns new DenariusRPC instance.
func NewDenariusRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &DenariusRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}

	return s, nil
}

type cmdGetInfo struct {
	Method string `json:"method"`
}

type resGetInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Chain         string `json:"chain"`
		Blocks        int    `json:"blocks"`
		Headers       int    `json:"headers"`
		Bestblockhash string `json:"bestblockhash"`
	} `json:"result"`
}

func (b *DenariusRPC) GetBlockChainInfo() (string, error) {
	glog.V(1).Info("rpc: getinfo")

	res := resGetInfo{}
	req := cmdGetInfo{Method: "getinfo"}
	err := b.Call(&req, &res)
	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result.Chain, nil
}

// Initialize initializes DenariusRPC instance.
func (b *DenariusRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewDenariusParser(params, b.ChainConfig)

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

// EstimateFee returns fee estimation.
func (b *DenariusRPC) EstimateFee(blocks int) (float64, error) {
	return b.EstimateSmartFee(blocks, true)
}
