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
}

// NewSocketIoServer creates new SocketIo interface to blockbook and returns its handle
func NewSocketIoServer(binding string, db *db.RocksDB, mempool *bchain.Mempool) (*SocketIoServer, error) {
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
	}

	server.On("message", s.onMessage)

	server.On("send", func(c *gosocketio.Channel, msg Message) string {
		glog.Info(c.Id(), "; ", msg)
		return "OK"
	})

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

func (s *SocketIoServer) onMessage(c *gosocketio.Channel, req map[string]json.RawMessage) string {
	var err error
	var rv []byte
	var rvi interface{}
	method := string(req["method"])
	params := req["params"]
	if method == "\"getAddressTxids\"" {
		addr, rr, err := unmarshalGetAddressTxids(params)
		if err == nil {
			rvi, err = s.getAddressTxids(addr, &rr)
		}
	} else {
		err = errors.New("unknown method")
	}
	if err == nil {
		rv, err = json.Marshal(rvi)
	}
	if err == nil {
		glog.Info(c.Id(), " ", method, " success, returning ", len(rv), " bytes")
		return string(rv)
	}
	glog.Error(c.Id(), " ", method, ": ", err)
	return ""
}

func unmarshalGetAddressTxids(params []byte) (addr []string, rr reqRange, err error) {
	var p []json.RawMessage
	err = json.Unmarshal(params, &p)
	if err != nil {
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
	var txids []string
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
