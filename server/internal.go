package server

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InternalServer is handle to internal http server
type InternalServer struct {
	https       *http.Server
	certFiles   string
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	is          *common.InternalState
}

type resAboutBlockbookInternal struct {
	Coin            string                       `json:"coin"`
	Host            string                       `json:"host"`
	Version         string                       `json:"version"`
	GitCommit       string                       `json:"gitcommit"`
	BuildTime       string                       `json:"buildtime"`
	InSync          bool                         `json:"inSync"`
	BestHeight      uint32                       `json:"bestHeight"`
	LastBlockTime   time.Time                    `json:"lastBlockTime"`
	InSyncMempool   bool                         `json:"inSyncMempool"`
	LastMempoolTime time.Time                    `json:"lastMempoolTime"`
	MempoolSize     int                          `json:"mempoolSize"`
	DbColumns       []common.InternalStateColumn `json:"dbColumns"`
}

// NewInternalServer creates new internal http interface to blockbook and returns its handle
func NewInternalServer(httpServerBinding string, certFiles string, db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, is *common.InternalState) (*InternalServer, error) {
	r := mux.NewRouter()
	https := &http.Server{
		Addr:    httpServerBinding,
		Handler: r,
	}
	s := &InternalServer{
		https:       https,
		certFiles:   certFiles,
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		is:          is,
	}

	r.HandleFunc("/", s.index)
	r.HandleFunc("/bestBlockHash", s.bestBlockHash)
	r.HandleFunc("/blockHash/{height}", s.blockHash)
	r.HandleFunc("/transactions/{address}/{lower}/{higher}", s.transactions)
	r.HandleFunc("/confirmedTransactions/{address}/{lower}/{higher}", s.confirmedTransactions)
	r.HandleFunc("/unconfirmedTransactions/{address}", s.unconfirmedTransactions)
	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)

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

func respondError(w http.ResponseWriter, err error, context string) {
	w.WriteHeader(http.StatusBadRequest)
	glog.Errorf("internal server: (context %s) error: %v", context, err)
}

func respondHashData(w http.ResponseWriter, hash string) {
	type hashData struct {
		Hash string `json:"hash"`
	}
	json.NewEncoder(w).Encode(hashData{
		Hash: hash,
	})
}

func (s *InternalServer) index(w http.ResponseWriter, r *http.Request) {
	vi := common.GetVersionInfo()
	ss, bh, st := s.is.GetSyncState()
	ms, mt, msz := s.is.GetMempoolSyncState()
	a := resAboutBlockbookInternal{
		Coin:            s.is.Coin,
		Host:            s.is.Host,
		Version:         vi.Version,
		GitCommit:       vi.GitCommit,
		BuildTime:       vi.BuildTime,
		InSync:          ss,
		BestHeight:      bh,
		LastBlockTime:   st,
		InSyncMempool:   ms,
		LastMempoolTime: mt,
		MempoolSize:     msz,
		DbColumns:       s.is.GetAllDBColumnStats(),
	}
	buf, err := json.MarshalIndent(a, "", "    ")
	if err != nil {
		glog.Error(err)
	}
	w.Write(buf)
}

func (s *InternalServer) bestBlockHash(w http.ResponseWriter, r *http.Request) {
	_, hash, err := s.db.GetBestBlock()
	if err != nil {
		respondError(w, err, "bestBlockHash")
		return
	}
	respondHashData(w, hash)
}

func (s *InternalServer) blockHash(w http.ResponseWriter, r *http.Request) {
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

func (s *InternalServer) getAddress(r *http.Request) (address string, err error) {
	address, ok := mux.Vars(r)["address"]
	if !ok {
		err = errors.New("Empty address")
	}
	return
}

func (s *InternalServer) getAddressAndHeightRange(r *http.Request) (address string, lower, higher uint32, err error) {
	address, err = s.getAddress(r)
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
	return address, uint32(lower64), uint32(higher64), err
}

type transactionList struct {
	Txid []string `json:"txid"`
}

func (s *InternalServer) unconfirmedTransactions(w http.ResponseWriter, r *http.Request) {
	address, err := s.getAddress(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("unconfirmedTransactions for address", address))
	}
	txs, err := s.chain.GetMempoolTransactions(address)
	if err != nil {
		respondError(w, err, fmt.Sprint("unconfirmedTransactions for address", address))
	}
	txList := transactionList{Txid: txs}
	json.NewEncoder(w).Encode(txList)
}

func (s *InternalServer) confirmedTransactions(w http.ResponseWriter, r *http.Request) {
	address, lower, higher, err := s.getAddressAndHeightRange(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("confirmedTransactions for address", address))
	}
	txList := transactionList{}
	err = s.db.GetTransactions(address, lower, higher, func(txid string, vout uint32, isOutput bool) error {
		txList.Txid = append(txList.Txid, txid)
		return nil
	})
	if err != nil {
		respondError(w, err, fmt.Sprint("confirmedTransactions for address", address))
	}
	json.NewEncoder(w).Encode(txList)
}

func (s *InternalServer) transactions(w http.ResponseWriter, r *http.Request) {
	address, lower, higher, err := s.getAddressAndHeightRange(r)
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txList := transactionList{}
	err = s.db.GetTransactions(address, lower, higher, func(txid string, vout uint32, isOutput bool) error {
		txList.Txid = append(txList.Txid, txid)
		return nil
	})
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txs, err := s.chain.GetMempoolTransactions(address)
	if err != nil {
		respondError(w, err, fmt.Sprint("transactions for address", address))
	}
	txList.Txid = append(txList.Txid, txs...)
	json.NewEncoder(w).Encode(txList)
}
