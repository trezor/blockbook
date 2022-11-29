package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const txsOnPage = 25
const blocksOnPage = 50
const mempoolTxsOnPage = 50
const txsInAPI = 1000

const secondaryCoinCookieName = "secondary_coin"

const (
	_ = iota
	apiV1
	apiV2
)

// PublicServer is a handle to public http server
type PublicServer struct {
	binding          string
	certFiles        string
	socketio         *SocketIoServer
	websocket        *WebsocketServer
	https            *http.Server
	db               *db.RocksDB
	txCache          *db.TxCache
	chain            bchain.BlockChain
	chainParser      bchain.BlockChainParser
	mempool          bchain.Mempool
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
func NewPublicServer(binding string, certFiles string, db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, explorerURL string, metrics *common.Metrics, is *common.InternalState, debugMode bool, enableSubNewTx bool) (*PublicServer, error) {

	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is)
	if err != nil {
		return nil, err
	}

	socketio, err := NewSocketIoServer(db, chain, mempool, txCache, metrics, is)
	if err != nil {
		return nil, err
	}

	websocket, err := NewWebsocketServer(db, chain, mempool, txCache, metrics, is, enableSubNewTx)
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
		websocket:        websocket,
		db:               db,
		txCache:          txCache,
		chain:            chain,
		chainParser:      chain.GetChainParser(),
		mempool:          mempool,
		explorerURL:      explorerURL,
		internalExplorer: explorerURL == "",
		metrics:          metrics,
		is:               is,
		debug:            debugMode,
	}
	s.templates = s.parseTemplates()

	// map only basic functions, the rest is enabled by method MapFullPublicInterface
	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	serveMux.Handle(path+"static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	// default handler
	serveMux.HandleFunc(path, s.htmlTemplateHandler(s.explorerIndex))
	// default API handler
	serveMux.HandleFunc(path+"api/", s.jsonHandler(s.apiIndex, apiV2))

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
	// support for test pages
	serveMux.Handle(path+"test-socketio.html", http.FileServer(http.Dir("./static/")))
	serveMux.Handle(path+"test-websocket.html", http.FileServer(http.Dir("./static/")))
	if s.internalExplorer {
		// internal explorer handlers
		serveMux.HandleFunc(path+"tx/", s.htmlTemplateHandler(s.explorerTx))
		serveMux.HandleFunc(path+"address/", s.htmlTemplateHandler(s.explorerAddress))
		serveMux.HandleFunc(path+"xpub/", s.htmlTemplateHandler(s.explorerXpub))
		serveMux.HandleFunc(path+"search/", s.htmlTemplateHandler(s.explorerSearch))
		serveMux.HandleFunc(path+"blocks", s.htmlTemplateHandler(s.explorerBlocks))
		serveMux.HandleFunc(path+"block/", s.htmlTemplateHandler(s.explorerBlock))
		serveMux.HandleFunc(path+"spending/", s.htmlTemplateHandler(s.explorerSpendingTx))
		serveMux.HandleFunc(path+"sendtx", s.htmlTemplateHandler(s.explorerSendTx))
		serveMux.HandleFunc(path+"mempool", s.htmlTemplateHandler(s.explorerMempool))
		if s.chainParser.GetChainType() == bchain.ChainEthereumType {
			serveMux.HandleFunc(path+"nft/", s.htmlTemplateHandler(s.explorerNftDetail))
		}
	} else {
		// redirect to wallet requests for tx and address, possibly to external site
		serveMux.HandleFunc(path+"tx/", s.txRedirect)
		serveMux.HandleFunc(path+"address/", s.addressRedirect)
	}
	// API calls
	// default api without version can be changed to different version at any time
	// use versioned api for stability

	var apiDefault int
	// ethereum supports only api V2
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		apiDefault = apiV2
	} else {
		apiDefault = apiV1
		// legacy v1 format
		serveMux.HandleFunc(path+"api/v1/block-index/", s.jsonHandler(s.apiBlockIndex, apiV1))
		serveMux.HandleFunc(path+"api/v1/tx-specific/", s.jsonHandler(s.apiTxSpecific, apiV1))
		serveMux.HandleFunc(path+"api/v1/tx/", s.jsonHandler(s.apiTx, apiV1))
		serveMux.HandleFunc(path+"api/v1/address/", s.jsonHandler(s.apiAddress, apiV1))
		serveMux.HandleFunc(path+"api/v1/utxo/", s.jsonHandler(s.apiUtxo, apiV1))
		serveMux.HandleFunc(path+"api/v1/block/", s.jsonHandler(s.apiBlock, apiV1))
		serveMux.HandleFunc(path+"api/v1/sendtx/", s.jsonHandler(s.apiSendTx, apiV1))
		serveMux.HandleFunc(path+"api/v1/estimatefee/", s.jsonHandler(s.apiEstimateFee, apiV1))
	}
	serveMux.HandleFunc(path+"api/block-index/", s.jsonHandler(s.apiBlockIndex, apiDefault))
	serveMux.HandleFunc(path+"api/tx-specific/", s.jsonHandler(s.apiTxSpecific, apiDefault))
	serveMux.HandleFunc(path+"api/tx/", s.jsonHandler(s.apiTx, apiDefault))
	serveMux.HandleFunc(path+"api/address/", s.jsonHandler(s.apiAddress, apiDefault))
	serveMux.HandleFunc(path+"api/xpub/", s.jsonHandler(s.apiXpub, apiDefault))
	serveMux.HandleFunc(path+"api/utxo/", s.jsonHandler(s.apiUtxo, apiDefault))
	serveMux.HandleFunc(path+"api/block/", s.jsonHandler(s.apiBlock, apiDefault))
	serveMux.HandleFunc(path+"api/rawblock/", s.jsonHandler(s.apiBlockRaw, apiDefault))
	serveMux.HandleFunc(path+"api/sendtx/", s.jsonHandler(s.apiSendTx, apiDefault))
	serveMux.HandleFunc(path+"api/estimatefee/", s.jsonHandler(s.apiEstimateFee, apiDefault))
	serveMux.HandleFunc(path+"api/balancehistory/", s.jsonHandler(s.apiBalanceHistory, apiDefault))
	// v2 format
	serveMux.HandleFunc(path+"api/v2/block-index/", s.jsonHandler(s.apiBlockIndex, apiV2))
	serveMux.HandleFunc(path+"api/v2/tx-specific/", s.jsonHandler(s.apiTxSpecific, apiV2))
	serveMux.HandleFunc(path+"api/v2/tx/", s.jsonHandler(s.apiTx, apiV2))
	serveMux.HandleFunc(path+"api/v2/address/", s.jsonHandler(s.apiAddress, apiV2))
	serveMux.HandleFunc(path+"api/v2/xpub/", s.jsonHandler(s.apiXpub, apiV2))
	serveMux.HandleFunc(path+"api/v2/utxo/", s.jsonHandler(s.apiUtxo, apiV2))
	serveMux.HandleFunc(path+"api/v2/block/", s.jsonHandler(s.apiBlock, apiV2))
	serveMux.HandleFunc(path+"api/v2/rawblock/", s.jsonHandler(s.apiBlockRaw, apiDefault))
	serveMux.HandleFunc(path+"api/v2/sendtx/", s.jsonHandler(s.apiSendTx, apiV2))
	serveMux.HandleFunc(path+"api/v2/estimatefee/", s.jsonHandler(s.apiEstimateFee, apiV2))
	serveMux.HandleFunc(path+"api/v2/feestats/", s.jsonHandler(s.apiFeeStats, apiV2))
	serveMux.HandleFunc(path+"api/v2/balancehistory/", s.jsonHandler(s.apiBalanceHistory, apiDefault))
	serveMux.HandleFunc(path+"api/v2/tickers/", s.jsonHandler(s.apiTickers, apiV2))
	serveMux.HandleFunc(path+"api/v2/multi-tickers/", s.jsonHandler(s.apiMultiTickers, apiV2))
	serveMux.HandleFunc(path+"api/v2/tickers-list/", s.jsonHandler(s.apiAvailableVsCurrencies, apiV2))
	// socket.io interface
	serveMux.Handle(path+"socket.io/", s.socketio.GetHandler())
	// websocket interface
	serveMux.Handle(path+"websocket", s.websocket.GetHandler())
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
	s.websocket.OnNewBlock(hash, height)
}

// OnNewFiatRatesTicker notifies users subscribed to bitcoind/fiatrates about new ticker
func (s *PublicServer) OnNewFiatRatesTicker(ticker *common.CurrencyRatesTicker) {
	s.websocket.OnNewFiatRatesTicker(ticker)
}

// OnNewTxAddr notifies users subscribed to notification about new tx
func (s *PublicServer) OnNewTxAddr(tx *bchain.Tx, desc bchain.AddressDescriptor) {
	s.socketio.OnNewTxAddr(tx.Txid, desc)
}

// OnNewTx notifies users subscribed to notification about new tx
func (s *PublicServer) OnNewTx(tx *bchain.MempoolTx) {
	s.websocket.OnNewTx(tx)
}

func (s *PublicServer) txRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), http.StatusFound)
	s.metrics.ExplorerViews.With(common.Labels{"action": "tx-redirect"}).Inc()
}

func (s *PublicServer) addressRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, joinURL(s.explorerURL, r.URL.Path), http.StatusFound)
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
	name := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	start := strings.LastIndex(name, ".")
	end := strings.LastIndex(name, "-")
	if start > 0 && end > start {
		name = name[start+1 : end]
	}
	return name
}

func (s *PublicServer) jsonHandler(handler func(r *http.Request, apiVersion int) (interface{}, error), apiVersion int) func(w http.ResponseWriter, r *http.Request) {
	type jsonError struct {
		Text       string `json:"error"`
		HTTPStatus int    `json:"-"`
	}
	handlerName := getFunctionName(handler)
	return func(w http.ResponseWriter, r *http.Request) {
		var data interface{}
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(handlerName, " recovered from panic: ", e)
				debug.PrintStack()
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
			err = json.NewEncoder(w).Encode(data)
			if err != nil {
				glog.Warning("json encode ", err)
			}
			s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Dec()
		}()
		s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Inc()
		data, err = handler(r, apiVersion)
		if err != nil || data == nil {
			if apiErr, ok := err.(*api.APIError); ok {
				if apiErr.Public {
					data = jsonError{apiErr.Error(), http.StatusBadRequest}
				} else {
					data = jsonError{apiErr.Error(), http.StatusInternalServerError}
				}
			} else {
				if err != nil {
					glog.Error(handlerName, " error: ", err)
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

func (s *PublicServer) newTemplateData(r *http.Request) *TemplateData {
	t := &TemplateData{
		CoinName:         s.is.Coin,
		CoinShortcut:     s.is.CoinShortcut,
		CoinLabel:        s.is.CoinLabel,
		ChainType:        s.chainParser.GetChainType(),
		InternalExplorer: s.internalExplorer && !s.is.InitialSync,
		TOSLink:          api.Text.TOSLink,
	}
	if s.is.HasFiatRates {
		// get the secondary coin and if it should be shown either from query parameters "secondary" and "use_secondary"
		// or from the cookie "secondary_coin" in the format secondary=use_secondary, for example EUR=true
		// the query parameters take precedence over the cookie
		var cookieSecondary string
		var cookieUseSecondary bool
		cookie, _ := r.Cookie(secondaryCoinCookieName)
		if cookie != nil {
			a := strings.Split(cookie.Value, "=")
			if len(a) == 2 {
				cookieSecondary = a[0]
				cookieUseSecondary, _ = strconv.ParseBool(a[1])
			}
		}
		secondary := strings.ToLower(r.URL.Query().Get("secondary"))
		if secondary == "" {
			if cookieSecondary != "" {
				secondary = strings.ToLower(cookieSecondary)
			} else {
				secondary = "usd"
			}
		}
		ticker := s.is.GetCurrentTicker(secondary, "")
		if ticker == nil && secondary != "usd" {
			secondary = "usd"
			ticker = s.is.GetCurrentTicker(secondary, "")
		}
		if ticker != nil {
			t.SecondaryCoin = strings.ToUpper(secondary)
			t.CurrentSecondaryCoinRate = float64(ticker.Rates[secondary])
			t.CurrentTicker = ticker
			t.UseSecondaryCoin, _ = strconv.ParseBool(r.URL.Query().Get("use_secondary"))
			if !t.UseSecondaryCoin {
				t.UseSecondaryCoin = cookieUseSecondary
			}
		}
	}
	return t
}

func (s *PublicServer) newTemplateDataWithError(text string, r *http.Request) *TemplateData {
	td := s.newTemplateData(r)
	td.Error = &api.APIError{Text: text}
	return td
}

func (s *PublicServer) htmlTemplateHandler(handler func(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error)) func(w http.ResponseWriter, r *http.Request) {
	handlerName := getFunctionName(handler)
	return func(w http.ResponseWriter, r *http.Request) {
		var t tpl
		var data *TemplateData
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(handlerName, " recovered from panic: ", e)
				debug.PrintStack()
				t = errorInternalTpl
				if s.debug {
					data = s.newTemplateDataWithError(fmt.Sprint("Internal server error: recovered from panic ", e), r)
				} else {
					data = s.newTemplateDataWithError("Internal server error", r)
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
			s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Dec()
		}()
		s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Inc()
		if s.debug {
			// reload templates on each request
			// to reflect changes during development
			s.templates = s.parseTemplates()
		}
		t, data, err = handler(w, r)
		if err != nil || (data == nil && t != noTpl) {
			t = errorInternalTpl
			if apiErr, ok := err.(*api.APIError); ok {
				data = s.newTemplateData(r)
				data.Error = apiErr
				if apiErr.Public {
					t = errorTpl
				}
			} else {
				if err != nil {
					glog.Error(handlerName, " error: ", err)
				}
				if s.debug {
					data = s.newTemplateDataWithError(fmt.Sprintf("Internal server error: %v, data %+v", err, data), r)
				} else {
					data = s.newTemplateDataWithError("Internal server error", r)
				}
			}
		}
		// if SecondaryCoin is specified, set secondary_coin cookie
		if data != nil && data.SecondaryCoin != "" {
			http.SetCookie(w, &http.Cookie{Name: secondaryCoinCookieName, Value: data.SecondaryCoin + "=" + strconv.FormatBool(data.UseSecondaryCoin), Path: "/"})
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
	xpubTpl
	blocksTpl
	blockTpl
	sendTransactionTpl
	mempoolTpl
	nftDetailTpl

	tplCount
)

// TemplateData is used to transfer data to the templates
type TemplateData struct {
	CoinName                 string
	CoinShortcut             string
	CoinLabel                string
	InternalExplorer         bool
	ChainType                bchain.ChainType
	Address                  *api.Address
	AddrStr                  string
	Tx                       *api.Tx
	Error                    *api.APIError
	Blocks                   *api.Blocks
	Block                    *api.Block
	Info                     *api.SystemInfo
	MempoolTxids             *api.MempoolTxids
	Page                     int
	PrevPage                 int
	NextPage                 int
	PagingRange              []int
	PageParams               template.URL
	TOSLink                  string
	SendTxHex                string
	Status                   string
	NonZeroBalanceTokens     bool
	TokenId                  string
	URI                      string
	ContractInfo             *bchain.ContractInfo
	SecondaryCoin            string
	UseSecondaryCoin         bool
	CurrentSecondaryCoinRate float64
	CurrentTicker            *common.CurrencyRatesTicker
	TxDate                   string
	TxSecondaryCoinRate      float64
	TxTicker                 *common.CurrencyRatesTicker
}

func (s *PublicServer) parseTemplates() []*template.Template {
	templateFuncMap := template.FuncMap{
		"timeSpan":                 timeSpan,
		"unixTimeSpan":             unixTimeSpan,
		"amountSpan":               s.amountSpan,
		"tokenAmountSpan":          s.tokenAmountSpan,
		"amountSatsSpan":           s.amountSatsSpan,
		"formattedAmountSpan":      s.formattedAmountSpan,
		"summaryValuesSpan":        s.summaryValuesSpan,
		"addressAlias":             addressAlias,
		"addressAliasSpan":         addressAliasSpan,
		"formatAmount":             s.formatAmount,
		"formatAmountWithDecimals": formatAmountWithDecimals,
		"formatInt64":              formatInt64,
		"formatInt":                formatInt,
		"formatUint32":             formatUint32,
		"formatBigInt":             formatBigInt,
		"setTxToTemplateData":      setTxToTemplateData,
		"feePerByte":               feePerByte,
		"isOwnAddress":             isOwnAddress,
		"toJSON":                   toJSON,
		"tokenTransfersCount":      tokenTransfersCount,
		"tokenCount":               tokenCount,
		"hasPrefix":                strings.HasPrefix,
		"jsStr":                    jsStr,
	}
	var createTemplate func(filenames ...string) *template.Template
	if s.debug {
		createTemplate = func(filenames ...string) *template.Template {
			if len(filenames) == 0 {
				panic("Missing templates")
			}
			return template.Must(template.New(filepath.Base(filenames[0])).Funcs(templateFuncMap).ParseFiles(filenames...))
		}
	} else {
		createTemplate = func(filenames ...string) *template.Template {
			if len(filenames) == 0 {
				panic("Missing templates")
			}
			t := template.New(filepath.Base(filenames[0])).Funcs(templateFuncMap)
			for _, filename := range filenames {
				b, err := os.ReadFile(filename)
				if err != nil {
					panic(err)
				}
				// perform very simple minification - replace leading spaces used as formatting and new lines
				r := regexp.MustCompile(`\n\s*`)
				b = r.ReplaceAll(b, []byte{})
				s := string(b)
				name := filepath.Base(filename)
				var tt *template.Template
				if name == t.Name() {
					tt = t
				} else {
					tt = t.New(name)
				}
				_, err = tt.Parse(s)
				if err != nil {
					panic(err)
				}
			}
			return t
		}
	}
	t := make([]*template.Template, tplCount)
	t[errorTpl] = createTemplate("./static/templates/error.html", "./static/templates/base.html")
	t[errorInternalTpl] = createTemplate("./static/templates/error.html", "./static/templates/base.html")
	t[indexTpl] = createTemplate("./static/templates/index.html", "./static/templates/base.html")
	t[blocksTpl] = createTemplate("./static/templates/blocks.html", "./static/templates/paging.html", "./static/templates/base.html")
	t[sendTransactionTpl] = createTemplate("./static/templates/sendtx.html", "./static/templates/base.html")
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		t[txTpl] = createTemplate("./static/templates/tx.html", "./static/templates/txdetail_ethereumtype.html", "./static/templates/base.html")
		t[addressTpl] = createTemplate("./static/templates/address.html", "./static/templates/txdetail_ethereumtype.html", "./static/templates/paging.html", "./static/templates/base.html")
		t[blockTpl] = createTemplate("./static/templates/block.html", "./static/templates/txdetail_ethereumtype.html", "./static/templates/paging.html", "./static/templates/base.html")
		t[nftDetailTpl] = createTemplate("./static/templates/tokenDetail.html", "./static/templates/base.html")
	} else {
		t[txTpl] = createTemplate("./static/templates/tx.html", "./static/templates/txdetail.html", "./static/templates/base.html")
		t[addressTpl] = createTemplate("./static/templates/address.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html")
		t[blockTpl] = createTemplate("./static/templates/block.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html")
	}
	t[xpubTpl] = createTemplate("./static/templates/xpub.html", "./static/templates/txdetail.html", "./static/templates/paging.html", "./static/templates/base.html")
	t[mempoolTpl] = createTemplate("./static/templates/mempool.html", "./static/templates/paging.html", "./static/templates/base.html")
	return t
}

func relativeTime(d int64) string {
	var u string
	if d < 60 {
		if d == 1 {
			u = " sec"
		} else {
			u = " secs"
		}
	} else if d < 3600 {
		d /= 60
		if d == 1 {
			u = " min"
		} else {
			u = " mins"
		}
	} else if d < 3600*24 {
		d /= 3600
		if d == 1 {
			u = " hour"
		} else {
			u = " hours"
		}
	} else {
		d /= 3600 * 24
		if d == 1 {
			u = " day"
		} else {
			u = " days"
		}
	}
	return strconv.FormatInt(d, 10) + u
}

func unixTimeSpan(ut int64) template.HTML {
	t := time.Unix(ut, 0)
	return timeSpan(&t)
}

var timeNow = time.Now

func timeSpan(t *time.Time) template.HTML {
	if t == nil {
		return ""
	}
	u := t.Unix()
	if u <= 0 {
		return ""
	}
	d := timeNow().Unix() - u
	f := t.UTC().Format("2006-01-02 15:04:05")
	if d < 0 {
		return template.HTML(f)
	}
	r := relativeTime(d)
	if d > 3600*24 {
		d = d % (3600 * 24)
		if d >= 3600 {
			r += " " + relativeTime(d)
		}
	} else if d > 3600 {
		d = d % 3600
		if d >= 60 {
			r += " " + relativeTime(d)
		}
	}
	return template.HTML(`<span tt="` + f + `">` + r + " ago</span>")
}

func toJSON(data interface{}) string {
	json, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(json)
}

func (s *PublicServer) formatAmount(a *api.Amount) string {
	if a == nil {
		return "0"
	}
	return s.chainParser.AmountToDecimalString((*big.Int)(a))
}

func formatAmountWithDecimals(a *api.Amount, d int) string {
	if a == nil {
		return "0"
	}
	return a.DecimalString(d)
}

func appendAmountSpan(rv *strings.Builder, class, amount, shortcut, txDate string) {
	rv.WriteString(`<span`)
	if class != "" {
		rv.WriteString(` class="`)
		rv.WriteString(class)
		rv.WriteString(`"`)
	}
	if txDate != "" {
		rv.WriteString(` tm="`)
		rv.WriteString(txDate)
		rv.WriteString(`"`)
	}
	rv.WriteString(">")
	i := strings.IndexByte(amount, '.')
	if i < 0 {
		appendSeparatedNumberSpans(rv, amount, "nc")
	} else {
		appendSeparatedNumberSpans(rv, amount[:i], "nc")
		rv.WriteString(`.`)
		rv.WriteString(`<span class="amt-dec">`)
		appendLeftSeparatedNumberSpans(rv, amount[i+1:], "ns")
		rv.WriteString("</span>")
	}
	if shortcut != "" {
		rv.WriteString(" ")
		rv.WriteString(shortcut)
	}
	rv.WriteString("</span>")
}

func appendAmountWrapperSpan(rv *strings.Builder, primary, symbol, classes string) {
	rv.WriteString(`<span class="amt`)
	if classes != "" {
		rv.WriteString(` `)
		rv.WriteString(classes)
	}
	rv.WriteString(`" cc="`)
	rv.WriteString(primary)
	rv.WriteString(" ")
	rv.WriteString(symbol)
	rv.WriteString(`">`)
}

func (s *PublicServer) amountSpan(a *api.Amount, td *TemplateData, classes string) template.HTML {
	primary := s.formatAmount(a)
	var rv strings.Builder
	appendAmountWrapperSpan(&rv, primary, td.CoinShortcut, classes)
	appendAmountSpan(&rv, "prim-amt", primary, td.CoinShortcut, "")
	if td.SecondaryCoin != "" {
		p, err := strconv.ParseFloat(primary, 64)
		if err == nil {
			currentSecondary := strconv.FormatFloat(p*td.CurrentSecondaryCoinRate, 'f', 2, 64)
			txSecondary := ""
			// if tx is specified, compute secondary amount is at the time of tx and amount with current rate is returned with class "csec-amt"
			if td.Tx != nil {
				if td.TxTicker == nil {
					date := time.Unix(td.Tx.Blocktime, 0).UTC()
					secondary := strings.ToLower(td.SecondaryCoin)
					ticker, _ := s.db.FiatRatesFindTicker(&date, secondary, "")
					if ticker != nil {
						td.TxSecondaryCoinRate = float64(ticker.Rates[secondary])
						// the ticker is from the midnight, valid for the whole day before
						td.TxDate = date.Add(-1 * time.Second).Format("2006-01-02")
						td.TxTicker = ticker
					}
				}
				if td.TxSecondaryCoinRate != 0 {
					txSecondary = strconv.FormatFloat(p*td.TxSecondaryCoinRate, 'f', 2, 64)
				}
			}
			if txSecondary != "" {
				appendAmountSpan(&rv, "sec-amt", txSecondary, td.SecondaryCoin, td.TxDate)
				appendAmountSpan(&rv, "csec-amt", currentSecondary, td.SecondaryCoin, "")
			} else {
				appendAmountSpan(&rv, "sec-amt", currentSecondary, td.SecondaryCoin, "")
			}
		}
	}
	rv.WriteString("</span>")
	return template.HTML(rv.String())
}

func (s *PublicServer) amountSatsSpan(a *api.Amount, td *TemplateData, classes string) template.HTML {
	var sats string
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		sats = a.DecimalString(9) // Gwei
	} else {
		sats = a.String()
	}
	var rv strings.Builder
	rv.WriteString(`<span`)
	if classes != "" {
		rv.WriteString(` class="`)
		rv.WriteString(classes)
		rv.WriteString(`"`)
	}
	rv.WriteString(` cc="`)
	rv.WriteString(sats)
	rv.WriteString(`">`)
	appendAmountSpan(&rv, "", sats, "", "")
	rv.WriteString("</span>")
	return template.HTML(rv.String())
}

func (s *PublicServer) tokenAmountSpan(t *api.TokenTransfer, td *TemplateData, classes string) template.HTML {
	primary := formatAmountWithDecimals(t.Value, t.Decimals)
	var rv strings.Builder
	appendAmountWrapperSpan(&rv, primary, td.CoinShortcut, classes)
	appendAmountSpan(&rv, "prim-amt", primary, t.Symbol, "")
	if td.SecondaryCoin != "" {
		var currentBase, currentSecondary, txBase, txSecondary string
		p, err := strconv.ParseFloat(primary, 64)
		if err == nil {
			baseRate, found := s.api.GetContractBaseRate(td.CurrentTicker, t.Contract, 0)
			if found {
				base := p * baseRate
				currentBase = strconv.FormatFloat(base, 'f', 6, 64)
				currentSecondary = strconv.FormatFloat(base*td.CurrentSecondaryCoinRate, 'f', 2, 64)
			}
			baseRate, found = s.api.GetContractBaseRate(td.TxTicker, t.Contract, td.Tx.Blocktime)
			if found {
				base := p * baseRate
				txBase = strconv.FormatFloat(base, 'f', 6, 64)
				txSecondary = strconv.FormatFloat(base*td.TxSecondaryCoinRate, 'f', 2, 64)
			}
		}
		if txBase != "" {
			appendAmountSpan(&rv, "base-amt", txBase, td.CoinShortcut, td.TxDate)
			if currentBase != "" {
				appendAmountSpan(&rv, "cbase-amt", currentBase, td.CoinShortcut, "")
			}
		} else if currentBase != "" {
			appendAmountSpan(&rv, "base-amt", currentBase, td.CoinShortcut, "")
		}
		if txSecondary != "" {
			appendAmountSpan(&rv, "sec-amt", txSecondary, td.SecondaryCoin, td.TxDate)
			if currentSecondary != "" {
				appendAmountSpan(&rv, "csec-amt", currentSecondary, td.SecondaryCoin, "")
			}
		} else if currentSecondary != "" {
			appendAmountSpan(&rv, "sec-amt", currentSecondary, td.SecondaryCoin, "")
		} else {
			appendAmountSpan(&rv, "sec-amt", "-", "", "")
		}
	}
	rv.WriteString("</span>")
	return template.HTML(rv.String())
}

func (s *PublicServer) formattedAmountSpan(a *api.Amount, d int, symbol string, td *TemplateData, classes string) template.HTML {
	if symbol == td.CoinShortcut {
		d = s.chainParser.AmountDecimals()
	}
	value := formatAmountWithDecimals(a, d)
	var rv strings.Builder
	appendAmountSpan(&rv, classes, value, symbol, "")
	return template.HTML(rv.String())
}

func (s *PublicServer) summaryValuesSpan(baseValue float64, secondaryValue float64, td *TemplateData) template.HTML {
	var rv strings.Builder
	if secondaryValue > 0 {
		appendAmountSpan(&rv, "", strconv.FormatFloat(secondaryValue, 'f', 2, 64), td.SecondaryCoin, "")
		if baseValue > 0 && s.chainParser.GetChainType() == bchain.ChainEthereumType {
			rv.WriteString(`<span class="base-value">(`)
			appendAmountSpan(&rv, "", strconv.FormatFloat(baseValue, 'f', 6, 64), td.CoinShortcut, "")
			rv.WriteString(")</span>")
		}
	} else {
		if td.SecondaryCoin != "" {
			rv.WriteString("-")
		}
	}
	return template.HTML(rv.String())
}

func formatInt(i int) template.HTML {
	return formatInt64(int64(i))
}

func formatUint32(i uint32) template.HTML {
	return formatInt64(int64(i))
}

func appendSeparatedNumberSpans(rv *strings.Builder, s, separatorClass string) {
	t := (len(s) - 1) / 3
	if t <= 0 {
		rv.WriteString(s)
	} else {
		t *= 3
		rv.WriteString(s[:len(s)-t])
		for i := len(s) - t; i < len(s); i += 3 {
			rv.WriteString(`<span class="`)
			rv.WriteString(separatorClass)
			rv.WriteString(`">`)
			rv.WriteString(s[i : i+3])
			rv.WriteString("</span>")
		}
	}
}

func appendLeftSeparatedNumberSpans(rv *strings.Builder, s, separatorClass string) {
	l := len(s)
	if l <= 3 {
		rv.WriteString(s)
	} else {
		rv.WriteString(s[:3])
		for i := 3; i < len(s); i += 3 {
			rv.WriteString(`<span class="`)
			rv.WriteString(separatorClass)
			rv.WriteString(`">`)
			e := i + 3
			if e > l {
				e = l
			}
			rv.WriteString(s[i:e])
			rv.WriteString("</span>")
		}
	}
}

func formatInt64(i int64) template.HTML {
	s := strconv.FormatInt(i, 10)
	var rv strings.Builder
	appendSeparatedNumberSpans(&rv, s, "ns")
	return template.HTML(rv.String())
}

func formatBigInt(i *big.Int) template.HTML {
	if i == nil {
		return ""
	}
	s := i.String()
	var rv strings.Builder
	appendSeparatedNumberSpans(&rv, s, "ns")
	return template.HTML(rv.String())
}

func getAddressAlias(a string, td *TemplateData) *api.AddressAlias {
	var alias api.AddressAlias
	var found bool
	if td.Block != nil {
		alias, found = td.Block.AddressAliases[a]
	} else if td.Address != nil {
		alias, found = td.Address.AddressAliases[a]
	} else if td.Tx != nil {
		alias, found = td.Tx.AddressAliases[a]
	}
	if !found {
		return nil
	}
	return &alias
}

func addressAlias(a string, td *TemplateData) string {
	alias := getAddressAlias(a, td)
	if alias == nil {
		return ""
	}
	return alias.Alias
}

func addressAliasSpan(a string, td *TemplateData) template.HTML {
	var rv strings.Builder
	alias := getAddressAlias(a, td)
	if alias == nil {
		rv.WriteString(`<span class="copyable">`)
		rv.WriteString(a)
	} else {
		rv.WriteString(`<span class="copyable" cc="`)
		rv.WriteString(a)
		rv.WriteString(`" alias-type="`)
		rv.WriteString(alias.Type)
		rv.WriteString(`">`)
		rv.WriteString(alias.Alias)
	}
	rv.WriteString("</span>")
	return template.HTML(rv.String())
}

// called from template to support txdetail.html functionality
func setTxToTemplateData(td *TemplateData, tx *api.Tx) *TemplateData {
	td.Tx = tx
	// reset the TxTicker if different Blocktime
	if td.TxTicker != nil && td.TxTicker.Timestamp.Unix() != tx.Blocktime {
		td.TxSecondaryCoinRate = 0
		td.TxTicker = nil
	}
	return td
}

// feePerByte returns fee per vByte or Byte if vsize is unknown
func feePerByte(tx *api.Tx) string {
	if tx.FeesSat != nil {
		if tx.VSize > 0 {
			return fmt.Sprintf("%.2f sat/vByte", float64(tx.FeesSat.AsInt64())/float64(tx.VSize))
		}
		if tx.Size > 0 {
			return fmt.Sprintf("%.2f sat/Byte", float64(tx.FeesSat.AsInt64())/float64(tx.Size))
		}
	}
	return ""
}

// isOwnAddress returns true if the address is the one that is being shown in the explorer
func isOwnAddress(td *TemplateData, a string) bool {
	return a == td.AddrStr
}

// called from template, returns count of token transfers of given type in a tx
func tokenTransfersCount(tx *api.Tx, t bchain.TokenTypeName) int {
	count := 0
	for i := range tx.TokenTransfers {
		if tx.TokenTransfers[i].Type == t {
			count++
		}
	}
	return count
}

// called from template, returns count of tokens in array of given type
func tokenCount(tokens []api.Token, t bchain.TokenTypeName) int {
	count := 0
	for i := range tokens {
		if tokens[i].Type == t {
			count++
		}
	}
	return count
}

func jsStr(s string) template.JSStr {
	return template.JSStr(s)
}

func (s *PublicServer) explorerTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var tx *api.Tx
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "tx"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		txid := r.URL.Path[i+1:]
		tx, err = s.api.GetTransaction(txid, false, true)
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData(r)
	data.Tx = tx
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
				http.Redirect(w, r, joinURL("/tx/", spendingTx), http.StatusFound)
				return noTpl, nil, nil
			}
		}
	}
	if err == nil {
		err = api.NewAPIError("Transaction not found", true)
	}
	return errorTpl, nil, err
}

func (s *PublicServer) getAddressQueryParams(r *http.Request, accountDetails api.AccountDetails, maxPageSize int) (int, int, api.AccountDetails, *api.AddressFilter, string, int) {
	var voutFilter = api.AddressFilterVoutOff
	page, ec := strconv.Atoi(r.URL.Query().Get("page"))
	if ec != nil {
		page = 0
	}
	pageSize, ec := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if ec != nil || pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	from, ec := strconv.Atoi(r.URL.Query().Get("from"))
	if ec != nil {
		from = 0
	}
	to, ec := strconv.Atoi(r.URL.Query().Get("to"))
	if ec != nil {
		to = 0
	}
	filterParam := r.URL.Query().Get("filter")
	if len(filterParam) > 0 {
		if filterParam == "inputs" {
			voutFilter = api.AddressFilterVoutInputs
		} else if filterParam == "outputs" {
			voutFilter = api.AddressFilterVoutOutputs
		} else {
			voutFilter, ec = strconv.Atoi(filterParam)
			if ec != nil || voutFilter < 0 {
				voutFilter = api.AddressFilterVoutOff
			}
		}
	}
	switch r.URL.Query().Get("details") {
	case "basic":
		accountDetails = api.AccountDetailsBasic
	case "tokens":
		accountDetails = api.AccountDetailsTokens
	case "tokenBalances":
		accountDetails = api.AccountDetailsTokenBalances
	case "txids":
		accountDetails = api.AccountDetailsTxidHistory
	case "txslight":
		accountDetails = api.AccountDetailsTxHistoryLight
	case "txs":
		accountDetails = api.AccountDetailsTxHistory
	}
	tokensToReturn := api.TokensToReturnNonzeroBalance
	switch r.URL.Query().Get("tokens") {
	case "derived":
		tokensToReturn = api.TokensToReturnDerived
	case "used":
		tokensToReturn = api.TokensToReturnUsed
	case "nonzero":
		tokensToReturn = api.TokensToReturnNonzeroBalance
	}
	gap, ec := strconv.Atoi(r.URL.Query().Get("gap"))
	if ec != nil {
		gap = 0
	}
	contract := r.URL.Query().Get("contract")
	return page, pageSize, accountDetails, &api.AddressFilter{
		Vout:           voutFilter,
		TokensToReturn: tokensToReturn,
		FromHeight:     uint32(from),
		ToHeight:       uint32(to),
		Contract:       contract,
	}, filterParam, gap
}

func (s *PublicServer) explorerAddress(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var addressParam string
	i := strings.LastIndexByte(r.URL.Path, '/')
	if i > 0 {
		addressParam = r.URL.Path[i+1:]
	}
	if len(addressParam) == 0 {
		return errorTpl, nil, api.NewAPIError("Missing address", true)
	}
	s.metrics.ExplorerViews.With(common.Labels{"action": "address"}).Inc()
	page, _, _, filter, filterParam, _ := s.getAddressQueryParams(r, api.AccountDetailsTxHistoryLight, txsOnPage)
	// do not allow details to be changed by query params
	data := s.newTemplateData(r)
	address, err := s.api.GetAddress(addressParam, page, txsOnPage, api.AccountDetailsTxHistoryLight, filter, strings.ToLower(data.SecondaryCoin))
	if err != nil {
		return errorTpl, nil, err
	}
	data.AddrStr = address.AddrStr
	data.Address = address
	data.Page = address.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(address.Page, address.TotalPages)
	if filterParam == "" && filter.Vout > -1 {
		filterParam = strconv.Itoa(filter.Vout)
	}
	if filterParam != "" {
		data.PageParams = template.URL("&filter=" + filterParam)
		data.Address.Filter = filterParam
	}
	return addressTpl, data, nil
}

func (s *PublicServer) explorerNftDetail(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		return errorTpl, nil, api.NewAPIError("Missing parameters", true)
	}
	tokenId := parts[len(parts)-1]
	contract := parts[len(parts)-2]
	uri, ci, err := s.api.GetEthereumTokenURI(contract, tokenId)
	s.metrics.ExplorerViews.With(common.Labels{"action": "nftDetail"}).Inc()
	if err != nil {
		return errorTpl, nil, api.NewAPIError(err.Error(), true)
	}
	if ci == nil {
		return errorTpl, nil, api.NewAPIError(fmt.Sprintf("Unknown contract %s", contract), true)
	}
	data := s.newTemplateData(r)
	data.TokenId = tokenId
	data.ContractInfo = ci
	data.URI = uri
	return nftDetailTpl, data, nil
}

func (s *PublicServer) explorerXpub(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var xpub string
	i := strings.LastIndex(r.URL.Path, "xpub/")
	if i > 0 {
		xpub = r.URL.Path[i+5:]
	}
	if len(xpub) == 0 {
		return errorTpl, nil, api.NewAPIError("Missing xpub", true)
	}
	s.metrics.ExplorerViews.With(common.Labels{"action": "xpub"}).Inc()
	page, _, _, filter, filterParam, gap := s.getAddressQueryParams(r, api.AccountDetailsTxHistoryLight, txsOnPage)
	// do not allow txsOnPage and details to be changed by query params
	address, err := s.api.GetXpubAddress(xpub, page, txsOnPage, api.AccountDetailsTxHistoryLight, filter, gap)
	if err != nil {
		if err == api.ErrUnsupportedXpub {
			err = api.NewAPIError("XPUB functionality is not supported", true)
		}
		return errorTpl, nil, err
	}
	data := s.newTemplateData(r)
	data.AddrStr = address.AddrStr
	data.Address = address
	data.Page = address.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(address.Page, address.TotalPages)
	if filterParam != "" {
		data.PageParams = template.URL("&filter=" + filterParam)
		data.Address.Filter = filterParam
	}
	data.NonZeroBalanceTokens = filter.TokensToReturn == api.TokensToReturnNonzeroBalance
	return xpubTpl, data, nil
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
	data := s.newTemplateData(r)
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
	data := s.newTemplateData(r)
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
	data := s.newTemplateData(r)
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
		address, err = s.api.GetXpubAddress(q, 0, 1, api.AccountDetailsBasic, &api.AddressFilter{Vout: api.AddressFilterVoutOff}, 0)
		if err == nil {
			http.Redirect(w, r, joinURL("/xpub/", url.QueryEscape(address.AddrStr)), http.StatusFound)
			return noTpl, nil, nil
		}
		block, err = s.api.GetBlock(q, 0, 1)
		if err == nil {
			http.Redirect(w, r, joinURL("/block/", block.Hash), http.StatusFound)
			return noTpl, nil, nil
		}
		tx, err = s.api.GetTransaction(q, false, false)
		if err == nil {
			http.Redirect(w, r, joinURL("/tx/", tx.Txid), http.StatusFound)
			return noTpl, nil, nil
		}
		address, err = s.api.GetAddress(q, 0, 1, api.AccountDetailsBasic, &api.AddressFilter{Vout: api.AddressFilterVoutOff}, "")
		if err == nil {
			http.Redirect(w, r, joinURL("/address/", address.AddrStr), http.StatusFound)
			return noTpl, nil, nil
		}
	}
	return errorTpl, nil, api.NewAPIError(fmt.Sprintf("No matching records found for '%v'", q), true)
}

func (s *PublicServer) explorerSendTx(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "sendtx"}).Inc()
	data := s.newTemplateData(r)
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

func (s *PublicServer) explorerMempool(w http.ResponseWriter, r *http.Request) (tpl, *TemplateData, error) {
	var mempoolTxids *api.MempoolTxids
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "mempool"}).Inc()
	page, ec := strconv.Atoi(r.URL.Query().Get("page"))
	if ec != nil {
		page = 0
	}
	mempoolTxids, err = s.api.GetMempool(page, mempoolTxsOnPage)
	if err != nil {
		return errorTpl, nil, err
	}
	data := s.newTemplateData(r)
	data.MempoolTxids = mempoolTxids
	data.Page = mempoolTxids.Page
	data.PagingRange, data.PrevPage, data.NextPage = getPagingRange(mempoolTxids.Page, mempoolTxids.TotalPages)
	return mempoolTpl, data, nil
}

func getPagingRange(page int, total int) ([]int, int, int) {
	// total==-1 means total is unknown, show only prev/next buttons
	if total >= 0 && total < 2 {
		return nil, 0, 0
	}
	var r []int
	pp, np := page-1, page+1
	if pp < 1 {
		pp = 1
	}
	if total > 0 {
		if np > total {
			np = total
		}
		r = make([]int, 0, 8)
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
	}
	return r, pp, np
}

func (s *PublicServer) apiIndex(r *http.Request, apiVersion int) (interface{}, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-index"}).Inc()
	return s.api.GetSystemInfo(false)
}

func (s *PublicServer) apiBlockIndex(r *http.Request, apiVersion int) (interface{}, error) {
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

func (s *PublicServer) apiTx(r *http.Request, apiVersion int) (interface{}, error) {
	var txid string
	i := strings.LastIndexByte(r.URL.Path, '/')
	if i > 0 {
		txid = r.URL.Path[i+1:]
	}
	if len(txid) == 0 {
		return nil, api.NewAPIError("Missing txid", true)
	}
	var tx *api.Tx
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tx"}).Inc()
	spendingTxs := false
	p := r.URL.Query().Get("spending")
	if len(p) > 0 {
		spendingTxs, err = strconv.ParseBool(p)
		if err != nil {
			return nil, api.NewAPIError("Parameter 'spending' cannot be converted to boolean", true)
		}
	}
	tx, err = s.api.GetTransaction(txid, spendingTxs, false)
	if err == nil && apiVersion == apiV1 {
		return s.api.TxToV1(tx), nil
	}
	return tx, err
}

func (s *PublicServer) apiTxSpecific(r *http.Request, apiVersion int) (interface{}, error) {
	var txid string
	i := strings.LastIndexByte(r.URL.Path, '/')
	if i > 0 {
		txid = r.URL.Path[i+1:]
	}
	if len(txid) == 0 {
		return nil, api.NewAPIError("Missing txid", true)
	}
	var tx json.RawMessage
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tx-specific"}).Inc()
	tx, err = s.chain.GetTransactionSpecific(&bchain.Tx{Txid: txid})
	if err == bchain.ErrTxNotFound {
		return nil, api.NewAPIError(fmt.Sprintf("Transaction '%v' not found", txid), true)
	}
	return tx, err
}

func (s *PublicServer) apiAddress(r *http.Request, apiVersion int) (interface{}, error) {
	var addressParam string
	i := strings.LastIndexByte(r.URL.Path, '/')
	if i > 0 {
		addressParam = r.URL.Path[i+1:]
	}
	if len(addressParam) == 0 {
		return nil, api.NewAPIError("Missing address", true)
	}
	var address *api.Address
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-address"}).Inc()
	page, pageSize, details, filter, _, _ := s.getAddressQueryParams(r, api.AccountDetailsTxidHistory, txsInAPI)
	secondaryCoin := strings.ToLower(r.URL.Query().Get("secondary"))
	address, err = s.api.GetAddress(addressParam, page, pageSize, details, filter, secondaryCoin)
	if err == nil && apiVersion == apiV1 {
		return s.api.AddressToV1(address), nil
	}
	return address, err
}

func (s *PublicServer) apiXpub(r *http.Request, apiVersion int) (interface{}, error) {
	var xpub string
	i := strings.LastIndex(r.URL.Path, "xpub/")
	if i > 0 {
		xpub = r.URL.Path[i+5:]
	}
	if len(xpub) == 0 {
		return nil, api.NewAPIError("Missing xpub", true)
	}
	var address *api.Address
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-xpub"}).Inc()
	page, pageSize, details, filter, _, gap := s.getAddressQueryParams(r, api.AccountDetailsTxidHistory, txsInAPI)
	address, err = s.api.GetXpubAddress(xpub, page, pageSize, details, filter, gap)
	if err == nil && apiVersion == apiV1 {
		return s.api.AddressToV1(address), nil
	}
	if err == api.ErrUnsupportedXpub {
		err = api.NewAPIError("XPUB functionality is not supported", true)
	}
	return address, err
}

func (s *PublicServer) apiUtxo(r *http.Request, apiVersion int) (interface{}, error) {
	var utxo []api.Utxo
	var err error
	if i := strings.LastIndex(r.URL.Path, "utxo/"); i > 0 {
		desc := r.URL.Path[i+5:]
		onlyConfirmed := false
		c := r.URL.Query().Get("confirmed")
		if len(c) > 0 {
			onlyConfirmed, err = strconv.ParseBool(c)
			if err != nil {
				return nil, api.NewAPIError("Parameter 'confirmed' cannot be converted to boolean", true)
			}
		}
		gap, ec := strconv.Atoi(r.URL.Query().Get("gap"))
		if ec != nil {
			gap = 0
		}
		utxo, err = s.api.GetXpubUtxo(desc, onlyConfirmed, gap)
		if err == nil {
			s.metrics.ExplorerViews.With(common.Labels{"action": "api-xpub-utxo"}).Inc()
		} else {
			utxo, err = s.api.GetAddressUtxo(desc, onlyConfirmed)
			s.metrics.ExplorerViews.With(common.Labels{"action": "api-address-utxo"}).Inc()
		}
		if err == nil && apiVersion == apiV1 {
			return s.api.AddressUtxoToV1(utxo), nil
		}
	}
	return utxo, err
}

func (s *PublicServer) apiBalanceHistory(r *http.Request, apiVersion int) (interface{}, error) {
	var history []api.BalanceHistory
	var fromTimestamp, toTimestamp int64
	var err error
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		gap, ec := strconv.Atoi(r.URL.Query().Get("gap"))
		if ec != nil {
			gap = 0
		}
		from := r.URL.Query().Get("from")
		if from != "" {
			fromTimestamp, err = strconv.ParseInt(from, 10, 64)
			if err != nil {
				return history, err
			}
		}
		to := r.URL.Query().Get("to")
		if to != "" {
			toTimestamp, err = strconv.ParseInt(to, 10, 64)
			if err != nil {
				return history, err
			}
		}
		var groupBy uint64
		groupBy, err = strconv.ParseUint(r.URL.Query().Get("groupBy"), 10, 32)
		if err != nil || groupBy == 0 {
			groupBy = 3600
		}
		fiat := r.URL.Query().Get("fiatcurrency")
		var fiatArray []string
		if fiat != "" {
			fiatArray = []string{fiat}
		}
		history, err = s.api.GetXpubBalanceHistory(r.URL.Path[i+1:], fromTimestamp, toTimestamp, fiatArray, gap, uint32(groupBy))
		if err == nil {
			s.metrics.ExplorerViews.With(common.Labels{"action": "api-xpub-balancehistory"}).Inc()
		} else {
			history, err = s.api.GetBalanceHistory(r.URL.Path[i+1:], fromTimestamp, toTimestamp, fiatArray, uint32(groupBy))
			s.metrics.ExplorerViews.With(common.Labels{"action": "api-address-balancehistory"}).Inc()
		}
	}
	return history, err
}

func (s *PublicServer) apiBlock(r *http.Request, apiVersion int) (interface{}, error) {
	var block *api.Block
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-block"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		page, ec := strconv.Atoi(r.URL.Query().Get("page"))
		if ec != nil {
			page = 0
		}
		block, err = s.api.GetBlock(r.URL.Path[i+1:], page, txsInAPI)
		if err == nil && apiVersion == apiV1 {
			return s.api.BlockToV1(block), nil
		}
	}
	return block, err
}

func (s *PublicServer) apiBlockRaw(r *http.Request, apiVersion int) (interface{}, error) {
	var block *api.BlockRaw
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-block-raw"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		block, err = s.api.GetBlockRaw(r.URL.Path[i+1:])
	}
	return block, err
}

func (s *PublicServer) apiFeeStats(r *http.Request, apiVersion int) (interface{}, error) {
	var feeStats *api.FeeStats
	var err error
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-feestats"}).Inc()
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		feeStats, err = s.api.GetFeeStats(r.URL.Path[i+1:])
	}
	return feeStats, err
}

type resultSendTransaction struct {
	Result string `json:"result"`
}

func (s *PublicServer) apiSendTx(r *http.Request, apiVersion int) (interface{}, error) {
	var err error
	var res resultSendTransaction
	var hex string
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-sendtx"}).Inc()
	if r.Method == http.MethodPost {
		data, err := io.ReadAll(r.Body)
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

// apiAvailableVsCurrencies returns a list of available versus currencies
func (s *PublicServer) apiAvailableVsCurrencies(r *http.Request, apiVersion int) (interface{}, error) {
	s.metrics.ExplorerViews.With(common.Labels{"action": "api-tickers-list"}).Inc()
	timestampString := strings.ToLower(r.URL.Query().Get("timestamp"))
	timestamp, err := strconv.ParseInt(timestampString, 10, 64)
	if err != nil {
		return nil, api.NewAPIError("Parameter \"timestamp\" is not a valid Unix timestamp.", true)
	}
	token := strings.ToLower(r.URL.Query().Get("token"))
	result, err := s.api.GetAvailableVsCurrencies(timestamp, token)
	return result, err
}

// apiTickers returns FiatRates ticker prices for the specified block or timestamp.
func (s *PublicServer) apiTickers(r *http.Request, apiVersion int) (interface{}, error) {
	var result *api.FiatTicker
	var err error

	currency := strings.ToLower(r.URL.Query().Get("currency"))
	var currencies []string
	if currency != "" {
		currencies = []string{currency}
	}
	token := strings.ToLower(r.URL.Query().Get("token"))

	if block := r.URL.Query().Get("block"); block != "" {
		// Get tickers for specified block height or block hash
		s.metrics.ExplorerViews.With(common.Labels{"action": "api-tickers-block"}).Inc()
		result, err = s.api.GetFiatRatesForBlockID(block, currencies, token)
	} else if timestampString := r.URL.Query().Get("timestamp"); timestampString != "" {
		// Get tickers for specified timestamp
		s.metrics.ExplorerViews.With(common.Labels{"action": "api-tickers-date"}).Inc()

		timestamp, err := strconv.ParseInt(timestampString, 10, 64)
		if err != nil {
			return nil, api.NewAPIError("Parameter 'timestamp' is not a valid Unix timestamp.", true)
		}

		resultTickers, err := s.api.GetFiatRatesForTimestamps([]int64{timestamp}, currencies, token)
		if err != nil {
			return nil, err
		}
		result = &resultTickers.Tickers[0]
	} else {
		// No parameters - get the latest available ticker
		s.metrics.ExplorerViews.With(common.Labels{"action": "api-tickers-last"}).Inc()
		result, err = s.api.GetCurrentFiatRates(currencies, token)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// apiMultiTickers returns FiatRates ticker prices for the specified comma separated list of timestamps.
func (s *PublicServer) apiMultiTickers(r *http.Request, apiVersion int) (interface{}, error) {
	var result []api.FiatTicker
	var err error

	currency := strings.ToLower(r.URL.Query().Get("currency"))
	var currencies []string
	if currency != "" {
		currencies = []string{currency}
	}
	token := strings.ToLower(r.URL.Query().Get("token"))
	if timestampString := r.URL.Query().Get("timestamp"); timestampString != "" {
		// Get tickers for specified timestamp
		s.metrics.ExplorerViews.With(common.Labels{"action": "api-multi-tickers-date"}).Inc()
		timestamps := strings.Split(timestampString, ",")
		t := make([]int64, len(timestamps))
		for i := range timestamps {
			t[i], err = strconv.ParseInt(timestamps[i], 10, 64)
			if err != nil {
				return nil, api.NewAPIError("Parameter 'timestamp' does not contain a valid Unix timestamp.", true)
			}
		}
		resultTickers, err := s.api.GetFiatRatesForTimestamps(t, currencies, token)
		if err != nil {
			return nil, err
		}
		result = resultTickers.Tickers
	} else {
		return nil, api.NewAPIError("Parameter 'timestamp' is missing.", true)
	}
	return result, nil
}

type resultEstimateFeeAsString struct {
	Result string `json:"result"`
}

func (s *PublicServer) apiEstimateFee(r *http.Request, apiVersion int) (interface{}, error) {
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
