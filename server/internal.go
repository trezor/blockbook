package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
)

// InternalServer is handle to internal http server
type InternalServer struct {
	htmlTemplates[InternalTemplateData]
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
func NewInternalServer(binding, certFiles string, db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState, fiatRates *fiat.FiatRates) (*InternalServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
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
		htmlTemplates: htmlTemplates[InternalTemplateData]{
			debug: true,
		},
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
	s.htmlTemplates.newTemplateData = s.newTemplateData
	s.htmlTemplates.newTemplateDataWithError = s.newTemplateDataWithError
	s.htmlTemplates.parseTemplates = s.parseTemplates
	s.templates = s.parseTemplates()

	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	serveMux.HandleFunc(path+"metrics", promhttp.Handler().ServeHTTP)
	serveMux.HandleFunc(path, s.index)
	serveMux.HandleFunc(path+"admin", s.htmlTemplateHandler(s.adminIndex))
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		serveMux.HandleFunc(path+"admin/internal-data-errors", s.htmlTemplateHandler(s.internalDataErrors))
	}
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

const (
	adminIndexTpl = iota + errorInternalTpl + 1
	adminInternalErrorsTpl

	internalTplCount
)

// InternalTemplateData is used to transfer data to the templates
type InternalTemplateData struct {
	CoinName               string
	CoinShortcut           string
	CoinLabel              string
	ChainType              bchain.ChainType
	Error                  *api.APIError
	InternalDataErrors     []db.BlockInternalDataError
	RefetchingInternalData bool
}

func (s *InternalServer) newTemplateData(r *http.Request) *InternalTemplateData {
	t := &InternalTemplateData{
		CoinName:     s.is.Coin,
		CoinShortcut: s.is.CoinShortcut,
		CoinLabel:    s.is.CoinLabel,
		ChainType:    s.chainParser.GetChainType(),
	}
	return t
}

func (s *InternalServer) newTemplateDataWithError(error *api.APIError, r *http.Request) *InternalTemplateData {
	td := s.newTemplateData(r)
	td.Error = error
	return td
}

func (s *InternalServer) parseTemplates() []*template.Template {
	templateFuncMap := template.FuncMap{
		"formatUint32": formatUint32,
	}
	createTemplate := func(filenames ...string) *template.Template {
		if len(filenames) == 0 {
			panic("Missing templates")
		}
		return template.Must(template.New(filepath.Base(filenames[0])).Funcs(templateFuncMap).ParseFiles(filenames...))
	}
	t := make([]*template.Template, internalTplCount)
	t[errorTpl] = createTemplate("./static/internal_templates/error.html", "./static/internal_templates/base.html")
	t[errorInternalTpl] = createTemplate("./static/internal_templates/error.html", "./static/internal_templates/base.html")
	t[adminIndexTpl] = createTemplate("./static/internal_templates/index.html", "./static/internal_templates/base.html")
	t[adminInternalErrorsTpl] = createTemplate("./static/internal_templates/block_internal_data_errors.html", "./static/internal_templates/base.html")
	return t
}

func (s *InternalServer) adminIndex(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	data := s.newTemplateData(r)
	return adminIndexTpl, data, nil
}

func (s *InternalServer) internalDataErrors(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	if r.Method == http.MethodPost {
		err := s.api.RefetchInternalData()
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData(r)
	internalErrors, err := s.db.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		return errorTpl, nil, err
	}
	data.InternalDataErrors = internalErrors
	data.RefetchingInternalData = s.api.IsRefetchingInternalData()
	return adminInternalErrorsTpl, data, nil
}
