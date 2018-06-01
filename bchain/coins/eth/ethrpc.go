package eth

import (
	"blockbook/bchain"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"

	ethereum "github.com/ethereum/go-ethereum"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// EthereumNet type specifies the type of ethereum network
type EthereumNet uint32

const (
	// MainNet is production network
	MainNet EthereumNet = 1
	// TestNet is Ropsten test network
	TestNet EthereumNet = 3
)

// EthereumRPC is an interface to JSON-RPC eth service.
type EthereumRPC struct {
	client               *ethclient.Client
	rpc                  *rpc.Client
	timeout              time.Duration
	rpcURL               string
	Parser               *EthereumParser
	Testnet              bool
	Network              string
	Mempool              *bchain.NonUTXOMempool
	bestHeaderMu         sync.Mutex
	bestHeader           *ethtypes.Header
	chanNewBlock         chan *ethtypes.Header
	newBlockSubscription *rpc.ClientSubscription
	chanNewTx            chan ethcommon.Hash
	newTxSubscription    *rpc.ClientSubscription
}

type configuration struct {
	RPCURL     string `json:"rpcURL"`
	RPCTimeout int    `json:"rpcTimeout"`
}

// NewEthereumRPC returns new EthRPC instance.
func NewEthereumRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	var err error
	var c configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}
	rc, err := rpc.Dial(c.RPCURL)
	if err != nil {
		return nil, err
	}
	ec := ethclient.NewClient(rc)

	s := &EthereumRPC{
		client: ec,
		rpc:    rc,
		rpcURL: c.RPCURL,
	}

	// always create parser
	s.Parser = NewEthereumParser()
	s.timeout = time.Duration(c.RPCTimeout) * time.Second

	// new blocks notifications handling
	// the subscription is done in Initialize
	s.chanNewBlock = make(chan *ethtypes.Header)
	go func() {
		for {
			h, ok := <-s.chanNewBlock
			if !ok {
				break
			}
			glog.V(2).Info("rpc: new block header ", h.Number)
			// update best header to the new header
			s.bestHeaderMu.Lock()
			s.bestHeader = h
			s.bestHeaderMu.Unlock()
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
				break
			}
			if glog.V(2) {
				glog.Info("rpc: new tx ", t.Hex())
			}
			pushHandler(bchain.NotificationNewTx)
		}
	}()

	return s, nil
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
	glog.Info("rpc: block chain ", b.Network)

	// subscriptions
	if err = b.subscribe(func() (*rpc.ClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		sub, err := b.rpc.EthSubscribe(ctx, b.chanNewBlock, "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newHeads")
		}
		b.newBlockSubscription = sub
		glog.Info("Subscribed to newHeads")
		return sub, nil
	}); err != nil {
		return err
	}
	if err = b.subscribe(func() (*rpc.ClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newTxSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		sub, err := b.rpc.EthSubscribe(ctx, b.chanNewTx, "newPendingTransactions")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newPendingTransactions")
		}
		b.newTxSubscription = sub
		glog.Info("Subscribed to newPendingTransactions")
		return sub, nil
	}); err != nil {
		return err
	}

	// create mempool
	b.Mempool = bchain.NewNonUTXOMempool(b)

	return nil
}

// subscribeNewBlocks subscribes to new blocks notification
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
			timer := time.NewTimer(time.Second)
			// try in 1 second interval to resubscribe
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
					timer.Reset(time.Second)
				}
			}
		}
	}()
	return nil
}

// Shutdown cleans up rpc interface to ethereum
func (b *EthereumRPC) Shutdown(ctx context.Context) error {
	if b.newBlockSubscription != nil {
		b.newBlockSubscription.Unsubscribe()
	}
	if b.newTxSubscription != nil {
		b.newTxSubscription.Unsubscribe()
	}
	if b.rpc != nil {
		b.rpc.Close()
	}
	close(b.chanNewBlock)
	glog.Info("rpc: shutdown")
	return nil
}

func (b *EthereumRPC) IsTestnet() bool {
	return b.Testnet
}

func (b *EthereumRPC) GetNetworkName() string {
	return b.Network
}

func (b *EthereumRPC) GetSubversion() string {
	return ""
}

// GetBlockChainInfo returns the NetworkID of the ethereum network
func (b *EthereumRPC) GetBlockChainInfo() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	id, err := b.client.NetworkID(ctx)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (b *EthereumRPC) getBestHeader() (*ethtypes.Header, error) {
	b.bestHeaderMu.Lock()
	defer b.bestHeaderMu.Unlock()
	if b.bestHeader == nil {
		var err error
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()
		b.bestHeader, err = b.client.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, err
		}
	}
	return b.bestHeader, nil
}

func (b *EthereumRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}
	return ethHashToHash(h.Hash()), nil
}

func (b *EthereumRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	// TODO - can it grow over 2^32 ?
	return uint32(h.Number.Uint64()), nil
}

func (b *EthereumRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	h, err := b.client.HeaderByNumber(ctx, &n)
	if err != nil {
		if err == ethereum.NotFound {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(err, "height %v", height)
	}
	return ethHashToHash(h.Hash()), nil
}

func (b *EthereumRPC) ethHeaderToBlockHeader(h *ethtypes.Header) (*bchain.BlockHeader, error) {
	hn := h.Number.Uint64()
	c, err := b.computeConfirmations(hn)
	if err != nil {
		return nil, err
	}
	return &bchain.BlockHeader{
		Hash:          ethHashToHash(h.Hash()),
		Height:        uint32(hn),
		Confirmations: int(c),
		// Next
		// Prev

	}, nil
}

func (b *EthereumRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	h, err := b.client.HeaderByHash(ctx, ethcommon.HexToHash(hash))
	if err != nil {
		if err == ethereum.NotFound {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	return b.ethHeaderToBlockHeader(h)
}

func (b *EthereumRPC) computeConfirmations(n uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bn := bh.Number.Uint64()
	// transaction in the best block has 1 confirmation
	return uint32(bn - n + 1), nil
}

// GetBlock returns block with given hash or height, hash has precedence if both passed
func (b *EthereumRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	if hash != "" {
		err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByHash", ethcommon.HexToHash(hash), true)
	} else {

		err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("%#x", height), true)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 {
		return nil, bchain.ErrBlockNotFound
	}
	// Decode header and transactions.
	var head *ethtypes.Header
	var body rpcBlock
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	if head == nil {
		return nil, bchain.ErrBlockNotFound
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	// Quick-verify transaction and uncle lists. This mostly helps with debugging the server.
	if head.UncleHash == ethtypes.EmptyUncleHash && len(body.UncleHashes) > 0 {
		return nil, errors.Annotatef(fmt.Errorf("server returned non-empty uncle list but block header indicates no uncles"), "hash %v, height %v", hash, height)
	}
	if head.UncleHash != ethtypes.EmptyUncleHash && len(body.UncleHashes) == 0 {
		return nil, errors.Annotatef(fmt.Errorf("server returned empty uncle list but block header indicates uncles"), "hash %v, height %v", hash, height)
	}
	if head.TxHash == ethtypes.EmptyRootHash && len(body.Transactions) > 0 {
		return nil, errors.Annotatef(fmt.Errorf("server returned non-empty transaction list but block header indicates no transactions"), "hash %v, height %v", hash, height)
	}
	if head.TxHash != ethtypes.EmptyRootHash && len(body.Transactions) == 0 {
		return nil, errors.Annotatef(fmt.Errorf("server returned empty transaction list but block header indicates transactions"), "hash %v, height %v", hash, height)
	}
	bbh, err := b.ethHeaderToBlockHeader(head)
	btxs := make([]bchain.Tx, len(body.Transactions))
	for i, tx := range body.Transactions {
		btx, err := b.Parser.ethTxToTx(&tx, int64(head.Time.Uint64()), uint32(bbh.Confirmations))
		if err != nil {
			return nil, errors.Annotatef(err, "hash %v, height %v, txid %v", hash, height, tx.Hash.String())
		}
		btxs[i] = *btx
	}
	bbk := bchain.Block{
		BlockHeader: *bbh,
		Txs:         btxs,
	}
	return &bbk, nil
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
	err := b.rpc.CallContext(ctx, &tx, "eth_getTransactionByHash", ethcommon.HexToHash(txid))
	if err != nil {
		return nil, err
	} else if tx == nil {
		return nil, ethereum.NotFound
	} else if tx.R == "" {
		return nil, errors.Annotatef(fmt.Errorf("server returned transaction without signature"), "txid %v", txid)
	}
	var btx *bchain.Tx
	if tx.BlockNumber == "" {
		// mempool tx
		btx, err = b.Parser.ethTxToTx(tx, 0, 0)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
	} else {
		// non mempool tx - we must read the block header to get the block time
		n, err := ethNumber(tx.BlockNumber)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		h, err := b.client.HeaderByHash(ctx, *tx.BlockHash)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		confirmations, err := b.computeConfirmations(uint64(n))
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		btx, err = b.Parser.ethTxToTx(tx, h.Time.Int64(), confirmations)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
	}
	return btx, nil
}

type rpcMempoolBlock struct {
	Transactions []string `json:"transactions"`
}

func (b *EthereumRPC) GetMempool() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	err = b.rpc.CallContext(ctx, &raw, "eth_getBlockByNumber", "pending", false)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, bchain.ErrBlockNotFound
	}
	var body rpcMempoolBlock
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body.Transactions, nil
}

// EstimateFee returns fee estimation.
func (b *EthereumRPC) EstimateFee(blocks int) (float64, error) {
	return b.EstimateSmartFee(blocks, true)
}

// EstimateSmartFee returns fee estimation.
func (b *EthereumRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	// TODO - what parameters of msg to use to get better estimate, maybe more data from the wallet are needed
	a := ethcommon.HexToAddress("0x1234567890123456789012345678901234567890")
	msg := ethereum.CallMsg{
		To: &a,
	}
	g, err := b.client.EstimateGas(ctx, msg)
	if err != nil {
		return 0, err
	}
	return float64(g), nil
}

// SendRawTransaction sends raw transaction.
func (b *EthereumRPC) SendRawTransaction(tx string) (string, error) {
	return "", errors.New("SendRawTransaction: not implemented")
}

func (b *EthereumRPC) ResyncMempool(onNewTxAddr func(txid string, addr string)) (int, error) {
	return b.Mempool.Resync(onNewTxAddr)
}

func (b *EthereumRPC) GetMempoolTransactions(address string) ([]string, error) {
	return b.Mempool.GetTransactions(address)
}

func (b *EthereumRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not implemented")
}

func (b *EthereumRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
