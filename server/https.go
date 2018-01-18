package server

import (
	"blockbook/db"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

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

	var h http.Handler = r
	h = handlers.LoggingHandler(os.Stdout, h)
	https.Handler = h

	return s, nil
}

// Run starts the server
func (s *HttpServer) Run() error {
	log.Printf("http server starting to listen on %s", s.https.Addr)
	return s.https.ListenAndServe()
}

// Close closes the server
func (s *HttpServer) Close() error {
	log.Printf("http server closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *HttpServer) Shutdown(ctx context.Context) error {
	log.Printf("http server shutdown")
	return s.https.Shutdown(ctx)
}

func respondError(w http.ResponseWriter, err error, context string) {
	w.WriteHeader(http.StatusBadRequest)
	log.Printf("http server %s error: %v", context, err)
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
		Version string `json:"version"`
	}
	json.NewEncoder(w).Encode(info{
		Version: "0.0.1",
	})
}

func (s *HttpServer) bestBlockHash(w http.ResponseWriter, r *http.Request) {
	hash, err := s.db.GetBestBlockHash()
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
