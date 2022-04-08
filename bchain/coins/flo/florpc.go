package flo

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// FloRPC is an interface to JSON-RPC bitcoind service.
type FloRPC struct {
	*btc.BitcoinRPC
}

// NewFloRPC returns new FloRPC instance.
func NewFloRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &FloRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes FloRPC instance.
func (f *FloRPC) Initialize() error {
	ci, err := f.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	f.Parser = NewFloParser(params, f.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		f.Testnet = false
		f.Network = "livenet"
	} else {
		f.Testnet = true
		f.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// GetBlock returns block with given hash.
func (f *FloRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" {
		hash, err = f.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	if !f.ParseBlocks {
		return f.GetBlockFull(hash)
	}
	// optimization
	if height > 0 {
		return f.GetBlockWithoutHeader(hash, height)
	}
	header, err := f.GetBlockHeader(hash)
	if err != nil {
		return nil, err
	}
	data, err := f.GetBlockBytes(hash)
	if err != nil {
		return nil, err
	}
	block, err := f.Parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	block.BlockHeader = *header
	return block, nil
}

// GetBlockFull returns block with given hash
func (f *FloRPC) GetBlockFull(hash string) (*bchain.Block, error) {
	glog.V(1).Info("rpc: getblock (verbosity=2) ", hash)

	res := btc.ResGetBlockFull{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 2
	err := f.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if btc.IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	for i := range res.Result.Txs {
		tx := &res.Result.Txs[i]
		for j := range tx.Vout {
			vout := &tx.Vout[j]
			// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
			vout.ValueSat, err = f.Parser.AmountToBigInt(vout.JsonValue)
			if err != nil {
				return nil, err
			}
			vout.JsonValue = ""
		}
	}

	return &res.Result, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (f *FloRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return f.GetTransaction(txid)
}
