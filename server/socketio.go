package server

import (
	"blockbook/bchain"
	"blockbook/db"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
)

// SocketIoServer is handle to SocketIoServer
type SocketIoServer struct {
	binding   string
	certFiles string
	server    *gosocketio.Server
	https     *http.Server
	db        *db.RocksDB
	mempool   *bchain.Mempool
	chain     *bchain.BitcoinRPC
}

// NewSocketIoServer creates new SocketIo interface to blockbook and returns its handle
func NewSocketIoServer(binding string, certFiles string, db *db.RocksDB, mempool *bchain.Mempool, chain *bchain.BitcoinRPC) (*SocketIoServer, error) {
	server := gosocketio.NewServer(transport.GetDefaultWebsocketTransport())

	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel) {
		glog.Info("Client connected ", c.Id())
	})

	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		glog.Info("Client disconnected ", c.Id())
	})

	server.On(gosocketio.OnError, func(c *gosocketio.Channel) {
		glog.Info("Client error ", c.Id())
	})

	type Message struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	addr, path := splitBinding(binding)
	serveMux := http.NewServeMux()
	serveMux.Handle(path, server)
	https := &http.Server{
		Addr:    addr,
		Handler: serveMux,
	}

	s := &SocketIoServer{
		binding:   binding,
		certFiles: certFiles,
		https:     https,
		server:    server,
		db:        db,
		mempool:   mempool,
		chain:     chain,
	}

	server.On("message", s.onMessage)
	server.On("subscribe", s.onSubscribe)

	return s, nil
}

func splitBinding(binding string) (addr string, path string) {
	i := strings.Index(binding, "/")
	if i >= 0 {
		return binding[0:i], binding[i:]
	}
	return binding, "/"
}

// Run starts the server
func (s *SocketIoServer) Run() error {
	if s.certFiles == "" {
		glog.Info("socketio server starting to listen on ws://", s.https.Addr)
		return s.https.ListenAndServe()
	}
	glog.Info("socketio server starting to listen on wss://", s.https.Addr)
	return s.https.ListenAndServeTLS(fmt.Sprint(s.certFiles, ".crt"), fmt.Sprint(s.certFiles, ".key"))
}

// Close closes the server
func (s *SocketIoServer) Close() error {
	glog.Infof("socketio server closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *SocketIoServer) Shutdown(ctx context.Context) error {
	glog.Infof("socketio server shutdown")
	return s.https.Shutdown(ctx)
}

type reqRange struct {
	Start            int  `json:"start"`
	End              int  `json:"end"`
	QueryMempol      bool `json:"queryMempol"`
	QueryMempoolOnly bool `json:"queryMempoolOnly"`
	From             int  `json:"from"`
	To               int  `json:"to"`
}

var onMessageHandlers = map[string]func(*SocketIoServer, json.RawMessage) (interface{}, error){
	"\"getAddressTxids\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		addr, rr, err := unmarshalGetAddressRequest(params)
		if err == nil {
			rv, err = s.getAddressTxids(addr, &rr)
		}
		return
	},
	"\"getAddressHistory\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		addr, rr, err := unmarshalGetAddressRequest(params)
		if err == nil {
			rv, err = s.getAddressHistory(addr, &rr)
		}
		return
	},
	"\"getBlockHeader\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		height, hash, err := unmarshalGetBlockHeader(params)
		if err == nil {
			rv, err = s.getBlockHeader(height, hash)
		}
		return
	},
	"\"estimateSmartFee\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		blocks, conservative, err := unmarshalEstimateSmartFee(params)
		if err == nil {
			rv, err = s.estimateSmartFee(blocks, conservative)
		}
		return
	},
	"\"getInfo\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		return s.getInfo()
	},
	"\"getDetailedTransaction\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		txid, err := unmarshalGetDetailedTransaction(params)
		if err == nil {
			rv, err = s.getDetailedTransaction(txid)
		}
		return
	},
	"\"sendTransaction\"": func(s *SocketIoServer, params json.RawMessage) (rv interface{}, err error) {
		tx, err := unmarshalSendTransaction(params)
		if err == nil {
			rv, err = s.sendTransaction(tx)
		}
		return
	},
}

func (s *SocketIoServer) onMessage(c *gosocketio.Channel, req map[string]json.RawMessage) interface{} {
	var err error
	var rv interface{}
	method := string(req["method"])
	params := req["params"]
	f, ok := onMessageHandlers[method]
	if ok {
		rv, err = f(s, params)
	} else {
		err = errors.New("unknown method")
	}
	if err == nil {
		glog.Info(c.Id(), " onMessage ", method, " success")
		return rv
	}
	glog.Error(c.Id(), " onMessage ", method, ": ", err)
	return ""
}

func unmarshalGetAddressRequest(params []byte) (addr []string, rr reqRange, err error) {
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
	err = json.Unmarshal(p[1], &rr)
	return
}

func (s *SocketIoServer) getAddressTxids(addr []string, rr *reqRange) ([]string, error) {
	txids := make([]string, 0)
	lower, higher := uint32(rr.To), uint32(rr.Start)
	for _, address := range addr {
		script, err := bchain.AddressToOutputScript(address)
		if err != nil {
			return nil, err
		}
		if !rr.QueryMempoolOnly {
			err = s.db.GetTransactions(script, lower, higher, func(txid string, vout uint32, isOutput bool) error {
				txids = append(txids, txid)
				if isOutput && rr.QueryMempol {
					input := s.mempool.GetInput(txid, vout)
					if input != "" {
						txids = append(txids, txid)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		if rr.QueryMempoolOnly || rr.QueryMempol {
			mtxids, err := s.mempool.GetTransactions(script)
			if err != nil {
				return nil, err
			}
			txids = append(txids, mtxids...)
		}
		if err != nil {
			return nil, err
		}
	}
	return txids, nil
}

type addressHistoryIndexes struct {
	InputIndexes  []int `json:"inputIndexes"`
	OutputIndexes []int `json:"outputIndexes"`
}

type txInputs struct {
	PrevTxID    string `json:"prevTxId,omitempty"`
	OutputIndex int    `json:"outputIndex"`
	Script      string `json:"script"`
	ScriptAsm   string `json:"scriptAsm"`
	Sequence    int64  `json:"sequence"`
	Address     string `json:"address"`
	Satoshis    int64  `json:"satoshis"`
}

type txOutputs struct {
	Satoshis    int64  `json:"satoshis"`
	Script      string `json:"script"`
	ScriptAsm   string `json:"scriptAsm"`
	SpentTxID   string `json:"spentTxId,omitempty"`
	SpentIndex  int    `json:"spentIndex,omitempty"`
	SpentHeight int    `json:"spentHeight,omitempty"`
	Address     string `json:"address"`
}

type resTx struct {
	Hex            string      `json:"hex"`
	BlockHash      string      `json:"blockHash,omitempty"`
	Height         int         `json:"height"`
	BlockTimestamp int64       `json:"blockTimestamp"`
	Version        int         `json:"version"`
	Hash           string      `json:"hash"`
	Locktime       int         `json:"locktime"`
	Size           int         `json:"size,omitempty"`
	Inputs         []txInputs  `json:"inputs"`
	InputSatoshis  int64       `json:"inputSatoshis"`
	Outputs        []txOutputs `json:"outputs"`
	OutputSatoshis int64       `json:"outputSatoshis"`
	FeeSatoshis    int64       `json:"feeSatoshis"`
}

type addressHistoryItem struct {
	Addresses     map[string]addressHistoryIndexes `json:"addresses"`
	Satoshis      int                              `json:"satoshis"`
	Confirmations int                              `json:"confirmations"`
	Tx            resTx                            `json:"tx"`
}

type resultGetAddressHistory struct {
	Result struct {
		TotalCount int                  `json:"totalCount"`
		Items      []addressHistoryItem `json:"items"`
	} `json:"result"`
}

func uniqueTxids(txids []string) []string {
	uniqueTxids := make([]string, 0, len(txids))
	txidsMap := make(map[string]struct{})
	for _, txid := range txids {
		_, e := txidsMap[txid]
		if !e {
			uniqueTxids = append(uniqueTxids, txid)
			txidsMap[txid] = struct{}{}
		}
	}
	return uniqueTxids
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func txToResTx(tx *bchain.Tx, height int, hi []txInputs, ho []txOutputs) resTx {
	return resTx{
		// BlockHash:      tx.BlockHash,
		BlockTimestamp: tx.Blocktime,
		// FeeSatoshis,
		Hash:   tx.Txid,
		Height: height,
		Hex:    tx.Hex,
		Inputs: hi,
		// InputSatoshis,
		Locktime: int(tx.LockTime),
		Outputs:  ho,
		// OutputSatoshis,
		Version: int(tx.Version),
	}
}

func (s *SocketIoServer) getAddressHistory(addr []string, rr *reqRange) (res resultGetAddressHistory, err error) {
	txids, err := s.getAddressTxids(addr, rr)
	if err != nil {
		return
	}
	bestheight, _, err := s.db.GetBestBlock()
	if err != nil {
		return
	}
	// todo - proper sorting of txids, probably by height desc
	txids = uniqueTxids(txids)
	res.Result.TotalCount = len(txids)
	res.Result.Items = make([]addressHistoryItem, 0)
	for i, txid := range txids {
		if i >= rr.From && i < rr.To {
			tx, err := s.chain.GetTransaction(txid)
			if err != nil {
				return res, err
			}
			ads := make(map[string]addressHistoryIndexes)
			hi := make([]txInputs, 0)
			ho := make([]txOutputs, 0)
			for _, vin := range tx.Vin {
				ai := txInputs{
					Script:      vin.ScriptSig.Hex,
					ScriptAsm:   vin.ScriptSig.Asm,
					Sequence:    int64(vin.Sequence),
					OutputIndex: int(vin.Vout),
				}
				stxid, _, err := s.db.GetSpentOutput(vin.Txid, vin.Vout)
				if err != nil {
					return res, err
				}
				if stxid != "" {
					otx, err := s.chain.GetTransaction(stxid)
					if err != nil {
						return res, err
					}
				SpentOutputLoop:
					for _, vout := range otx.Vout {
						for _, a := range vout.ScriptPubKey.Addresses {
							if stringInSlice(a, addr) {
								ai.Address = a
								hi, ok := ads[a]
								if ok {
									hi.InputIndexes = append(hi.InputIndexes, int(vout.N))
								} else {
									hi := addressHistoryIndexes{}
									hi.InputIndexes = append(hi.InputIndexes, int(vout.N))
									hi.OutputIndexes = make([]int, 0)
									ads[a] = hi
								}
								break SpentOutputLoop
							}
						}
					}
				}
				hi = append(hi, ai)
			}
			for _, vout := range tx.Vout {
				ao := txOutputs{
					Satoshis:   int64(vout.Value * 10E8),
					Script:     vout.ScriptPubKey.Hex,
					ScriptAsm:  vout.ScriptPubKey.Asm,
					SpentIndex: int(vout.N),
				}
				for _, a := range vout.ScriptPubKey.Addresses {
					if stringInSlice(a, addr) {
						ao.Address = a
						hi, ok := ads[a]
						if ok {
							hi.OutputIndexes = append(hi.OutputIndexes, int(vout.N))
						} else {
							hi := addressHistoryIndexes{}
							hi.InputIndexes = make([]int, 0)
							hi.OutputIndexes = append(hi.OutputIndexes, int(vout.N))
							ads[a] = hi
						}
						break
					}
				}
				ho = append(ho, ao)
			}
			ahi := addressHistoryItem{}
			ahi.Addresses = ads
			ahi.Confirmations = int(tx.Confirmations)
			var height int
			if tx.Confirmations == 0 {
				height = -1
			} else {
				height = int(bestheight) - int(tx.Confirmations)
			}
			ahi.Tx = txToResTx(tx, height, hi, ho)
			res.Result.Items = append(res.Result.Items, ahi)
		}
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
		Hash          string `json:"hash"`
		Version       int    `json:"version"`
		Confirmations int    `json:"confirmations"`
		Height        int    `json:"height"`
		ChainWork     string `json:"chainWork"`
		NextHash      string `json:"nextHash"`
		MerkleRoot    string `json:"merkleRoot"`
		Time          int    `json:"time"`
		MedianTime    int    `json:"medianTime"`
		Nonce         int    `json:"nonce"`
		Bits          string `json:"bits"`
		Difficulty    int    `json:"difficulty"`
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
	Result float64 `json:"result"`
}

func (s *SocketIoServer) estimateSmartFee(blocks int, conservative bool) (res resultEstimateSmartFee, err error) {
	fee, err := s.chain.EstimateSmartFee(blocks, conservative)
	if err != nil {
		return
	}
	res.Result = fee
	return
}

type resultGetInfo struct {
	Result struct {
		Version         int     `json:"version"`
		ProtocolVersion int     `json:"protocolVersion"`
		Blocks          int     `json:"blocks"`
		TimeOffset      int     `json:"timeOffset"`
		Connections     int     `json:"connections"`
		Proxy           string  `json:"proxy"`
		Difficulty      float64 `json:"difficulty"`
		Testnet         bool    `json:"testnet"`
		RelayFee        float64 `json:"relayFee"`
		Errors          string  `json:"errors"`
		Network         string  `json:"network"`
		Subversion      string  `json:"subversion"`
		LocalServices   string  `json:"localServices"`
	} `json:"result"`
}

func (s *SocketIoServer) getInfo() (res resultGetInfo, err error) {
	// trezor is interested only in best block height
	height, _, err := s.db.GetBestBlock()
	if err != nil {
		return
	}
	res.Result.Blocks = int(height)
	return
}

func unmarshalGetDetailedTransaction(params []byte) (hash string, err error) {
	p, err := unmarshalArray(params, 1)
	if err != nil {
		return
	}
	hash, ok := p[0].(string)
	if ok {
		return
	}
	err = errors.New("incorrect parameter")
	return
}

type resultGetDetailedTransaction struct {
	Result resTx `json:"result"`
}

func (s *SocketIoServer) getDetailedTransaction(txid string) (res resultGetDetailedTransaction, err error) {
	bestheight, _, err := s.db.GetBestBlock()
	if err != nil {
		return
	}
	tx, err := s.chain.GetTransaction(txid)
	if err != nil {
		return res, err
	}
	hi := make([]txInputs, 0)
	ho := make([]txOutputs, 0)
	for _, vin := range tx.Vin {
		ai := txInputs{
			Script:      vin.ScriptSig.Hex,
			ScriptAsm:   vin.ScriptSig.Asm,
			Sequence:    int64(vin.Sequence),
			OutputIndex: int(vin.Vout),
		}
		hi = append(hi, ai)
	}
	for _, vout := range tx.Vout {
		ao := txOutputs{
			Satoshis:   int64(vout.Value * 10E8),
			Script:     vout.ScriptPubKey.Hex,
			ScriptAsm:  vout.ScriptPubKey.Asm,
			SpentIndex: int(vout.N),
		}
		ho = append(ho, ao)
	}
	var height int
	if tx.Confirmations == 0 {
		height = -1
	} else {
		height = int(bestheight) - int(tx.Confirmations)
	}
	res.Result = txToResTx(tx, height, hi, ho)
	return
}

func unmarshalSendTransaction(params []byte) (tx string, err error) {
	p, err := unmarshalArray(params, 1)
	if err != nil {
		return
	}
	tx, ok := p[0].(string)
	if ok {
		return
	}
	err = errors.New("incorrect parameter")
	return
}

type resultSendTransaction struct {
	Result string `json:"result"`
}

func (s *SocketIoServer) sendTransaction(tx string) (res resultSendTransaction, err error) {
	txid, err := s.chain.SendRawTransaction(tx)
	if err != nil {
		return res, err
	}
	res.Result = txid
	return
}

func (s *SocketIoServer) onSubscribe(c *gosocketio.Channel, req json.RawMessage) interface{} {
	glog.Info(c.Id(), " onSubscribe ", req)
	return nil
}
