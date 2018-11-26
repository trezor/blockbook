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
	"io/ioutil"
	"math/big"
	"net/http"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const txsOnPage = 25
const blocksOnPage = 50
const txsInAPI = 1000

// PublicServer is a handle to public http server
type PublicServer struct {
	binding          string
	certFiles        string
	socketio         *SocketIoServer
	https            *http.Server
	db               *db.RocksDB
	txCache          *db.TxCache
	chain            bchain.BlockChain
	chainParser      bchain.BlockChainParser
	api              *api.Worker
	explorerURL      string
	internalExplorer bool
	metrics          *common.Metrics
	is               *common.InternalState
	templates        []*template.Template
	debug            bool
}

// NewPublicServer creates new public server http interface to blockbook and returns its handle
// only basic functionality is mapped, to map all functions, call
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
		binding:          binding,
		certFiles:        certFiles,
		https:            https,
		api:              api,
		socketio:         socketio,
		db:               db,
		txCache:          txCache,
		chain:            chain,
		chainParser:      chain.GetChainParser(),
		explorerURL:      explorerURL,
		internalExplorer: explorerURL == "",
		metrics:          metrics,
		is:               is,
		debug:            debugMode,
	}
	s.templates = parseTemplates()

	// map only basic functions, the rest is enabled by method MapFullPublicInterface
	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	serveMux.Handle(path+"static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	// default handler
	serveMux.HandleFunc(path, s.htmlTemplateHandler(s.explorerIndex))
	// default API handler
	serveMux.HandleFunc(path+"api/", s.jsonHandler(s.apiIndex))

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

// ConnectFullPublicInterface enables complete public functionality
func (s *PublicServer) ConnectFullPublicInterface() {
	serveMux := s.https.Handler.(*http.ServeMux)
	_, path := splitBinding(s.binding)
	// support for tests of socket.io interface
	serveMux.Handle(path+"test.html", http.FileServer(http.Dir("./static/")))
	if s.internalExplorer {
		// internal explorer handlers
		serveMux.HandleFunc(path+"tx/", s.htmlTemplateHandler(s.explorerTx))
		serveMux.HandleFunc(path+"address/", s.htmlTemplateHandler(s.explorerAddress))
		serveMux.HandleFunc(path+"search/", s.htmlTemplateHandler(s.explorerSearch))
		serveMux.HandleFunc(path+"blocks", s.htmlTemplateHandler(s.explorerBlocks))
		serveMux.HandleFunc(path+"block/", s.htmlTemplateHandler(s.explorerBlock))
		serveMux.HandleFunc(path+"spending/", s.htmlTemplateHandler(s.explorerSpendingTx))
		serveMux.HandleFunc(path+"sendtx", s.htmlTemplateHandler(s.explorerSendTx))
	} else {
		// redirect to wallet requests for tx and address, possibly to external site
		serveMux.HandleFunc(path+"tx/", s.txRedirect)
		serveMux.HandleFunc(path+"address/", s.addressRedirect)
	}
	// API calls
	serveMux.HandleFunc(path+"api/block-index/", s.jsonHandler(s.apiBlockIndex))
	serveMux.HandleFunc(path+"api/tx/", s.jsonHandler(s.apiTx))
	serveMux.HandleFunc(path+"api/tx-specific/", s.jsonHandler(s.apiTxSpecific))
	serveMux.HandleFunc(path+"api/address/", s.jsonHandler(s.apiAddress))
	serveMux.HandleFunc(path+"api/utxo/", s.jsonHandler(s.apiAddressUtxo))
	serveMux.HandleFunc(path+"api/block/", s.jsonHandler(s.apiBlock))
	serveMux.HandleFunc(path+"api/sendtx/", s.jsonHandler(s.apiSendTx))
	serveMux.HandleFunc(path+"api/estimatefee/", s.jsonHandler(s.apiEstimateFee))
	// socket.io interface
	serveMux.Handle(path+"socket.io/", s.socketio.GetHandler())
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
func (s *PublicServer) OnNewTxAddr(txid string, desc bchain.AddressDescriptor, isOutput bool) {
	s.socketio.OnNewTxAddr(txid, desc, isOutput)
}

func (s *PublicServer) txRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), 302)
	s.metrics.ExplorerViews.With(common.Labels{"action": "tx-redirect"}).Inc()
}

func (s *PublicServer) addressRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), 302)
	s.metrics.ExplorerViews.With(common.Labels{"action": "address-redirect"}).Inc()
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
		Text       string `json:"error"`
		HTTPStatus int    `json:"-"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var data interface{}
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(getFunctionName(handler), " recovered from panic: ", e)
				if s.debug {
					data = jsonError{fmt.Sprint("Internal server error: recovered from panic ", e), http.StatusInternalServerError}
				} else {
					data = jsonError{"Internal server error", http.StatusInternalServerError}
				}
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if e, isError := data.(jsonError); isError {
				w.WriteHeader(e.HTTPStatus)
			}
			json.NewEncoder(w).Encode(data)
		}()
		data, err = handler(r)
		if err != nil || data == nil {
			if apiErr, ok := err.(*api.APIError); ok {
				if apiErr.Public {
					data = jsonError{apiErr.Error(), http.StatusBadRequest}
				} else {
					data = jsonError{apiErr.Error(), http.StatusInternalServerError}
				}
			} else {
				if err != nil {
					glog.Error(getFunctionName(handler), " error: ", err)
				}
				if s.debug {
					if data != nil {
						data = jsonError{fmt.Sprintf("Internal server error: %v, data %+v", err, data), http.StatusInternalServerError}
					} else {
						data = jsonError{fmt.Sprintf("Internal server error: %v", err), http.StatusInternalServerError}
					}
				} else {
					data = jsonError{"Internal server error", http.StatusInternalServerError}
				}
			}
		}
	}
}

func (s *PublicServer) newTemplateData() *TemplateData {
	return &TemplateData{
		CoinName:         s.is.Coin,
		CoinShortcut:     s.is.CoinShortcut,
		CoinLabel:        s.is.CoinLabel,
		InternalExplorer: s.internalExplorer && !s.is.InitialSync,
		TOSLink:          api.Text.TOSLink,
	}
}

func (s *PublicServer) newTemplateDataWithError(text string) *TemplateData {
	td := s.newTemplateData()
	td.Error = &api.APIError{Text: text}
	return td
}

func (s *PublicServer) htmlTemplateHandler(handler func(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var t tpl
		var data *TemplateData
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(getFunctionName(handler), " recovered from panic: ", e)
				t = errorInternalTpl
				if s.debug {
					data = s.newTemplateDataWithError(fmt.Sprint("Internal server error: recovered from panic ", e))
				} else {
					data = s.newTemplateDataWithError("Internal server error")
				}
			}
			// noTpl means the handler completely handled the request
			if t != noTpl {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				// return 500 Internal Server Error with errorInternalTpl
				if t == errorInternalTpl {
					w.WriteHeader(http.StatusInternalServerError)
				}
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
			t = errorInternalTpl
			if apiErr, ok := err.(*api.APIError); ok {
				data = s.newTemplateData()
				data.Error = apiErr
				if apiErr.Public {
					t = errorTpl
				}
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
	errorInternalTpl
	indexTpl
	txTpl
	addressTpl
	blocksTpl
	blockTpl
	sendTransactionTpl

	tplCount
)

// TemplateData is used to transfer data to the templates
type TemplateData struct {
	CoinName         string
	CoinShortcut     string
	CoinLabel        string
	InternalExplorer bool
	Address          *api.Address
	AddrStr          string
	Tx               *api.Tx
	TxSpecific       json.RawMessage
	Error            *api.APIError
	Blocks           *api.Blocks
	Block            *api.Block
	Info             *api.SystemInfo
	Page             int
	PrevPage         int
	NextPage         int
	PagingRange      []int
	TOSLink          string
	SendTxHex        string
	Status           string
}

func parseTemplates() []*template.Template {
	templateFuncMap := template.FuncMap{
		"formatTime":          formatTime,
		"formatUnixTime":      formatUnixTime,
		"formatAmount":        formatAmount,
		"setTxToTemplateData": setTxToTemplateData,
		"stringInSlice":       stringInSlice,
	}
	t := make([]*template.Template, tplCount)
	t[errorTpl] = template.Must(template.New("error").Funcs(templateFuncMap).ParseFiles("./static/templates/error.html", "./static/templates/base.html"))
	t[errorInternalTpl] = template.Must(template.New("error").Funcs(templateFuncMap).ParseFiles("./static/templates/error.html", "./static/templates/base.html"))
	t[indexTpl] = template.Must(template.New("index").Funcs(templateFuncMap).ParseFiles("./static/templates/index.html", "./static/templates/base.html"))
	t[txTpl] = template.Must(template.New("tx").Funcs(templateFuncMap).ParseFiles("./static/templates/tx.html", "./static/templates/txdetail.html", "./static/templates/base.html"))
	t[addressTpl] = template.Must(template.New("address").Funcs(templateFuncMap).ParseFiles("./static/templates/address.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html"))
	t[blocksTpl] = template.Must(template.New("blocks").Funcs(templateFuncMap).ParseFiles("./static/templates/blocks.html", "./static/templates/paging.html", "./static/templates/base.html"))
	t[blockTpl] = template.Must(template.New("block").Funcs(templateFuncMap).ParseFiles("./static/templates/block.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html"))
	t[sendTransactionTpl] = template.Must(template.New("block").Funcs(templateFuncMap).ParseFiles("./static/templates/sendtx.html", "./static/templates/base.html"))
	return t
}

func formatUnixTime(ut int64) string {
	return formatTime(time.Unix(ut, 0))
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC1123)
}

// for now return the string as it is
// in future could be used to do coin specific formatting
func formatAmount(a string) string {
	if a == "" {
		return "0"
	}
	return a
}

// called from template to support txdetail.html functionality
func setTxToTemplateData(td *TemplateData, tx *api.Tx) *TemplateData {
	td.Tx = tx
	return td
}

func (s *PublicServer) explorerTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var tx *api.Tx
	var txSpecific json.RawMessage
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "tx"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		tx, err = s.api.GetTransaction(txid, false)
		if err != nil {
			return errorTpl, nil, err
		}
		txSpecific, err = s.chain.GetTransactionSpecific(txid)
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData()
	data.Tx = tx
	data.TxSpecific = txSpecific
	return txTpl, data, nil
}

func (s *PublicServer) explorerSpendingTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "spendingtx"}).Inc()
	var err error
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) > 2 {
		tx := parts[len(parts)-2]
		n, ec := strconv.Atoi(parts[len(parts)-1])
		if ec == nil {
			spendingTx, err := s.api.GetSpendingTxid(tx, n)
			if err == nil && spendingTx != "" {
				http.Redirect(w, r, joinURL("/tx/", spendingTx), 302)
				return noTpl, nil, nil
			}
		}
	}
	if err == nil {
		err = api.NewAPIError("Transaction not found", true)
	}
	return errorTpl, nil, err
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

func (s *PublicServer) explorerBlocks(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var blocks *api.Blocks
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "blocks"}).Inc()
	page, ec := strconv.Atoi(r.URL.Query().Get("page"))
	if ec != nil {
		page = 0
	}
	blocks, err = s.api.GetBlocks(page, blocksOnPage)
	if err != nil {
		return errorTpl, nil, err
	}
	data := s.newTemplateData()
	data.Blocks = blocks
	data.Page = blocks.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(blocks.Page, blocks.TotalPages)
	return blocksTpl, data, nil
}

func (s *PublicServer) explorerBlock(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var block *api.Block
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "block"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		page, ec := strconv.Atoi(r.URL.Query().Get("page"))
		if ec != nil {
			page = 0
		}
		block, err = s.api.GetBlock(r.URL.Path[i+1:], page, txsOnPage)
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData()
	data.Block = block
	data.Page = block.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(block.Page, block.TotalPages)
	return blockTpl, data, nil
}

func (s *PublicServer) explorerIndex(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var si *api.SystemInfo
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "index"}).Inc()
	si, err = s.api.GetSystemInfo(false)
	if err != nil {
		return errorTpl, nil, err
	}
	data := s.newTemplateData()
	data.Info = si
	return indexTpl, data, nil
}

func (s *PublicServer) explorerSearch(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var tx *api.Tx
	var address *api.Address
	var block *api.Block
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "search"}).Inc()
	if len(q) > 0 {
		block, err = s.api.GetBlock(q, 0, 1)
		if err == nil {
			http.Redirect(w, r, joinURL("/block/", block.Hash), 302)
			return noTpl, nil, nil
		}
		tx, err = s.api.GetTransaction(q, false)
		if err == nil {
			http.Redirect(w, r, joinURL("/tx/", tx.Txid), 302)
			return noTpl, nil, nil
		}
		address, err = s.api.GetAddress(q, 0, 1, true)
		if err == nil {
			http.Redirect(w, r, joinURL("/address/", address.AddrStr), 302)
			return noTpl, nil, nil
		}
	}
	return errorTpl, nil, api.NewAPIError(fmt.Sprintf("No matching records found for '%v'", q), true)
}

func (s *PublicServer) explorerSendTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "sendtx"}).Inc()
	data := s.newTemplateData()
	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			return sendTransactionTpl, data, err
		}
		hex := r.FormValue("hex")
		if len(hex) > 0 {
			res, err := s.chain.SendRawTransaction(hex)
			if err != nil {
				data.SendTxHex = hex
				data.Error = &api.APIError{Text: err.Error(), Public: true}
				return sendTransactionTpl, data, nil
			}
			data.Status = "Transaction sent, result " + res
		}
	}
	return sendTransactionTpl, data, nil
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

func (s *PublicServer) apiIndex(r *http.Request) (interface{}, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-index"}).Inc()
	return s.api.GetSystemInfo(false)
}

func (s *PublicServer) apiBlockIndex(r *http.Request) (interface{}, error) {
	type resBlockIndex struct {
		BlockHash string `json:"blockHash"`
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
	}, nil
}

func (s *PublicServer) apiTx(r *http.Request) (interface{}, error) {
	var tx *api.Tx
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tx"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		spendingTxs := false
		p := r.URL.Query().Get("spending")
		if len(p) > 0 {
			spendingTxs, err = strconv.ParseBool(p)
			if err != nil {
				return nil, api.NewAPIError("Parameter 'spending' cannot be converted to boolean", true)
			}
		}
		tx, err = s.api.GetTransaction(txid, spendingTxs)
	}
	return tx, err
}

func (s *PublicServer) apiTxSpecific(r *http.Request) (interface{}, error) {
	var tx json.RawMessage
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tx-specific"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		tx, err = s.chain.GetTransactionSpecific(txid)
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

func (s *PublicServer) apiAddressUtxo(r *http.Request) (interface{}, error) {
	var utxo []api.AddressUtxo
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-address"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		onlyConfirmed := false
		c := r.URL.Query().Get("confirmed")
		if len(c) > 0 {
			onlyConfirmed, err = strconv.ParseBool(c)
			if err != nil {
				return nil, api.NewAPIError("Parameter 'confirmed' cannot be converted to boolean", true)
			}
		}
		utxo, err = s.api.GetAddressUtxo(r.URL.Path[i+1:], onlyConfirmed)
	}
	return utxo, err
}

func (s *PublicServer) apiBlock(r *http.Request) (interface{}, error) {
	var block *api.Block
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-block"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		page, ec := strconv.Atoi(r.URL.Query().Get("page"))
		if ec != nil {
			page = 0
		}
		block, err = s.api.GetBlock(r.URL.Path[i+1:], page, txsInAPI)
	}
	return block, err
}

type resultSendTransaction struct {
	Result string `json:"result"`
}

func (s *PublicServer) apiSendTx(r *http.Request) (interface{}, error) {
	var err error
	var res resultSendTransaction
	var hex string
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-sendtx"}).Inc()
	if r.Method == http.MethodPost {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, api.NewAPIError("Missing tx blob", true)
		}
		hex = string(data)
	} else {
		if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
			hex = r.URL.Path[i+1:]
		}
	}
	if len(hex) > 0 {
		res.Result, err = s.chain.SendRawTransaction(hex)
		if err != nil {
			return nil, api.NewAPIError(err.Error(), true)
		}
		return res, nil
	}
	return nil, api.NewAPIError("Missing tx blob", true)
}

type resultEstimateFeeAsString struct {
	Result string `json:"result"`
}

func (s *PublicServer) apiEstimateFee(r *http.Request) (interface{}, error) {
	var res resultEstimateFeeAsString
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-estimatefee"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		b := r.URL.Path[i+1:]
		if len(b) > 0 {
			blocks, err := strconv.Atoi(b)
			if err != nil {
				return nil, api.NewAPIError("Parameter 'number of blocks' is not a number", true)
			}
			conservative := true
			c := r.URL.Query().Get("conservative")
			if len(c) > 0 {
				conservative, err = strconv.ParseBool(c)
				if err != nil {
					return nil, api.NewAPIError("Parameter 'conservative' cannot be converted to boolean", true)
				}
			}
			var fee big.Int
			fee, err = s.chain.EstimateSmartFee(blocks, conservative)
			if err != nil {
				fee, err = s.chain.EstimateFee(blocks)
				if err != nil {
					return nil, err
				}
			}
			res.Result = s.chainParser.AmountToDecimalString(&fee)
			return res, nil
		}
	}
	return nil, api.NewAPIError("Missing parameter 'number of blocks'", true)
}
