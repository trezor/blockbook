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

// HTTPServer is handle to HttpServer
type HTTPServer struct {
	https       *http.Server
	certFiles   string
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
}

// NewHTTPServer creates new REST interface to blockbook and returns its handle
func NewHTTPServer(httpServerBinding string, certFiles string, db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache) (*HTTPServer, error) {
	https := &http.Server{
		Addr: httpServerBinding,
	}
	s := &HTTPServer{
		https:       https,
		certFiles:   certFiles,
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
	}

	r := mux.NewRouter()
	r.HandleFunc("/", s.info)
	r.HandleFunc("/bestBlockHash", s.bestBlockHash)
	r.HandleFunc("/blockHash/{height}", s.blockHash)
	r.HandleFunc("/transactions/{address}/{lower}/{higher}", s.transactions)
	r.HandleFunc("/confirmedTransactions/{address}/{lower}/{higher}", s.confirmedTransactions)
	r.HandleFunc("/unconfirmedTransactions/{address}", s.unconfirmedTransactions)

	var h http.Handler = r
	h = handlers.LoggingHandler(os.Stderr, h)
	https.Handler = h

	return s, nil
}

// Run starts the server
func (s *HTTPServer) Run() error {
	if s.certFiles == "" {
		glog.Info("http server starting to listen on http://", s.https.Addr)
		return s.https.ListenAndServe()
	}
	glog.Info("http server starting to listen on https://", s.https.Addr)
	return s.https.ListenAndServeTLS(fmt.Sprint(s.certFiles, ".crt"), fmt.Sprint(s.certFiles, ".key"))
}

// Close closes the server
func (s *HTTPServer) Close() error {
	glog.Infof("http server closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	glog.Infof("http server shutdown")
	return s.https.Shutdown(ctx)
}

func respondError(w http.ResponseWriter, err error, context string) {
	w.WriteHeader(http.StatusBadRequest)
	glog.Errorf("http server (context %s) error: %v", context, err)
}

func respondHashData(w http.ResponseWriter, hash string) {
	type hashData struct {
		Hash string `json:"hash"`
	}
	json.NewEncoder(w).Encode(hashData{
		Hash: hash,
	})
}

func (s *HTTPServer) info(w http.ResponseWriter, r *http.Request) {
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

func (s *HTTPServer) bestBlockHash(w http.ResponseWriter, r *http.Request) {
	_, hash, err := s.db.GetBestBlock()
	if err != nil {
		respondError(w, err, "bestBlockHash")
		return
	}
	respondHashData(w, hash)
}

func (s *HTTPServer) blockHash(w http.ResponseWriter, r *http.Request) {
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

func (s *HTTPServer) getAddress(r *http.Request) (address string, script []byte, err error) {
	address = mux.Vars(r)["address"]
	script, err = s.chainParser.AddressToOutputScript(address)
	return
}

func (s *HTTPServer) getAddressAndHeightRange(r *http.Request) (address string, script []byte, lower, higher uint32, err error) {
	address, script, err = s.getAddress(r)
	if err != nil {
		return
	}
	higher64, err := strconv.ParseUint(mux.Vars(r)["higher"], 10, 32)
	if err != nil {
		return
	}
	lower64, err := strconv.ParseUint(mux.Vars(r)["lower"], 10, 32)
	if err != nil {
		return
	}
	return address, script, uint32(lower64), uint32(higher64), err
}

type transactionList struct {
	Txid []string `json:"txid"`
}

func (s *HTTPServer) unconfirmedTransactions(w http.ResponseWriter, r *http.Request) {
	address, script, err := s.getAddress(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("unconfirmedTransactions for address", address))
	}
	txs, err := s.chain.GetMempoolTransactions(script)
	if err != nil {
		respondError(w, err, fmt.Sprint("unconfirmedTransactions for address", address))
	}
	txList := transactionList{Txid: txs}
	json.NewEncoder(w).Encode(txList)
}

func (s *HTTPServer) confirmedTransactions(w http.ResponseWriter, r *http.Request) {
	address, script, lower, higher, err := s.getAddressAndHeightRange(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("confirmedTransactions for address", address))
	}
	txList := transactionList{}
	err = s.db.GetTransactions(script, lower, higher, func(txid string, vout uint32, isOutput bool) error {
		txList.Txid = append(txList.Txid, txid)
		return nil
	})
	if err != nil {
		respondError(w, err, fmt.Sprint("confirmedTransactions for address", address))
	}
	json.NewEncoder(w).Encode(txList)
}

func (s *HTTPServer) transactions(w http.ResponseWriter, r *http.Request) {
	address, script, lower, higher, err := s.getAddressAndHeightRange(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txList := transactionList{}
	err = s.db.GetTransactions(script, lower, higher, func(txid string, vout uint32, isOutput bool) error {
		txList.Txid = append(txList.Txid, txid)
		if isOutput {
			input := s.chain.GetMempoolSpentOutput(txid, vout)
			if input != "" {
				txList.Txid = append(txList.Txid, txid)
			}
		}
		return nil
	})
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txs, err := s.chain.GetMempoolTransactions(script)
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txList.Txid = append(txList.Txid, txs...)
	json.NewEncoder(w).Encode(txList)
}
