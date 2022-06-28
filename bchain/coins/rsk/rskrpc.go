package rsk

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	urlParser "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// EthereumNet type specifies the type of ethereum network
type EthereumNet uint32

const (
	// MainNet is production network
	MainNet EthereumNet = 30
	// TestNet is test network
	TestNet EthereumNet = 31
)

// Configuration represents json config file
type Configuration struct {
	CoinName                    string `json:"coin_name"`
	CoinShortcut                string `json:"coin_shortcut"`
	RPCURL                      string `json:"rpc_url"`
	WSURL                       string `json:"ws_url"`
	RPCTimeout                  int    `json:"rpc_timeout"`
	BlockAddressesToKeep        int    `json:"block_addresses_to_keep"`
	MempoolTxTimeoutHours       int    `json:"mempoolTxTimeoutHours"`
	QueryBackendOnMempoolResync bool   `json:"queryBackendOnMempoolResync"`
}

// EthereumRPC is an interface to JSON-RPC eth service.
type EthereumRPC struct {
	*bchain.BaseChain
	client               *ethclient.Client
	rpc                  *rpc.Client
	wsclient             *ethclient.Client
	wsrpc                *rpc.Client
	timeout              time.Duration
	Parser               *EthereumParser
	Mempool              *bchain.MempoolEthereumType
	mempoolInitialized   bool
	bestHeaderLock       sync.Mutex
	bestHeader           *RSKHeader
	bestHeaderTime       time.Time
	chanNewBlock         chan *RSKHeader
	newBlockSubscription *rpc.ClientSubscription
	chanNewTx            chan ethcommon.Hash
	newTxSubscription    *rpc.ClientSubscription
	ChainConfig          *Configuration
}

// NewRSKRPC returns new RSKRPC instance.
func NewRSKRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	var err error
	var c Configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}
	// keep at least 100 mappings block->addresses to allow rollback
	if c.BlockAddressesToKeep < 100 {
		c.BlockAddressesToKeep = 100
	}

	rc, ec, err := openRPC(c.RPCURL)
	if err != nil {
		return nil, err
	}

	wsrc, wsec, wserr := openRPC(c.WSURL)
	if wserr != nil {
		return nil, wserr
	}

	s := &EthereumRPC{
		BaseChain:   &bchain.BaseChain{},
		client:      ec,
		rpc:         rc,
		ChainConfig: &c,
		wsclient:    wsec,
		wsrpc:       wsrc,
	}

	// always create parser
	s.Parser = NewEthereumParser(c.BlockAddressesToKeep)
	s.timeout = time.Duration(c.RPCTimeout) * time.Second

	// new blocks notifications handling
	// the subscription is done in Initialize
	s.chanNewBlock = make(chan *RSKHeader)
	go func() {
		for {
			h, ok := <-s.chanNewBlock
			if !ok {
				glog.Error("chanNewBlock not ok")
				break
			}
			glog.V(2).Info("rpc: new block header ", h.Number)
			// update best header to the new header
			s.bestHeaderLock.Lock()
			s.bestHeader = h
			s.bestHeaderTime = time.Now()
			s.bestHeaderLock.Unlock()
			// notify blockbook
			pushHandler(bchain.NotificationNewBlock)
		}
	}()

	// new mempool transaction notifications handling
	// the subscription is done in Initialize
	s.chanNewTx = make(chan ethcommon.Hash)
	go func() {
		for {
			t, ok := <-s.chanNewTx
			if !ok {
				glog.Error("chanNewTx not ok")
				break
			}
			hex := t.Hex()
			if glog.V(2) {
				glog.Error("rpc: new tx ", hex)
			}
			s.Mempool.AddTransactionToMempool(hex)
			pushHandler(bchain.NotificationNewTx)
		}
	}()

	return s, nil
}

func openRPC(url string) (*rpc.Client, *ethclient.Client, error) {
	parsedUrl, err := urlParser.Parse(url)
	if err != nil {
		return nil, nil, err
	}

	var rc *rpc.Client
	if parsedUrl.Scheme == "http" || parsedUrl.Scheme == "https" {
		// DisableKeepAlives to fix connection reset issue
		rc, err = rpc.DialHTTPWithClient(url, &http.Client{Transport: &http.Transport{
			DisableKeepAlives: true,
		}})
	} else {
		rc, err = rpc.Dial(url)
	}
	if err != nil {
		return nil, nil, err
	}
	ec := ethclient.NewClient(rc)
	return rc, ec, nil
}

// Initialize initializes ethereum rpc interface
func (b *EthereumRPC) Initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	id, err := b.client.NetworkID(ctx)
	if err != nil {
		return err
	}
	// parameters for getInfo request
	switch EthereumNet(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "livenet"
		break
	case TestNet:
		b.Testnet = true
		b.Network = "testnet"
		break
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	return nil
}

// CreateMempool creates mempool if not already created, however does not initialize it
func (b *EthereumRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		b.Mempool = bchain.NewMempoolEthereumType(chain, b.ChainConfig.MempoolTxTimeoutHours, b.ChainConfig.QueryBackendOnMempoolResync)
		glog.Error("mempool created, MempoolTxTimeoutHours=", b.ChainConfig.MempoolTxTimeoutHours, ", QueryBackendOnMempoolResync=", b.ChainConfig.QueryBackendOnMempoolResync)
	}
	return b.Mempool, nil
}

// InitializeMempool creates subscriptions to newHeads and newPendingTransactions
func (b *EthereumRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Mempool not created")
	}

	// get initial mempool transactions
	txs, err := b.GetMempoolTransactions()
	if err != nil {
		return err
	}

	for _, txid := range txs {
		b.Mempool.AddTransactionToMempool(txid)
	}

	b.Mempool.OnNewTxAddr = onNewTxAddr
	b.Mempool.OnNewTx = onNewTx

	if err = b.subscribeEvents(); err != nil {
		return err
	}

	b.mempoolInitialized = true

	return nil
}

func (b *EthereumRPC) subscribeEvents() error {
	// subscriptions
	if err := b.subscribe(func() (*rpc.ClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		sub, err := b.wsrpc.EthSubscribe(ctx, b.chanNewBlock, "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newHeads")
		}
		b.newBlockSubscription = sub
		glog.Info("Subscribed to newHeads")
		return sub, nil
	}); err != nil {
		return err
	}

	// newPendingTransactions
	if err := b.subscribe(func() (*rpc.ClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newTxSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		sub, err := b.wsrpc.EthSubscribe(ctx, b.chanNewTx, "newPendingTransactions")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newPendingTransactions")
		}
		b.newTxSubscription = sub
		glog.Info("Subscribed to newPendingTransactions")
		return sub, nil
	}); err != nil {
		return err
	}

	return nil
}

// subscribe subscribes notification and tries to resubscribe in case of error
func (b *EthereumRPC) subscribe(f func() (*rpc.ClientSubscription, error)) error {
	s, err := f()
	if err != nil {
		return err
	}
	go func() {
	Loop:
		for {
			// wait for error in subscription
			e := <-s.Err()
			// nil error means sub.Unsubscribe called, exit goroutine
			if e == nil {
				return
			}
			glog.Error("Subscription error ", e)
			timer := time.NewTimer(time.Second * 2)
			// try in 2 second interval to resubscribe
			for {
				select {
				case e = <-s.Err():
					if e == nil {
						return
					}
				case <-timer.C:
					ns, err := f()
					if err == nil {
						// subscription successful, restart wait for next error
						s = ns
						continue Loop
					}
					glog.Error("Resubscribe error ", err)
					timer.Reset(time.Second * 2)
				}
			}
		}
	}()
	return nil
}

func (b *EthereumRPC) closeRPC() {
	if b.newBlockSubscription != nil {
		b.newBlockSubscription.Unsubscribe()
	}
	if b.newTxSubscription != nil {
		b.newTxSubscription.Unsubscribe()
	}
	if b.rpc != nil {
		b.rpc.Close()
	}
	if b.wsrpc != nil {
		b.wsrpc.Close()
	}
}

func (b *EthereumRPC) reconnectRPC() error {
	glog.Info("Reconnecting RPC")
	b.closeRPC()
	rc, ec, err := openRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}
	b.rpc = rc
	b.client = ec

	wsrc, wsec, wserr := openRPC(b.ChainConfig.WSURL)
	if wserr != nil {
		return wserr
	}
	b.wsrpc = wsrc
	b.wsclient = wsec

	return b.subscribeEvents()
}

// Shutdown cleans up rpc interface to ethereum
func (b *EthereumRPC) Shutdown(ctx context.Context) error {
	b.closeRPC()
	close(b.chanNewBlock)
	glog.Info("rpc: shutdown")
	return nil
}

// GetCoinName returns coin name
func (b *EthereumRPC) GetCoinName() string {
	return b.ChainConfig.CoinName
}

// GetSubversion returns empty string, ethereum does not have subversion
func (b *EthereumRPC) GetSubversion() string {
	return ""
}

// GetChainInfo returns information about the connected backend
func (b *EthereumRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	id, err := b.client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}
	var ver string
	if err := b.rpc.CallContext(ctx, &ver, "web3_clientVersion"); err != nil {
		return nil, err
	}
	i, err2 := strconv.ParseUint(stripHex(h.Number), 16, 64)
	if err2 != nil {
		return nil, err2
	}
	rv := &bchain.ChainInfo{
		Blocks:        int(i),
		Bestblockhash: h.Hash.Hex(),
		Difficulty:    h.Difficulty, //strconv.FormatUint(h.Difficulty, 10),
		Version:       ver,
	}
	idi := int(id.Uint64())
	if idi == 1 {
		rv.Chain = "mainnet"
	} else {
		rv.Chain = "testnet " + strconv.Itoa(idi)
	}
	return rv, nil
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	return hexutil.EncodeBig(number)
}

// StringInt create a type alias for type int
type StringInt int

func stripHex(hexaString string) string {
	// replace 0x or 0X with empty String
	numberStr := strings.Replace(hexaString, "0x", "", -1)
	numberStr = strings.Replace(numberStr, "0X", "", -1)
	return numberStr
}

func (st *StringInt) UnmarshalJSON(b string) error {
	value, err := strconv.ParseInt(stripHex(b), 16, 64)
	glog.Error(value)
	if err != nil {
		///the string might not be of integer type
		///so return an error
		return err

	}
	*st = StringInt(value)
	return nil
}

// Header represents a block header in the RSK blockchain.
type RSKHeader struct {
	Hash        ethcommon.Hash    `json:"hash"`
	ParentHash  ethcommon.Hash    `json:"parentHash"`
	UncleHash   ethcommon.Hash    `json:"sha3Uncles"`
	Coinbase    ethcommon.Address `json:"miner"`
	Root        ethcommon.Hash    `json:"stateRoot"`
	TxHash      ethcommon.Hash    `json:"transactionsRoot"`
	ReceiptHash ethcommon.Hash    `json:"receiptsRoot"`
	//Bloom       []byte            `json:"logsBloom"`
	Difficulty string `json:"difficulty"`
	Number     string `json:"number"`
	GasLimit   string `json:"gasLimit"`
	GasUsed    string `json:"gasUsed"`
	Time       string `json:"timestamp"`
	//Extra       []byte            `json:"extraData"`
}

// HeaderByNumber returns a RSK block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func RSKHeaderByNumber(b *EthereumRPC, ctx context.Context, number *big.Int) (*RSKHeader, error) {
	var head *RSKHeader
	err := b.rpc.CallContext(ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return head, err
}

func (b *EthereumRPC) getBestHeader() (*RSKHeader, error) {
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	// if the best header was not updated for 15 minutes, there could be a subscription problem, reconnect RPC
	// do it only in case of normal operation, not initial synchronization
	if b.bestHeaderTime.Add(15*time.Minute).Before(time.Now()) && !b.bestHeaderTime.IsZero() && b.mempoolInitialized {
		err := b.reconnectRPC()
		if err != nil {
			return nil, err
		}
		b.bestHeader = nil
	}
	if b.bestHeader == nil {
		var err error
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		b.bestHeader, err = RSKHeaderByNumber(b, ctx, nil)
		if err != nil {
			b.bestHeader = nil
			return nil, err
		}
		b.bestHeaderTime = time.Now()
	}
	
	return b.bestHeader, nil
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}

	return h.Hash.Hex(), nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}

	i, err2 := strconv.ParseUint(stripHex(h.Number), 16, 64)
	if err2 != nil {
		return 0, err2
	}

	return uint32(i), nil
}

// GetBlockHash returns hash of block in best-block-chain at given height
func (b *EthereumRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	h, err := RSKHeaderByNumber(b, ctx, &n)
	if err != nil {
		if err == ethereum.NotFound {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(err, "height %v", height)
	}
	return h.Hash.Hex(), nil
}

func (b *EthereumRPC) ethHeaderToBlockHeader(h *rpcHeader) (*bchain.BlockHeader, error) {
	height, err := ethNumber(h.Number)
	if err != nil {
		return nil, err
	}
	c, err := b.computeConfirmations(uint64(height))
	if err != nil {
		return nil, err
	}
	time, err := ethNumber(h.Time)
	if err != nil {
		return nil, err
	}
	size, err := ethNumber(h.Size)
	if err != nil {
		return nil, err
	}
	return &bchain.BlockHeader{
		Hash:          h.Hash,
		Prev:          h.ParentHash,
		Height:        uint32(height),
		Confirmations: int(c),
		Time:          time,
		Size:          int(size),
	}, nil
}

// GetBlockHeader returns header of block with given hash
func (b *EthereumRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	raw, err := b.getBlockRaw(hash, 0, false)
	if err != nil {
		return nil, err
	}
	var h rpcHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	return b.ethHeaderToBlockHeader(&h)
}

func (b *EthereumRPC) computeConfirmations(n uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bn := bh.Number
	// transaction in the best block has 1 confirmation
	i, err2 := strconv.ParseUint(stripHex(bn), 16, 64)
	if err2 != nil {
		return 0, err2
	}
	return uint32(i - n + 1), nil
}

func (b *EthereumRPC) getBlockRaw(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	if hash != "" {
		if hash == "pending" {
			err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByNumber", hash, fullTxs)
		} else {
			err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByHash", ethcommon.HexToHash(hash), fullTxs)
		}
	} else {
		err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("%#x", height), fullTxs)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 {
		return nil, bchain.ErrBlockNotFound
	}
	return raw, nil
}

func (b *EthereumRPC) getERC20EventsForBlock(blockNumber string) (map[string][]*rpcLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var logs []rpcLogWithTxHash
	err := b.rpc.CallContext(ctx, &logs, "eth_getLogs", map[string]interface{}{
		"fromBlock": blockNumber,
		"toBlock":   blockNumber,
		"topics":    []string{erc20TransferEventSignature},
	})
	//blockNumber = "0x2c69c4"
	if err != nil {
		return nil, errors.Annotatef(err, "blockNumber %v", blockNumber)
	}
	r := make(map[string][]*rpcLog)
	for i := range logs {
		l := &logs[i]
		r[l.Hash] = append(r[l.Hash], &l.rpcLog)
	}

	return r, nil
}

// GetBlock returns block with given hash or height, hash has precedence if both passed
func (b *EthereumRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	//height = 3369482
	//hash = "0x15c6838e384d348a5eeebffd2e66df06b31abb153134b671dc6a28b7155369ec"
	raw, err := b.getBlockRaw(hash, height, true)
	if err != nil {
		return nil, err
	}
	var head rpcHeader
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	var body rpcBlockTransactions
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	bbh, err := b.ethHeaderToBlockHeader(&head)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	// get ERC20 events
	logs, err := b.getERC20EventsForBlock(head.Number)
	if err != nil {
		return nil, err
	}

	btxs := make([]bchain.Tx, len(body.Transactions))
	for i := range body.Transactions {
		tx := &body.Transactions[i]
		btx, err := b.Parser.ethTxToTx(tx, &rpcReceipt{Logs: logs[tx.Hash]}, bbh.Time, uint32(bbh.Confirmations), true)
		if err != nil {
			return nil, errors.Annotatef(err, "hash %v, height %v, txid %v", hash, height, tx.Hash)
		}
		btxs[i] = *btx
		if b.mempoolInitialized {
			b.Mempool.RemoveTransactionFromMempool(tx.Hash)
		}
	}
	bbk := bchain.Block{
		BlockHeader: *bbh,
		Txs:         btxs,
	}
	return &bbk, nil
}

// GetBlockInfo returns extended header (more info than in bchain.BlockHeader) with a list of txids
func (b *EthereumRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	raw, err := b.getBlockRaw(hash, 0, false)
	if err != nil {
		return nil, err
	}
	var head rpcHeader
	var txs rpcBlockTxids
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if err = json.Unmarshal(raw, &txs); err != nil {
		return nil, err
	}
	bch, err := b.ethHeaderToBlockHeader(&head)
	if err != nil {
		return nil, err
	}
	return &bchain.BlockInfo{
		BlockHeader: *bch,
		Difficulty:  common.JSONNumber(head.Difficulty),
		Nonce:       common.JSONNumber(head.Nonce),
		Txids:       txs.Transactions,
	}, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (b *EthereumRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return b.GetTransaction(txid)
}

// GetTransaction returns a transaction by the transaction ID.
func (b *EthereumRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var tx *rpcTransaction
	hash := ethcommon.HexToHash(txid)
	err := b.rpc.CallContext(ctx, &tx, "eth_getTransactionByHash", hash)
	if err != nil {
		return nil, err
	} else if tx == nil {
		if b.mempoolInitialized {
			b.Mempool.RemoveTransactionFromMempool(txid)
		}
		return nil, bchain.ErrTxNotFound
	}
	var btx *bchain.Tx
	if tx.BlockNumber == "" {
		// mempool tx
		btx, err = b.Parser.ethTxToTx(tx, nil, 0, 0, true)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
	} else {
		// non mempool tx - read the block header to get the block time
		raw, err := b.getBlockRaw(tx.BlockHash, 0, false)
		if err != nil {
			return nil, err
		}
		var ht struct {
			Time string `json:"timestamp"`
		}
		if err := json.Unmarshal(raw, &ht); err != nil {
			return nil, errors.Annotatef(err, "hash %v", hash)
		}
		var time int64
		if time, err = ethNumber(ht.Time); err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		var receipt rpcReceipt
		err = b.rpc.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		n, err := ethNumber(tx.BlockNumber)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		confirmations, err := b.computeConfirmations(uint64(n))
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		btx, err = b.Parser.ethTxToTx(tx, &receipt, time, confirmations, true)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		// remove tx from mempool if it is there
		if b.mempoolInitialized {
			b.Mempool.RemoveTransactionFromMempool(txid)
		}
	}
	return btx, nil
}

// GetTransactionSpecific returns json as returned by backend, with all coin specific data
func (b *EthereumRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	csd, ok := tx.CoinSpecificData.(completeTransaction)
	if !ok {
		ntx, err := b.GetTransaction(tx.Txid)
		if err != nil {
			return nil, err
		}
		csd, ok = ntx.CoinSpecificData.(completeTransaction)
		if !ok {
			return nil, errors.New("Cannot get CoinSpecificData")
		}
	}
	m, err := json.Marshal(&csd)
	return json.RawMessage(m), err
}

// GetMempoolTransactions returns transactions in mempool
func (b *EthereumRPC) GetMempoolTransactions() ([]string, error) {
	raw, err := b.getBlockRaw("pending", 0, false)
	if err != nil {
		return nil, err
	}
	var body rpcBlockTxids
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
	}
	return body.Transactions, nil
}

// EstimateFee returns fee estimation
func (b *EthereumRPC) EstimateFee(blocks int) (big.Int, error) {
	return b.EstimateSmartFee(blocks, true)
}

// EstimateSmartFee returns fee estimation
func (b *EthereumRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var r big.Int
	gp, err := b.client.SuggestGasPrice(ctx)
	if err == nil && b != nil {
		r = *gp
	}
	return r, err
}

func getStringFromMap(p string, params map[string]interface{}) (string, bool) {
	v, ok := params[p]
	if ok {
		s, ok := v.(string)
		return s, ok
	}
	return "", false
}

// EthereumTypeEstimateGas returns estimation of gas consumption for given transaction parameters
func (b *EthereumRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	msg := ethereum.CallMsg{}
	s, ok := getStringFromMap("from", params)
	if ok && len(s) > 0 {
		msg.From = ethcommon.HexToAddress(s)
	}
	s, ok = getStringFromMap("to", params)
	if ok && len(s) > 0 {
		a := ethcommon.HexToAddress(s)
		msg.To = &a
	}
	s, ok = getStringFromMap("data", params)
	if ok && len(s) > 0 {
		msg.Data = ethcommon.FromHex(s)
	}
	s, ok = getStringFromMap("value", params)
	if ok && len(s) > 0 {
		msg.Value, _ = hexutil.DecodeBig(s)
	}
	s, ok = getStringFromMap("gas", params)
	if ok && len(s) > 0 {
		msg.Gas, _ = hexutil.DecodeUint64(s)
	}
	s, ok = getStringFromMap("gasPrice", params)
	if ok && len(s) > 0 {
		msg.GasPrice, _ = hexutil.DecodeBig(s)
	}
	return b.client.EstimateGas(ctx, msg)
}

// SendRawTransaction sends raw transaction
func (b *EthereumRPC) SendRawTransaction(hex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var raw json.RawMessage
	err := b.rpc.CallContext(ctx, &raw, "eth_sendRawTransaction", hex)
	if err != nil {
		return "", err
	} else if len(raw) == 0 {
		return "", errors.New("SendRawTransaction: failed")
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.Annotatef(err, "raw result %v", raw)
	}
	if result == "" {
		return "", errors.New("SendRawTransaction: failed, empty result")
	}
	return result, nil
}

// EthereumTypeGetBalance returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	return b.client.BalanceAt(ctx, ethcommon.BytesToAddress(addrDesc), nil)
}

// EthereumTypeGetNonce returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	return b.client.NonceAt(ctx, ethcommon.BytesToAddress(addrDesc), nil)
}

// GetChainParser returns ethereum BlockChainParser
func (b *EthereumRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
