package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// Network type specifies the type of ethereum network
type Network uint32

const (
	// MainNet is production network
	MainNet Network = 1
	// TestNetSepolia is Sepolia test network
	TestNetSepolia Network = 11155111
	// TestNetHolesky is Holesky test network
	TestNetHolesky Network = 17000
	// TestNetHoodi is Hoodi test network
	TestNetHoodi Network = 560048
)

// Configuration represents json config file
type Configuration struct {
	CoinName                        string `json:"coin_name"`
	CoinShortcut                    string `json:"coin_shortcut"`
	Network                         string `json:"network"`
	RPCURL                          string `json:"rpc_url"`
	RPCTimeout                      int    `json:"rpc_timeout"`
	BlockAddressesToKeep            int    `json:"block_addresses_to_keep"`
	AddressAliases                  bool   `json:"address_aliases,omitempty"`
	MempoolTxTimeoutHours           int    `json:"mempoolTxTimeoutHours"`
	QueryBackendOnMempoolResync     bool   `json:"queryBackendOnMempoolResync"`
	ProcessInternalTransactions     bool   `json:"processInternalTransactions"`
	ProcessZeroInternalTransactions bool   `json:"processZeroInternalTransactions"`
	ConsensusNodeVersionURL         string `json:"consensusNodeVersion"`
	DisableMempoolSync              bool   `json:"disableMempoolSync,omitempty"`
	Eip1559Fees                     bool   `json:"eip1559Fees,omitempty"`
	AlternativeEstimateFee          string `json:"alternative_estimate_fee,omitempty"`
	AlternativeEstimateFeeParams    string `json:"alternative_estimate_fee_params,omitempty"`
}

// EthereumRPC is an interface to JSON-RPC eth service.
type EthereumRPC struct {
	*bchain.BaseChain
	Client                    bchain.EVMClient
	RPC                       bchain.EVMRPCClient
	MainNetChainID            Network
	Timeout                   time.Duration
	Parser                    *EthereumParser
	PushHandler               func(bchain.NotificationType)
	OpenRPC                   func(string) (bchain.EVMRPCClient, bchain.EVMClient, error)
	Mempool                   *bchain.MempoolEthereumType
	mempoolInitialized        bool
	bestHeaderLock            sync.Mutex
	bestHeader                bchain.EVMHeader
	bestHeaderTime            time.Time
	NewBlock                  bchain.EVMNewBlockSubscriber
	newBlockSubscription      bchain.EVMClientSubscription
	NewTx                     bchain.EVMNewTxSubscriber
	newTxSubscription         bchain.EVMClientSubscription
	ChainConfig               *Configuration
	supportedStakingPools     []string
	stakingPoolNames          []string
	stakingPoolContracts      []string
	alternativeFeeProvider    alternativeFeeProviderInterface
	alternativeSendTxProvider *AlternativeSendTxProvider
}

// ProcessInternalTransactions specifies if internal transactions are processed
var ProcessInternalTransactions bool

// NewEthereumRPC returns new EthRPC instance.
func NewEthereumRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
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

	s := &EthereumRPC{
		BaseChain:   &bchain.BaseChain{},
		ChainConfig: &c,
	}

	ProcessInternalTransactions = c.ProcessInternalTransactions

	// always create parser
	s.Parser = NewEthereumParser(c.BlockAddressesToKeep, c.AddressAliases)
	s.Timeout = time.Duration(c.RPCTimeout) * time.Second
	s.PushHandler = pushHandler

	return s, nil
}

// OpenRPC opens RPC connection to ETH backend
var OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
	opts := []rpc.ClientOption{}
	opts = append(opts, rpc.WithWebsocketMessageSizeLimit(0))
	r, err := rpc.DialOptions(context.Background(), url, opts...)
	if err != nil {
		return nil, nil, err
	}
	rc := &EthereumRPCClient{Client: r}
	ec := &EthereumClient{Client: ethclient.NewClient(r)}
	return rc, ec, nil
}

// Initialize initializes ethereum rpc interface
func (b *EthereumRPC) Initialize() error {
	b.OpenRPC = OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = &EthereumNewBlock{channel: make(chan *types.Header)}
	b.NewTx = &EthereumNewTx{channel: make(chan ethcommon.Hash)}

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "livenet"
	case TestNetSepolia:
		b.Testnet = true
		b.Network = "sepolia"
	case TestNetHolesky:
		b.Testnet = true
		b.Network = "holesky"
	case TestNetHoodi:
		b.Testnet = true
		b.Network = "hoodi"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	err = b.initStakingPools()
	if err != nil {
		return err
	}

	b.InitAlternativeProviders()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

// InitAlternativeProviders initializes alternative providers
func (b *EthereumRPC) InitAlternativeProviders() {
	b.initAlternativeFeeProvider()

	network := b.ChainConfig.Network
	if network == "" {
		network = b.ChainConfig.CoinShortcut
	}
	b.alternativeSendTxProvider = NewAlternativeSendTxProvider(network, b.ChainConfig.RPCTimeout, b.ChainConfig.MempoolTxTimeoutHours)
}

// CreateMempool creates mempool if not already created, however does not initialize it
func (b *EthereumRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		b.Mempool = bchain.NewMempoolEthereumType(chain, b.ChainConfig.MempoolTxTimeoutHours, b.ChainConfig.QueryBackendOnMempoolResync)
		glog.Info("mempool created, MempoolTxTimeoutHours=", b.ChainConfig.MempoolTxTimeoutHours, ", QueryBackendOnMempoolResync=", b.ChainConfig.QueryBackendOnMempoolResync, ", DisableMempoolSync=", b.ChainConfig.DisableMempoolSync)
		if b.alternativeSendTxProvider != nil {
			b.alternativeSendTxProvider.SetupMempool(b.Mempool, b.removeTransactionFromMempool)
		}

	}
	return b.Mempool, nil
}

// InitializeMempool creates subscriptions to newHeads and newPendingTransactions
func (b *EthereumRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Mempool not created")
	}

	var err error
	var txs []string
	// get initial mempool transactions
	// workaround for an occasional `decoding block` error from getBlockRaw - try 3 times with a delay and then proceed
	for i := 0; i < 3; i++ {
		txs, err = b.GetMempoolTransactions()
		if err == nil {
			break
		}
		glog.Error("GetMempoolTransaction ", err)
		time.Sleep(time.Second * 5)
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
	if err := b.subscribe(func() (bchain.EVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.RPC.EthSubscribe(ctx, b.NewBlock.Channel(), "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newHeads")
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
			added := b.Mempool.AddTransactionToMempool(hex)
			if added {
				b.PushHandler(bchain.NotificationNewTx)
			}
		}
	}()

	if !b.ChainConfig.DisableMempoolSync {
		// new mempool transaction subscription
		if err := b.subscribe(func() (bchain.EVMClientSubscription, error) {
			// invalidate the previous subscription - it is either the first one or there was an error
			b.newTxSubscription = nil
			ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
			defer cancel()
			sub, err := b.RPC.EthSubscribe(ctx, b.NewTx.Channel(), "newPendingTransactions")
			if err != nil {
				return nil, errors.Annotatef(err, "EthSubscribe newPendingTransactions")
			}
			b.newTxSubscription = sub
			glog.Info("Subscribed to newPendingTransactions")
			return sub, nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// subscribe subscribes notification and tries to resubscribe in case of error
func (b *EthereumRPC) subscribe(f func() (bchain.EVMClientSubscription, error)) error {
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

func (b *EthereumRPC) initAlternativeFeeProvider() {
	var err error
	if b.ChainConfig.AlternativeEstimateFee == "1inch" {
		if b.alternativeFeeProvider, err = NewOneInchFeesProvider(b, b.ChainConfig.AlternativeEstimateFeeParams); err != nil {
			glog.Error("New1InchFeesProvider error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	} else if b.ChainConfig.AlternativeEstimateFee == "infura" {
		if b.alternativeFeeProvider, err = NewInfuraFeesProvider(b, b.ChainConfig.AlternativeEstimateFeeParams); err != nil {
			glog.Error("NewInfuraFeesProvider error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	}
	if b.alternativeFeeProvider != nil {
		glog.Info("Using alternative fee provider ", b.ChainConfig.AlternativeEstimateFee)
	}

}

func (b *EthereumRPC) closeRPC() {
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

func (b *EthereumRPC) reconnectRPC() error {
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

// Shutdown cleans up rpc interface to ethereum
func (b *EthereumRPC) Shutdown(ctx context.Context) error {
	b.closeRPC()
	b.NewBlock.Close()
	b.NewTx.Close()
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

func (b *EthereumRPC) getConsensusVersion() string {
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
	body, err := io.ReadAll(resp.Body)
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
func (b *EthereumRPC) GetChainInfo() (*bchain.ChainInfo, error) {
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

func (b *EthereumRPC) getBestHeader() (bchain.EVMHeader, error) {
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
func (b *EthereumRPC) UpdateBestHeader(h bchain.EVMHeader) {
	glog.V(2).Info("rpc: new block header ", h.Number().Uint64())
	b.bestHeaderLock.Lock()
	b.bestHeader = h
	b.bestHeaderTime = time.Now()
	b.bestHeaderLock.Unlock()
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}
	return h.Hash(), nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	return uint32(h.Number().Uint64()), nil
}

// GetBlockHash returns hash of block in best-block-chain at given height
func (b *EthereumRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, &n)
	if err != nil {
		if err == ethereum.NotFound {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(err, "height %v", height)
	}
	return h.Hash(), nil
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
	bn := bh.Number().Uint64()
	// transaction in the best block has 1 confirmation
	return uint32(bn - n + 1), nil
}

func (b *EthereumRPC) getBlockRaw(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	if hash != "" {
		if hash == "pending" {
			err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByNumber", hash, fullTxs)
		} else {
			err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByHash", ethcommon.HexToHash(hash), fullTxs)
		}
	} else {
		err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("%#x", height), fullTxs)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 || (len(raw) == 4 && string(raw) == "null") {
		return nil, bchain.ErrBlockNotFound
	}
	return raw, nil
}

func (b *EthereumRPC) processEventsForBlock(blockNumber string) (map[string][]*bchain.RpcLog, []bchain.AddressAliasRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var logs []rpcLogWithTxHash
	var ensRecords []bchain.AddressAliasRecord
	err := b.RPC.CallContext(ctx, &logs, "eth_getLogs", map[string]interface{}{
		"fromBlock": blockNumber,
		"toBlock":   blockNumber,
	})
	if err != nil {
		return nil, nil, errors.Annotatef(err, "eth_getLogs blockNumber %v", blockNumber)
	}
	r := make(map[string][]*bchain.RpcLog)
	for i := range logs {
		l := &logs[i]
		r[l.Hash] = append(r[l.Hash], &l.RpcLog)
		ens := getEnsRecord(l)
		if ens != nil {
			ensRecords = append(ensRecords, *ens)
		}
	}
	return r, ensRecords, nil
}

type rpcCallTrace struct {
	// CREATE, CREATE2, SELFDESTRUCT, CALL, CALLCODE, DELEGATECALL, STATICCALL
	Type   string         `json:"type"`
	From   string         `json:"from"`
	To     string         `json:"to"`
	Value  string         `json:"value"`
	Error  string         `json:"error"`
	Output string         `json:"output"`
	Calls  []rpcCallTrace `json:"calls"`
}

type rpcTraceResult struct {
	Result rpcCallTrace `json:"result"`
}

func (b *EthereumRPC) getCreationContractInfo(contract string, height uint32) *bchain.ContractInfo {
	// do not fetch fetchContractInfo in sync, it slows it down
	// the contract will be fetched only when asked by a client
	// ci, err := b.fetchContractInfo(contract)
	// if ci == nil || err != nil {
	ci := &bchain.ContractInfo{
		Contract: contract,
	}
	// }
	ci.Standard = bchain.UnhandledTokenStandard
	ci.Type = bchain.UnhandledTokenStandard
	ci.CreatedInBlock = height
	return ci
}

func (b *EthereumRPC) processCallTrace(call *rpcCallTrace, d *bchain.EthereumInternalData, contracts []bchain.ContractInfo, blockHeight uint32) []bchain.ContractInfo {
	value, err := hexutil.DecodeBig(call.Value)
	if err != nil {
		value = new(big.Int)
	}
	if call.Type == "CREATE" || call.Type == "CREATE2" {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Type:  bchain.CREATE,
			Value: *value,
			From:  call.From,
			To:    call.To, // new contract address
		})
		contracts = append(contracts, *b.getCreationContractInfo(call.To, blockHeight))
	} else if call.Type == "SELFDESTRUCT" {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Type:  bchain.SELFDESTRUCT,
			Value: *value,
			From:  call.From, // destroyed contract address
			To:    call.To,
		})
		contracts = append(contracts, bchain.ContractInfo{Contract: call.From, DestructedInBlock: blockHeight})
	} else if call.Type == "DELEGATECALL" {
		// ignore DELEGATECALL (geth v1.11 the changed tracer behavior)
		// 	https://github.com/ethereum/go-ethereum/issues/26726
	} else if err == nil && (value.BitLen() > 0 || b.ChainConfig.ProcessZeroInternalTransactions) {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Value: *value,
			From:  call.From,
			To:    call.To,
		})
	}
	if call.Error != "" {
		d.Error = call.Error
	}
	for i := range call.Calls {
		contracts = b.processCallTrace(&call.Calls[i], d, contracts, blockHeight)
	}
	return contracts
}

// getInternalDataForBlock fetches debug trace using callTracer, extracts internal transfers and creations and destructions of contracts
func (b *EthereumRPC) getInternalDataForBlock(blockHash string, blockHeight uint32, transactions []bchain.RpcTransaction) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {
	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)
	if ProcessInternalTransactions {
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		var trace []rpcTraceResult
		err := b.RPC.CallContext(ctx, &trace, "debug_traceBlockByHash", blockHash, map[string]interface{}{"tracer": "callTracer"})
		if err != nil {
			glog.Error("debug_traceBlockByHash block ", blockHash, ", error ", err)
			return data, contracts, err
		}
		if len(trace) != len(data) {
			if len(trace) < len(data) {
				for i := range transactions {
					tx := &transactions[i]
					// bridging transactions in Polygon do not create trace and cause mismatch between the trace size and block size, it is necessary to adjust the trace size
					// bridging transaction that from and to zero address
					if tx.To == "0x0000000000000000000000000000000000000000" && tx.From == "0x0000000000000000000000000000000000000000" {
						if i >= len(trace) {
							trace = append(trace, rpcTraceResult{})
						} else {
							trace = append(trace[:i+1], trace[i:]...)
							trace[i] = rpcTraceResult{}
						}
					}
				}
			}
			if len(trace) != len(data) {
				e := fmt.Sprint("trace length does not match block length ", len(trace), "!=", len(data))
				glog.Error("debug_traceBlockByHash block ", blockHash, ", error: ", e)
				return data, contracts, errors.New(e)
			} else {
				glog.Warning("debug_traceBlockByHash block ", blockHash, ", trace adjusted to match the number of transactions in block")
			}
		}
		for i, result := range trace {
			r := &result.Result
			d := &data[i]
			if r.Type == "CREATE" || r.Type == "CREATE2" {
				d.Type = bchain.CREATE
				d.Contract = r.To
				contracts = append(contracts, *b.getCreationContractInfo(d.Contract, blockHeight))
			} else if r.Type == "SELFDESTRUCT" {
				d.Type = bchain.SELFDESTRUCT
			}
			for j := range r.Calls {
				contracts = b.processCallTrace(&r.Calls[j], d, contracts, blockHeight)
			}
			if r.Error != "" {
				baseError := PackInternalTransactionError(r.Error)
				if len(baseError) > 1 {
					// n, _ := ethNumber(transactions[i].BlockNumber)
					// glog.Infof("Internal Data Error %d %s: unknown base error %s", n, transactions[i].Hash, baseError)
					baseError = strings.ToUpper(baseError[:1]) + baseError[1:] + ". "
				}
				outputError := ParseErrorFromOutput(r.Output)
				if len(outputError) > 0 {
					d.Error = baseError + strings.ToUpper(outputError[:1]) + outputError[1:]
				} else {
					traceError := PackInternalTransactionError(d.Error)
					if traceError == baseError {
						d.Error = baseError
					} else {
						d.Error = baseError + traceError
					}
				}
				// n, _ := ethNumber(transactions[i].BlockNumber)
				// glog.Infof("Internal Data Error %d %s: %s", n, transactions[i].Hash, UnpackInternalTransactionError([]byte(d.Error)))
			}
		}
	}
	return data, contracts, nil
}

// GetBlock returns block with given hash or height, hash has precedence if both passed
func (b *EthereumRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
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
	// get block events
	// TODO - could be possibly done in parallel to getInternalDataForBlock
	logs, ens, err := b.processEventsForBlock(head.Number)
	if err != nil {
		return nil, err
	}
	// error fetching internal data does not stop the block processing
	var blockSpecificData *bchain.EthereumBlockSpecificData
	internalData, contracts, err := b.getInternalDataForBlock(head.Hash, bbh.Height, body.Transactions)
	// pass internalData error and ENS records in blockSpecificData to be stored
	if err != nil || len(ens) > 0 || len(contracts) > 0 {
		blockSpecificData = &bchain.EthereumBlockSpecificData{}
		if err != nil {
			blockSpecificData.InternalDataError = err.Error()
			// glog.Info("InternalDataError ", bbh.Height, ": ", err.Error())
		}
		if len(ens) > 0 {
			blockSpecificData.AddressAliasRecords = ens
			// glog.Info("ENS", ens)
		}
		if len(contracts) > 0 {
			blockSpecificData.Contracts = contracts
			// glog.Info("Contracts", contracts)
		}
	}

	btxs := make([]bchain.Tx, len(body.Transactions))
	for i := range body.Transactions {
		tx := &body.Transactions[i]
		btx, err := b.Parser.ethTxToTx(tx, &bchain.RpcReceipt{Logs: logs[tx.Hash]}, &internalData[i], bbh.Time, uint32(bbh.Confirmations), true)
		if err != nil {
			return nil, errors.Annotatef(err, "hash %v, height %v, txid %v", hash, height, tx.Hash)
		}
		btxs[i] = *btx
		b.removeTransactionFromMempool(tx.Hash)
	}
	bbk := bchain.Block{
		BlockHeader:      *bbh,
		Txs:              btxs,
		CoinSpecificData: blockSpecificData,
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

func (b *EthereumRPC) removeTransactionFromMempool(txid string) {
	// remove tx from mempool
	if b.mempoolInitialized {
		b.Mempool.RemoveTransactionFromMempool(txid)
	}
	// remove tx from mempool txs fetched by alternative method
	if b.alternativeSendTxProvider != nil {
		b.alternativeSendTxProvider.RemoveTransaction(txid)
	}
}

// GetTransaction returns a transaction by the transaction ID.
func (b *EthereumRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var tx *bchain.RpcTransaction
	var txFound bool
	var err error
	hash := ethcommon.HexToHash(txid)
	if b.alternativeSendTxProvider != nil {
		tx, txFound = b.alternativeSendTxProvider.GetTransaction(txid)
	}
	if !txFound {
		tx = &bchain.RpcTransaction{}
		err = b.RPC.CallContext(ctx, tx, "eth_getTransactionByHash", hash)
		if err != nil {
			return nil, err
		}
	}
	if *tx == (bchain.RpcTransaction{}) {
		b.removeTransactionFromMempool(txid)
		return nil, bchain.ErrTxNotFound
	}
	var btx *bchain.Tx
	if tx.BlockNumber == "" {
		// mempool tx
		btx, err = b.Parser.ethTxToTx(tx, nil, nil, 0, 0, true)
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
			Time          string `json:"timestamp"`
			BaseFeePerGas string `json:"baseFeePerGas"`
		}
		if err := json.Unmarshal(raw, &ht); err != nil {
			return nil, errors.Annotatef(err, "hash %v", hash)
		}
		var time int64
		if time, err = ethNumber(ht.Time); err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		tx.BaseFeePerGas = ht.BaseFeePerGas
		var receipt bchain.RpcReceipt
		err = b.RPC.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash)
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
		btx, err = b.Parser.ethTxToTx(tx, &receipt, nil, time, confirmations, true)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		b.removeTransactionFromMempool(txid)
	}
	return btx, nil
}

// GetTransactionSpecific returns json as returned by backend, with all coin specific data
func (b *EthereumRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		ntx, err := b.GetTransaction(tx.Txid)
		if err != nil {
			return nil, err
		}
		csd, ok = ntx.CoinSpecificData.(bchain.EthereumSpecificData)
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
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r big.Int
	gp, err := b.Client.SuggestGasPrice(ctx)
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

// EthereumTypeEstimateGas returns estimation of gas consumption for given transaction parameters
func (b *EthereumRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := ethereum.CallMsg{}
	if s, ok := GetStringFromMap("from", params); ok && len(s) > 0 {
		msg.From = ethcommon.HexToAddress(s)
	}
	if s, ok := GetStringFromMap("to", params); ok && len(s) > 0 {
		a := ethcommon.HexToAddress(s)
		msg.To = &a
	}
	if s, ok := GetStringFromMap("data", params); ok && len(s) > 0 {
		msg.Data = ethcommon.FromHex(s)
	}
	if s, ok := GetStringFromMap("value", params); ok && len(s) > 0 {
		msg.Value, _ = hexutil.DecodeBig(s)
	}
	if s, ok := GetStringFromMap("gas", params); ok && len(s) > 0 {
		msg.Gas, _ = hexutil.DecodeUint64(s)
	}
	if s, ok := GetStringFromMap("gasPrice", params); ok && len(s) > 0 {
		msg.GasPrice, _ = hexutil.DecodeBig(s)
	}

	if b.alternativeSendTxProvider != nil {
		result, err := b.alternativeSendTxProvider.callHttpStringResult(
			b.alternativeSendTxProvider.urls[0],
			"eth_estimateGas",
			params,
		)
		if err == nil {
			return hexutil.DecodeUint64(result)
		}
	}
	return b.Client.EstimateGas(ctx, msg)
}

// EthereumTypeGetEip1559Fees retrieves Eip1559Fees, if supported
func (b *EthereumRPC) EthereumTypeGetEip1559Fees() (*bchain.Eip1559Fees, error) {
	if !b.ChainConfig.Eip1559Fees {
		return nil, nil
	}
	// if there is an alternative provider, use it
	if b.alternativeFeeProvider != nil {
		return b.alternativeFeeProvider.GetEip1559Fees()
	}

	// otherwise use algorithm from here https://docs.alchemy.com/docs/how-to-build-a-gas-fee-estimator-using-eip-1559
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	var maxPriorityFeePerGas hexutil.Big
	err := b.RPC.CallContext(ctx, &maxPriorityFeePerGas, "eth_maxPriorityFeePerGas")
	if err != nil {
		return nil, err
	}

	var fees bchain.Eip1559Fees

	type history struct {
		OldestBlock   string     `json:"oldestBlock"`
		Reward        [][]string `json:"reward"`
		BaseFeePerGas []string   `json:"baseFeePerGas"`
		GasUsedRatio  []float64  `json:"gasUsedRatio"`
	}
	var h history
	percentiles := []int{
		20, // low
		70, // medium
		90, // high
		99, // instant
	}
	blocks := 4

	err = b.RPC.CallContext(ctx, &h, "eth_feeHistory", blocks, "pending", percentiles)
	if err != nil {
		return nil, err
	}
	if len(h.BaseFeePerGas) < blocks {
		return nil, nil
	}

	hs, _ := json.Marshal(h)
	baseFee, _ := hexutil.DecodeUint64(h.BaseFeePerGas[blocks-1])
	fees.BaseFeePerGas = big.NewInt(int64(baseFee))
	maxBasePriorityFee := maxPriorityFeePerGas.ToInt().Int64()
	glog.Info("eth_maxPriorityFeePerGas ", maxPriorityFeePerGas)
	glog.Info("eth_feeHistory ", string(hs))

	for i := 0; i < 4; i++ {
		var f bchain.Eip1559Fee
		priorityFee := int64(0)
		for j := 0; j < len(h.Reward); j++ {
			p, _ := hexutil.DecodeUint64(h.Reward[j][i])
			priorityFee += int64(p)
		}
		priorityFee = priorityFee / int64(len(h.Reward))
		f.MaxFeePerGas = big.NewInt(priorityFee)
		f.MaxPriorityFeePerGas = big.NewInt(maxBasePriorityFee)
		maxBasePriorityFee *= 2
		switch i {
		case 0:
			fees.Low = &f
		case 1:
			fees.Medium = &f
		case 2:
			fees.High = &f
		default:
			fees.Instant = &f
		}
	}
	return &fees, err
}

// SendRawTransaction sends raw transaction
func (b *EthereumRPC) SendRawTransaction(hex string, disableAlternativeRPC bool) (string, error) {
	var txid string
	var retErr error

	if !disableAlternativeRPC && b.alternativeSendTxProvider != nil {
		txid, retErr = b.alternativeSendTxProvider.SendRawTransaction(hex)
		if retErr == nil {
			return txid, nil
		}
		if b.alternativeSendTxProvider.UseOnlyAlternativeProvider() {
			return txid, retErr
		}
	}

	txid, retErr = b.callRpcStringResult("eth_sendRawTransaction", hex)
	if b.ChainConfig.DisableMempoolSync {
		// add transactions submitted by us to mempool if sync is disabled
		b.Mempool.AddTransactionToMempool(txid)
	}
	return txid, retErr
}

// EthereumTypeGetRawTransaction gets raw transaction in hex format
func (b *EthereumRPC) EthereumTypeGetRawTransaction(txid string) (string, error) {
	return b.callRpcStringResult("eth_getRawTransactionByHash", txid)
}

// Helper function for calling ETH RPC with parameters and getting string result
func (b *EthereumRPC) callRpcStringResult(rpcMethod string, args ...interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	err := b.RPC.CallContext(ctx, &raw, rpcMethod, args...)
	if err != nil {
		return "", err
	} else if len(raw) == 0 {
		return "", errors.New(rpcMethod + " : failed")
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.Annotatef(err, "raw result %v", raw)
	}
	if result == "" {
		return "", errors.New(rpcMethod + " : failed, empty result")
	}
	return result, nil
}

// EthereumTypeGetBalance returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// EthereumTypeGetNonce returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	var result string
	var err error
	var usedAlternative bool

	ethAddress := ethcommon.BytesToAddress(addrDesc)

	if b.alternativeSendTxProvider != nil {
		result, err = b.alternativeSendTxProvider.callHttpStringResult(
			b.alternativeSendTxProvider.urls[0],
			"eth_getTransactionCount",
			ethAddress,
			"pending",
		)
		if err == nil && result != "" {
			usedAlternative = true
		} else {
			glog.Errorf("Alternative provider failed for eth_getTransactionCount: %v, falling back to primary RPC", err)
		}
	}

	if !usedAlternative {
		result, err = b.callRpcStringResult("eth_getTransactionCount", ethAddress, "pending")
		if err != nil {
			glog.Errorf("Primary RPC failed for eth_getTransactionCount: %v", err)
			return 0, err
		}
	}

	nonce, err := hexutil.DecodeUint64(result)
	if err != nil {
		glog.Errorf("Failed to parse nonce result '%s': %v", result, err)
		return 0, err
	}

	return nonce, nil
}

// GetChainParser returns ethereum BlockChainParser
func (b *EthereumRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
