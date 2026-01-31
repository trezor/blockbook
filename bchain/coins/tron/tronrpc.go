package tron

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"

	"math/big"
)

const (
	// MainNet is production network
	MainNet     eth.Network = 11111
	TestNetNile eth.Network = 201910292

	TRC10TokenType   bchain.TokenStandardName = "TRC10"
	TRC20TokenType   bchain.TokenStandardName = "TRC20"
	TRC721TokenType  bchain.TokenStandardName = "TRC721"
	TRC1155TokenType bchain.TokenStandardName = "TRC1155"
)

type TronConfiguration struct {
	eth.Configuration
	MessageQueueBinding string `json:"message_queue_binding"`
	HttpUrlTemplate     string `json:"tron_http_url_template"`
}

type tronBroadcastHexResponse struct {
	Result  bool   `json:"result"`
	TxID    string `json:"txid"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type tronGetTransactionByIDResponse struct {
	RawDataHex string `json:"raw_data_hex"`
}

type TronRPC struct {
	*eth.EthereumRPC
	Parser      *TronParser
	ChainConfig *TronConfiguration
	mq          *bchain.MQ
	http        TronHTTP
}

func NewTronRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	var cfg TronConfiguration
	err = json.Unmarshal(config, &cfg)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid Tron configuration file")
	}

	cfg.Eip1559Fees = false

	bchain.EthereumTokenStandardMap = []bchain.TokenStandardName{TRC20TokenType, TRC721TokenType, TRC1155TokenType}

	tronRpc := &TronRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
		Parser:      NewTronParser(cfg.BlockAddressesToKeep, cfg.AddressAliases),
	}

	tronRpc.EthereumRPC.Parser = tronRpc.Parser
	tronRpc.ChainConfig = &cfg
	tronRpc.PushHandler = pushHandler

	tronHTTP := NewTronHTTPClient(cfg.HttpUrlTemplate, time.Duration(cfg.RPCTimeout)*time.Second)
	tronRpc.http = tronHTTP

	internalProvider := NewTronInternalDataProvider(
		tronHTTP,
		time.Duration(cfg.RPCTimeout)*time.Second,
	)

	tronRpc.EthereumRPC.InternalDataProvider = internalProvider

	return tronRpc, nil
}

// OpenRPC opens an RPC connection to the Tron backend (wsURL is unused – Tron has no WS subscriptions)
var OpenRPC = func(url, _ string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
	opts := []rpc.ClientOption{}
	opts = append(opts, rpc.WithWebsocketMessageSizeLimit(0))

	r, err := rpc.DialOptions(context.Background(), url, opts...)
	if err != nil {
		return nil, nil, err
	}

	rpcClient := &TronRPCClient{Client: r}
	ethClient := ethclient.NewClient(r) // Ethereum client for compatibility
	tc := &TronClient{
		Client:    ethClient,
		rpcClient: rpcClient,
	}

	return rpcClient, tc, nil
}

// Initialize Tron RPC
func (b *TronRPC) Initialize() error {
	b.OpenRPC = OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL, "")
	if err != nil {
		return err
	}

	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch eth.Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "mainnet"
	case TestNetNile:
		b.Testnet = true
		b.Network = "nile"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	log.Info("TronRPC: initialized Tron blockchain: ", b.Network)
	return nil
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
// need to overwrite this because the getBestHeader method in EthRpc is
// relying on the subscription
func (b *TronRPC) GetBestBlockHash() (string, error) {
	var err error
	var header bchain.EVMHeader

	header, err = b.getBestHeader()
	if err != nil {
		return "", err
	}

	return header.Hash(), nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *TronRPC) GetBestBlockHeight() (uint32, error) {
	var err error
	var header bchain.EVMHeader

	header, err = b.getBestHeader()
	if err != nil {
		return 0, err
	}

	return uint32(header.Number().Uint64()), nil
}

func (b *TronRPC) getBestHeader() (bchain.EVMHeader, error) {
	var err error
	var header bchain.EVMHeader
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	header, err = b.Client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	b.UpdateBestHeader(header)
	return header, nil
}

// GetChainParser returns Tron-specific BlockChainParser
func (b *TronRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}

func (b *TronRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		b.Mempool = bchain.NewMempoolEthereumType(chain, b.ChainConfig.MempoolTxTimeoutHours, b.ChainConfig.QueryBackendOnMempoolResync)
	}
	return b.Mempool, nil
}

func (b *TronRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Tron Mempool not created")
	}
	b.Mempool.OnNewTxAddr = onNewTxAddr
	b.Mempool.OnNewTx = onNewTx

	if b.mq == nil {
		tronTopics := bchain.SubscriptionTopics{
			BlockSubscribe: "block",
			BlockReceive:   "blockTrigger",
			TxSubscribe:    "",
			TxReceive:      "",
		}

		mq, err := bchain.NewMQ(b.ChainConfig.MessageQueueBinding, b.PushHandler, tronTopics)
		if err != nil {
			return err
		}
		b.mq = mq
	}

	return nil
}

func (b *TronRPC) Shutdown(ctx context.Context) error {
	if b.mq != nil {
		if err := b.mq.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *TronRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	tx, err := b.EthereumRPC.GetTransaction(txid)
	if err != nil {
		return nil, err
	}

	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)

	if !ok {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}

	if len(tx.Vout) > 0 &&
		tx.Vout[0].ScriptPubKey.Addresses == nil &&
		csd.Receipt != nil &&
		csd.Receipt.ContractAddress != "" {
		tx.Vout = []bchain.Vout{{
			ValueSat: tx.Vout[0].ValueSat,
			N:        0,
			ScriptPubKey: bchain.ScriptPubKey{
				Addresses: []string{ToTronAddressFromAddress(csd.Receipt.ContractAddress)},
			},
		}}

		csd.InternalData = &bchain.EthereumInternalData{
			Type:     bchain.CREATE,
			Contract: ToTronAddressFromAddress(csd.Receipt.ContractAddress),
		}
		tx.CoinSpecificData = csd
	}

	return tx, nil
}

func (b *TronRPC) getTransactionReceipt(txid string) (*bchain.RpcReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	hash := ethcommon.HexToHash(txid)
	var receipt bchain.RpcReceipt
	err := b.RPC.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get transaction receipt for txid %v", txid)
	}

	return &receipt, nil
}

// Tron does not have any method for getting mempool transactions (does not support parameter 'pending' in eth_getBlockByNumber)
// https://developers.tron.network/reference/eth_getblockbynumber
func (b *TronRPC) GetMempoolTransactions() ([]string, error) {
	return []string{}, nil
}

func (b *TronRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// EthereumTypeGetNonce returns current balance of an address
func (b *TronRPC) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.Client.NonceAt(ctx, addrDesc, nil)
}

// GetContractInfo returns information about a contract
func (b *TronRPC) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	contract, err := b.EthereumRPC.GetContractInfo(contractDesc)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, nil
	}
	contract.Contract = ToTronAddressFromAddress(contract.Contract)
	glog.Infof("Getting contract info for: %s", contract.Contract)
	return contract, nil
}

func (b *TronRPC) SendRawTransaction(tx string, disableAlternativeRPC bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	req := map[string]string{
		"transaction": strings.TrimPrefix(tx, "0x"),
	}
	var resp tronBroadcastHexResponse
	if err := b.http.Request(ctx, "/wallet/broadcasthex", req, &resp); err != nil {
		return "", err
	}
	if !resp.Result {
		if resp.Code != "" || resp.Message != "" {
			return "", errors.Errorf("Tron broadcasthex failed: %s %s", resp.Code, resp.Message)
		}
		return "", errors.New("Tron broadcasthex failed")
	}

	txid := resp.TxID
	if !strings.HasPrefix(txid, "0x") {
		txid = "0x" + txid
	}
	if b.ChainConfig.DisableMempoolSync && b.Mempool != nil {
		b.Mempool.AddTransactionToMempool(txid)
	}
	return txid, nil
}

func (b *TronRPC) EthereumTypeGetRawTransaction(txid string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	req := map[string]string{
		"value": strings.TrimPrefix(txid, "0x"),
	}
	var resp tronGetTransactionByIDResponse
	if err := b.http.Request(ctx, "/wallet/gettransactionbyid", req, &resp); err != nil {
		return "", err
	}
	if resp.RawDataHex == "" {
		return "", errors.Errorf("Tron gettransactionbyid returned empty raw_data_hex for %s", txid)
	}
	rawHex := resp.RawDataHex
	if !strings.HasPrefix(rawHex, "0x") {
		rawHex = "0x" + rawHex
	}
	return rawHex, nil
}
