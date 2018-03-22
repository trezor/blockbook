package eth

import (
	"blockbook/bchain"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
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

type EthereumNet uint32

const (
	MainNet EthereumNet = 1
	TestNet EthereumNet = 3
)

// EthRPC is an interface to JSON-RPC eth service.
type EthRPC struct {
	client       *ethclient.Client
	rpc          *rpc.Client
	timeout      time.Duration
	rpcURL       string
	Parser       *EthParser
	Testnet      bool
	Network      string
	Mempool      *bchain.Mempool
	bestHeaderMu sync.Mutex
	bestHeader   *ethtypes.Header
}

type configuration struct {
	RPCURL     string `json:"rpcURL"`
	RPCTimeout int    `json:"rpcTimeout"`
}

// NewEthRPC returns new EthRPC instance.
func NewEthRPC(config json.RawMessage, pushHandler func(*bchain.MQMessage)) (bchain.BlockChain, error) {
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

	s := &EthRPC{
		client: ec,
		rpc:    rc,
		rpcURL: c.RPCURL,
	}

	// always create parser
	s.Parser = &EthParser{}
	s.timeout = time.Duration(c.RPCTimeout) * time.Second

	return s, nil
}

func (b *EthRPC) Initialize() error {
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

	// b.Mempool = bchain.NewMempool(s, metrics)
	return nil
}

func (b *EthRPC) Shutdown() error {
	return nil
}

func (b *EthRPC) IsTestnet() bool {
	return b.Testnet
}

func (b *EthRPC) GetNetworkName() string {
	return b.Network
}

func (b *EthRPC) getBestHeader() (*ethtypes.Header, error) {
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

func ethHashToHash(h ethcommon.Hash) string {
	return h.Hex()[2:]
}

func (b *EthRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}
	return ethHashToHash(h.Hash()), nil
}

func (b *EthRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	// TODO - can it grow over 2^32 ?
	return uint32(h.Number.Uint64()), nil
}

func (b *EthRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	h, err := b.client.HeaderByNumber(ctx, &n)
	if err != nil {
		return "", err
	}
	return ethHashToHash(h.Hash()), nil
}

func (b *EthRPC) ethHeaderToBlockHeader(h *ethtypes.Header) (*bchain.BlockHeader, error) {
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

func (b *EthRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	h, err := b.client.HeaderByHash(ctx, ethcommon.HexToHash(hash))
	if err != nil {
		return nil, err
	}
	return b.ethHeaderToBlockHeader(h)
}

func (b *EthRPC) computeConfirmations(n uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bn := bh.Number.Uint64()
	return uint32(bn - n), nil
}

type rpcTransaction struct {
	tx *ethtypes.Transaction
	txExtraInfo
}

type txExtraInfo struct {
	BlockNumber      *string
	BlockHash        ethcommon.Hash
	From             ethcommon.Address
	TransactionIndex string `json:"transactionIndex"`
}

type rpcBlock struct {
	Hash         ethcommon.Hash   `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
	UncleHashes  []ethcommon.Hash `json:"uncles"`
}

func ethTxToTx(rtx *rpcTransaction, blocktime int64, confirmations uint32) (*bchain.Tx, error) {
	txid := ethHashToHash(rtx.tx.Hash())
	n, err := strconv.ParseInt(rtx.TransactionIndex, 16, 64)
	if err != nil {
		return nil, err
	}
	var from, to string
	ethTo := rtx.tx.To()
	if ethTo != nil {
		to = ethTo.Hex()[2:]
	}
	from = rtx.From.Hex()[2:]
	return &bchain.Tx{
		Blocktime:     blocktime,
		Confirmations: confirmations,
		// Hex
		// LockTime
		Time: blocktime,
		Txid: txid,
		Vin: []bchain.Vin{
			{
				Addresses: []string{from},
				// Coinbase
				// ScriptSig
				// Sequence
				// Txid
				// Vout
			},
		},
		Vout: []bchain.Vout{
			{
				N:     uint32(n),
				Value: float64(rtx.tx.Value().Int64()),
				ScriptPubKey: bchain.ScriptPubKey{
					// Hex
					Addresses: []string{to},
				},
			},
		},
	}, nil
}

func (b *EthRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var raw json.RawMessage
	err := b.rpc.CallContext(ctx, &raw, "eth_getBlockByHash", ethcommon.HexToHash(hash), true)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, ethereum.NotFound
	}
	// Decode header and transactions.
	var head *ethtypes.Header
	var body rpcBlock
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	// Quick-verify transaction and uncle lists. This mostly helps with debugging the server.
	if head.UncleHash == ethtypes.EmptyUncleHash && len(body.UncleHashes) > 0 {
		return nil, fmt.Errorf("server returned non-empty uncle list but block header indicates no uncles")
	}
	if head.UncleHash != ethtypes.EmptyUncleHash && len(body.UncleHashes) == 0 {
		return nil, fmt.Errorf("server returned empty uncle list but block header indicates uncles")
	}
	if head.TxHash == ethtypes.EmptyRootHash && len(body.Transactions) > 0 {
		return nil, fmt.Errorf("server returned non-empty transaction list but block header indicates no transactions")
	}
	if head.TxHash != ethtypes.EmptyRootHash && len(body.Transactions) == 0 {
		return nil, fmt.Errorf("server returned empty transaction list but block header indicates transactions")
	}
	bbh, err := b.ethHeaderToBlockHeader(head)
	btxs := make([]bchain.Tx, len(body.Transactions))
	for i, tx := range body.Transactions {
		btx, err := ethTxToTx(&tx, int64(head.Time.Uint64()), uint32(bbh.Confirmations))
		if err != nil {
			return nil, err
		}
		btxs[i] = *btx
	}
	bbk := bchain.Block{
		BlockHeader: *bbh,
		Txs:         btxs,
	}
	return &bbk, nil
}

func (b *EthRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var json *rpcTransaction
	err := b.rpc.CallContext(ctx, &json, "eth_getTransactionByHash", ethcommon.HexToHash(txid))
	if err != nil {
		return nil, err
	} else if json == nil {
		return nil, ethereum.NotFound
	} else if _, r, _ := json.tx.RawSignatureValues(); r == nil {
		return nil, fmt.Errorf("server returned transaction without signature")
	}
	var btx *bchain.Tx
	if json.BlockNumber == nil {
		// mempool tx
		btx, err = ethTxToTx(json, 0, 0)
		if err != nil {
			return nil, err
		}
	} else {
		// non mempool tx - we must read the block header to get the block time
		n, err := strconv.ParseInt((*json.BlockNumber)[2:], 16, 64)
		if err != nil {
			return nil, err
		}
		h, err := b.client.HeaderByHash(ctx, json.BlockHash)
		if err != nil {
			return nil, err
		}
		confirmations, err := b.computeConfirmations(uint64(n))
		if err != nil {
			return nil, err
		}
		btx, err = ethTxToTx(json, h.Time.Int64(), confirmations)
		if err != nil {
			return nil, err
		}
	}
	return btx, nil
}

func (b *EthRPC) GetMempool() ([]string, error) {
	panic("not implemented")
}

func (b *EthRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	panic("not implemented")
}

func (b *EthRPC) SendRawTransaction(tx string) (string, error) {
	panic("not implemented")
}

func (b *EthRPC) ResyncMempool(onNewTxAddr func(txid string, addr string)) error {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolTransactions(address string) ([]string, error) {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolSpentOutput(outputTxid string, vout uint32) string {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	panic("not implemented")
}

func (b *EthRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
