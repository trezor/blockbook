package grs

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// GroestlcoinRPC is an interface to JSON-RPC service
type GroestlcoinRPC struct {
	*btc.BitcoinRPC
}

// NewGroestlcoinRPC returns new GroestlcoinRPC instance
func NewGroestlcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}
	g := &GroestlcoinRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	g.RPCMarshaler = btc.JSONMarshalerV1{}
	return g, nil
}

// Initialize initializes GroestlcoinRPC instance.
func (g *GroestlcoinRPC) Initialize() error {
	ci, err := g.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	g.Parser = NewGroestlcoinParser(params, g.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		g.Testnet = false
		g.Network = "livenet"
	} else {
		g.Testnet = true
		g.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// GetBlock returns block with given hash.
func (g *GroestlcoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = g.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := btc.ResGetBlockThin{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err = g.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	txs := make([]bchain.Tx, 0, len(res.Result.Txids))
	for _, txid := range res.Result.Txids {
		tx, err := g.GetTransaction(txid)
		if err != nil {
			if err == bchain.ErrTxNotFound {
				glog.Errorf("rpc: getblock: skipping transanction in block %s due error: %s", hash, err)
				continue
			}
			return nil, err
		}
		txs = append(txs, *tx)
	}
	block := &bchain.Block{
		BlockHeader: res.Result.BlockHeader,
		Txs:         txs,
	}
	return block, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (g *GroestlcoinRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return g.GetTransaction(txid)
}
