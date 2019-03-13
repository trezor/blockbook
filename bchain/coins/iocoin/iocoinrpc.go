package iocoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
)

// IocoinRPC is an interface to JSON-RPC namecoin service.
type IocoinRPC struct {
	*btc.BitcoinRPC
}

type CmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
	} `json:"params"`
}

// NewIocoinRPC returns new IocoinRPC instance.
func NewIocoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &IocoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes IocoinRPC instance.
func (b *IocoinRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewIocoinParser(params, b.ChainConfig)

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

// GetBlock returns block with given hash.
func (b *IocoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error

	glog.Info("XXXX GetBlock ",  height)
	if hash == "" {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	res := btc.ResGetBlockThin{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	err = b.Call(&req, &res)
	glog.Info("XXXX get tx ids ")
	txs := make([]bchain.Tx, 0, len(res.Result.Txids))
	for _, txid := range res.Result.Txids {
		  glog.Info("XXXX txid ", txid)
		tx, err := b.GetTransaction(txid)
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
/*func (b *IocoinRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	  glog.Info("XXXX gettransaction");
	r, err := b.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}
	tx, err := b.Parser.ParseTxFromJson(r)
	//tx.CoinSpecificData = r
	//if err != nil {
//		return nil, errors.Annotatef(err, "txid %v", txid)
//	}
	return tx, nil
}

func (b *IocoinRPC) getRawTransaction(txid string) (json.RawMessage, error) {
	glog.Info("XXXX rpc: getrawtransaction ", txid)

	res := btc.ResGetRawTransaction{}
	req := btc.CmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = true
	err := b.Call(&req, &res)
	if res.Error != nil {
		  return nil, err
	}
	return res.Result, nil
}*/
