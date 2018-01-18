package server

import (
	"blockbook/db"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type HttpServer struct {
	https *http.Server
	db    *db.RocksDB
}

func New(db *db.RocksDB) (*HttpServer, error) {
	https := &http.Server{
		Addr: ":8333",
	}
	s := &HttpServer{
		https: https,
		db:    db,
	}

	r := mux.NewRouter()

	r.HandleFunc("/", s.Info)

	var h http.Handler = r
	h = handlers.LoggingHandler(os.Stdout, h)
	https.Handler = h

	return s, nil
}

func (s *HttpServer) Run() error {
	fmt.Printf("http server starting on port %s", s.https.Addr)
	return s.https.ListenAndServe()
}

func (s *HttpServer) Close() error {
	fmt.Printf("http server closing")
	return s.https.Close()
}

func (s *HttpServer) Shutdown(ctx context.Context) error {
	fmt.Printf("http server shutdown")
	return s.https.Shutdown(ctx)
}

func (s *HttpServer) Info(w http.ResponseWriter, r *http.Request) {
	type info struct {
		Version string `json:"version"`
	}
	json.NewEncoder(w).Encode(info{
		Version: "0.0.1",
	})
}
