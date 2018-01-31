package server

import (
	"blockbook/bchain"
	"blockbook/db"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/golang/glog"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type HttpServer struct {
	https *http.Server
	db    *db.RocksDB
}

func New(httpServerBinding string, db *db.RocksDB) (*HttpServer, error) {
	https := &http.Server{
		Addr: httpServerBinding,
	}
	s := &HttpServer{
		https: https,
		db:    db,
	}

	r := mux.NewRouter()

	r.HandleFunc("/", s.info)
	r.HandleFunc("/bestBlockHash", s.bestBlockHash)
	r.HandleFunc("/blockHash/{height}", s.blockHash)
	r.HandleFunc("/transactions/{address}/{lower}/{higher}", s.transactions)

	var h http.Handler = r
	h = handlers.LoggingHandler(os.Stderr, h)
	https.Handler = h

	return s, nil
}

// Run starts the server
func (s *HttpServer) Run() error {
	glog.Infof("http server starting to listen on %s", s.https.Addr)
	return s.https.ListenAndServe()
}

// Close closes the server
func (s *HttpServer) Close() error {
	glog.Infof("http server closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *HttpServer) Shutdown(ctx context.Context) error {
	glog.Infof("http server shutdown")
	return s.https.Shutdown(ctx)
}

func respondError(w http.ResponseWriter, err error, context string) {
	w.WriteHeader(http.StatusBadRequest)
	glog.Errorf("http server %s error: %v", context, err)
}

func respondHashData(w http.ResponseWriter, hash string) {
	type hashData struct {
		Hash string `json:"hash"`
	}
	json.NewEncoder(w).Encode(hashData{
		Hash: hash,
	})
}

func (s *HttpServer) info(w http.ResponseWriter, r *http.Request) {
	type info struct {
		Version         string `json:"version"`
		BestBlockHeight uint32 `json:"bestBlockHeight"`
		BestBlockHash   string `json:"bestBlockHash"`
	}

	height, hash, err := s.db.GetBestBlock()
	if err != nil {
		glog.Errorf("https info: %v", err)
	}

	json.NewEncoder(w).Encode(info{
		Version:         "0.0.1",
		BestBlockHeight: height,
		BestBlockHash:   hash,
	})
}

func (s *HttpServer) bestBlockHash(w http.ResponseWriter, r *http.Request) {
	_, hash, err := s.db.GetBestBlock()
	if err != nil {
		respondError(w, err, "bestBlockHash")
		return
	}
	respondHashData(w, hash)
}

func (s *HttpServer) blockHash(w http.ResponseWriter, r *http.Request) {
	heightString := mux.Vars(r)["height"]
	var hash string
	height, err := strconv.ParseUint(heightString, 10, 32)
	if err == nil {
		hash, err = s.db.GetBlockHash(uint32(height))
	}
	if err != nil {
		respondError(w, err, fmt.Sprintf("blockHash %s", heightString))
	} else {
		respondHashData(w, hash)
	}
}

func (s *HttpServer) transactions(w http.ResponseWriter, r *http.Request) {
	type transactionList struct {
		Txid []string `json:"txid"`
	}
	address := mux.Vars(r)["address"]
	higher, err := strconv.ParseUint(mux.Vars(r)["higher"], 10, 32)
	if err != nil {
		respondError(w, err, fmt.Sprintf("address %s", address))
	}
	lower, err := strconv.ParseUint(mux.Vars(r)["lower"], 10, 32)
	if err != nil {
		respondError(w, err, fmt.Sprintf("address %s", address))
	}
	script, err := bchain.AddressToOutputScript(address)
	if err != nil {
		respondError(w, err, fmt.Sprintf("address %s", address))
	}
	txList := transactionList{}
	err = s.db.GetTransactions(script, uint32(lower), uint32(higher), func(txid string) error {
		txList.Txid = append(txList.Txid, txid)
		return nil
	})
	if err != nil {
		respondError(w, err, fmt.Sprintf("address %s", address))
	}
	json.NewEncoder(w).Encode(txList)

}
