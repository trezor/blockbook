package server

import (
	"blockbook/api"
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"encoding/json"
	"encoding/hex"
	"math/big"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	gosocketio "github.com/martinboehm/golang-socketio"
	"github.com/martinboehm/golang-socketio/transport"
)

// SocketIoServer is handle to SocketIoServer
type SocketIoServer struct {
	server      *gosocketio.Server
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	mempool     bchain.Mempool
	metrics     *common.Metrics
	is          *common.InternalState
	api         *api.Worker
}

// NewSocketIoServer creates new SocketIo interface to blockbook and returns its handle
func NewSocketIoServer(db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState) (*SocketIoServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, is)
	if err != nil {
		return nil, err
	}

	server := gosocketio.NewServer(transport.GetDefaultWebsocketTransport())

	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel) {
		glog.Info("Client connected ", c.Id())
		metrics.SocketIOClients.Inc()
	})

	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		glog.Info("Client disconnected ", c.Id())
		metrics.SocketIOClients.Dec()
	})

	server.On(gosocketio.OnError, func(c *gosocketio.Channel) {
		glog.Error("Client error ", c.Id())
	})

	type Message struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	s := &SocketIoServer{
		server:      server,
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		mempool:     mempool,
		metrics:     metrics,
		is:          is,
		api:         api,
	}

	server.On("message", s.onMessage)
	server.On("subscribe", s.onSubscribe)

	return s, nil
}

// GetHandler returns socket.io http handler
func (s *SocketIoServer) GetHandler() http.Handler {
	return s.server
}

type addrOpts struct {
	Start            int  `json:"start"`
	End              int  `json:"end"`
	QueryMempoolOnly bool `json:"queryMempoolOnly"`
	From             int  `json:"from"`
	To               int  `json:"to"`
}

type assetOpts struct {
	Start            int  `json:"start"`
	End              int  `json:"end"`
	QueryMempoolOnly bool `json:"queryMempoolOnly"`
	From             int  `json:"from"`
	To               int  `json:"to"`
	AssetsMask 	     bchain.AssetsMask
}

var onMessageHandlers = map[string]func(*SocketIoServer, json.RawMessage) (interface{}, error){
	"getAddressTxids": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		addr, opts, err := unmarshalGetAddressRequest(params)
		if err == nil {
			rv, err = s.getAddressTxids(addr, &opts)
		}
		return
	},
	"getAddressHistory": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		addr, opts, err := unmarshalGetAddressRequest(params)
		if err == nil {
			rv, err = s.getAddressHistory(addr, &opts)
		}
		return
	},
	"getAssetTxids": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		asset, opts, err := unmarshalGetAssetRequest(params)
		if err == nil {
			rv, err = s.getAssetTxids(asset, &opts)
		}
		return
	},
	"getAssetHistory": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		asset, opts, err := unmarshalGetAssetRequest(params)
		if err == nil {
			rv, err = s.getAssetHistory(asset, &opts)
		}
		return
	},
	"getBlockHeader": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		height, hash, err := unmarshalGetBlockHeader(params)
		if err == nil {
			rv, err = s.getBlockHeader(height, hash)
		}
		return
	},
	"estimateSmartFee": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		blocks, conservative, err := unmarshalEstimateSmartFee(params)
		if err == nil {
			rv, err = s.estimateSmartFee(blocks, conservative)
		}
		return
	},
	"estimateFee": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		blocks, err := unmarshalEstimateFee(params)
		if err == nil {
			rv, err = s.estimateFee(blocks)
		}
		return
	},
	"getInfo": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		return s.getInfo()
	},
	"getDetailedTransaction": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		txid, err := unmarshalGetDetailedTransaction(params)
		if err == nil {
			rv, err = s.getDetailedTransaction(txid)
		}
		return
	},
	"sendTransaction": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		tx, err := unmarshalStringParameter(params)
		if err == nil {
			rv, err = s.sendTransaction(tx)
		}
		return
	},
	"getMempoolEntry": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		txid, err := unmarshalStringParameter(params)
		if err == nil {
			rv, err = s.getMempoolEntry(txid)
		}
		return
	},
}

type resultError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *SocketIoServer) onMessage(c *gosocketio.Channel, req map[string]json.RawMessage) (rv interface{}) {
	var err error
	method := strings.Trim(string(req["method"]), "\"")
	defer func() {
		if r := recover(); r != nil {
			glog.Error(c.Id(), " onMessage ", method, " recovered from panic: ", r)
			debug.PrintStack()
			e := resultError{}
			e.Error.Message = "Internal error"
			rv = e
		}
	}()
	t := time.Now()
	params := req["params"]
	defer s.metrics.SocketIOReqDuration.With(common.Labels{"method": method}).Observe(float64(time.Since(t)) / 1e3) // in microseconds
	f, ok := onMessageHandlers[method]
	if ok {
		rv, err = f(s, params)
	} else {
		err = errors.New("unknown method")
	}
	if err == nil {
		glog.V(1).Info(c.Id(), " onMessage ", method, " success")
		s.metrics.SocketIORequests.With(common.Labels{"method": method, "status": "success"}).Inc()
		return rv
	}
	glog.Error(c.Id(), " onMessage ", method, ": ", errors.ErrorStack(err), ", data ", string(params))
	s.metrics.SocketIORequests.With(common.Labels{"method": method, "status": "failure"}).Inc()
	e := resultError{}
	e.Error.Message = err.Error()
	return e
}

func unmarshalGetAddressRequest(params []byte) (addr []string, opts addrOpts, err error) {
	var p []json.RawMessage
	err = json.Unmarshal(params, &p)
	if err != nil {
		return
	}
	if len(p) != 2 {
		err = errors.New("incorrect number of parameters")
		return
	}
	err = json.Unmarshal(p[0], &addr)
	if err != nil {
		return
	}
	err = json.Unmarshal(p[1], &opts)
	return
}

func unmarshalGetAssetRequest(params []byte) (asset string, opts assetOpts, err error) {
	var p []json.RawMessage
	err = json.Unmarshal(params, &p)
	if err != nil {
		return
	}
	if len(p) != 2 {
		err = errors.New("incorrect number of parameters")
		return
	}
	err = json.Unmarshal(p[0], &asset)
	if err != nil {
		return
	}
	err = json.Unmarshal(p[1], &opts)
	return
}

type resultAddressTxids struct {
	Result []string `json:"result"`
}

func (s *SocketIoServer) getAddressTxids(addr []string, opts *addrOpts) (res resultAddressTxids, err error) {
	txids := make([]string, 0, 8)
	lower, higher := uint32(opts.End), uint32(opts.Start)
	for _, address := range addr {
		if !opts.QueryMempoolOnly {
			err = s.db.GetTransactions(address, lower, higher, func(txid string, height uint32, indexes []int32) error {
				txids = append(txids, txid)
				return nil
			})
			if err != nil {
				return res, err
			}
		} else {
			o, err := s.mempool.GetTransactions(address)
			if err != nil {
				return res, err
			}
			for _, m := range o {
				txids = append(txids, m.Txid)
			}
		}
	}
	res.Result = api.GetUniqueTxids(txids)
	return res, nil
}

func (s *SocketIoServer) getAssetTxids(asset string, opts *assetOpts) (res resultAddressTxids, err error) {
	txids := make([]string, 0, 8)
	lower, higher := uint32(opts.End), uint32(opts.Start)
	assetBitMask := opts.AssetsMask
	assetGuid, err := strconv.Atoi(asset)
	if err != nil {
		return res, err
	}
	if !opts.QueryMempoolOnly {
		err = s.db.GetTxAssets(uint32(assetGuid), lower, higher, assetBitMask, func(txidsIn []string) error {
			txids = append(txids, txidsIn...)
			return nil
		})
		if err != nil {
			return res, err
		}
	} else {
		o := s.mempool.GetTxAssets(uint32(assetGuid))
		for _, m := range o {
			txids = append(txids, m.Txid)
		}
	}
	res.Result = api.GetUniqueTxids(txids)
	return res, nil
}

type addressHistoryIndexes struct {
	InputIndexes  []int `json:"inputIndexes"`
	OutputIndexes []int `json:"outputIndexes"`
}

type txInputs struct {
	Txid        *string `json:"txid"`
	OutputIndex int     `json:"outputIndex"`
	Script      *string `json:"script"`
	// ScriptAsm   *string `json:"scriptAsm"`
	Sequence int64   `json:"sequence"`
	Address  *string `json:"address"`
	Satoshis int64   `json:"satoshis"`
}

type txOutputs struct {
	Satoshis int64   `json:"satoshis"`
	Script   *string `json:"script"`
	// ScriptAsm   *string `json:"scriptAsm"`
	// SpentTxID   *string `json:"spentTxId,omitempty"`
	// SpentIndex  int     `json:"spentIndex,omitempty"`
	// SpentHeight int     `json:"spentHeight,omitempty"`
	Address *string `json:"address"`
}

type resTx struct {
	Hex string `json:"hex"`
	// BlockHash      string      `json:"blockHash,omitempty"`
	Height         int    `json:"height"`
	BlockTimestamp int64  `json:"blockTimestamp,omitempty"`
	Version        int    `json:"version"`
	Hash           string `json:"hash"`
	Locktime       int    `json:"locktime,omitempty"`
	// Size           int         `json:"size,omitempty"`
	Inputs         []txInputs  `json:"inputs"`
	InputSatoshis  int64       `json:"inputSatoshis,omitempty"`
	Outputs        []txOutputs `json:"outputs"`
	OutputSatoshis int64       `json:"outputSatoshis,omitempty"`
	FeeSatoshis    int64       `json:"feeSatoshis,omitempty"`		   
	TokenTransferSummary []*bchain.TokenTransferSummary   `json:"tokenTransfers,omitempty"`
}

type addressHistoryItem struct {
	Addresses     map[string]*addressHistoryIndexes `json:"addresses"`
	Satoshis      int64                             `json:"satoshis"`
	Confirmations int                               `json:"confirmations"`
	Tx            resTx                             `json:"tx"`
	Tokens	      map[uint32]*api.TokenBalanceHistory 		`json:"tokens,omitempty"`	
}

type resultGetAddressHistory struct {
	Result struct {
		TotalCount int                  `json:"totalCount"`
		Items      []addressHistoryItem `json:"items"`
	} `json:"result"`
}
type resultGetAssetHistory struct {
	Result struct {
		TotalCount int                  `json:"totalCount"`
		AssetDetails  *api.AssetSpecific `json:"asset"`
		Items      []addressHistoryItem `json:"items"`
	} `json:"result"`
}
func txToResTx(tx *api.Tx) resTx {
	var resultTx resTx 
	inputs := make([]txInputs, len(tx.Vin))
	for i := range tx.Vin {
		vin := &tx.Vin[i]
		txid := vin.Txid
		script := vin.Hex
		input := txInputs{
			Txid:        &txid,
			Script:      &script,
			Sequence:    int64(vin.Sequence),
			OutputIndex: int(vin.Vout),
			Satoshis:    vin.ValueSat.AsInt64(),
		}
		if len(vin.Addresses) > 0 {
			a := vin.Addresses[0]
			input.Address = &a
		}
		inputs[i] = input
	}
	outputs := make([]txOutputs, len(tx.Vout))
	for i := range tx.Vout {
		vout := &tx.Vout[i]
		script := vout.Hex
		output := txOutputs{
			Satoshis: vout.ValueSat.AsInt64(),
			Script:   &script,
		}
		if len(vout.Addresses) > 0 {
			a := vout.Addresses[0]
			output.Address = &a
		}
		outputs[i] = output
	}
	if len(tx.TokenTransferSummary) > 0 {
		resultTx.TokenTransferSummary = tx.TokenTransferSummary
	}
	var h int
	var blocktime int64
	if tx.Confirmations == 0 {
		h = -1
	} else {
		h = int(tx.Blockheight)
		blocktime = tx.Blocktime
	}
	resultTx.BlockTimestamp = blocktime
	resultTx.FeeSatoshis =    tx.FeesSat.AsInt64()
	resultTx.Hash =           tx.Txid
	resultTx.Height =         h
	resultTx.Hex =            tx.Hex
	resultTx.Inputs =         inputs
	resultTx.InputSatoshis =  tx.ValueInSat.AsInt64()
	resultTx.Locktime =       int(tx.Locktime)
	resultTx.Outputs =        outputs
	resultTx.OutputSatoshis = tx.ValueOutSat.AsInt64()
	resultTx.Version =        int(tx.Version)
	return resultTx
}

func addressInSlice(s, t []string) string {
	for _, sa := range s {
		for _, ta := range t {
			if ta == sa {
				return sa
			}
		}
	}
	return ""
}

func (s *SocketIoServer) getAddressesFromVout(vout *bchain.Vout) ([]string, error) {
	addrDesc, err := s.chainParser.GetAddrDescFromVout(vout)
	if err != nil {
		return nil, err
	}
	voutAddr, _, err := s.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		return nil, err
	}
	return voutAddr, nil
}

func (s *SocketIoServer) getAddressHistory(addr []string, opts *addrOpts) (res resultGetAddressHistory, err error) {
	txr, err := s.getAddressTxids(addr, opts)
	if err != nil {
		return
	}
	txids := txr.Result
	res.Result.TotalCount = len(txids)
	res.Result.Items = make([]addressHistoryItem, 0, 8)
	to := len(txids)
	if to > opts.To {
		to = opts.To
	}
	ahi := addressHistoryItem{}
	for txi := opts.From; txi < to; txi++ {
		tx, err := s.api.GetTransaction(txids[txi], false, false)
		if err != nil {
			return res, err
		}
		ads := make(map[string]*addressHistoryIndexes)
		var totalSat big.Int
		for i := range tx.Vin {
			vin := &tx.Vin[i]
			a := addressInSlice(vin.Addresses, addr)
			if a != "" {
				hi := ads[a]
				if hi == nil {
					hi = &addressHistoryIndexes{OutputIndexes: []int{}}
					ads[a] = hi
				}
				hi.InputIndexes = append(hi.InputIndexes, int(vin.N))
				if vin.ValueSat != nil {
					totalSat.Sub(&totalSat, (*big.Int)(vin.ValueSat))
				}
				if vin.AssetInfo != nil {
					if ahi.Tokens == nil {
						ahi.Tokens = map[uint32]*api.TokenBalanceHistory{}
					}
					token, ok := ahi.Tokens[uint32(vin.AssetInfo.AssetGuid)]
					if !ok {
						token = &api.TokenBalanceHistory{AssetGuid: uint32(vin.AssetInfo.AssetGuid), ReceivedSat: &bchain.Amount{}, SentSat: &bchain.Amount{}}
						ahi.Tokens[uint32(vin.AssetInfo.AssetGuid)] = token
					}
					(*big.Int)(token.SentSat).Add((*big.Int)(token.SentSat), vin.AssetInfo.ValueSat)
				}
			}
		}
		for i := range tx.Vout {
			vout := &tx.Vout[i]
			a := addressInSlice(vout.Addresses, addr)
			if a != "" {
				hi := ads[a]
				if hi == nil {
					hi = &addressHistoryIndexes{InputIndexes: []int{}}
					ads[a] = hi
				}
				hi.OutputIndexes = append(hi.OutputIndexes, int(vout.N))
				if vout.ValueSat != nil {
					totalSat.Add(&totalSat, (*big.Int)(vout.ValueSat))
				}
				if vout.AssetInfo != nil {
					if ahi.Tokens == nil {
						ahi.Tokens = map[uint32]*api.TokenBalanceHistory{}
					}
					token, ok := ahi.Tokens[uint32(vout.AssetInfo.AssetGuid)]
					if !ok {
						token = &api.TokenBalanceHistory{AssetGuid: uint32(vout.AssetInfo.AssetGuid), ReceivedSat: &bchain.Amount{}, SentSat: &bchain.Amount{}}
						ahi.Tokens[uint32(vout.AssetInfo.AssetGuid)] = token
					}
					(*big.Int)(token.ReceivedSat).Add((*big.Int)(token.ReceivedSat), vout.AssetInfo.ValueSat)
				}
			}
		}
		if len(ahi.Tokens) <= 0 {
			ahi.Tokens = nil
		}
		ahi.Addresses = ads
		ahi.Confirmations = int(tx.Confirmations)
		ahi.Satoshis = totalSat.Int64()
		ahi.Tx = txToResTx(tx)
		res.Result.Items = append(res.Result.Items, ahi)
		// }
	}
	return
}
func (s *SocketIoServer) getAssetHistory(asset string, opts *assetOpts) (res resultGetAssetHistory, err error) {
	txr, err := s.getAssetTxids(asset, opts)
	if err != nil {
		return
	}
	txids := txr.Result
	res.Result.TotalCount = len(txids)
	res.Result.Items = make([]addressHistoryItem, 0, 8)
	to := len(txids)
	if to > opts.To {
		to = opts.To
	}
	ahi := addressHistoryItem{}
	ahi.Tokens = map[uint32]*api.TokenBalanceHistory{}
	guid, errAG := strconv.Atoi(asset)
	if errAG != nil {
		return res, errAG
	}
	assetGuid := uint32(guid)
	for txi := opts.From; txi < to; txi++ {
		tx, err := s.api.GetTransaction(txids[txi], false, false)
		if err != nil {
			return res, err
		}
		ads := make(map[string]*addressHistoryIndexes)
		var totalSat big.Int
		for i := range tx.Vin {
			vin := &tx.Vin[i]
			if vin.AssetInfo != nil && vin.AssetInfo.AssetGuid == assetGuid {
				a, _, err := s.chainParser.GetAddressesFromAddrDesc(vin.AddrDesc)
				if err != nil {
					return res, err
				}
				for _, addr := range a {
					hi := ads[addr]
					if hi == nil {
						hi = &addressHistoryIndexes{OutputIndexes: []int{}}
						ads[addr] = hi
					}
					hi.InputIndexes = append(hi.InputIndexes, int(vin.N))
				}
				if vin.ValueSat != nil {
					totalSat.Sub(&totalSat, (*big.Int)(vin.ValueSat))
				}
				token, ok := ahi.Tokens[uint32(vin.AssetInfo.AssetGuid)]
				if !ok {
					token = &api.TokenBalanceHistory{AssetGuid: uint32(vin.AssetInfo.AssetGuid), ReceivedSat: &bchain.Amount{}, SentSat: &bchain.Amount{}}
					ahi.Tokens[uint32(vin.AssetInfo.AssetGuid)] = token
				}
				(*big.Int)(token.SentSat).Add((*big.Int)(token.SentSat), vin.AssetInfo.ValueSat)
			}
		}
		for i := range tx.Vout {
			vout := &tx.Vout[i]
			if vout.AssetInfo != nil && vout.AssetInfo.AssetGuid == assetGuid {
				a, _, err := s.chainParser.GetAddressesFromAddrDesc(vout.AddrDesc)
				if err != nil {
					return res, err
				}
				for _, addr := range a {
					hi := ads[addr]
					if hi == nil {
						hi = &addressHistoryIndexes{InputIndexes: []int{}}
						ads[addr] = hi
					}
					hi.OutputIndexes = append(hi.OutputIndexes, int(vout.N))
				}
				if vout.ValueSat != nil {
					totalSat.Add(&totalSat, (*big.Int)(vout.ValueSat))
				}

				token, ok := ahi.Tokens[uint32(vout.AssetInfo.AssetGuid)]
				if !ok {
					token = &api.TokenBalanceHistory{AssetGuid: uint32(vout.AssetInfo.AssetGuid), ReceivedSat: &bchain.Amount{}, SentSat: &bchain.Amount{}}
					ahi.Tokens[uint32(vout.AssetInfo.AssetGuid)] = token
				}
				(*big.Int)(token.ReceivedSat).Add((*big.Int)(token.ReceivedSat), vout.AssetInfo.ValueSat)
				
			}
		}
		ahi.Addresses = ads
		dbAsset, errAsset := s.db.GetAsset(assetGuid, nil)
		if errAsset != nil || dbAsset == nil {
			if err == nil{
				return res, errors.New("getAssetHistory Asset not found")
			}
			return res, errAsset
		}
		if len(ahi.Tokens) <= 0 {
			ahi.Tokens = nil
		}
		ahi.Confirmations = int(tx.Confirmations)
		ahi.Satoshis = totalSat.Int64()
		ahi.Tx = txToResTx(tx)
		res.Result.AssetDetails =	&api.AssetSpecific{
			AssetGuid:		assetGuid,
			Symbol:			dbAsset.AssetObj.Symbol,
			AddrStr: 		dbAsset.AddrDesc.String(),
			Contract:		"0x" + hex.EncodeToString(dbAsset.AssetObj.Contract),
			Balance:		(*bchain.Amount)(big.NewInt(dbAsset.AssetObj.Balance)),
			TotalSupply:	(*bchain.Amount)(big.NewInt(dbAsset.AssetObj.TotalSupply)),
			MaxSupply:		(*bchain.Amount)(big.NewInt(dbAsset.AssetObj.MaxSupply)),
			Decimals:		int(dbAsset.AssetObj.Precision),
			UpdateFlags:	dbAsset.AssetObj.UpdateFlags,
		}
		json.Unmarshal(dbAsset.AssetObj.PubData, &res.Result.AssetDetails.PubData)
		res.Result.Items = append(res.Result.Items, ahi)
		// }
	}
	return
}

func unmarshalArray(params []byte, np int) (p []interface{}, err error) {
	err = json.Unmarshal(params, &p)
	if err != nil {
		return
	}
	if len(p) != np {
		err = errors.New("incorrect number of parameters")
		return
	}
	return
}

func unmarshalGetBlockHeader(params []byte) (height uint32, hash string, err error) {
	p, err := unmarshalArray(params, 1)
	if err != nil {
		return
	}
	fheight, ok := p[0].(float64)
	if ok {
		return uint32(fheight), "", nil
	}
	hash, ok = p[0].(string)
	if ok {
		return
	}
	err = errors.New("incorrect parameter")
	return
}

type resultGetBlockHeader struct {
	Result struct {
		Hash          string  `json:"hash"`
		Version       int     `json:"version"`
		Confirmations int     `json:"confirmations"`
		Height        int     `json:"height"`
		ChainWork     string  `json:"chainWork"`
		NextHash      string  `json:"nextHash"`
		MerkleRoot    string  `json:"merkleRoot"`
		Time          int     `json:"time"`
		MedianTime    int     `json:"medianTime"`
		Nonce         int     `json:"nonce"`
		Bits          string  `json:"bits"`
		Difficulty    float64 `json:"difficulty"`
	} `json:"result"`
}

func (s *SocketIoServer) getBlockHeader(height uint32, hash string) (res resultGetBlockHeader, err error) {
	if hash == "" {
		// trezor is interested only in hash
		hash, err = s.db.GetBlockHash(height)
		if err != nil {
			return
		}
		res.Result.Hash = hash
		return
	}
	bh, err := s.chain.GetBlockHeader(hash)
	if err != nil {
		return
	}
	res.Result.Hash = bh.Hash
	res.Result.Confirmations = bh.Confirmations
	res.Result.Height = int(bh.Height)
	res.Result.NextHash = bh.Next
	return
}

func unmarshalEstimateSmartFee(params []byte) (blocks int, conservative bool, err error) {
	p, err := unmarshalArray(params, 2)
	if err != nil {
		return
	}
	fblocks, ok := p[0].(float64)
	if !ok {
		err = errors.New("Invalid parameter blocks")
		return
	}
	blocks = int(fblocks)
	conservative, ok = p[1].(bool)
	if !ok {
		err = errors.New("Invalid parameter conservative")
		return
	}
	return
}

type resultEstimateSmartFee struct {
	// for compatibility reasons use float64
	Result float64 `json:"result"`
}

func (s *SocketIoServer) estimateSmartFee(blocks int, conservative bool) (res resultEstimateSmartFee, err error) {
	fee, err := s.chain.EstimateSmartFee(blocks, conservative)
	if err != nil {
		return
	}
	res.Result, err = strconv.ParseFloat(s.chainParser.AmountToDecimalString(&fee), 64)
	return
}

func unmarshalEstimateFee(params []byte) (blocks int, err error) {
	p, err := unmarshalArray(params, 1)
	if err != nil {
		return
	}
	fblocks, ok := p[0].(float64)
	if !ok {
		err = errors.New("Invalid parameter nblocks")
		return
	}
	blocks = int(fblocks)
	return
}

type resultEstimateFee struct {
	// for compatibility reasons use float64
	Result float64 `json:"result"`
}

func (s *SocketIoServer) estimateFee(blocks int) (res resultEstimateFee, err error) {
	fee, err := s.chain.EstimateFee(blocks)
	if err != nil {
		return
	}
	res.Result, err = strconv.ParseFloat(s.chainParser.AmountToDecimalString(&fee), 64)
	return
}

type resultGetInfo struct {
	Result struct {
		Version         int     `json:"version,omitempty"`
		ProtocolVersion int     `json:"protocolVersion,omitempty"`
		Blocks          int     `json:"blocks"`
		TimeOffset      int     `json:"timeOffset,omitempty"`
		Connections     int     `json:"connections,omitempty"`
		Proxy           string  `json:"proxy,omitempty"`
		Difficulty      float64 `json:"difficulty,omitempty"`
		Testnet         bool    `json:"testnet"`
		RelayFee        float64 `json:"relayFee,omitempty"`
		Errors          string  `json:"errors,omitempty"`
		Network         string  `json:"network,omitempty"`
		Subversion      string  `json:"subversion,omitempty"`
		LocalServices   string  `json:"localServices,omitempty"`
		CoinName        string  `json:"coin_name,omitempty"`
		About           string  `json:"about,omitempty"`
	} `json:"result"`
}

func (s *SocketIoServer) getInfo() (res resultGetInfo, err error) {
	_, height, _ := s.is.GetSyncState()
	res.Result.Blocks = int(height)
	res.Result.Testnet = s.chain.IsTestnet()
	res.Result.Network = s.chain.GetNetworkName()
	res.Result.Subversion = s.chain.GetSubversion()
	res.Result.CoinName = s.chain.GetCoinName()
	res.Result.About = api.Text.BlockbookAbout
	return
}

func unmarshalStringParameter(params []byte) (s string, err error) {
	p, err := unmarshalArray(params, 1)
	if err != nil {
		return
	}
	s, ok := p[0].(string)
	if ok {
		return
	}
	err = errors.New("incorrect parameter")
	return
}

func unmarshalGetDetailedTransaction(params []byte) (txid string, err error) {
	var p []json.RawMessage
	err = json.Unmarshal(params, &p)
	if err != nil {
		return
	}
	if len(p) != 1 {
		err = errors.New("incorrect number of parameters")
		return
	}
	err = json.Unmarshal(p[0], &txid)
	if err != nil {
		return
	}
	return
}

type resultGetDetailedTransaction struct {
	Result resTx `json:"result"`
}

func (s *SocketIoServer) getDetailedTransaction(txid string) (res resultGetDetailedTransaction, err error) {
	tx, err := s.api.GetTransaction(txid, false, false)
	if err != nil {
		return res, err
	}
	res.Result = txToResTx(tx)
	return
}

func (s *SocketIoServer) sendTransaction(tx string) (res resultSendTransaction, err error) {
	txid, err := s.chain.SendRawTransaction(tx)
	if err != nil {
		return res, err
	}
	res.Result = txid
	return
}

type resultGetMempoolEntry struct {
	Result *bchain.MempoolEntry `json:"result"`
}

func (s *SocketIoServer) getMempoolEntry(txid string) (res resultGetMempoolEntry, err error) {
	entry, err := s.chain.GetMempoolEntry(txid)
	if err != nil {
		return res, err
	}
	res.Result = entry
	return
}

// onSubscribe expects two event subscriptions based on the req parameter (including the doublequotes):
// "bitcoind/hashblock"
// "bitcoind/addresstxid",["2MzTmvPJLZaLzD9XdN3jMtQA5NexC3rAPww","2NAZRJKr63tSdcTxTN3WaE9ZNDyXy6PgGuv"]
func (s *SocketIoServer) onSubscribe(c *gosocketio.Channel, req []byte) interface{} {
	defer func() {
		if r := recover(); r != nil {
			glog.Error(c.Id(), " onSubscribe recovered from panic: ", r)
			debug.PrintStack()
		}
	}()

	onError := func(id, sc, err, detail string) {
		glog.Error(id, " onSubscribe ", err, ": ", detail)
		s.metrics.SocketIOSubscribes.With(common.Labels{"channel": sc, "status": "failure"}).Inc()
	}

	r := string(req)
	glog.V(1).Info(c.Id(), " onSubscribe ", r)
	var sc string
	i := strings.Index(r, "\",[")
	if i > 0 {
		var addrs []string
		sc = r[1:i]
		if sc != "bitcoind/addresstxid" {
			onError(c.Id(), sc, "invalid data", "expecting bitcoind/addresstxid, req: "+r)
			return nil
		}
		err := json.Unmarshal([]byte(r[i+2:]), &addrs)
		if err != nil {
			onError(c.Id(), sc, "invalid data", err.Error()+", req: "+r)
			return nil
		}
		// normalize the addresses to AddressDescriptor
		descs := make([]bchain.AddressDescriptor, len(addrs))
		for i, a := range addrs {
			d, err := s.chainParser.GetAddrDescFromAddress(a)
			if err != nil {
				onError(c.Id(), sc, "invalid address "+a, err.Error()+", req: "+r)
				return nil
			}
			descs[i] = d
		}
		for _, d := range descs {
			c.Join("bitcoind/addresstxid-" + string(d))
		}
	} else {
		sc = r[1 : len(r)-1]
		if sc != "bitcoind/hashblock" {
			onError(c.Id(), sc, "invalid data", "expecting bitcoind/hashblock, req: "+r)
			return nil
		}
		c.Join(sc)
	}
	s.metrics.SocketIOSubscribes.With(common.Labels{"channel": sc, "status": "success"}).Inc()
	return nil
}

// OnNewBlockHash notifies users subscribed to bitcoind/hashblock about new block
func (s *SocketIoServer) OnNewBlockHash(hash string) {
	c := s.server.BroadcastTo("bitcoind/hashblock", "bitcoind/hashblock", hash)
	glog.Info("broadcasting new block hash ", hash, " to ", c, " channels")
}

// OnNewTxAddr notifies users subscribed to bitcoind/addresstxid about new block
func (s *SocketIoServer) OnNewTxAddr(txid string, desc bchain.AddressDescriptor) {
	addr, searchable, err := s.chainParser.GetAddressesFromAddrDesc(desc)
	if err != nil {
		glog.Error("GetAddressesFromAddrDesc error ", err, " for descriptor ", desc)
	} else if searchable && len(addr) == 1 {
		data := map[string]interface{}{"address": addr[0], "txid": txid}
		c := s.server.BroadcastTo("bitcoind/addresstxid-"+string(desc), "bitcoind/addresstxid", data)
		if c > 0 {
			glog.Info("broadcasting new txid ", txid, " for addr ", addr[0], " to ", c, " channels")
		}
	}
}
