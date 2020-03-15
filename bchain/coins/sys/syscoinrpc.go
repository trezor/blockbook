package syscoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"
	"github.com/juju/errors"
	"github.com/golang/glog"
)

// SyscoinRPC is an interface to JSON-RPC bitcoind service
type SyscoinRPC struct {
	*btc.BitcoinRPC
}

// NewSyscoinRPC returns new SyscoinRPC instance
func NewSyscoinRPC(config json.RawMessage, pushHandler func(notificationType bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &SyscoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes SyscoinRPC instance.
func (b *SyscoinRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewSyscoinParser(params, b.ChainConfig)

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

// GetBlock returns block with given hash
func (b *SyscoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	if !b.ParseBlocks {
		return b.GetBlockFull(hash)
	}
	return b.GetBlockWithoutHeader(hash, height)
}

type CmdAssetAllocationSend struct {
	Method string `json:"method"`
	Params struct {
		Asset    int    `json:"asset_guid"`
		Sender   string `json:"address_sender"`
		Receiver string `json:"address_receiver"`
		Amount   string `json:"amount"`
	} `json:"params"`
}
type ResAssetAllocationSend struct {
	Error  *bchain.RPCError `json:"error"`
	Result json.RawMessage      `json:"result"`
}

func (b *SyscoinRPC) AssetAllocationSend(asset int, sender string, receiver string, amount string) (json.RawMessage, error) {
	glog.V(1).Info("rpc: assetallocationsend ", asset)

	res := ResAssetAllocationSend{}
	req := CmdAssetAllocationSend{Method: "assetallocationsend"}
	req.Params.Asset = asset
	req.Params.Sender = sender
	req.Params.Receiver = receiver
	req.Params.Amount = amount
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "asset %v", asset)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "asset %v", asset)
	}
	return res.Result, nil
}