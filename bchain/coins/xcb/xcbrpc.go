package xcb

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/core-coin/go-core/v2"
	xcbcommon "github.com/core-coin/go-core/v2/common"
	"github.com/core-coin/go-core/v2/common/hexutil"
	"github.com/core-coin/go-core/v2/core/types"
	"github.com/core-coin/go-core/v2/rpc"
	"github.com/core-coin/go-core/v2/xcbclient"
	"github.com/cryptohub-digital/blockbook-fork/bchain"
	"github.com/cryptohub-digital/blockbook-fork/common"
	"github.com/golang/glog"
	"github.com/juju/errors"
)

// Network type specifies the type of core-coin network
type Network uint32

const (
	// MainNet is production network
	MainNet Network = 1
	// TestNet is Devin test network
	TestNet Network = 3
)

// Configuration represents json config file
type Configuration struct {
	CoinName                        string `json:"coin_name"`
	CoinShortcut                    string `json:"coin_shortcut"`
	RPCURL                          string `json:"rpc_url"`
	RPCTimeout                      int    `json:"rpc_timeout"`
	BlockAddressesToKeep            int    `json:"block_addresses_to_keep"`
	AddressAliases                  bool   `json:"address_aliases,omitempty"`
	MempoolTxTimeoutHours           int    `json:"mempoolTxTimeoutHours"`
	QueryBackendOnMempoolResync     bool   `json:"queryBackendOnMempoolResync"`
	ProcessInternalTransactions     bool   `json:"processInternalTransactions"`
	ProcessZeroInternalTransactions bool   `json:"processZeroInternalTransactions"`
	ConsensusNodeVersionURL         string `json:"consensusNodeVersion"`
}

// CoreblockchainRPC is an interface to JSON-RPC xcb service.
type CoreblockchainRPC struct {
	*bchain.BaseChain
	Client               CVMClient
	RPC                  CVMRPCClient
	MainNetChainID       Network
	Timeout              time.Duration
	Parser               *CoreCoinParser
	PushHandler          func(bchain.NotificationType)
	OpenRPC              func(string) (CVMRPCClient, CVMClient, error)
	Mempool              *bchain.MempoolCoreCoinType
	mempoolInitialized   bool
	bestHeaderLock       sync.Mutex
	bestHeader           CVMHeader
	bestHeaderTime       time.Time
	NewBlock             CVMNewBlockSubscriber
	newBlockSubscription CVMClientSubscription
	NewTx                CVMNewTxSubscriber
	newTxSubscription    CVMClientSubscription
	ChainConfig          *Configuration
}

// ProcessInternalTransactions specifies if internal transactions are processed
var ProcessInternalTransactions bool

// NewCoreblockchainRPC returns new XcbRPC instance.
func NewCoreblockchainRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
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

	s := &CoreblockchainRPC{
		BaseChain:   &bchain.BaseChain{},
		ChainConfig: &c,
	}

	ProcessInternalTransactions = c.ProcessInternalTransactions

	// overwrite TokenTypeMap with bsc specific token type names
	bchain.TokenTypeMap = []bchain.TokenTypeName{XRC20TokenType, bchain.ERC771TokenType, bchain.ERC1155TokenType}

	// always create parser
	s.Parser = NewCoreCoinParser(c.BlockAddressesToKeep)
	s.Timeout = time.Duration(c.RPCTimeout) * time.Second
	s.PushHandler = pushHandler

	return s, nil
}

// Initialize initializes core coin rpc interface
func (b *CoreblockchainRPC) Initialize() error {
	b.OpenRPC = func(url string) (CVMRPCClient, CVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &CoreCoinRPCClient{Client: r}
		ec := &CoreblockchainClient{Client: xcbclient.NewClient(r)}
		return rc, ec, nil
	}

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = &CoreCoinNewBlock{channel: make(chan *types.Header)}
	b.NewTx = &CoreCoinNewTx{channel: make(chan xcbcommon.Hash)}

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}
	xcbcommon.DefaultNetworkID = xcbcommon.NetworkID(id.Int64())
	// parameters for getInfo request
	switch Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "mainnet"
	case TestNet:
		b.Testnet = true
		b.Network = "devin"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}
	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// CreateMempool creates mempool if not already created, however does not initialize it
func (b *CoreblockchainRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		b.Mempool = bchain.NewMempoolCoreCoinType(chain, b.ChainConfig.MempoolTxTimeoutHours, b.ChainConfig.QueryBackendOnMempoolResync)
		glog.Info("mempool created, MempoolTxTimeoutHours=", b.ChainConfig.MempoolTxTimeoutHours, ", QueryBackendOnMempoolResync=", b.ChainConfig.QueryBackendOnMempoolResync)
	}
	return b.Mempool, nil
}

// InitializeMempool creates subscriptions to newHeads and newPendingTransactions
func (b *CoreblockchainRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
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

func (b *CoreblockchainRPC) subscribeEvents() error {
	// new block notifications handling
	go func() {
		for {
			h, ok := b.NewBlock.Read()
			if !ok {
				break
			}
			b.UpdateBestHeader(h)
			// notify blockbook
			b.PushHandler(bchain.NotificationNewBlock)
		}
	}()

	// new block subscription
	if err := b.subscribe(func() (CVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.RPC.XcbSubscribe(ctx, b.NewBlock.Channel(), "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "XcbSubscribe newHeads")
		}
		b.newBlockSubscription = sub
		glog.Info("Subscribed to newHeads")
		return sub, nil
	}); err != nil {
		return err
	}

	// new mempool transaction notifications handling
	go func() {
		for {
			t, ok := b.NewTx.Read()
			if !ok {
				break
			}
			hex := t.Hex()
			if glog.V(2) {
				glog.Info("rpc: new tx ", hex)
			}
			b.Mempool.AddTransactionToMempool(hex)
			b.PushHandler(bchain.NotificationNewTx)
		}
	}()

	// new mempool transaction subscription
	if err := b.subscribe(func() (CVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newTxSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.RPC.XcbSubscribe(ctx, b.NewTx.Channel(), "newPendingTransactions")
		if err != nil {
			return nil, errors.Annotatef(err, "XcbSubscribe newPendingTransactions")
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
func (b *CoreblockchainRPC) subscribe(f func() (CVMClientSubscription, error)) error {
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

func (b *CoreblockchainRPC) closeRPC() {
	if b.newBlockSubscription != nil {
		b.newBlockSubscription.Unsubscribe()
	}
	if b.newTxSubscription != nil {
		b.newTxSubscription.Unsubscribe()
	}
	if b.RPC != nil {
		b.RPC.Close()
	}
}

func (b *CoreblockchainRPC) reconnectRPC() error {
	glog.Info("Reconnecting RPC")
	b.closeRPC()
	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}
	b.RPC = rc
	b.Client = ec
	return b.subscribeEvents()
}

// Shutdown cleans up rpc interface to xcb
func (b *CoreblockchainRPC) Shutdown(ctx context.Context) error {
	b.closeRPC()
	b.NewBlock.Close()
	b.NewTx.Close()
	glog.Info("rpc: shutdown")
	return nil
}

// GetCoinName returns coin name
func (b *CoreblockchainRPC) GetCoinName() string {
	return b.ChainConfig.CoinName
}

// GetSubversion returns empty string, core coin does not have subversion
func (b *CoreblockchainRPC) GetSubversion() string {
	return ""
}

func (b *CoreblockchainRPC) getConsensusVersion() string {
	if b.ChainConfig.ConsensusNodeVersionURL == "" {
		return ""
	}
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := httpClient.Get(b.ChainConfig.ConsensusNodeVersionURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		glog.Error("getConsensusVersion ", err)
		return ""
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Error("getConsensusVersion ", err)
		return ""
	}
	type consensusVersion struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	var v consensusVersion
	err = json.Unmarshal(body, &v)
	if err != nil {
		glog.Error("getConsensusVersion ", err)
		return ""
	}
	return v.Data.Version
}

// GetChainInfo returns information about the connected backend
func (b *CoreblockchainRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}
	var ver string
	if err := b.RPC.CallContext(ctx, &ver, "web3_clientVersion"); err != nil {
		return nil, err
	}
	consensusVersion := b.getConsensusVersion()
	rv := &bchain.ChainInfo{
		Blocks:           int(h.Number().Int64()),
		Bestblockhash:    h.Hash(),
		Difficulty:       h.Difficulty().String(),
		Version:          ver,
		ConsensusVersion: consensusVersion,
	}
	idi := int(id.Uint64())
	if idi == int(b.MainNetChainID) {
		rv.Chain = "mainnet"
	} else {
		rv.Chain = "testnet " + strconv.Itoa(idi)
	}
	return rv, nil
}

func (b *CoreblockchainRPC) getBestHeader() (CVMHeader, error) {
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
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		b.bestHeader, err = b.Client.HeaderByNumber(ctx, nil)
		if err != nil {
			b.bestHeader = nil
			return nil, err
		}
		b.bestHeaderTime = time.Now()
	}
	return b.bestHeader, nil
}

// UpdateBestHeader keeps track of the latest block header confirmed on chain
func (b *CoreblockchainRPC) UpdateBestHeader(h CVMHeader) {
	glog.V(2).Info("rpc: new block header ", h.Number())
	b.bestHeaderLock.Lock()
	b.bestHeader = h
	b.bestHeaderTime = time.Now()
	b.bestHeaderLock.Unlock()
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
func (b *CoreblockchainRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}
	return h.Hash(), nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *CoreblockchainRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	return uint32(h.Number().Uint64()), nil
}

// GetBlockHash returns hash of block in best-block-chain at given height
func (b *CoreblockchainRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, &n)
	if err != nil {
		if err == core.NotFound {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(err, "height %v", height)
	}
	return h.Hash(), nil
}

func (b *CoreblockchainRPC) xcbHeaderToBlockHeader(h *rpcHeader) (*bchain.BlockHeader, error) {
	height, err := xcbNumber(h.Number)
	if err != nil {
		return nil, err
	}
	c, err := b.computeConfirmations(uint64(height))
	if err != nil {
		return nil, err
	}
	time, err := xcbNumber(h.Time)
	if err != nil {
		return nil, err
	}
	size, err := xcbNumber(h.Size)
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
func (b *CoreblockchainRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	raw, err := b.getBlockRaw(hash, 0, false)
	if err != nil {
		return nil, err
	}
	var h rpcHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	return b.xcbHeaderToBlockHeader(&h)
}

func (b *CoreblockchainRPC) computeConfirmations(n uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bn := bh.Number().Uint64()
	// transaction in the best block has 1 confirmation
	return uint32(bn - n + 1), nil
}

func (b *CoreblockchainRPC) getBlockRaw(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	if hash != "" {
		if hash == "pending" {
			err = b.RPC.CallContext(ctx, &raw, "xcb_getBlockByNumber", hash, fullTxs)
		} else {
			err = b.RPC.CallContext(ctx, &raw, "xcb_getBlockByHash", xcbcommon.HexToHash(hash), fullTxs)
		}
	} else {
		err = b.RPC.CallContext(ctx, &raw, "xcb_getBlockByNumber", fmt.Sprintf("%#x", height), fullTxs)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 || (len(raw) == 4 && string(raw) == "null") {
		return nil, bchain.ErrBlockNotFound
	}
	return raw, nil
}

func (b *CoreblockchainRPC) getXrc20EventsForBlock(blockNumber string) (map[string][]*RpcLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var logs []rpcLogWithTxHash
	err := b.RPC.CallContext(ctx, &logs, "xcb_getLogs", map[string]interface{}{
		"fromBlock": blockNumber,
		"toBlock":   blockNumber,
		"topics":    []string{xrc20TransferEventSignature},
	})
	if err != nil {
		return nil, errors.Annotatef(err, "xcb_getLogs blockNumber %v", blockNumber)
	}
	r := make(map[string][]*RpcLog)
	for i := range logs {
		l := &logs[i]
		r[l.Hash] = append(r[l.Hash], &l.RpcLog)
	}
	return r, nil
}

// GetBlock returns block with given hash or height, hash has precedence if both passed
func (b *CoreblockchainRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
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
	bbh, err := b.xcbHeaderToBlockHeader(&head)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	// get xrc20 events
	logs, err := b.getXrc20EventsForBlock(head.Number)
	if err != nil {
		return nil, err
	}
	btxs := make([]bchain.Tx, len(body.Transactions))
	for i := range body.Transactions {
		tx := &body.Transactions[i]
		btx, err := b.Parser.xcbTxToTx(tx, &RpcReceipt{Logs: logs[tx.Hash]}, bbh.Time, uint32(bbh.Confirmations))
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
func (b *CoreblockchainRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
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
	bch, err := b.xcbHeaderToBlockHeader(&head)
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
func (b *CoreblockchainRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return b.GetTransaction(txid)
}

// GetTransaction returns a transaction by the transaction ID.
func (b *CoreblockchainRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var tx *RpcTransaction
	hash := xcbcommon.HexToHash(txid)
	err := b.RPC.CallContext(ctx, &tx, "xcb_getTransactionByHash", hash)
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
		btx, err = b.Parser.xcbTxToTx(tx, nil, 0, 0)
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
		if time, err = xcbNumber(ht.Time); err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		var receipt RpcReceipt
		err = b.RPC.CallContext(ctx, &receipt, "xcb_getTransactionReceipt", hash)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		n, err := xcbNumber(tx.BlockNumber)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		confirmations, err := b.computeConfirmations(uint64(n))
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		btx, err = b.Parser.xcbTxToTx(tx, &receipt, time, confirmations)
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
func (b *CoreblockchainRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	csd, ok := tx.CoinSpecificData.(CoreCoinSpecificData)
	if !ok {
		ntx, err := b.GetTransaction(tx.Txid)
		if err != nil {
			return nil, err
		}
		csd, ok = ntx.CoinSpecificData.(CoreCoinSpecificData)
		if !ok {
			return nil, errors.New("Cannot get CoinSpecificData")
		}
	}
	m, err := json.Marshal(&csd)
	return json.RawMessage(m), err
}

// GetMempoolTransactions returns transactions in mempool
func (b *CoreblockchainRPC) GetMempoolTransactions() ([]string, error) {
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
func (b *CoreblockchainRPC) EstimateFee(blocks int) (big.Int, error) {
	return b.EstimateSmartFee(blocks, true)
}

// EstimateSmartFee returns fee estimation
func (b *CoreblockchainRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r big.Int
	gp, err := b.Client.SuggestEnergyPrice(ctx)
	if err == nil && b != nil {
		r = *gp
	}
	return r, err
}

// GetStringFromMap attempts to return the value for a specific key in a map as a string if valid,
// otherwise returns an empty string with false indicating there was no key found, or the value was not a string
func GetStringFromMap(p string, params map[string]interface{}) (string, bool) {
	v, ok := params[p]
	if ok {
		s, ok := v.(string)
		return s, ok
	}
	return "", false
}

// CoreCoinTypeEstimateEnergy returns estimation of energy consumption for given transaction parameters
func (b *CoreblockchainRPC) CoreCoinTypeEstimateEnergy(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := core.CallMsg{}
	if s, ok := GetStringFromMap("from", params); ok && len(s) > 0 {
		addr, err := xcbcommon.HexToAddress(s)
		if err != nil {
			return 0, err
		}
		msg.From = addr
	}
	if s, ok := GetStringFromMap("to", params); ok && len(s) > 0 {
		a, err := xcbcommon.HexToAddress(s)
		if err != nil {
			return 0, err
		}
		msg.To = &a
	}
	if s, ok := GetStringFromMap("data", params); ok && len(s) > 0 {
		msg.Data = xcbcommon.FromHex(s)
	}
	if s, ok := GetStringFromMap("value", params); ok && len(s) > 0 {
		msg.Value, _ = hexutil.DecodeBig(s)
	}
	if s, ok := GetStringFromMap("energy", params); ok && len(s) > 0 {
		msg.Energy, _ = hexutil.DecodeUint64(s)
	}
	if s, ok := GetStringFromMap("energyPrice", params); ok && len(s) > 0 {
		msg.EnergyPrice, _ = hexutil.DecodeBig(s)
	}
	return b.Client.EstimateEnergy(ctx, msg)
}

// SendRawTransaction sends raw transaction
func (b *CoreblockchainRPC) SendRawTransaction(hex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	err := b.RPC.CallContext(ctx, &raw, "xcb_sendRawTransaction", hex)
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

// CoreCoinTypeGetBalance returns current balance of an address
func (b *CoreblockchainRPC) CoreCoinTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// CoreCoinTypeGetNonce returns current balance of an address
func (b *CoreblockchainRPC) CoreCoinTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.Client.NonceAt(ctx, addrDesc, nil)
}

// GetChainParser returns core coin BlockChainParser
func (b *CoreblockchainRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
