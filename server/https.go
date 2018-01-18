package server

import (
	"blockbook/db"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type server struct {
	https *http.Server
	db    *db.RocksDB
}

func New(db *db.RocksDB) (*server, error) {
	https := &http.Server{
		Addr: ":8333",
	}
	s := &server{
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

func (s *server) Run() error {
	fmt.Printf("http server starting")
	return s.https.ListenAndServe()
}

func (s *server) Close() error {
	fmt.Printf("http server closing")
	return s.https.Close()
}

func (s *server) Shutdown() error {
	fmt.Printf("http server shutdown")
	return s.https.Shutdown()
}

func (s *server) Info(w http.ResponseWriter, r *http.Request) {
	type info struct {
		Version string `json:"version"`
	}
	json.NewEncoder(w).Encode(info{
		Version: "0.0.1",
	})
}
