package coins

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/bch"
	"github.com/trezor/blockbook/bchain/coins/bellcoin"
	"github.com/trezor/blockbook/bchain/coins/bitcore"
	"github.com/trezor/blockbook/bchain/coins/bitzeny"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/btg"
	"github.com/trezor/blockbook/bchain/coins/cpuchain"
	"github.com/trezor/blockbook/bchain/coins/dash"
	"github.com/trezor/blockbook/bchain/coins/dcr"
	"github.com/trezor/blockbook/bchain/coins/deeponion"
	"github.com/trezor/blockbook/bchain/coins/digibyte"
	"github.com/trezor/blockbook/bchain/coins/divi"
	"github.com/trezor/blockbook/bchain/coins/dogecoin"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/bchain/coins/firo"
	"github.com/trezor/blockbook/bchain/coins/flo"
	"github.com/trezor/blockbook/bchain/coins/fujicoin"
	"github.com/trezor/blockbook/bchain/coins/gamecredits"
	"github.com/trezor/blockbook/bchain/coins/grs"
	"github.com/trezor/blockbook/bchain/coins/koto"
	"github.com/trezor/blockbook/bchain/coins/liquid"
	"github.com/trezor/blockbook/bchain/coins/litecoin"
	"github.com/trezor/blockbook/bchain/coins/monacoin"
	"github.com/trezor/blockbook/bchain/coins/monetaryunit"
	"github.com/trezor/blockbook/bchain/coins/myriad"
	"github.com/trezor/blockbook/bchain/coins/namecoin"
	"github.com/trezor/blockbook/bchain/coins/nuls"
	"github.com/trezor/blockbook/bchain/coins/omotenashicoin"
	"github.com/trezor/blockbook/bchain/coins/pivx"
	"github.com/trezor/blockbook/bchain/coins/polis"
	"github.com/trezor/blockbook/bchain/coins/qtum"
	"github.com/trezor/blockbook/bchain/coins/ravencoin"
	"github.com/trezor/blockbook/bchain/coins/ritocoin"
	"github.com/trezor/blockbook/bchain/coins/snowgem"
	"github.com/trezor/blockbook/bchain/coins/trezarcoin"
	"github.com/trezor/blockbook/bchain/coins/unobtanium"
	"github.com/trezor/blockbook/bchain/coins/vertcoin"
	"github.com/trezor/blockbook/bchain/coins/viacoin"
	"github.com/trezor/blockbook/bchain/coins/vipstarcoin"
	"github.com/trezor/blockbook/bchain/coins/zec"
	"github.com/trezor/blockbook/common"
)

type blockChainFactory func(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error)

// BlockChainFactories is a map of constructors of coin RPC interfaces
var BlockChainFactories = make(map[string]blockChainFactory)

func init() {
	BlockChainFactories["Bitcoin"] = btc.NewBitcoinRPC
	BlockChainFactories["Testnet"] = btc.NewBitcoinRPC
	BlockChainFactories["Signet"] = btc.NewBitcoinRPC
	BlockChainFactories["Zcash"] = zec.NewZCashRPC
	BlockChainFactories["Zcash Testnet"] = zec.NewZCashRPC
	BlockChainFactories["Ethereum"] = eth.NewEthereumRPC
	BlockChainFactories["Ethereum Classic"] = eth.NewEthereumRPC
	BlockChainFactories["Ethereum Testnet Ropsten"] = eth.NewEthereumRPC
	BlockChainFactories["Ethereum Testnet Goerli"] = eth.NewEthereumRPC
	BlockChainFactories["Bcash"] = bch.NewBCashRPC
	BlockChainFactories["Bcash Testnet"] = bch.NewBCashRPC
	BlockChainFactories["Bgold"] = btg.NewBGoldRPC
	BlockChainFactories["Bgold Testnet"] = btg.NewBGoldRPC
	BlockChainFactories["Dash"] = dash.NewDashRPC
	BlockChainFactories["Dash Testnet"] = dash.NewDashRPC
	BlockChainFactories["Decred"] = dcr.NewDecredRPC
	BlockChainFactories["Decred Testnet"] = dcr.NewDecredRPC
	BlockChainFactories["GameCredits"] = gamecredits.NewGameCreditsRPC
	BlockChainFactories["Koto"] = koto.NewKotoRPC
	BlockChainFactories["Koto Testnet"] = koto.NewKotoRPC
	BlockChainFactories["Litecoin"] = litecoin.NewLitecoinRPC
	BlockChainFactories["Litecoin Testnet"] = litecoin.NewLitecoinRPC
	BlockChainFactories["Dogecoin"] = dogecoin.NewDogecoinRPC
	BlockChainFactories["Vertcoin"] = vertcoin.NewVertcoinRPC
	BlockChainFactories["Vertcoin Testnet"] = vertcoin.NewVertcoinRPC
	BlockChainFactories["Namecoin"] = namecoin.NewNamecoinRPC
	BlockChainFactories["Monacoin"] = monacoin.NewMonacoinRPC
	BlockChainFactories["Monacoin Testnet"] = monacoin.NewMonacoinRPC
	BlockChainFactories["MonetaryUnit"] = monetaryunit.NewMonetaryUnitRPC
	BlockChainFactories["DigiByte"] = digibyte.NewDigiByteRPC
	BlockChainFactories["DigiByte Testnet"] = digibyte.NewDigiByteRPC
	BlockChainFactories["Myriad"] = myriad.NewMyriadRPC
	BlockChainFactories["Liquid"] = liquid.NewLiquidRPC
	BlockChainFactories["Groestlcoin"] = grs.NewGroestlcoinRPC
	BlockChainFactories["Groestlcoin Testnet"] = grs.NewGroestlcoinRPC
	BlockChainFactories["PIVX"] = pivx.NewPivXRPC
	BlockChainFactories["PIVX Testnet"] = pivx.NewPivXRPC
	BlockChainFactories["Polis"] = polis.NewPolisRPC
	BlockChainFactories["Firo"] = firo.NewFiroRPC
	BlockChainFactories["Fujicoin"] = fujicoin.NewFujicoinRPC
	BlockChainFactories["Flo"] = flo.NewFloRPC
	BlockChainFactories["Bellcoin"] = bellcoin.NewBellcoinRPC
	BlockChainFactories["Qtum"] = qtum.NewQtumRPC
	BlockChainFactories["Viacoin"] = viacoin.NewViacoinRPC
	BlockChainFactories["Qtum Testnet"] = qtum.NewQtumRPC
	BlockChainFactories["NULS"] = nuls.NewNulsRPC
	BlockChainFactories["VIPSTARCOIN"] = vipstarcoin.NewVIPSTARCOINRPC
	BlockChainFactories["ZelCash"] = zec.NewZCashRPC
	BlockChainFactories["Ravencoin"] = ravencoin.NewRavencoinRPC
	BlockChainFactories["Ritocoin"] = ritocoin.NewRitocoinRPC
	BlockChainFactories["Divi"] = divi.NewDiviRPC
	BlockChainFactories["CPUchain"] = cpuchain.NewCPUchainRPC
	BlockChainFactories["Unobtanium"] = unobtanium.NewUnobtaniumRPC
	BlockChainFactories["DeepOnion"] = deeponion.NewDeepOnionRPC
	BlockChainFactories["SnowGem"] = snowgem.NewSnowGemRPC
	BlockChainFactories["Bitcore"] = bitcore.NewBitcoreRPC
	BlockChainFactories["Omotenashicoin"] = omotenashicoin.NewOmotenashiCoinRPC
	BlockChainFactories["Omotenashicoin Testnet"] = omotenashicoin.NewOmotenashiCoinRPC
	BlockChainFactories["BitZeny"] = bitzeny.NewBitZenyRPC
	BlockChainFactories["Trezarcoin"] = trezarcoin.NewTrezarcoinRPC
}

// GetCoinNameFromConfig gets coin name and coin shortcut from config file
func GetCoinNameFromConfig(configfile string) (string, string, string, error) {
	data, err := ioutil.ReadFile(configfile)
	if err != nil {
		return "", "", "", errors.Annotatef(err, "Error reading file %v", configfile)
	}
	var cn struct {
		CoinName     string `json:"coin_name"`
		CoinShortcut string `json:"coin_shortcut"`
		CoinLabel    string `json:"coin_label"`
	}
	err = json.Unmarshal(data, &cn)
	if err != nil {
		return "", "", "", errors.Annotatef(err, "Error parsing file %v", configfile)
	}
	return cn.CoinName, cn.CoinShortcut, cn.CoinLabel, nil
}

// NewBlockChain creates bchain.BlockChain and bchain.Mempool for the coin passed by the parameter coin
func NewBlockChain(coin string, configfile string, pushHandler func(bchain.NotificationType), metrics *common.Metrics) (bchain.BlockChain, bchain.Mempool, error) {
	data, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "Error reading file %v", configfile)
	}
	var config json.RawMessage
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "Error parsing file %v", configfile)
	}
	bcf, ok := BlockChainFactories[coin]
	if !ok {
		return nil, nil, errors.New(fmt.Sprint("Unsupported coin '", coin, "'. Must be one of ", reflect.ValueOf(BlockChainFactories).MapKeys()))
	}
	bc, err := bcf(config, pushHandler)
	if err != nil {
		return nil, nil, err
	}
	err = bc.Initialize()
	if err != nil {
		return nil, nil, err
	}
	mempool, err := bc.CreateMempool(bc)
	if err != nil {
		return nil, nil, err
	}
	return &blockChainWithMetrics{b: bc, m: metrics}, &mempoolWithMetrics{mempool: mempool, m: metrics}, nil
}

type blockChainWithMetrics struct {
	b bchain.BlockChain
	m *common.Metrics
}

func (c *blockChainWithMetrics) observeRPCLatency(method string, start time.Time, err error) {
	var e string
	if err != nil {
		e = "failure"
	}
	c.m.RPCLatency.With(common.Labels{"method": method, "error": e}).Observe(float64(time.Since(start)) / 1e6) // in milliseconds
}

func (c *blockChainWithMetrics) Initialize() error {
	return c.b.Initialize()
}

func (c *blockChainWithMetrics) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	return c.b.CreateMempool(chain)
}

func (c *blockChainWithMetrics) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	return c.b.InitializeMempool(addrDescForOutpoint, onNewTxAddr, onNewTx)
}

func (c *blockChainWithMetrics) Shutdown(ctx context.Context) error {
	return c.b.Shutdown(ctx)
}

func (c *blockChainWithMetrics) IsTestnet() bool {
	return c.b.IsTestnet()
}

func (c *blockChainWithMetrics) GetNetworkName() string {
	return c.b.GetNetworkName()
}

func (c *blockChainWithMetrics) GetCoinName() string {
	return c.b.GetCoinName()
}

func (c *blockChainWithMetrics) GetSubversion() string {
	return c.b.GetSubversion()
}

func (c *blockChainWithMetrics) GetChainInfo() (v *bchain.ChainInfo, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetChainInfo", s, err) }(time.Now())
	return c.b.GetChainInfo()
}

func (c *blockChainWithMetrics) GetBestBlockHash() (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBestBlockHash", s, err) }(time.Now())
	return c.b.GetBestBlockHash()
}

func (c *blockChainWithMetrics) GetBestBlockHeight() (v uint32, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBestBlockHeight", s, err) }(time.Now())
	return c.b.GetBestBlockHeight()
}

func (c *blockChainWithMetrics) GetBlockHash(height uint32) (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlockHash", s, err) }(time.Now())
	return c.b.GetBlockHash(height)
}

func (c *blockChainWithMetrics) GetBlockHeader(hash string) (v *bchain.BlockHeader, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlockHeader", s, err) }(time.Now())
	return c.b.GetBlockHeader(hash)
}

func (c *blockChainWithMetrics) GetBlock(hash string, height uint32) (v *bchain.Block, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlock", s, err) }(time.Now())
	return c.b.GetBlock(hash, height)
}

func (c *blockChainWithMetrics) GetBlockInfo(hash string) (v *bchain.BlockInfo, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlockInfo", s, err) }(time.Now())
	return c.b.GetBlockInfo(hash)
}

func (c *blockChainWithMetrics) GetMempoolTransactions() (v []string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolTransactions", s, err) }(time.Now())
	return c.b.GetMempoolTransactions()
}

func (c *blockChainWithMetrics) GetTransaction(txid string) (v *bchain.Tx, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetTransaction", s, err) }(time.Now())
	return c.b.GetTransaction(txid)
}

func (c *blockChainWithMetrics) GetTransactionSpecific(tx *bchain.Tx) (v json.RawMessage, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetTransactionSpecific", s, err) }(time.Now())
	return c.b.GetTransactionSpecific(tx)
}

func (c *blockChainWithMetrics) GetTransactionForMempool(txid string) (v *bchain.Tx, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetTransactionForMempool", s, err) }(time.Now())
	return c.b.GetTransactionForMempool(txid)
}

func (c *blockChainWithMetrics) EstimateSmartFee(blocks int, conservative bool) (v big.Int, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EstimateSmartFee", s, err) }(time.Now())
	return c.b.EstimateSmartFee(blocks, conservative)
}

func (c *blockChainWithMetrics) EstimateFee(blocks int) (v big.Int, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EstimateFee", s, err) }(time.Now())
	return c.b.EstimateFee(blocks)
}

func (c *blockChainWithMetrics) SendRawTransaction(tx string) (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("SendRawTransaction", s, err) }(time.Now())
	return c.b.SendRawTransaction(tx)
}

func (c *blockChainWithMetrics) GetMempoolEntry(txid string) (v *bchain.MempoolEntry, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolEntry", s, err) }(time.Now())
	return c.b.GetMempoolEntry(txid)
}

func (c *blockChainWithMetrics) GetChainParser() bchain.BlockChainParser {
	return c.b.GetChainParser()
}

func (c *blockChainWithMetrics) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (v *big.Int, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EthereumTypeGetBalance", s, err) }(time.Now())
	return c.b.EthereumTypeGetBalance(addrDesc)
}

func (c *blockChainWithMetrics) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (v uint64, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EthereumTypeGetNonce", s, err) }(time.Now())
	return c.b.EthereumTypeGetNonce(addrDesc)
}

func (c *blockChainWithMetrics) EthereumTypeEstimateGas(params map[string]interface{}) (v uint64, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EthereumTypeEstimateGas", s, err) }(time.Now())
	return c.b.EthereumTypeEstimateGas(params)
}

func (c *blockChainWithMetrics) EthereumTypeGetErc20ContractInfo(contractDesc bchain.AddressDescriptor) (v *bchain.Erc20Contract, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EthereumTypeGetErc20ContractInfo", s, err) }(time.Now())
	return c.b.EthereumTypeGetErc20ContractInfo(contractDesc)
}

func (c *blockChainWithMetrics) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (v *big.Int, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EthereumTypeGetErc20ContractInfo", s, err) }(time.Now())
	return c.b.EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc)
}

type mempoolWithMetrics struct {
	mempool bchain.Mempool
	m       *common.Metrics
}

func (c *mempoolWithMetrics) observeRPCLatency(method string, start time.Time, err error) {
	var e string
	if err != nil {
		e = "failure"
	}
	c.m.RPCLatency.With(common.Labels{"method": method, "error": e}).Observe(float64(time.Since(start)) / 1e6) // in milliseconds
}

func (c *mempoolWithMetrics) Resync() (count int, err error) {
	defer func(s time.Time) { c.observeRPCLatency("ResyncMempool", s, err) }(time.Now())
	count, err = c.mempool.Resync()
	if err == nil {
		c.m.MempoolSize.Set(float64(count))
	}
	return count, err
}

func (c *mempoolWithMetrics) GetTransactions(address string) (v []bchain.Outpoint, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolTransactions", s, err) }(time.Now())
	return c.mempool.GetTransactions(address)
}

func (c *mempoolWithMetrics) GetAddrDescTransactions(addrDesc bchain.AddressDescriptor) (v []bchain.Outpoint, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolTransactionsForAddrDesc", s, err) }(time.Now())
	return c.mempool.GetAddrDescTransactions(addrDesc)
}

func (c *mempoolWithMetrics) GetAllEntries() (v bchain.MempoolTxidEntries) {
	defer func(s time.Time) { c.observeRPCLatency("GetAllEntries", s, nil) }(time.Now())
	return c.mempool.GetAllEntries()
}

func (c *mempoolWithMetrics) GetTransactionTime(txid string) uint32 {
	return c.mempool.GetTransactionTime(txid)
}
