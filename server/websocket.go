package server

import (
	"encoding/json"
	"math/big"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
)

const upgradeFailed = "Upgrade failed: "
const outChannelSize = 500
const defaultTimeout = 60 * time.Second

// allRates is a special "currency" parameter that means all available currencies
const allFiatRates = "!ALL!"

var (
	// ErrorMethodNotAllowed is returned when client tries to upgrade method other than GET
	ErrorMethodNotAllowed = errors.New("Method not allowed")

	connectionCounter uint64
)

type websocketChannel struct {
	id            uint64
	conn          *websocket.Conn
	out           chan *WsRes
	ip            string
	requestHeader http.Header
	alive         bool
	aliveLock     sync.Mutex
	addrDescs     []string // subscribed address descriptors as strings
}

// WebsocketServer is a handle to websocket server
type WebsocketServer struct {
	upgrader                        *websocket.Upgrader
	db                              *db.RocksDB
	txCache                         *db.TxCache
	chain                           bchain.BlockChain
	chainParser                     bchain.BlockChainParser
	mempool                         bchain.Mempool
	metrics                         *common.Metrics
	is                              *common.InternalState
	api                             *api.Worker
	block0hash                      string
	newBlockSubscriptions           map[*websocketChannel]string
	newBlockSubscriptionsLock       sync.Mutex
	newTransactionEnabled           bool
	newTransactionSubscriptions     map[*websocketChannel]string
	newTransactionSubscriptionsLock sync.Mutex
	addressSubscriptions            map[string]map[*websocketChannel]string
	addressSubscriptionsLock        sync.Mutex
	fiatRatesSubscriptions          map[string]map[*websocketChannel]string
	fiatRatesTokenSubscriptions     map[*websocketChannel][]string
	fiatRatesSubscriptionsLock      sync.Mutex
}

// NewWebsocketServer creates new websocket interface to blockbook and returns its handle
func NewWebsocketServer(db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState, fiatRates *fiat.FiatRates) (*WebsocketServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
	if err != nil {
		return nil, err
	}
	b0, err := db.GetBlockHash(0)
	if err != nil {
		return nil, err
	}
	s := &WebsocketServer{
		upgrader: &websocket.Upgrader{
			ReadBufferSize:  1024 * 32,
			WriteBufferSize: 1024 * 32,
			CheckOrigin:     checkOrigin,
		},
		db:                          db,
		txCache:                     txCache,
		chain:                       chain,
		chainParser:                 chain.GetChainParser(),
		mempool:                     mempool,
		metrics:                     metrics,
		is:                          is,
		api:                         api,
		block0hash:                  b0,
		newBlockSubscriptions:       make(map[*websocketChannel]string),
		newTransactionEnabled:       is.EnableSubNewTx,
		newTransactionSubscriptions: make(map[*websocketChannel]string),
		addressSubscriptions:        make(map[string]map[*websocketChannel]string),
		fiatRatesSubscriptions:      make(map[string]map[*websocketChannel]string),
		fiatRatesTokenSubscriptions: make(map[*websocketChannel][]string),
	}
	return s, nil
}

// allow all origins
func checkOrigin(r *http.Request) bool {
	return true
}

func getIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-Ip")
	if ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// ServeHTTP sets up handler of websocket channel
func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, upgradeFailed+ErrorMethodNotAllowed.Error(), 503)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, upgradeFailed+err.Error(), 503)
		return
	}
	c := &websocketChannel{
		id:            atomic.AddUint64(&connectionCounter, 1),
		conn:          conn,
		out:           make(chan *WsRes, outChannelSize),
		ip:            getIP(r),
		requestHeader: r.Header,
		alive:         true,
	}
	go s.inputLoop(c)
	go s.outputLoop(c)
	s.onConnect(c)
}

// GetHandler returns http handler
func (s *WebsocketServer) GetHandler() http.Handler {
	return s
}

func (s *WebsocketServer) closeChannel(c *websocketChannel) {
	if c.CloseOut() {
		c.conn.Close()
		s.onDisconnect(c)
	}
}

func (c *websocketChannel) CloseOut() bool {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	if c.alive {
		c.alive = false
		//clean out
		close(c.out)
		for len(c.out) > 0 {
			<-c.out
		}
		return true
	}
	return false
}

func (c *websocketChannel) DataOut(data *WsRes) {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	if c.alive {
		if len(c.out) < outChannelSize-1 {
			c.out <- data
		} else {
			glog.Warning("Channel ", c.id, " overflow, closing")
			// close the connection but do not call CloseOut - would call duplicate c.aliveLock.Lock
			// CloseOut will be called because the closed connection will cause break in the inputLoop
			c.conn.Close()
		}
	}
}

func (s *WebsocketServer) inputLoop(c *websocketChannel) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("recovered from panic: ", r, ", ", c.id)
			debug.PrintStack()
			s.closeChannel(c)
		}
	}()
	for {
		t, d, err := c.conn.ReadMessage()
		if err != nil {
			s.closeChannel(c)
			return
		}
		switch t {
		case websocket.TextMessage:
			var req WsReq
			err := json.Unmarshal(d, &req)
			if err != nil {
				glog.Error("Error parsing message from ", c.id, ", ", string(d), ", ", err)
				s.closeChannel(c)
				return
			}
			go s.onRequest(c, &req)
		case websocket.BinaryMessage:
			glog.Error("Binary message received from ", c.id, ", ", c.ip)
			s.closeChannel(c)
			return
		case websocket.PingMessage:
			c.conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(defaultTimeout))
		case websocket.CloseMessage:
			s.closeChannel(c)
			return
		case websocket.PongMessage:
			// do nothing
		}
	}
}

func (s *WebsocketServer) outputLoop(c *websocketChannel) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("recovered from panic: ", r, ", ", c.id)
			s.closeChannel(c)
		}
	}()
	for m := range c.out {
		err := c.conn.WriteJSON(m)
		if err != nil {
			glog.Error("Error sending message to ", c.id, ", ", err)
			s.closeChannel(c)
			return
		}
	}
}

func (s *WebsocketServer) onConnect(c *websocketChannel) {
	glog.Info("Client connected ", c.id, ", ", c.ip)
	s.metrics.WebsocketClients.Inc()
}

func (s *WebsocketServer) onDisconnect(c *websocketChannel) {
	s.unsubscribeNewBlock(c)
	s.unsubscribeNewTransaction(c)
	s.unsubscribeAddresses(c)
	s.unsubscribeFiatRates(c)
	glog.Info("Client disconnected ", c.id, ", ", c.ip)
	s.metrics.WebsocketClients.Dec()
}

var requestHandlers = map[string]func(*WebsocketServer, *websocketChannel, *WsReq) (interface{}, error){
	"getAccountInfo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r, err := unmarshalGetAccountInfoRequest(req.Params)
		if err == nil {
			rv, err = s.getAccountInfo(r)
		}
		return
	},
	"getInfo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.getInfo()
	},
	"getBlockHash": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBlockHashReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getBlockHash(r.Height)
		}
		return
	},
	"getBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		if !s.is.ExtendedIndex {
			return nil, errors.New("Not supported")
		}
		r := WsBlockReq{}
		err = json.Unmarshal(req.Params, &r)
		if r.PageSize == 0 {
			r.PageSize = 1000000
		}
		if err == nil {
			rv, err = s.getBlock(r.Id, r.Page, r.PageSize)
		}
		return
	},
	"getAccountUtxo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsAccountUtxoReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getAccountUtxo(r.Descriptor)
		}
		return
	},
	"getBalanceHistory": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBalanceHistoryReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			if r.From <= 0 {
				r.From = 0
			}
			if r.To <= 0 {
				r.To = 0
			}
			if r.GroupBy <= 0 {
				r.GroupBy = 3600
			}
			rv, err = s.api.GetXpubBalanceHistory(r.Descriptor, r.From, r.To, r.Currencies, r.Gap, r.GroupBy)
			if err != nil {
				rv, err = s.api.GetBalanceHistory(r.Descriptor, r.From, r.To, r.Currencies, r.GroupBy)
			}
		}
		return
	},
	"getTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsTransactionReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getTransaction(r.Txid)
		}
		return
	},
	"getTransactionSpecific": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsTransactionSpecificReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getTransactionSpecific(r.Txid)
		}
		return
	},
	"estimateFee": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.estimateFee(c, req.Params)
	},
	"sendTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsSendTransactionReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.sendTransaction(r.Hex)
		}
		return
	},
	"getMempoolFilters": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsMempoolFiltersReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getMempoolFilters(&r)
		}
		return
	},
	"subscribeNewBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.subscribeNewBlock(c, req)
	},
	"unsubscribeNewBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeNewBlock(c)
	},
	"subscribeNewTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.subscribeNewTransaction(c, req)
	},
	"unsubscribeNewTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeNewTransaction(c)
	},
	"subscribeAddresses": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		ad, err := s.unmarshalAddresses(req.Params)
		if err == nil {
			rv, err = s.subscribeAddresses(c, ad, req)
		}
		return
	},
	"unsubscribeAddresses": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeAddresses(c)
	},
	"subscribeFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		var r WsSubscribeFiatRatesReq
		err = json.Unmarshal(req.Params, &r)
		if err != nil {
			return nil, err
		}
		r.Currency = strings.ToLower(r.Currency)
		for i := range r.Tokens {
			r.Tokens[i] = strings.ToLower(r.Tokens[i])
		}
		return s.subscribeFiatRates(c, &r, req)
	},
	"unsubscribeFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeFiatRates(c)
	},
	"ping": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := struct{}{}
		return r, nil
	},
	"getCurrentFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsCurrentFiatRatesReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getCurrentFiatRates(r.Currencies, r.Token)
		}
		return
	},
	"getFiatRatesForTimestamps": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsFiatRatesForTimestampsReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getFiatRatesForTimestamps(r.Timestamps, r.Currencies, r.Token)
		}
		return
	},
	"getFiatRatesTickersList": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsFiatRatesTickersListReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getAvailableVsCurrencies(r.Timestamp, r.Token)
		}
		return
	},
}

func (s *WebsocketServer) onRequest(c *websocketChannel, req *WsReq) {
	var err error
	var data interface{}
	defer func() {
		if r := recover(); r != nil {
			glog.Error("Client ", c.id, ", onRequest ", req.Method, " recovered from panic: ", r)
			debug.PrintStack()
			e := resultError{}
			e.Error.Message = "Internal error"
			data = e
		}
		// nil data means no response
		if data != nil {
			c.DataOut(&WsRes{
				ID:   req.ID,
				Data: data,
			})
		}
		s.metrics.WebsocketPendingRequests.With((common.Labels{"method": req.Method})).Dec()
	}()
	t := time.Now()
	s.metrics.WebsocketPendingRequests.With((common.Labels{"method": req.Method})).Inc()
	defer s.metrics.WebsocketReqDuration.With(common.Labels{"method": req.Method}).Observe(float64(time.Since(t)) / 1e3) // in microseconds
	f, ok := requestHandlers[req.Method]
	if ok {
		data, err = f(s, c, req)
		if err == nil {
			glog.V(1).Info("Client ", c.id, " onRequest ", req.Method, " success")
			s.metrics.WebsocketRequests.With(common.Labels{"method": req.Method, "status": "success"}).Inc()
		} else {
			if apiErr, ok := err.(*api.APIError); !ok || !apiErr.Public {
				glog.Error("Client ", c.id, " onMessage ", req.Method, ": ", errors.ErrorStack(err), ", data ", string(req.Params))
			}
			s.metrics.WebsocketRequests.With(common.Labels{"method": req.Method, "status": "failure"}).Inc()
			e := resultError{}
			e.Error.Message = err.Error()
			data = e
		}
	} else {
		glog.V(1).Info("Client ", c.id, " onMessage ", req.Method, ": unknown method, data ", string(req.Params))
	}
}

func unmarshalGetAccountInfoRequest(params []byte) (*WsAccountInfoReq, error) {
	var r WsAccountInfoReq
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *WebsocketServer) getAccountInfo(req *WsAccountInfoReq) (res *api.Address, err error) {
	var opt api.AccountDetails
	switch req.Details {
	case "tokens":
		opt = api.AccountDetailsTokens
	case "tokenBalances":
		opt = api.AccountDetailsTokenBalances
	case "txids":
		opt = api.AccountDetailsTxidHistory
	case "txslight":
		opt = api.AccountDetailsTxHistoryLight
	case "txs":
		opt = api.AccountDetailsTxHistory
	default:
		opt = api.AccountDetailsBasic
	}
	var tokensToReturn api.TokensToReturn
	switch req.Tokens {
	case "used":
		tokensToReturn = api.TokensToReturnUsed
	case "nonzero":
		tokensToReturn = api.TokensToReturnNonzeroBalance
	default:
		tokensToReturn = api.TokensToReturnDerived
	}
	filter := api.AddressFilter{
		FromHeight:     uint32(req.FromHeight),
		ToHeight:       uint32(req.ToHeight),
		Contract:       req.ContractFilter,
		Vout:           api.AddressFilterVoutOff,
		TokensToReturn: tokensToReturn,
	}
	if req.PageSize == 0 {
		req.PageSize = txsOnPage
	}
	a, err := s.api.GetXpubAddress(req.Descriptor, req.Page, req.PageSize, opt, &filter, req.Gap, strings.ToLower(req.SecondaryCurrency))
	if err != nil {
		return s.api.GetAddress(req.Descriptor, req.Page, req.PageSize, opt, &filter, strings.ToLower(req.SecondaryCurrency))
	}
	return a, nil
}

func (s *WebsocketServer) getAccountUtxo(descriptor string) (api.Utxos, error) {
	utxo, err := s.api.GetXpubUtxo(descriptor, false, 0)
	if err != nil {
		return s.api.GetAddressUtxo(descriptor, false)
	}
	return utxo, nil
}

func (s *WebsocketServer) getTransaction(txid string) (*api.Tx, error) {
	return s.api.GetTransaction(txid, false, false)
}

func (s *WebsocketServer) getTransactionSpecific(txid string) (interface{}, error) {
	return s.chain.GetTransactionSpecific(&bchain.Tx{Txid: txid})
}

func (s *WebsocketServer) getInfo() (*WsInfoRes, error) {
	vi := common.GetVersionInfo()
	bi := s.is.GetBackendInfo()
	height, hash, err := s.db.GetBestBlock()
	if err != nil {
		return nil, err
	}
	return &WsInfoRes{
		Name:       s.is.Coin,
		Shortcut:   s.is.CoinShortcut,
		Decimals:   s.chainParser.AmountDecimals(),
		BestHeight: int(height),
		BestHash:   hash,
		Version:    vi.Version,
		Block0Hash: s.block0hash,
		Testnet:    s.chain.IsTestnet(),
		Backend: WsBackendInfo{
			Version:          bi.Version,
			Subversion:       bi.Subversion,
			ConsensusVersion: bi.ConsensusVersion,
			Consensus:        bi.Consensus,
		},
	}, nil
}

func (s *WebsocketServer) getBlockHash(height int) (*WsBlockHashRes, error) {
	h, err := s.db.GetBlockHash(uint32(height))
	if err != nil {
		return nil, err
	}
	return &WsBlockHashRes{
		Hash: h,
	}, nil
}

func (s *WebsocketServer) getBlock(id string, page, pageSize int) (interface{}, error) {
	block, err := s.api.GetBlock(id, page, pageSize)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (s *WebsocketServer) estimateFee(c *websocketChannel, params []byte) (interface{}, error) {
	var r WsEstimateFeeReq
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, err
	}
	res := make([]WsEstimateFeeRes, len(r.Blocks))
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		gas, err := s.chain.EthereumTypeEstimateGas(r.Specific)
		if err != nil {
			return nil, err
		}
		sg := strconv.FormatUint(gas, 10)
		b := 1
		if len(r.Blocks) > 0 {
			b = r.Blocks[0]
		}
		fee, err := s.api.EstimateFee(b, true)
		if err != nil {
			return nil, err
		}
		for i := range r.Blocks {
			res[i].FeePerUnit = fee.String()
			res[i].FeeLimit = sg
			fee.Mul(&fee, new(big.Int).SetUint64(gas))
			res[i].FeePerTx = fee.String()
		}
	} else {
		conservative := true
		v, ok := r.Specific["conservative"]
		if ok {
			vc, ok := v.(bool)
			if ok {
				conservative = vc
			}
		}
		txSize := 0
		v, ok = r.Specific["txsize"]
		if ok {
			f, ok := v.(float64)
			if ok {
				txSize = int(f)
			}
		}
		for i, b := range r.Blocks {
			fee, err := s.api.EstimateFee(b, conservative)
			if err != nil {
				return nil, err
			}
			res[i].FeePerUnit = fee.String()
			if txSize > 0 {
				fee.Mul(&fee, big.NewInt(int64(txSize)))
				fee.Add(&fee, big.NewInt(500))
				fee.Div(&fee, big.NewInt(1000))
				res[i].FeePerTx = fee.String()
			}
		}
	}
	return res, nil
}

func (s *WebsocketServer) sendTransaction(tx string) (res resultSendTransaction, err error) {
	txid, err := s.chain.SendRawTransaction(tx)
	if err != nil {
		return res, err
	}
	res.Result = txid
	return
}

func (s *WebsocketServer) getMempoolFilters(r *WsMempoolFiltersReq) (res bchain.MempoolTxidFilterEntries, err error) {
	res, err = s.mempool.GetTxidFilterEntries(r.ScriptType, r.FromTimestamp)
	return
}

type subscriptionResponse struct {
	Subscribed bool `json:"subscribed"`
}
type subscriptionResponseMessage struct {
	Subscribed bool   `json:"subscribed"`
	Message    string `json:"message"`
}

func (s *WebsocketServer) subscribeNewBlock(c *websocketChannel, req *WsReq) (res interface{}, err error) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	s.newBlockSubscriptions[c] = req.ID
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeNewBlock"})).Set(float64(len(s.newBlockSubscriptions)))
	return &subscriptionResponse{true}, nil
}

func (s *WebsocketServer) unsubscribeNewBlock(c *websocketChannel) (res interface{}, err error) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	delete(s.newBlockSubscriptions, c)
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeNewBlock"})).Set(float64(len(s.newBlockSubscriptions)))
	return &subscriptionResponse{false}, nil
}

func (s *WebsocketServer) subscribeNewTransaction(c *websocketChannel, req *WsReq) (res interface{}, err error) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	if !s.newTransactionEnabled {
		return &subscriptionResponseMessage{false, "subscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}, nil
	}
	s.newTransactionSubscriptions[c] = req.ID
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeNewTransaction"})).Set(float64(len(s.newTransactionSubscriptions)))
	return &subscriptionResponse{true}, nil
}

func (s *WebsocketServer) unsubscribeNewTransaction(c *websocketChannel) (res interface{}, err error) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	if !s.newTransactionEnabled {
		return &subscriptionResponseMessage{false, "unsubscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}, nil
	}
	delete(s.newTransactionSubscriptions, c)
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeNewTransaction"})).Set(float64(len(s.newTransactionSubscriptions)))
	return &subscriptionResponse{false}, nil
}

func (s *WebsocketServer) unmarshalAddresses(params []byte) ([]string, error) {
	r := WsSubscribeAddressesReq{}
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(r.Addresses))
	for i, a := range r.Addresses {
		ad, err := s.chainParser.GetAddrDescFromAddress(a)
		if err != nil {
			return nil, err
		}
		rv[i] = string(ad)
	}
	return rv, nil
}

// unsubscribe addresses without addressSubscriptionsLock - can be called only from subscribeAddresses and unsubscribeAddresses
func (s *WebsocketServer) doUnsubscribeAddresses(c *websocketChannel) {
	for _, ads := range c.addrDescs {
		sa, e := s.addressSubscriptions[ads]
		if e {
			for sc := range sa {
				if sc == c {
					delete(sa, c)
				}
			}
			if len(sa) == 0 {
				delete(s.addressSubscriptions, ads)
			}
		}
	}
	c.addrDescs = nil
}

func (s *WebsocketServer) subscribeAddresses(c *websocketChannel, addrDesc []string, req *WsReq) (res interface{}, err error) {
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	// unsubscribe all previous subscriptions
	s.doUnsubscribeAddresses(c)
	for _, ads := range addrDesc {
		as, ok := s.addressSubscriptions[ads]
		if !ok {
			as = make(map[*websocketChannel]string)
			s.addressSubscriptions[ads] = as
		}
		as[c] = req.ID
	}
	c.addrDescs = addrDesc
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeAddresses"})).Set(float64(len(s.addressSubscriptions)))
	return &subscriptionResponse{true}, nil
}

// unsubscribeAddresses unsubscribes all address subscriptions by this channel
func (s *WebsocketServer) unsubscribeAddresses(c *websocketChannel) (res interface{}, err error) {
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	s.doUnsubscribeAddresses(c)
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeAddresses"})).Set(float64(len(s.addressSubscriptions)))
	return &subscriptionResponse{false}, nil
}

// unsubscribe fiat rates without fiatRatesSubscriptionsLock - can be called only from subscribeFiatRates and unsubscribeFiatRates
func (s *WebsocketServer) doUnsubscribeFiatRates(c *websocketChannel) {
	for fr, sa := range s.fiatRatesSubscriptions {
		for sc := range sa {
			if sc == c {
				delete(sa, c)
			}
		}
		if len(sa) == 0 {
			delete(s.fiatRatesSubscriptions, fr)
		}
	}
	delete(s.fiatRatesTokenSubscriptions, c)
}

// subscribeFiatRates subscribes all FiatRates subscriptions by this channel
func (s *WebsocketServer) subscribeFiatRates(c *websocketChannel, d *WsSubscribeFiatRatesReq, req *WsReq) (res interface{}, err error) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	// unsubscribe all previous subscriptions
	s.doUnsubscribeFiatRates(c)
	currency := d.Currency
	if currency == "" {
		currency = allFiatRates
	} else {
		currency = strings.ToLower(currency)
	}
	as, ok := s.fiatRatesSubscriptions[currency]
	if !ok {
		as = make(map[*websocketChannel]string)
		s.fiatRatesSubscriptions[currency] = as
	}
	as[c] = req.ID
	if len(d.Tokens) != 0 {
		s.fiatRatesTokenSubscriptions[c] = d.Tokens
	}
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeFiatRates"})).Set(float64(len(s.fiatRatesSubscriptions)))
	return &subscriptionResponse{true}, nil
}

// unsubscribeFiatRates unsubscribes all FiatRates subscriptions by this channel
func (s *WebsocketServer) unsubscribeFiatRates(c *websocketChannel) (res interface{}, err error) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	s.doUnsubscribeFiatRates(c)
	s.metrics.WebsocketSubscribes.With((common.Labels{"method": "subscribeFiatRates"})).Set(float64(len(s.fiatRatesSubscriptions)))
	return &subscriptionResponse{false}, nil
}

func (s *WebsocketServer) onNewBlockAsync(hash string, height uint32) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	data := struct {
		Height uint32 `json:"height"`
		Hash   string `json:"hash"`
	}{
		Height: height,
		Hash:   hash,
	}
	for c, id := range s.newBlockSubscriptions {
		c.DataOut(&WsRes{
			ID:   id,
			Data: &data,
		})
	}
	glog.Info("broadcasting new block ", height, " ", hash, " to ", len(s.newBlockSubscriptions), " channels")
}

// OnNewBlock is a callback that broadcasts info about new block to subscribed clients
func (s *WebsocketServer) OnNewBlock(hash string, height uint32) {
	go s.onNewBlockAsync(hash, height)
}

func (s *WebsocketServer) sendOnNewTx(tx *api.Tx) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	for c, id := range s.newTransactionSubscriptions {
		c.DataOut(&WsRes{
			ID:   id,
			Data: &tx,
		})
	}
	glog.Info("broadcasting new tx ", tx.Txid, " to ", len(s.newTransactionSubscriptions), " channels")
}

func (s *WebsocketServer) sendOnNewTxAddr(stringAddressDescriptor string, tx *api.Tx) {
	addrDesc := bchain.AddressDescriptor(stringAddressDescriptor)
	addr, _, err := s.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		glog.Error("GetAddressesFromAddrDesc error ", err, " for ", addrDesc)
		return
	}
	if len(addr) == 1 {
		data := struct {
			Address string  `json:"address"`
			Tx      *api.Tx `json:"tx"`
		}{
			Address: addr[0],
			Tx:      tx,
		}
		s.addressSubscriptionsLock.Lock()
		defer s.addressSubscriptionsLock.Unlock()
		as, ok := s.addressSubscriptions[stringAddressDescriptor]
		if ok {
			for c, id := range as {
				c.DataOut(&WsRes{
					ID:   id,
					Data: &data,
				})
			}
			glog.Info("broadcasting new tx ", tx.Txid, ", addr ", addr[0], " to ", len(as), " channels")
		}
	}
}

func (s *WebsocketServer) getNewTxSubscriptions(tx *bchain.MempoolTx) map[string]struct{} {
	// check if there is any subscription in inputs, outputs and token transfers
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	subscribed := make(map[string]struct{})
	for i := range tx.Vin {
		sad := string(tx.Vin[i].AddrDesc)
		if len(sad) > 0 {
			as, ok := s.addressSubscriptions[sad]
			if ok && len(as) > 0 {
				subscribed[sad] = struct{}{}
			}
		}
	}
	for i := range tx.Vout {
		addrDesc, err := s.chainParser.GetAddrDescFromVout(&tx.Vout[i])
		if err == nil && len(addrDesc) > 0 {
			sad := string(addrDesc)
			as, ok := s.addressSubscriptions[sad]
			if ok && len(as) > 0 {
				subscribed[sad] = struct{}{}
			}
		}
	}
	for i := range tx.TokenTransfers {
		addrDesc, err := s.chainParser.GetAddrDescFromAddress(tx.TokenTransfers[i].From)
		if err == nil && len(addrDesc) > 0 {
			sad := string(addrDesc)
			as, ok := s.addressSubscriptions[sad]
			if ok && len(as) > 0 {
				subscribed[sad] = struct{}{}
			}
		}
		addrDesc, err = s.chainParser.GetAddrDescFromAddress(tx.TokenTransfers[i].To)
		if err == nil && len(addrDesc) > 0 {
			sad := string(addrDesc)
			as, ok := s.addressSubscriptions[sad]
			if ok && len(as) > 0 {
				subscribed[sad] = struct{}{}
			}
		}
	}
	return subscribed
}

func (s *WebsocketServer) onNewTxAsync(tx *bchain.MempoolTx, subscribed map[string]struct{}) {
	atx, err := s.api.GetTransactionFromMempoolTx(tx)
	if err != nil {
		glog.Error("GetTransactionFromMempoolTx error ", err, " for ", tx.Txid)
		return
	}
	s.sendOnNewTx(atx)
	for stringAddressDescriptor := range subscribed {
		s.sendOnNewTxAddr(stringAddressDescriptor, atx)
	}
}

// OnNewTx is a callback that broadcasts info about a tx affecting subscribed address
func (s *WebsocketServer) OnNewTx(tx *bchain.MempoolTx) {
	subscribed := s.getNewTxSubscriptions(tx)
	if len(s.newTransactionSubscriptions) > 0 || len(subscribed) > 0 {
		go s.onNewTxAsync(tx, subscribed)
	}
}

func (s *WebsocketServer) broadcastTicker(currency string, rates map[string]float32, ticker *common.CurrencyRatesTicker) {
	as, ok := s.fiatRatesSubscriptions[currency]
	if ok && len(as) > 0 {
		data := struct {
			Rates interface{} `json:"rates"`
		}{
			Rates: rates,
		}
		for c, id := range as {
			var tokens []string
			if ticker != nil {
				tokens = s.fiatRatesTokenSubscriptions[c]
			}
			if len(tokens) > 0 {
				dataWithTokens := struct {
					Rates      interface{}        `json:"rates"`
					TokenRates map[string]float32 `json:"tokenRates,omitempty"`
				}{
					Rates:      rates,
					TokenRates: map[string]float32{},
				}
				for _, token := range tokens {
					rate := ticker.TokenRateInCurrency(token, currency)
					if rate > 0 {
						dataWithTokens.TokenRates[token] = rate
					}
				}
				c.DataOut(&WsRes{
					ID:   id,
					Data: &dataWithTokens,
				})
			} else {
				c.DataOut(&WsRes{
					ID:   id,
					Data: &data,
				})
			}
		}
		glog.Info("broadcasting new rates for currency ", currency, " to ", len(as), " channels")
	}
}

// OnNewFiatRatesTicker is a callback that broadcasts info about fiat rates affecting subscribed currency
func (s *WebsocketServer) OnNewFiatRatesTicker(ticker *common.CurrencyRatesTicker) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	for currency, rate := range ticker.Rates {
		s.broadcastTicker(currency, map[string]float32{currency: rate}, ticker)
	}
	s.broadcastTicker(allFiatRates, ticker.Rates, nil)
}

func (s *WebsocketServer) getCurrentFiatRates(currencies []string, token string) (*api.FiatTicker, error) {
	ret, err := s.api.GetCurrentFiatRates(currencies, strings.ToLower(token))
	return ret, err
}

func (s *WebsocketServer) getFiatRatesForTimestamps(timestamps []int64, currencies []string, token string) (*api.FiatTickers, error) {
	ret, err := s.api.GetFiatRatesForTimestamps(timestamps, currencies, strings.ToLower(token))
	return ret, err
}

func (s *WebsocketServer) getAvailableVsCurrencies(timestamp int64, token string) (*api.AvailableVsCurrencies, error) {
	ret, err := s.api.GetAvailableVsCurrencies(timestamp, strings.ToLower(token))
	return ret, err
}
