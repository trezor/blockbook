package server

import (
	"blockbook/bchain"
	"blockbook/db"
	"context"
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
		c.Join("chat")
	})

	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		glog.Info("Client disconnected ", c.Id())
	})

	type Message struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	server.On("send", func(c *gosocketio.Channel, msg Message) string {
		c.BroadcastTo("chat", "message", msg)
		return "OK"
	})

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
