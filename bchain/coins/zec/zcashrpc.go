package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"encoding/json"

	"github.com/btcsuite/btcd/wire"

	"github.com/golang/glog"
	"github.com/juju/errors"
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

func (z *ZCashRPC) Initialize(mempool *bchain.Mempool) error {
	z.Mempool = mempool

	chainName, err := z.GetBlockChainInfo()
	if err != nil {
		return err
	}

	params := GetChainParams(chainName)

	// always create parser
	z.Parser = &ZCashBlockParser{
		btc.BitcoinBlockParser{
			Params: params,
		},
	}

	// parameters for getInfo request
	if params.Net == wire.MainNet {
		z.Testnet = false
		z.Network = "livenet"
	} else {
		z.Testnet = true
		z.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

type untypedArrayParams struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// getblockhash

type resGetBlockHash struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getblock

type resGetBlockThin struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.ThinBlock `json:"result"`
}

// getrawtransaction

type resGetRawTransaction struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.Tx        `json:"result"`
}

// getblockheader

type resGetBlockHeader struct {
	Error  *bchain.RPCError   `json:"error"`
	Result bchain.BlockHeader `json:"result"`
}

// GetBlock returns block with given hash.
func (z *ZCashRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := resGetBlockThin{}
	req := untypedArrayParams{Method: "getblock"}
	req.Params = append(req.Params, hash)
	req.Params = append(req.Params, true)
	err := z.Call(req.Method, &req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	txs := make([]bchain.Tx, len(res.Result.Txids))
	for i, txid := range res.Result.Txids {
		tx, err := z.GetTransaction(txid)
		if err != nil {
			return nil, err
		}
		txs[i] = *tx
	}
	block := &bchain.Block{
		BlockHeader: res.Result.BlockHeader,
		Txs:         txs,
	}
	return block, nil
}

// GetTransaction returns a transaction by the transaction ID.
func (z *ZCashRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := resGetRawTransaction{}
	req := untypedArrayParams{Method: "getrawtransaction"}
	req.Params = append(req.Params, txid)
	req.Params = append(req.Params, 1)
	err := z.Call(req.Method, &req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return &res.Result, nil
}

// GetBlockHash returns hash of block in best-block-chain at given height.
func (z *ZCashRPC) GetBlockHash(height uint32) (string, error) {
	glog.V(1).Info("rpc: getblockhash ", height)

	res := resGetBlockHash{}
	req := untypedArrayParams{Method: "getblockhash"}
	req.Params = append(req.Params, height)
	err := z.Call(req.Method, &req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "height %v", height)
	}
	if res.Error != nil {
		return "", errors.Annotatef(res.Error, "height %v", height)
	}
	return res.Result, nil
}

// GetBlockHeader returns header of block with given hash.
func (z *ZCashRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	glog.V(1).Info("rpc: getblockheader")

	res := resGetBlockHeader{}
	req := untypedArrayParams{Method: "getblockheader"}
	req.Params = append(req.Params, hash)
	req.Params = append(req.Params, true)
	err := z.Call(req.Method, &req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}
