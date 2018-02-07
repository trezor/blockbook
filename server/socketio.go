package server

import (
	"blockbook/bchain"
	"blockbook/db"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
)

// SocketIoServer is handle to SocketIoServer
type SocketIoServer struct {
	binding string
	server  *gosocketio.Server
	https   *http.Server
	db      *db.RocksDB
	mempool *bchain.Mempool
	chain   *bchain.BitcoinRPC
}

// NewSocketIoServer creates new SocketIo interface to blockbook and returns its handle
func NewSocketIoServer(binding string, db *db.RocksDB, mempool *bchain.Mempool, chain *bchain.BitcoinRPC) (*SocketIoServer, error) {
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
		binding: binding,
		https:   https,
		server:  server,
		db:      db,
		mempool: mempool,
		chain:   chain,
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
	glog.Info("socketio server starting to listen on ", s.https.Addr)
	return s.https.ListenAndServe()
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
		addr, rr, err := unmarshalGetAddressTxids(params)
		if err == nil {
			rv, err = s.getAddressTxids(addr, &rr)
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

func unmarshalGetAddressTxids(params []byte) (addr []string, rr reqRange, err error) {
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
		if height == 0 {
			height, hash, err = s.db.GetBestBlock()
			if err != nil {
				return
			}
		} else {
			hash, err = s.db.GetBlockHash(height)
			if err != nil {
				return
			}
		}
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

func (s *SocketIoServer) onSubscribe(c *gosocketio.Channel, req map[string]json.RawMessage) interface{} {
	glog.Info(c.Id(), " onSubscribe ", req)
	return nil
}
