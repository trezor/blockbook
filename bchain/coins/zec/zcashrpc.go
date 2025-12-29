package zec

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
)

// ZCashRPC is an interface to JSON-RPC bitcoind service
type ZCashRPC struct {
	*btc.BitcoinRPC
}

// ResGetBlockChainInfo is a response to GetChainInfo request
type ResGetBlockChainInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Chain         string            `json:"chain"`
		Blocks        int               `json:"blocks"`
		Headers       int               `json:"headers"`
		Bestblockhash string            `json:"bestblockhash"`
		Difficulty    common.JSONNumber `json:"difficulty"`
		Pruned        bool              `json:"pruned"`
		SizeOnDisk    int64             `json:"size_on_disk"`
		Consensus     struct {
			Chaintip  string `json:"chaintip"`
			Nextblock string `json:"nextblock"`
		} `json:"consensus"`
	} `json:"result"`
}

// NewZCashRPC returns new ZCashRPC instance
func NewZCashRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}
	z := &ZCashRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
	}
	z.RPCMarshaler = JSONMarshalerV1Zebra{}
	z.ChainConfig.SupportsEstimateSmartFee = false
	return z, nil
}

// Initialize initializes ZCashRPC instance
func (z *ZCashRPC) Initialize() error {
	ci, err := z.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	z.Parser = NewZCashParser(params, z.ChainConfig)

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

// GetChainInfo return info about the blockchain
func (z *ZCashRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	chainInfo := ResGetBlockChainInfo{}
	err := z.Call(&btc.CmdGetBlockChainInfo{Method: "getblockchaininfo"}, &chainInfo)
	if err != nil {
		return nil, err
	}
	if chainInfo.Error != nil {
		return nil, chainInfo.Error
	}

	// networkinfo not supported by zebra
	networkInfo := btc.ResGetNetworkInfo{}

	zebrad := "zebra"
	cmd := exec.Command("/opt/coins/nodes/zcash/bin/zebrad", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err == nil {
		zebrad = out.String()
	}

	return &bchain.ChainInfo{
		Bestblockhash:   chainInfo.Result.Bestblockhash,
		Blocks:          chainInfo.Result.Blocks,
		Chain:           chainInfo.Result.Chain,
		Difficulty:      string(chainInfo.Result.Difficulty),
		Headers:         chainInfo.Result.Headers,
		SizeOnDisk:      chainInfo.Result.SizeOnDisk,
		Version:         zebrad,
		Subversion:      string(networkInfo.Result.Subversion),
		ProtocolVersion: string(networkInfo.Result.ProtocolVersion),
		Timeoffset:      networkInfo.Result.Timeoffset,
		Consensus:       chainInfo.Result.Consensus,
		Warnings:        networkInfo.Result.Warnings,
	}, nil
}

// GetBlock returns block with given hash.
func (z *ZCashRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	type rpcBlock struct {
		bchain.BlockHeader
		Txs []bchain.Tx `json:"tx"`
	}
	type resGetBlockV1 struct {
		Error  *bchain.RPCError `json:"error"`
		Result bchain.BlockInfo `json:"result"`
	}
	type resGetBlockV2 struct {
		Error  *bchain.RPCError `json:"error"`
		Result rpcBlock         `json:"result"`
	}

	var err error
	if hash == "" && height > 0 {
		hash, err = z.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	var rawResponse json.RawMessage
	resV2 := resGetBlockV2{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 2
	err = z.Call(&req, &rawResponse)
	if err != nil {
		// Check if it's a memory error and fall back
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "memory capacity exceeded") || strings.Contains(errStr, "response is too big") {
			glog.Warningf("getblock verbosity=2 failed for block %v, falling back to individual tx fetches", hash)
			return z.getBlockWithFallback(hash)
		}
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	// hack for ZCash, where the field "valueZat" is used instead of "valueSat"
	rawResponse = bytes.ReplaceAll(rawResponse, []byte(`"valueZat"`), []byte(`"valueSat"`))
	err = json.Unmarshal(rawResponse, &resV2)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}

	// Check if verbosity=2 returned an RPC error
	if resV2.Error != nil {
		// Check if error is memory-related (case-insensitive)
		errorMsg := strings.ToLower(resV2.Error.Message)
		if strings.Contains(errorMsg, "memory capacity exceeded") || strings.Contains(errorMsg, "response is too big") {
			glog.Warningf("getblock verbosity=2 returned memory error for block %v, falling back to verbosity=1 + individual tx fetches", hash)
			return z.getBlockWithFallback(hash)
		}
		return nil, errors.Annotatef(resV2.Error, "hash %v", hash)
	}

	block := &bchain.Block{
		BlockHeader: resV2.Result.BlockHeader,
		Txs:         resV2.Result.Txs,
	}

	// transactions fetched in block with verbosity 2 do not contain txids, so we need to get it separately
	resV1 := resGetBlockV1{}
	req.Params.Verbosity = 1
	err = z.Call(&req, &resV1)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if resV1.Error != nil {
		return nil, errors.Annotatef(resV1.Error, "hash %v", hash)
	}
	for i := range resV1.Result.Txids {
		block.Txs[i].Txid = resV1.Result.Txids[i]
	}
	return block, nil
}

// getBlockWithFallback fetches block using verbosity=1 and then fetches each transaction individually
func (z *ZCashRPC) getBlockWithFallback(hash string) (*bchain.Block, error) {
	type resGetBlockV1 struct {
		Error  *bchain.RPCError `json:"error"`
		Result bchain.BlockInfo `json:"result"`
	}

	// Get block header and txids using verbosity=1
	resV1 := resGetBlockV1{}
	req := btc.CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err := z.Call(&req, &resV1)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if resV1.Error != nil {
		return nil, errors.Annotatef(resV1.Error, "hash %v", hash)
	}

	// Create block with header from verbosity=1 response
	block := &bchain.Block{
		BlockHeader: resV1.Result.BlockHeader,
		Txs:         make([]bchain.Tx, 0, len(resV1.Result.Txids)),
	}

	// Fetch each transaction individually
	for _, txid := range resV1.Result.Txids {
		tx, err := z.GetTransaction(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to fetch tx %v for block %v", txid, hash)
		}
		block.Txs = append(block.Txs, *tx)
	}

	return block, nil
}

// GetTransaction returns a transaction by the transaction ID
func (z *ZCashRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	r, err := z.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}
	// hack for ZCash, where the field "valueZat" is used instead of "valueSat"
	r = bytes.ReplaceAll(r, []byte(`"valueZat"`), []byte(`"valueSat"`))
	tx, err := z.Parser.ParseTxFromJson(r)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	tx.Blocktime = tx.Time
	tx.Txid = txid
	tx.CoinSpecificData = r
	return tx, nil
}

// getRawTransaction returns json as returned by backend, with all coin specific data
func (z *ZCashRPC) getRawTransaction(txid string) (json.RawMessage, error) {
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
		if btc.IsMissingTx(res.Error) {
			return nil, bchain.ErrTxNotFound
		}
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return res.Result, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (z *ZCashRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return z.GetTransaction(txid)
}

// GetMempoolEntry returns mempool data for given transaction
func (z *ZCashRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not implemented")
}

// GetBlockRaw is not supported
func (z *ZCashRPC) GetBlockRaw(hash string) (string, error) {
	return "", errors.New("GetBlockRaw: not supported")
}

// JSONMarshalerV1 is used for marshalling requests to legacy Bitcoin Type RPC interfaces
type JSONMarshalerV1Zebra struct{}

// Marshal converts struct passed by parameter to JSON
func (JSONMarshalerV1Zebra) Marshal(v interface{}) ([]byte, error) {
	u := cmdUntypedParams{}

	switch v := v.(type) {
	case *btc.CmdGetBlock:
		u.Method = v.Method
		u.Params = append(u.Params, v.Params.BlockHash)
		u.Params = append(u.Params, v.Params.Verbosity)
	case *btc.CmdGetRawTransaction:
		var n int
		if v.Params.Verbose {
			n = 1
		}
		u.Method = v.Method
		u.Params = append(u.Params, v.Params.Txid)
		u.Params = append(u.Params, n)
	default:
		{
			v := reflect.ValueOf(v).Elem()

			f := v.FieldByName("Method")
			if !f.IsValid() || f.Kind() != reflect.String {
				return nil, btc.ErrInvalidValue
			}
			u.Method = f.String()

			f = v.FieldByName("Params")
			if f.IsValid() {
				var arr []interface{}
				switch f.Kind() {
				case reflect.Slice:
					arr = make([]interface{}, f.Len())
					for i := 0; i < f.Len(); i++ {
						arr[i] = f.Index(i).Interface()
					}
				case reflect.Struct:
					arr = make([]interface{}, f.NumField())
					for i := 0; i < f.NumField(); i++ {
						arr[i] = f.Field(i).Interface()
					}
				default:
					return nil, btc.ErrInvalidValue
				}
				u.Params = arr
			}
		}
	}
	u.Id = "-"
	if u.Params == nil {
		u.Params = make([]interface{}, 0)
	}
	d, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	return d, nil
}

type cmdUntypedParams struct {
	Method string        `json:"method"`
	Id     string        `json:"id"`
	Params []interface{} `json:"params"`
}
