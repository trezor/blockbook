package tron

import (
	"context"
	"encoding/json"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet     eth.Network = 11111
	TestNetNile eth.Network = 201910292

	TRC10TokenType   bchain.TokenStandardName = "TRC10"
	TRC20TokenType   bchain.TokenStandardName = "TRC20"
	TRC721TokenType  bchain.TokenStandardName = "TRC721"
	TRC1155TokenType bchain.TokenStandardName = "TRC1155"

	tronBestHeaderMaxAge = 30 * time.Second
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

type tronGetTransactionListFromPendingResponse struct {
	TxID []string `json:"txId,omitempty"`
}

type tronTxRet struct {
	ContractRet string      `json:"contractRet,omitempty"`
	Fee         interface{} `json:"fee,omitempty"`
}

type tronTxContractValue struct {
	OwnerAddress    string       `json:"owner_address,omitempty"`
	ToAddress       string       `json:"to_address,omitempty"`
	ContractAddress string       `json:"contract_address,omitempty"`
	ReceiverAddress string       `json:"receiver_address,omitempty"`
	Resource        interface{}  `json:"resource,omitempty"`
	Amount          interface{}  `json:"amount,omitempty"`
	CallValue       interface{}  `json:"call_value,omitempty"`
	FrozenBalance   interface{}  `json:"frozen_balance,omitempty"`
	UnfreezeBalance interface{}  `json:"unfreeze_balance,omitempty"`
	Balance         interface{}  `json:"balance,omitempty"`
	Votes           []tronTxVote `json:"votes,omitempty"`
	Data            string       `json:"data,omitempty"`
}

type tronTxVote struct {
	VoteAddress string      `json:"vote_address,omitempty"`
	VoteCount   interface{} `json:"vote_count,omitempty"`
}

type tronTxContract struct {
	Type      string `json:"type"`
	Parameter struct {
		Value tronTxContractValue `json:"value"`
	} `json:"parameter"`
}

type tronGetTransactionByIDResponse struct {
	Ret            []tronTxRet `json:"ret,omitempty"`
	TxID           string      `json:"txID,omitempty"`
	BlockNumber    interface{} `json:"blockNumber,omitempty"`
	BlockTimestamp interface{} `json:"block_timestamp,omitempty"`
	RawDataHex     string      `json:"raw_data_hex"`
	RawData        struct {
		Timestamp interface{}      `json:"timestamp,omitempty"`
		FeeLimit  interface{}      `json:"fee_limit,omitempty"`
		Contract  []tronTxContract `json:"contract"`
	} `json:"raw_data"`
}

type TronRPC struct {
	*eth.EthereumRPC
	Parser             *TronParser
	ChainConfig        *TronConfiguration
	mq                 *bchain.MQ
	http               TronHTTP
	bestHeaderLock     sync.Mutex
	bestHeader         bchain.EVMHeader
	bestHeaderTime     time.Time
	newBlockNotifyCh   chan struct{}
	newBlockNotifyOnce sync.Once
}

func NewTronRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	ethereumRPC, err := eth.NewEthereumRPC(config, pushHandler)
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
		EthereumRPC:      ethereumRPC.(*eth.EthereumRPC),
		Parser:           NewTronParser(cfg.BlockAddressesToKeep, cfg.AddressAliases),
		newBlockNotifyCh: make(chan struct{}, 1),
	}
	ethChainConfig := tronRpc.EthereumRPC.ChainConfig

	tronRpc.Parser.HotAddressMinContracts = ethChainConfig.HotAddressMinContracts
	tronRpc.Parser.HotAddressLRUCacheSize = ethChainConfig.HotAddressLRUCacheSize
	tronRpc.Parser.HotAddressMinHits = ethChainConfig.HotAddressMinHits
	tronRpc.Parser.AddrContractsCacheMinSize = ethChainConfig.AddressContractsCacheMinSize
	tronRpc.Parser.AddrContractsCacheMaxBytes = ethChainConfig.AddressContractsCacheMaxBytes

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

	return strip0xPrefix(header.Hash()), nil
}

// GetBlockHash returns block hash in Tron API format (without 0x prefix).
func (b *TronRPC) GetBlockHash(height uint32) (string, error) {
	hash, err := b.EthereumRPC.GetBlockHash(height)
	if err != nil {
		return "", err
	}
	return strip0xPrefix(hash), nil
}

// GetChainInfo returns information about connected backend with Tron-formatted IDs (without 0x).
func (b *TronRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	ci, err := b.EthereumRPC.GetChainInfo()
	if err != nil {
		return nil, err
	}
	ci.Bestblockhash = strip0xPrefix(ci.Bestblockhash)
	return ci, nil
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

// GetBlockHeader returns block header with Tron-formatted hashes (without 0x).
func (b *TronRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	ethHash := normalizeHexString(hash)
	bh, err := b.EthereumRPC.GetBlockHeader(ethHash)
	if err != nil {
		return nil, err
	}
	bh.Hash = strip0xPrefix(bh.Hash)
	bh.Prev = strip0xPrefix(bh.Prev)
	bh.Next = strip0xPrefix(bh.Next)
	return bh, nil
}

// GetBlockInfo returns block info with Tron-formatted hashes and txids (without 0x).
func (b *TronRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	ethHash := normalizeHexString(hash)
	bi, err := b.EthereumRPC.GetBlockInfo(ethHash)
	if err != nil {
		return nil, err
	}
	bi.Hash = strip0xPrefix(bi.Hash)
	bi.Prev = strip0xPrefix(bi.Prev)
	bi.Next = strip0xPrefix(bi.Next)
	for i := range bi.Txids {
		bi.Txids[i] = strip0xPrefix(bi.Txids[i])
	}
	return bi, nil
}

func (b *TronRPC) getBestHeader() (bchain.EVMHeader, error) {
	// During initial sync (before ZeroMQ is initialized) there is no push-based
	// tip refresh, so always read the latest header from the backend.
	if b.mq == nil {
		_, err := b.refreshBestHeaderFromChain()
		if err != nil {
			return nil, err
		}
		b.bestHeaderLock.Lock()
		defer b.bestHeaderLock.Unlock()
		if b.bestHeader == nil || b.bestHeader.Number() == nil {
			return nil, errors.New("best header is nil")
		}
		return b.bestHeader, nil
	}

	b.bestHeaderLock.Lock()
	cachedHeader := b.bestHeader
	cachedAt := b.bestHeaderTime
	b.bestHeaderLock.Unlock()

	if cachedHeader != nil && cachedAt.Add(tronBestHeaderMaxAge).After(time.Now()) {
		return cachedHeader, nil
	}

	_, err := b.refreshBestHeaderFromChain()
	if err != nil {
		return nil, err
	}

	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	if b.bestHeader == nil || b.bestHeader.Number() == nil {
		return nil, errors.New("best header is nil")
	}
	return b.bestHeader, nil
}

func (b *TronRPC) setBestHeader(h bchain.EVMHeader) bool {
	if h == nil || h.Number() == nil {
		return false
	}
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	changed := false
	if b.bestHeader == nil || b.bestHeader.Number() == nil {
		changed = true
	} else {
		prevNum := b.bestHeader.Number().Uint64()
		newNum := h.Number().Uint64()
		if prevNum != newNum || b.bestHeader.Hash() != h.Hash() {
			changed = true
		}
	}
	b.bestHeader = h
	b.bestHeaderTime = time.Now()
	b.UpdateBestHeader(h)
	return changed
}

func (b *TronRPC) refreshBestHeaderFromChain() (bool, error) {
	if b.Client == nil {
		return false, errors.New("rpc client not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	if h == nil || h.Number() == nil {
		return false, errors.New("best header is nil")
	}
	return b.setBestHeader(h), nil
}

func (b *TronRPC) signalNewBlock() {
	select {
	case b.newBlockNotifyCh <- struct{}{}:
	default:
	}
}

func (b *TronRPC) newBlockNotifier() {
	for range b.newBlockNotifyCh {
		updated, err := b.refreshBestHeaderFromChain()
		if err != nil {
			glog.Error("refreshBestHeaderFromChain ", err)
			continue
		}
		if updated && b.PushHandler != nil {
			b.PushHandler(bchain.NotificationNewBlock)
		}
	}
}

func (b *TronRPC) handleMQNotification(nt bchain.NotificationType) {
	if nt == bchain.NotificationNewBlock {
		b.signalNewBlock()
		return
	}
	if b.PushHandler != nil {
		b.PushHandler(nt)
	}
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
	b.newBlockNotifyOnce.Do(func() {
		go b.newBlockNotifier()
	})

	if b.mq == nil {
		tronTopics := bchain.SubscriptionTopics{
			BlockSubscribe: "block",
			BlockReceive:   "blockTrigger",
			TxSubscribe:    "",
			TxReceive:      "",
		}

		mq, err := bchain.NewMQ(b.ChainConfig.MessageQueueBinding, b.handleMQNotification, tronTopics)
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

func (b *TronRPC) getTransactionByIDRequired(txid string) (*tronGetTransactionByIDResponse, error) {
	txByID, err := b.getTransactionByID(txid)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if !tronHasTxByIDData(txByID) {
		return nil, errors.Annotatef(bchain.ErrTxNotFound, "txid %v", txid)
	}
	return txByID, nil
}

func (b *TronRPC) getTransactionInfoByIDOptional(txid string) *tronGetTransactionInfoByIDResponse {
	txInfo, err := b.getTransactionInfoByID(txid)
	if err != nil {
		glog.V(1).Infof("Tron gettransactioninfobyid tx %v: %v", txid, err)
		return nil
	}
	return txInfo
}

func (b *TronRPC) computeConfirmationsFromBlockNumber(txid string, blockNumber uint64, hasBlockNumber bool) uint32 {
	if !hasBlockNumber {
		return 0
	}
	confirmations, err := b.computeBlockConfirmations(blockNumber)
	if err != nil {
		glog.V(1).Infof("Tron eth_blockNumber tx %v: %v", txid, err)
		return 0
	}
	return confirmations
}

func (b *TronRPC) computeBlockConfirmations(blockNumber uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bestHeight := bh.Number().Uint64()
	if bestHeight < blockNumber {
		return 0, nil
	}
	return uint32(bestHeight - blockNumber + 1), nil
}

func (b *TronRPC) buildTxFromHTTPData(txid string, txByID *tronGetTransactionByIDResponse, txInfo *tronGetTransactionInfoByIDResponse, blockTime int64, confirmations uint32, internalData *bchain.EthereumInternalData) (*bchain.Tx, error) {
	csd := tronBuildEthereumSpecificData(txid, txByID, txInfo)
	csd.InternalData = internalData

	tx, err := b.Parser.EthTxToTx(csd.Tx, csd.Receipt, csd.InternalData, blockTime, confirmations, true)
	if err != nil {
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

		contractAddress := ToTronAddressFromAddress(csd.Receipt.ContractAddress)
		if csd.InternalData == nil {
			csd.InternalData = &bchain.EthereumInternalData{
				Type:     bchain.CREATE,
				Contract: contractAddress,
			}
		} else if csd.InternalData.Contract == "" {
			csd.InternalData.Type = bchain.CREATE
			csd.InternalData.Contract = contractAddress
		}
	}
	tx.Txid = strip0xPrefix(tx.Txid)
	tx.CoinSpecificData = csd
	return tx, nil
}

func (b *TronRPC) getTransactionByIDMapForBlockWithContext(ctx context.Context, hash string, blockHeight uint32) (map[string]*tronGetTransactionByIDResponse, error) {
	var (
		blockResp *tronGetBlockResponse
		err       error
	)
	if hash != "" && hash != "pending" {
		blockResp, err = requestBlockByID(ctx, b.http, hash)
	} else {
		blockResp, err = requestBlockByNum(ctx, b.http, blockHeight)
	}
	if err != nil {
		return nil, err
	}
	if blockResp == nil {
		return nil, nil
	}
	return mapTransactionByID(blockResp.Transactions), nil
}

func (b *TronRPC) getTransactionByIDMapForBlock(hash string, blockHeight uint32) (map[string]*tronGetTransactionByIDResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.getTransactionByIDMapForBlockWithContext(ctx, hash, blockHeight)
}

type tronRPCBlockHeader struct {
	Hash       string `json:"hash"`
	ParentHash string `json:"parentHash"`
	Number     string `json:"number"`
	Time       string `json:"timestamp"`
	Size       string `json:"size"`
}

type tronRPCBlockWithTransactions struct {
	tronRPCBlockHeader
	Transactions []bchain.RpcTransaction `json:"transactions"`
}

// GetBlock returns block with given hash or height, hash has precedence if both passed.
// Tron implementation enriches each tx with data from Tron HTTP endpoints and does not call EthereumRPC.GetBlock.
func (b *TronRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	raw, err := b.EthereumRPC.GetBlockRawByHashOrHeight(hash, height, true)
	if err != nil {
		return nil, err
	}
	var block tronRPCBlockWithTransactions
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}

	blockNumber, ok := tronUint64(block.Number)
	if !ok {
		return nil, errors.Errorf("invalid block number %q", block.Number)
	}
	blockTime, ok := tronUint64(block.Time)
	if !ok {
		return nil, errors.Errorf("invalid block timestamp %q", block.Time)
	}
	blockSize, ok := tronUint64(block.Size)
	if !ok {
		return nil, errors.Errorf("invalid block size %q", block.Size)
	}

	confirmations, err := b.computeBlockConfirmations(blockNumber)
	if err != nil {
		return nil, err
	}

	bbh := bchain.BlockHeader{
		Hash:          strip0xPrefix(block.Hash),
		Prev:          strip0xPrefix(block.ParentHash),
		Height:        uint32(blockNumber),
		Confirmations: int(confirmations),
		Time:          int64(blockTime),
		Size:          int(blockSize),
	}

	txInfosByID := map[string]*tronGetTransactionInfoByIDResponse{}
	txByIDByID := map[string]*tronGetTransactionByIDResponse{}
	internalData := make([]bchain.EthereumInternalData, len(block.Transactions))
	contracts := make([]bchain.ContractInfo, 0)
	var internalErr error

	if len(block.Transactions) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()

		type txInfosResult struct {
			infos []tronGetTransactionInfoByIDResponse
			err   error
		}
		type txByIDResult struct {
			txByID map[string]*tronGetTransactionByIDResponse
			err    error
		}

		infosCh := make(chan txInfosResult, 1)
		txByIDCh := make(chan txByIDResult, 1)

		go func() {
			infos, err := requestTransactionInfoByBlockNum(ctx, b.http, bbh.Height)
			infosCh <- txInfosResult{infos: infos, err: err}
		}()
		go func() {
			txByID, err := b.getTransactionByIDMapForBlockWithContext(ctx, hash, bbh.Height)
			txByIDCh <- txByIDResult{txByID: txByID, err: err}
		}()

		infosRes := <-infosCh
		if infosRes.err != nil {
			return nil, errors.Annotatef(infosRes.err, "height %v", bbh.Height)
		}
		if m := mapTransactionInfoByID(infosRes.infos); m != nil {
			txInfosByID = m
		}

		txByIDRes := <-txByIDCh
		if txByIDRes.err != nil {
			return nil, errors.Annotatef(txByIDRes.err, "height %v", bbh.Height)
		}
		if txByIDRes.txByID != nil {
			txByIDByID = txByIDRes.txByID
		}

		if eth.ProcessInternalTransactions {
			internalData, contracts, internalErr = buildInternalDataFromTronInfos(
				tronTxInfosFromResponses(infosRes.infos),
				block.Transactions,
				bbh.Height,
			)
		}
	}

	txs := make([]bchain.Tx, len(block.Transactions))
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		txByID := txByIDByID[strip0xPrefix(tx.Hash)]
		if txByID == nil {
			txByID, err = b.getTransactionByIDRequired(tx.Hash)
			if err != nil {
				return nil, err
			}
		}

		txInfo := txInfosByID[strip0xPrefix(tx.Hash)]
		if txInfo == nil {
			return nil, errors.Errorf("Tron gettransactioninfobyblocknum missing tx %v in block %v", tx.Hash, bbh.Height)
		}

		var txInternalData *bchain.EthereumInternalData
		if i < len(internalData) {
			txInternalData = &internalData[i]
		}

		rebuiltTx, err := b.buildTxFromHTTPData(tx.Hash, txByID, txInfo, bbh.Time, confirmations, txInternalData)
		if err != nil {
			return nil, err
		}
		txs[i] = *rebuiltTx

		if b.Mempool != nil {
			b.Mempool.RemoveTransactionFromMempool(tx.Hash)
		}
	}

	var blockSpecificData *bchain.EthereumBlockSpecificData
	if internalErr != nil || len(contracts) > 0 {
		blockSpecificData = &bchain.EthereumBlockSpecificData{}
		if internalErr != nil {
			blockSpecificData.InternalDataError = internalErr.Error()
		}
		if len(contracts) > 0 {
			blockSpecificData.Contracts = contracts
		}
	}

	return &bchain.Block{
		BlockHeader:      bbh,
		Txs:              txs,
		CoinSpecificData: blockSpecificData,
	}, nil
}

func (b *TronRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	txByID, err := b.getTransactionByIDRequired(txid)
	if err != nil {
		return nil, err
	}
	txInfo := b.getTransactionInfoByIDOptional(txid)

	blockTime, blockNumber, hasBlockNumber := tronTxMeta(txByID, txInfo)
	confirmations := b.computeConfirmationsFromBlockNumber(txid, blockNumber, hasBlockNumber)
	return b.buildTxFromHTTPData(txid, txByID, txInfo, blockTime, confirmations, nil)
}

// GetTransactionSpecific returns tx-specific JSON in Tron API format (without 0x in tx hash fields).
func (b *TronRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
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
	csdCopy := csd
	if csd.Tx != nil {
		txCopy := *csd.Tx
		txCopy.Hash = strip0xPrefix(txCopy.Hash)
		txCopy.BlockHash = strip0xPrefix(txCopy.BlockHash)
		csdCopy.Tx = &txCopy
	}
	m, err := json.Marshal(&csdCopy)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (b *TronRPC) GetMempoolTransactions() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	var resp tronGetTransactionListFromPendingResponse
	if err := b.http.Request(ctx, "/wallet/gettransactionlistfrompending", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	if len(resp.TxID) == 0 {
		return []string{}, nil
	}
	return resp.TxID, nil
}

func (b *TronRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// EthereumTypeEstimateGas supports both EVM hex and Tron Base58 in `from`/`to`.
func (b *TronRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	normalizedParams := params
	if len(params) > 0 {
		normalizedParams = make(map[string]interface{}, len(params))
		for k, v := range params {
			normalizedParams[k] = v
		}
	}
	for _, field := range []string{"from", "to"} {
		address, ok := eth.GetStringFromMap(field, normalizedParams)
		if !ok || address == "" {
			continue
		}
		hexAddress, err := b.Parser.FromTronAddressToHex(address)
		if err != nil {
			return 0, err
		}
		if hexAddress != "" {
			normalizedParams[field] = hexAddress
		}
	}
	return b.EthereumRPC.EthereumTypeEstimateGas(normalizedParams)
}

// EthereumTypeRpcCall supports both EVM hex and Tron Base58 in `to`/`from`.
func (b *TronRPC) EthereumTypeRpcCall(data, to, from string) (string, error) {
	normalizedTo := to
	if to != "" {
		hexAddress, err := b.Parser.FromTronAddressToHex(to)
		if err != nil {
			return "", err
		}
		normalizedTo = hexAddress
	}
	normalizedFrom := from
	if from != "" {
		hexAddress, err := b.Parser.FromTronAddressToHex(from)
		if err != nil {
			return "", err
		}
		normalizedFrom = hexAddress
	}
	return b.EthereumRPC.EthereumTypeRpcCall(data, normalizedTo, normalizedFrom)
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
		"transaction": strip0xPrefix(tx),
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

	if b.ChainConfig != nil && b.ChainConfig.DisableMempoolSync && b.Mempool != nil {
		b.Mempool.AddTransactionToMempool(resp.TxID)
	}
	return resp.TxID, nil
}

func (b *TronRPC) EthereumTypeGetRawTransaction(txid string) (string, error) {
	resp, err := b.getTransactionByID(txid)
	if err != nil {
		return "", err
	}
	if resp.RawDataHex == "" {
		return "", errors.Errorf("Tron gettransactionbyid returned empty raw_data_hex for %s", txid)
	}
	return normalizeHexString(resp.RawDataHex), nil
}
