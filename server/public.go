package server

import (
	"blockbook/api"
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const blockbookAbout = "Blockbook - blockchain indexer for TREZOR wallet https://trezor.io/. Do not use for any other purpose."
const txsOnPage = 25
const txsInAPI = 1000

// PublicServer is a handle to public http server
type PublicServer struct {
	binding     string
	certFiles   string
	socketio    *SocketIoServer
	https       *http.Server
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	api         *api.Worker
	explorerURL string
	metrics     *common.Metrics
	is          *common.InternalState
	templates   []*template.Template
	debug       bool
}

// NewPublicServer creates new public server http interface to blockbook and returns its handle
func NewPublicServer(binding string, certFiles string, db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, explorerURL string, metrics *common.Metrics, is *common.InternalState, debugMode bool) (*PublicServer, error) {

	api, err := api.NewWorker(db, chain, txCache, is)
	if err != nil {
		return nil, err
	}

	socketio, err := NewSocketIoServer(db, chain, txCache, metrics, is)
	if err != nil {
		return nil, err
	}

	addr, path := splitBinding(binding)
	serveMux := http.NewServeMux()
	https := &http.Server{
		Addr:    addr,
		Handler: serveMux,
	}

	s := &PublicServer{
		binding:     binding,
		certFiles:   certFiles,
		https:       https,
		api:         api,
		socketio:    socketio,
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		explorerURL: explorerURL,
		metrics:     metrics,
		is:          is,
		debug:       debugMode,
	}

	// favicon
	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	// support for tests of socket.io interface
	serveMux.Handle(path+"test.html", http.FileServer(http.Dir("./static/")))
	// redirect to wallet requests for tx and address, possibly to external site
	serveMux.HandleFunc(path+"tx/", s.txRedirect)
	serveMux.HandleFunc(path+"address/", s.addressRedirect)
	// explorer
	serveMux.HandleFunc(path+"explorer/tx/", s.htmlTemplateHandler(s.explorerTx))
	serveMux.HandleFunc(path+"explorer/address/", s.htmlTemplateHandler(s.explorerAddress))
	serveMux.HandleFunc(path+"explorer/search/", s.htmlTemplateHandler(s.explorerSearch))
	serveMux.Handle(path+"static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	// API calls
	serveMux.HandleFunc(path+"api/block-index/", s.jsonHandler(s.apiBlockIndex))
	serveMux.HandleFunc(path+"api/tx/", s.jsonHandler(s.apiTx))
	serveMux.HandleFunc(path+"api/address/", s.jsonHandler(s.apiAddress))
	// handle socket.io
	serveMux.Handle(path+"socket.io/", socketio.GetHandler())
	// default handler
	serveMux.HandleFunc(path, s.index)

	s.templates = parseTemplates()

	return s, nil
}

// Run starts the server
func (s *PublicServer) Run() error {
	if s.certFiles == "" {
		glog.Info("public server: starting to listen on http://", s.https.Addr)
		return s.https.ListenAndServe()
	}
	glog.Info("public server starting to listen on https://", s.https.Addr)
	return s.https.ListenAndServeTLS(fmt.Sprint(s.certFiles, ".crt"), fmt.Sprint(s.certFiles, ".key"))
}

// Close closes the server
func (s *PublicServer) Close() error {
	glog.Infof("public server: closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *PublicServer) Shutdown(ctx context.Context) error {
	glog.Infof("public server: shutdown")
	return s.https.Shutdown(ctx)
}

// OnNewBlock notifies users subscribed to bitcoind/hashblock about new block
func (s *PublicServer) OnNewBlock(hash string, height uint32) {
	s.socketio.OnNewBlockHash(hash)
}

// OnNewTxAddr notifies users subscribed to bitcoind/addresstxid about new block
func (s *PublicServer) OnNewTxAddr(txid string, addr string, isOutput bool) {
	s.socketio.OnNewTxAddr(txid, addr, isOutput)
}

func (s *PublicServer) txRedirect(w http.ResponseWriter, r *http.Request) {
	if s.explorerURL != "" {
		http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), 302)
		s.metrics.ExplorerViews.With(common.Labels{"action": "tx-redirect"}).Inc()
	}
}

func (s *PublicServer) addressRedirect(w http.ResponseWriter, r *http.Request) {
	if s.explorerURL != "" {
		http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), 302)
		s.metrics.ExplorerViews.With(common.Labels{"action": "address-redirect"}).Inc()
	}
}

func splitBinding(binding string) (addr string, path string) {
	i := strings.Index(binding, "/")
	if i >= 0 {
		return binding[0:i], binding[i:]
	}
	return binding, "/"
}

func joinURL(base string, part string) string {
	if len(base) > 0 {
		if len(base) > 0 && base[len(base)-1] == '/' && len(part) > 0 && part[0] == '/' {
			return base + part[1:]
		}
		return base + part
	}
	return part
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (s *PublicServer) jsonHandler(handler func(r *http.Request) (interface{}, error)) func(w http.ResponseWriter, r *http.Request) {
	type jsonError struct {
		Error string `json:"error"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var data interface{}
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(getFunctionName(handler), " recovered from panic: ", e)
				if s.debug {
					data = jsonError{fmt.Sprint("Internal server error: recovered from panic ", e)}
				} else {
					data = jsonError{"Internal server error"}
				}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(data)
		}()
		data, err = handler(r)
		if err != nil || data == nil {
			if apiErr, ok := err.(*api.ApiError); ok {
				data = jsonError{apiErr.Error()}
			} else {
				if err != nil {
					glog.Error(getFunctionName(handler), " error: ", err)
				}
				if s.debug {
					data = jsonError{fmt.Sprintf("Internal server error: %v, data %+v", err, data)}
				} else {
					data = jsonError{"Internal server error"}
				}
			}
		}
	}
}

func (s *PublicServer) newTemplateData() *TemplateData {
	return &TemplateData{
		CoinName:     s.is.Coin,
		CoinShortcut: s.is.CoinShortcut,
	}
}

func (s *PublicServer) newTemplateDataWithError(text string) *TemplateData {
	return &TemplateData{
		CoinName:     s.is.Coin,
		CoinShortcut: s.is.CoinShortcut,
		Error:        &api.ApiError{Text: text},
	}
}
func (s *PublicServer) htmlTemplateHandler(handler func(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var t tpl
		var data *TemplateData
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(getFunctionName(handler), " recovered from panic: ", e)
				t = errorTpl
				if s.debug {
					data = s.newTemplateDataWithError(fmt.Sprint("Internal server error: recovered from panic ", e))
				} else {
					data = s.newTemplateDataWithError("Internal server error")
				}
			}
			// noTpl means the handler completely handled the request
			if t != noTpl {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if err := s.templates[t].ExecuteTemplate(w, "base.html", data); err != nil {
					glog.Error(err)
				}
			}
		}()
		if s.debug {
			// reload templates on each request
			// to reflect changes during development
			s.templates = parseTemplates()
		}
		t, data, err = handler(w, r)
		if err != nil || (data == nil && t != noTpl) {
			t = errorTpl
			if apiErr, ok := err.(*api.ApiError); ok {
				data = s.newTemplateData()
				data.Error = apiErr
			} else {
				if err != nil {
					glog.Error(getFunctionName(handler), " error: ", err)
				}
				if s.debug {
					data = s.newTemplateDataWithError(fmt.Sprintf("Internal server error: %v, data %+v", err, data))
				} else {
					data = s.newTemplateDataWithError("Internal server error")
				}
			}
		}
	}
}

type tpl int

const (
	noTpl = tpl(iota)
	errorTpl
	txTpl
	addressTpl

	tplCount
)

type TemplateData struct {
	CoinName     string
	CoinShortcut string
	Address      *api.Address
	AddrStr      string
	Tx           *api.Tx
	Error        *api.ApiError
	Page         int
	PrevPage     int
	NextPage     int
	PagingRange  []int
}

func parseTemplates() []*template.Template {
	templateFuncMap := template.FuncMap{
		"formatUnixTime":      formatUnixTime,
		"formatAmount":        formatAmount,
		"setTxToTemplateData": setTxToTemplateData,
		"stringInSlice":       stringInSlice,
	}
	t := make([]*template.Template, tplCount)
	t[errorTpl] = template.Must(template.New("error").Funcs(templateFuncMap).ParseFiles("./static/templates/error.html", "./static/templates/base.html"))
	t[txTpl] = template.Must(template.New("tx").Funcs(templateFuncMap).ParseFiles("./static/templates/tx.html", "./static/templates/txdetail.html", "./static/templates/base.html"))
	t[addressTpl] = template.Must(template.New("address").Funcs(templateFuncMap).ParseFiles("./static/templates/address.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html"))
	return t
}

func formatUnixTime(ut int64) string {
	return time.Unix(ut, 0).Format(time.RFC1123)
}

// for now return the string as it is
// in future could be used to do coin specific formatting
func formatAmount(a string) string {
	return a
}

// called from template to support txdetail.html functionality
func setTxToTemplateData(td *TemplateData, tx *api.Tx) *TemplateData {
	td.Tx = tx
	return td
}

func (s *PublicServer) explorerTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var tx *api.Tx
	s.metrics.ExplorerViews.With(common.Labels{"action": "tx"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		bestheight, _, err := s.db.GetBestBlock()
		if err == nil {
			tx, err = s.api.GetTransaction(txid, bestheight, true)
		}
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData()
	data.Tx = tx
	return txTpl, data, nil
}

func (s *PublicServer) explorerAddress(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var address *api.Address
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "address"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		page, ec := strconv.Atoi(r.URL.Query().Get("page"))
		if ec != nil {
			page = 0
		}
		address, err = s.api.GetAddress(r.URL.Path[i+1:], page, txsOnPage, false)
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData()
	data.AddrStr = address.AddrStr
	data.Address = address
	data.Page = address.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(address.Page, address.TotalPages)
	return addressTpl, data, nil
}

func (s *PublicServer) explorerSearch(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var tx *api.Tx
	var address *api.Address
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "search"}).Inc()
	if len(q) > 0 {
		if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
			bestheight, _, err := s.db.GetBestBlock()
			if err == nil {
				tx, err = s.api.GetTransaction(q, bestheight, false)
				if err == nil {
					http.Redirect(w, r, joinURL("/explorer/tx/", tx.Txid), 302)
					return noTpl, nil, nil
				}
			}
			address, err = s.api.GetAddress(q, 0, 1, true)
			if err == nil {
				http.Redirect(w, r, joinURL("/explorer/address/", address.AddrStr), 302)
				return noTpl, nil, nil
			}
		}
	}
	if err == nil {
		err = api.NewApiError(fmt.Sprintf("No matching records found for '%v'", q), true)
	}
	return errorTpl, nil, err
}

func getPagingRange(page int, total int) ([]int, int, int) {
	if total < 2 {
		return nil, 0, 0
	}
	pp, np := page-1, page+1
	if np > total {
		np = total
	}
	if pp < 1 {
		pp = 1
	}
	r := make([]int, 0, 8)
	if total < 6 {
		for i := 1; i <= total; i++ {
			r = append(r, i)
		}
	} else {
		r = append(r, 1)
		if page > 3 {
			r = append(r, 0)
		}
		if pp == 1 {
			if page == 1 {
				r = append(r, np)
				r = append(r, np+1)
				r = append(r, np+2)
			} else {
				r = append(r, page)
				r = append(r, np)
				r = append(r, np+1)
			}
		} else if np == total {
			if page == total {
				r = append(r, pp-2)
				r = append(r, pp-1)
				r = append(r, pp)
			} else {
				r = append(r, pp-1)
				r = append(r, pp)
				r = append(r, page)
			}
		} else {
			r = append(r, pp)
			r = append(r, page)
			r = append(r, np)
		}
		if page <= total-3 {
			r = append(r, 0)
		}
		r = append(r, total)
	}
	return r, pp, np
}

type resAboutBlockbookPublic struct {
	Coin            string    `json:"coin"`
	Host            string    `json:"host"`
	Version         string    `json:"version"`
	GitCommit       string    `json:"gitcommit"`
	BuildTime       string    `json:"buildtime"`
	InSync          bool      `json:"inSync"`
	BestHeight      uint32    `json:"bestHeight"`
	LastBlockTime   time.Time `json:"lastBlockTime"`
	InSyncMempool   bool      `json:"inSyncMempool"`
	LastMempoolTime time.Time `json:"lastMempoolTime"`
	About           string    `json:"about"`
}

// TODO - this is temporary, return html status page
func (s *PublicServer) index(w http.ResponseWriter, r *http.Request) {
	vi := common.GetVersionInfo()
	ss, bh, st := s.is.GetSyncState()
	ms, mt, _ := s.is.GetMempoolSyncState()
	a := resAboutBlockbookPublic{
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
		About:           blockbookAbout,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	buf, err := json.MarshalIndent(a, "", "    ")
	if err != nil {
		glog.Error(err)
	}
	w.Write(buf)
}

func (s *PublicServer) apiBlockIndex(r *http.Request) (interface{}, error) {
	type resBlockIndex struct {
		BlockHash string `json:"blockHash"`
		About     string `json:"about"`
	}
	var err error
	var hash string
	height := -1
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		if h, err := strconv.Atoi(r.URL.Path[i+1:]); err == nil {
			height = h
		}
	}
	if height >= 0 {
		hash, err = s.db.GetBlockHash(uint32(height))
	} else {
		_, hash, err = s.db.GetBestBlock()
	}
	if err != nil {
		glog.Error(err)
		return nil, err
	}
	return resBlockIndex{
		BlockHash: hash,
		About:     blockbookAbout,
	}, nil
}

func (s *PublicServer) apiTx(r *http.Request) (interface{}, error) {
	var tx *api.Tx
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tx"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		bestheight, _, err := s.db.GetBestBlock()
		if err == nil {
			tx, err = s.api.GetTransaction(txid, bestheight, true)
		}
	}
	return tx, err
}

func (s *PublicServer) apiAddress(r *http.Request) (interface{}, error) {
	var address *api.Address
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-address"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		page, ec := strconv.Atoi(r.URL.Query().Get("page"))
		if ec != nil {
			page = 0
		}
		address, err = s.api.GetAddress(r.URL.Path[i+1:], page, txsInAPI, true)
	}
	return address, err
}
