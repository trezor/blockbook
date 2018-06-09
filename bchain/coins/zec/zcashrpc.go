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
	z.RPCMarshaler = btc.JSONMarshalerV1{}
	return z, nil
}

// Initialize initializes ZCashRPC instance.
func (z *ZCashRPC) Initialize() error {
	chainName, err := z.GetChainInfoAndInitializeMempool(z)
	if err != nil {
		return err
	}

	params := GetChainParams(chainName)

	z.Parser = NewZCashParser(z.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		z.Testnet = false
		z.Network = "livenet"
	} else {
		z.Testnet = true
		z.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
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

	res := btc.ResGetBlockThin{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err = z.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	txs := make([]bchain.Tx, 0, len(res.Result.Txids))
	for _, txid := range res.Result.Txids {
		tx, err := z.GetTransaction(txid)
		if err != nil {
			if isInvalidTx(err) {
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

	res := btc.ResGetRawTransaction{}
	req := btc.CmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = true
	err := z.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	tx, err := z.Parser.ParseTxFromJson(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

// EstimateSmartFee returns fee estimation.
func (z *ZCashRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	glog.V(1).Info("rpc: estimatesmartfee")

	// return z.estimateFee(blocks)
	return z.EstimateFee(blocks)
}

// GetMempoolEntry returns mempool data for given transaction
func (z *ZCashRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not implemented")
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}
