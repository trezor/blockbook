package xzc

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

type ZcoinRPC struct {
	*btc.BitcoinRPC
}

func NewZcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	bc, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	zc := &ZcoinRPC{
		BitcoinRPC: bc.(*btc.BitcoinRPC),
	}

	zc.ChainConfig.Parse = true
	zc.ChainConfig.SupportsEstimateFee = true
	zc.ChainConfig.SupportsEstimateSmartFee = false
	zc.ParseBlocks = true

	return zc, nil
}

func (zc *ZcoinRPC) Initialize() error {
	chainName, err := zc.GetChainInfoAndInitializeMempool(zc)
	if err != nil {
		return err
	}

	params := GetChainParams(chainName)

	// always create parser
	zc.Parser = NewZcoinParser(params, zc.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		zc.Testnet = false
		zc.Network = "livenet"
	} else {
		zc.Testnet = true
		zc.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

func (zc *ZcoinRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	glog.V(1).Info("rpc: getblock (verbosity=true) ", hash)

	res := btc.ResGetBlockInfo{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = true
	err := zc.Call(&req, &res)

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

func (zc *ZcoinRPC) GetBlockRaw(hash string) ([]byte, error) {
	glog.V(1).Info("rpc: getblock (verbosity=false) ", hash)

	res := btc.ResGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = false
	err := zc.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return hex.DecodeString(res.Result)
}

func (zc *ZcoinRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction nonverbose ", txid)

	res := btc.ResGetRawTransactionNonverbose{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 0
	err := zc.Call(&req, &res)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	data, err := hex.DecodeString(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	tx, err := zc.Parser.ParseTx(data)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

func (zc *ZcoinRPC) GetTransactionSpecific(txid string) (json.RawMessage, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := btc.ResGetRawTransaction{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 1
	err := zc.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return res.Result, nil
}

func (zc *ZcoinRPC) EstimateFee(blocks int) (big.Int, error) {
	glog.V(1).Info("rpc: estimatefee ", blocks)

	res := btc.ResEstimateFee{}
	req := btc.CmdEstimateFee{Method: "estimatefee"}
	req.Params.Blocks = blocks
	err := zc.Call(&req, &res)

	var r big.Int
	if err != nil {
		return r, err
	}
	if res.Error != nil {
		return r, res.Error
	}

	// -1 mean zero fee
	if res.Result == "-1" {
		return r, nil
	}

	r, err = zc.Parser.AmountToBigInt(res.Result)
	if err != nil {
		return r, err
	}

	return r, nil
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}

type cmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbosity bool   `json:"verbosity"`
	} `json:"params"`
}

type cmdGetRawTransaction struct {
	Method string `json:"method"`
	Params struct {
		Txid    string `json:"txid"`
		Verbose int    `json:"verbose"`
	} `json:"params"`
}
