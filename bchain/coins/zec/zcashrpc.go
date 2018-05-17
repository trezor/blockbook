package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

type ZCashRPC struct {
	*btc.BitcoinRPC
}

func NewZCashRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}
	z := &ZCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	return z, nil
}

// Initialize initializes ZCashRPC instance.
func (z *ZCashRPC) Initialize() error {
	_, err := z.GetChainInfoAndInitializeMempool(z)
	if err != nil {
		return err
	}

	z.Parser = &ZCashParser{}
	z.Testnet = false
	z.Network = "livenet"

	glog.Info("rpc: block chain mainnet")

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

// estimatefee

type resEstimateFee struct {
	Error  *bchain.RPCError `json:"error"`
	Result float64          `json:"result"`
}

// GetBlock returns block with given hash.
func (z *ZCashRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = z.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := resGetBlockThin{}
	req := untypedArrayParams{Method: "getblock"}
	req.Params = append(req.Params, hash)
	req.Params = append(req.Params, true)
	err = z.Call(&req, &res)

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
			if isInvalidTx(err) {
				glog.Errorf("rpc: getblock: skipping transanction in block %s due error: %s", hash, err)
				continue
			}
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

func isInvalidTx(err error) bool {
	switch e1 := err.(type) {
	case *errors.Err:
		switch e2 := e1.Cause().(type) {
		case *bchain.RPCError:
			if e2.Code == -5 { // "No information available about transaction"
				return true
			}
		}
	}

	return false
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (z *ZCashRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return z.GetTransaction(txid)
}

// GetTransaction returns a transaction by the transaction ID.
func (z *ZCashRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := resGetRawTransaction{}
	req := untypedArrayParams{Method: "getrawtransaction"}
	req.Params = append(req.Params, txid)
	req.Params = append(req.Params, 1)
	err := z.Call(&req, &res)

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
	err := z.Call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "height %v", height)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return "", bchain.ErrBlockNotFound
		}
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
	err := z.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// EstimateSmartFee returns fee estimation.
func (z *ZCashRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	glog.V(1).Info("rpc: estimatesmartfee")

	return z.estimateFee(blocks)
}

// EstimateFee returns fee estimation.
func (z *ZCashRPC) EstimateFee(blocks int) (float64, error) {
	glog.V(1).Info("rpc: estimatefee ", blocks)

	return z.estimateFee(blocks)
}

func (z *ZCashRPC) estimateFee(blocks int) (float64, error) {
	res := resEstimateFee{}
	req := untypedArrayParams{Method: "estimatefee"}
	req.Params = append(req.Params, blocks)
	err := z.Call(&req, &res)

	if err != nil {
		return 0, err
	}
	if res.Error != nil {
		return 0, res.Error
	}
	return res.Result, nil
}

// GetMempoolEntry returns mempool data for given transaction
func (z *ZCashRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not implemented")
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}
