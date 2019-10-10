package denarius

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// DenariusRPC is an interface to JSON-RPC denariusd service.
type DenariusRPC struct {
	*btc.BitcoinRPC
}

// NewDenariusRPC returns new DenariusRPC instance.
func NewDenariusRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}
	d := &DenariusRPC{
		b.(*btc.BitcoinRPC),
	}
	d.RPCMarshaler = btc.JSONMarshalerV1{}

	return d, nil
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
func (d *DenariusRPC) Initialize() error {
	chainName, err := d.GetChainInfoAndInitializeMempool(d)
	s := &DenariusRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes DenariusRPC instance.
func (b *DenariusRPC) Initialize() error {
	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}
	params := GetChainParams(chainName)

	// always create parser
	d.Parser = NewDenariusParser(d.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		d.Testnet = false
		d.Network = "livenet"
	} else {
		d.Testnet = true
		d.Network = "testnet"
		
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

// GetBlock returns block with given hash.
/*
func (d *DenariusRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = d.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := btc.ResGetBlockThin{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err = d.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	txs := make([]bchain.Tx, 0, len(res.Result.Txids))
	for _, txid := range res.Result.Txids {
		tx, err := d.GetTransaction(txid)
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
*/

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
func (d *DenariusRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return d.GetTransaction(txid)
}

// EstimateFee returns fee estimation.
func (d *DenariusRPC) EstimateFee(blocks int) (float64, error) {
	return d.EstimateSmartFee(blocks, true)
}

// GetMempoolEntry returns mempool data for given transaction
func (d *DenariusRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not implemented")
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}

func (b *DenariusRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
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
