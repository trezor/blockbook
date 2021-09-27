package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// InternalServer is handle to internal http server
type InternalServer struct {
	https       *http.Server
	certFiles   string
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	mempool     bchain.Mempool
	is          *common.InternalState
	api         *api.Worker
}

// NewInternalServer creates new internal http interface to blockbook and returns its handle
func NewInternalServer(binding, certFiles string, db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState) (*InternalServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is)
	if err != nil {
		return nil, err
	}

	addr, path := splitBinding(binding)
	serveMux := http.NewServeMux()
	https := &http.Server{
		Addr:    addr,
		Handler: serveMux,
	}
	s := &InternalServer{
		https:       https,
		certFiles:   certFiles,
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		mempool:     mempool,
		is:          is,
		api:         api,
	}

	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	serveMux.HandleFunc(path+"metrics", promhttp.Handler().ServeHTTP)
	serveMux.HandleFunc(path, s.index)

	return s, nil
}

// Run starts the server
func (s *InternalServer) Run() error {
	if s.certFiles == "" {
		glog.Info("internal server: starting to listen on http://", s.https.Addr)
		return s.https.ListenAndServe()
	}
	glog.Info("internal server: starting to listen on https://", s.https.Addr)
	return s.https.ListenAndServeTLS(fmt.Sprint(s.certFiles, ".crt"), fmt.Sprint(s.certFiles, ".key"))
}

// Close closes the server
func (s *InternalServer) Close() error {
	glog.Infof("internal server: closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *InternalServer) Shutdown(ctx context.Context) error {
	glog.Infof("internal server: shutdown")
	return s.https.Shutdown(ctx)
}

func (s *InternalServer) index(w http.ResponseWriter, r *http.Request) {
	si, err := s.api.GetSystemInfo(true)
	if err != nil {
		glog.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	buf, err := json.MarshalIndent(si, "", "    ")
	if err != nil {
		glog.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(buf)
}
