package db

import (
	"blockbook/bchain"
	"blockbook/common"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	"unsafe"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/tecbot/gorocksdb"
	"github.com/martinboehm/btcutil/txscript"
)

const dbVersion = 5

const maxAddrDescLen = 1024
// iterator creates snapshot, which takes lots of resources
// when doing huge scan, it is better to close it and reopen from time to time to free the resources
const refreshIterator = 5000000

// FiatRatesTimeFormat is a format string for storing FiatRates timestamps in rocksdb
const FiatRatesTimeFormat = "20060102150405" // YYYYMMDDhhmmss

// CurrencyRatesTicker contains coin ticker data fetched from API
type CurrencyRatesTicker struct {
	Timestamp *time.Time // return as unix timestamp in API
	Rates     map[string]float64
}

// ResultTickerAsString contains formatted CurrencyRatesTicker data
type ResultTickerAsString struct {
	Timestamp int64              `json:"ts,omitempty"`
	Rates     map[string]float64 `json:"rates"`
	Error     string             `json:"error,omitempty"`
}

// ResultTickersAsString contains a formatted CurrencyRatesTicker list
type ResultTickersAsString struct {
	Tickers []ResultTickerAsString `json:"tickers"`
}

// ResultTickerListAsString contains formatted data about available currency tickers
type ResultTickerListAsString struct {
	Timestamp int64    `json:"ts,omitempty"`
	Tickers   []string `json:"available_currencies"`
	Error     string   `json:"error,omitempty"`
}

// RepairRocksDB calls RocksDb db repair function
func RepairRocksDB(name string) error {
	glog.Infof("rocksdb: repair")
	opts := gorocksdb.NewDefaultOptions()
	return gorocksdb.RepairDb(name, opts)
}

type connectBlockStats struct {
	txAddressesHit  int
	txAddressesMiss int
	balancesHit     int
	balancesMiss    int
}


// RocksDB handle
type RocksDB struct {
	path         string
	db           *gorocksdb.DB
	wo           *gorocksdb.WriteOptions
	ro           *gorocksdb.ReadOptions
	cfh          []*gorocksdb.ColumnFamilyHandle
	chainParser  bchain.BlockChainParser
	is           *common.InternalState
	metrics      *common.Metrics
	cache        *gorocksdb.Cache
	maxOpenFiles int
	cbs          connectBlockStats
}

const (
	cfDefault = iota
	cfHeight
	cfAddresses
	cfBlockTxs
	cfTransactions
	cfFiatRates
	// BitcoinType
	cfAddressBalance
	cfTxAddresses
	// SyscoinType
	cfAssets
	cfTxAssets
	// EthereumType
	cfAddressContracts = cfAddressBalance

)

// common columns
var cfNames []string
var cfBaseNames = []string{"default", "height", "addresses", "blockTxs", "transactions", "fiatRates"}

// type specific columns
var cfNamesBitcoinType = []string{"addressBalance", "txAddresses", "assets", "txAssets"}
var cfNamesEthereumType = []string{"addressContracts"}

func openDB(path string, c *gorocksdb.Cache, openFiles int) (*gorocksdb.DB, []*gorocksdb.ColumnFamilyHandle, error) {
	// opts with bloom filter
	opts := createAndSetDBOptions(10, c, openFiles)
	// opts for addresses without bloom filter
	// from documentation: if most of your queries are executed using iterators, you shouldn't set bloom filter
	optsAddresses := createAndSetDBOptions(0, c, openFiles)
	// default, height, addresses, blockTxids, transactions
	cfOptions := []*gorocksdb.Options{opts, opts, optsAddresses, opts, opts, opts}
	// append type specific options
	count := len(cfNames) - len(cfOptions)
	for i := 0; i < count; i++ {
		cfOptions = append(cfOptions, opts)
	}
	db, cfh, err := gorocksdb.OpenDbColumnFamilies(opts, path, cfNames, cfOptions)
	if err != nil {
		return nil, nil, err
	}
	return db, cfh, nil
}

// NewRocksDB opens an internal handle to RocksDB environment.  Close
// needs to be called to release it.
func NewRocksDB(path string, cacheSize, maxOpenFiles int, parser bchain.BlockChainParser, metrics *common.Metrics) (d *RocksDB, err error) {
	glog.Infof("rocksdb: opening %s, required data version %v, cache size %v, max open files %v", path, dbVersion, cacheSize, maxOpenFiles)

	cfNames = append([]string{}, cfBaseNames...)
	chainType := parser.GetChainType()
	if chainType == bchain.ChainBitcoinType {
		cfNames = append(cfNames, cfNamesBitcoinType...)
	} else if chainType == bchain.ChainEthereumType {
		cfNames = append(cfNames, cfNamesEthereumType...)
	} else {
		return nil, errors.New("Unknown chain type")
	}

	c := gorocksdb.NewLRUCache(cacheSize)
	db, cfh, err := openDB(path, c, maxOpenFiles)
	if err != nil {
		return nil, err
	}
	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()
	return &RocksDB{path, db, wo, ro, cfh, parser, nil, metrics, c, maxOpenFiles, connectBlockStats{}}, nil
}

func (d *RocksDB) closeDB() error {
	for _, h := range d.cfh {
		h.Destroy()
	}
	d.db.Close()
	d.db = nil
	return nil
}

// FiatRatesConvertDate checks if the date is in correct format and returns the Time object.
// Possible formats are: YYYYMMDDhhmmss, YYYYMMDDhhmm, YYYYMMDDhh, YYYYMMDD
func FiatRatesConvertDate(date string) (*time.Time, error) {
	for format := FiatRatesTimeFormat; len(format) >= 8; format = format[:len(format)-2] {
		convertedDate, err := time.Parse(format, date)
		if err == nil {
			return &convertedDate, nil
		}
	}
	msg := "Date \"" + date + "\" does not match any of available formats. "
	msg += "Possible formats are: YYYYMMDDhhmmss, YYYYMMDDhhmm, YYYYMMDDhh, YYYYMMDD"
	return nil, errors.New(msg)
}

// FiatRatesStoreTicker stores ticker data at the specified time
func (d *RocksDB) FiatRatesStoreTicker(ticker *CurrencyRatesTicker) error {
	if len(ticker.Rates) == 0 {
		return errors.New("Error storing ticker: empty rates")
	} else if ticker.Timestamp == nil {
		return errors.New("Error storing ticker: empty timestamp")
	}
	ratesMarshalled, err := json.Marshal(ticker.Rates)
	if err != nil {
		glog.Error("Error marshalling ticker rates: ", err)
		return err
	}
	timeFormatted := ticker.Timestamp.UTC().Format(FiatRatesTimeFormat)
	err = d.db.PutCF(d.wo, d.cfh[cfFiatRates], []byte(timeFormatted), ratesMarshalled)
	if err != nil {
		glog.Error("Error storing ticker: ", err)
		return err
	}
	return nil
}

// FiatRatesFindTicker gets FiatRates data closest to the specified timestamp
func (d *RocksDB) FiatRatesFindTicker(tickerTime *time.Time) (*CurrencyRatesTicker, error) {
	ticker := &CurrencyRatesTicker{}
	tickerTimeFormatted := tickerTime.UTC().Format(FiatRatesTimeFormat)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.Seek([]byte(tickerTimeFormatted)); it.Valid(); it.Next() {
		timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
		if err != nil {
			glog.Error("FiatRatesFindTicker time parse error: ", err)
			return nil, err
		}
		timeObj = timeObj.UTC()
		ticker.Timestamp = &timeObj
		err = json.Unmarshal(it.Value().Data(), &ticker.Rates)
		if err != nil {
			glog.Error("FiatRatesFindTicker error unpacking rates: ", err)
			return nil, err
		}
		break
	}
	if err := it.Err(); err != nil {
		glog.Error("FiatRatesFindTicker Iterator error: ", err)
		return nil, err
	}
	if !it.Valid() {
		return nil, nil // ticker not found
	}
	return ticker, nil
}

// FiatRatesFindLastTicker gets the last FiatRates record
func (d *RocksDB) FiatRatesFindLastTicker() (*CurrencyRatesTicker, error) {
	ticker := &CurrencyRatesTicker{}
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.SeekToLast(); it.Valid(); it.Next() {
		timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
		if err != nil {
			glog.Error("FiatRatesFindTicker time parse error: ", err)
			return nil, err
		}
		timeObj = timeObj.UTC()
		ticker.Timestamp = &timeObj
		err = json.Unmarshal(it.Value().Data(), &ticker.Rates)
		if err != nil {
			glog.Error("FiatRatesFindTicker error unpacking rates: ", err)
			return nil, err
		}
		break
	}
	if err := it.Err(); err != nil {
		glog.Error("FiatRatesFindLastTicker Iterator error: ", err)
		return ticker, err
	}
	if !it.Valid() {
		return nil, nil // ticker not found
	}
	return ticker, nil
}

// Close releases the RocksDB environment opened in NewRocksDB.
func (d *RocksDB) Close() error {
	if d.db != nil {
		// store the internal state of the app
		if d.is != nil && d.is.DbState == common.DbStateOpen {
			d.is.DbState = common.DbStateClosed
			if err := d.StoreInternalState(d.is); err != nil {
				glog.Info("internalState: ", err)
			}
		}
		glog.Infof("rocksdb: close")
		d.closeDB()
		d.wo.Destroy()
		d.ro.Destroy()
	}
	return nil
}

// Reopen reopens the database
// It closes and reopens db, nobody can access the database during the operation!
func (d *RocksDB) Reopen() error {
	err := d.closeDB()
	if err != nil {
		return err
	}
	d.db = nil
	db, cfh, err := openDB(d.path, d.cache, d.maxOpenFiles)
	if err != nil {
		return err
	}
	d.db, d.cfh = db, cfh
	return nil
}

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

// GetMemoryStats returns memory usage statistics as reported by RocksDB
func (d *RocksDB) GetMemoryStats() string {
	var total, indexAndFilter, memtable int
	type columnStats struct {
		name           string
		indexAndFilter string
		memtable       string
	}
	cs := make([]columnStats, len(cfNames))
	for i := 0; i < len(cfNames); i++ {
		cs[i].name = cfNames[i]
		cs[i].indexAndFilter = d.db.GetPropertyCF("rocksdb.estimate-table-readers-mem", d.cfh[i])
		cs[i].memtable = d.db.GetPropertyCF("rocksdb.cur-size-all-mem-tables", d.cfh[i])
		indexAndFilter += atoi(cs[i].indexAndFilter)
		memtable += atoi(cs[i].memtable)
	}
	m := struct {
		cacheUsage       int
		pinnedCacheUsage int
		columns          []columnStats
	}{
		cacheUsage:       d.cache.GetUsage(),
		pinnedCacheUsage: d.cache.GetPinnedUsage(),
		columns:          cs,
	}
	total = m.cacheUsage + indexAndFilter + memtable
	return fmt.Sprintf("Total %d, indexAndFilter %d, memtable %d, %+v", total, indexAndFilter, memtable, m)
}

// StopIteration is returned by callback function to signal stop of iteration
type StopIteration struct{}

func (e *StopIteration) Error() string {
	return ""
}

// GetTransactionsCallback is called by GetTransactions/GetAddrDescTransactions for each found tx
// indexes contain array of indexes (input negative, output positive) in tx where is given address
type GetTransactionsCallback func(txid string, height uint32, indexes []int32) error

// GetTransactions finds all input/output transactions for address
// Transaction are passed to callback function.
func (d *RocksDB) GetTransactions(address string, lower uint32, higher uint32, fn GetTransactionsCallback) (err error) {
	if glog.V(1) {
		glog.Infof("rocksdb: address get %s %d-%d ", address, lower, higher)
	}
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return err
	}
	return d.GetAddrDescTransactions(addrDesc, lower, higher, bchain.AllMask, fn)
}

// GetAddrDescTransactions finds all input/output transactions for address descriptor
// Transaction are passed to callback function in the order from newest block to the oldest
func (d *RocksDB) GetAddrDescTransactions(addrDesc bchain.AddressDescriptor, lower uint32, higher uint32, assetsBitMask bchain.AssetsMask, fn GetTransactionsCallback) (err error) {
	assetsBitMaskUint := uint32(assetsBitMask)
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	txIndexUnpackedLen := d.chainParser.PackedTxIndexLen()
	addrDescLen := len(addrDesc)
	startKey := d.chainParser.PackAddressKey(addrDesc, higher)
	stopKey := d.chainParser.PackAddressKey(addrDesc, lower)
	indexes := make([]int32, 0, 16)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfAddresses])
	defer it.Close()
	for it.Seek(startKey); it.Valid(); it.Next() {
		key := it.Key().Data()
		if bytes.Compare(key, stopKey) > 0 {
			break
		}
		if len(key) != addrDescLen+bchain.PackedHeightBytes {
			if glog.V(2) {
				glog.Warningf("rocksdb: addrDesc %s - mixed with %s", addrDesc, hex.EncodeToString(key))
			}
			continue
		}
		val := it.Value().Data()
		if glog.V(2) {
			glog.Infof("rocksdb: addresses %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
		}
		_, height, err := d.chainParser.UnpackAddressKey(key)
		if err != nil {
			return err
		}
		for len(val) > txIndexUnpackedLen {
			mask, l := d.chainParser.UnpackTxIndexType(val)
			maskUint := uint32(mask)
			tx, err := d.chainParser.UnpackTxid(val[l:l+txidUnpackedLen])
			if err != nil {
				return err
			}
			indexes = indexes[:0]
			val = val[l+txidUnpackedLen:]
			err = d.chainParser.UnpackTxIndexes(&indexes, &val)
			if err != nil {
				glog.Warningf("rocksdb: addresses contain incorrect data %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
				break
			}
			if assetsBitMask == bchain.AllMask || mask == bchain.AllMask || (assetsBitMaskUint & maskUint) == maskUint {
				if err := fn(tx, height, indexes); err != nil {
					if _, ok := err.(*StopIteration); ok {
						return nil
					}
					return err
				}
			}
		}
		if len(val) != 0 {
			glog.Warningf("rocksdb: addresses contain incorrect data %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
		}
	}
	return nil
}

const (
	opInsert = 0
	opDelete = 1
)

// ConnectBlock indexes addresses in the block and stores them in db
func (d *RocksDB) ConnectBlock(block *bchain.Block) error {
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()

	if glog.V(2) {
		glog.Infof("rocksdb: insert %d %s", block.Height, block.Hash)
	}

	chainType := d.chainParser.GetChainType()

	if err := d.writeHeightFromBlock(wb, block, opInsert); err != nil {
		return err
	}
	addresses := make(bchain.AddressesMap)
	if chainType == bchain.ChainBitcoinType {
		assets := make(map[uint32]*bchain.Asset)
		txAssets := make(bchain.TxAssetMap, 0)
		txAddressesMap := make(map[string]*bchain.TxAddresses)
		balances := make(map[string]*bchain.AddrBalance)
		if err := d.processAddressesBitcoinType(block, addresses, txAddressesMap, balances, assets, txAssets); err != nil {
			return err
		}
		if err := d.storeTxAddresses(wb, txAddressesMap); err != nil {
			return err
		}
		if err := d.storeBalances(wb, balances); err != nil {
			return err
		}
		if err := d.storeAndCleanupBlockTxs(wb, block); err != nil {
			return err
		}
		if err := d.storeAssets(wb, assets); err != nil {
			return err
		}
		if err := d.storeTxAssets(wb, txAssets); err != nil {
			return err
		}
	} else if chainType == bchain.ChainEthereumType {
		addressContracts := make(map[string]*AddrContracts)
		blockTxs, err := d.processAddressesEthereumType(block, addresses, addressContracts)
		if err != nil {
			return err
		}
		if err := d.storeAddressContracts(wb, addressContracts); err != nil {
			return err
		}
		if err := d.storeAndCleanupBlockTxsEthereumType(wb, block, blockTxs); err != nil {
			return err
		}
	} else {
		return errors.New("Unknown chain type")
	}
	if err := d.storeAddresses(wb, block.Height, addresses); err != nil {
		return err
	}
	if err := d.db.Write(d.wo, wb); err != nil {
		return err
	}
	d.is.AppendBlockTime(uint32(block.Time))
	return nil
}

func (d *RocksDB) resetValueSatToZero(valueSat *big.Int, addrDesc bchain.AddressDescriptor, logText string) {
	ad, _, err := d.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		glog.Warningf("rocksdb: unparsable address hex '%v' reached negative %s %v, resetting to 0. Parser error %v", addrDesc, logText, valueSat.String(), err)
	} else {
		glog.Warningf("rocksdb: address %v hex '%v' reached negative %s %v, resetting to 0", ad, addrDesc, logText, valueSat.String())
	}
	valueSat.SetInt64(0)
}

// GetAndResetConnectBlockStats gets statistics about cache usage in connect blocks and resets the counters
func (d *RocksDB) GetAndResetConnectBlockStats() string {
	s := fmt.Sprintf("%+v", d.cbs)
	d.cbs = connectBlockStats{}
	return s
}

func (d *RocksDB) processAddressesBitcoinType(block *bchain.Block, addresses bchain.AddressesMap, txAddressesMap map[string]*bchain.TxAddresses, balances map[string]*bchain.AddrBalance, assets map[uint32]*bchain.Asset, txAssets bchain.TxAssetMap) error {
	blockTxIDs := make([][]byte, len(block.Txs))
	blockTxAddresses := make([]*bchain.TxAddresses, len(block.Txs))
	blockTxAssetAddresses := make(bchain.TxAssetAddressMap)
	// first process all outputs so that inputs can refer to txs in this block
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		var asset *bchain.Asset = nil
		isActivate := d.chainParser.IsAssetActivateTx(tx.Version)
		isAssetTx := d.chainParser.IsAssetTx(tx.Version)
		isAssetSendTx := d.chainParser.IsAssetSendTx(tx.Version)
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		blockTxIDs[txi] = btxID
		ta := bchain.TxAddresses{Version: tx.Version, Height: block.Height}
		ta.Outputs = make([]bchain.TxOutput, len(tx.Vout))
		txAddressesMap[string(btxID)] = &ta
		blockTxAddresses[txi] = &ta
		maxAddrDescLen := d.chainParser.GetMaxAddrLength()
		assetsMask := d.chainParser.GetAssetsMaskFromVersion(tx.Version)
		for i, output := range tx.Vout {
			tao := &ta.Outputs[i]
			tao.ValueSat = output.ValueSat
			mask := bchain.BaseCoinMask
			if output.AssetInfo != nil {
				mask = assetsMask
			}
			addrDesc, err := d.chainParser.GetAddrDescFromVout(&output)
			if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
				if err != nil {
					// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
					if err != bchain.ErrAddressMissing {
						glog.Warningf("rocksdb: addrDesc: %v - height %d, tx %v, output %v, error %v", err, block.Height, tx.Txid, output, err)
					}
				} else {
					glog.V(1).Infof("rocksdb: height %d, tx %v, vout %v, skipping addrDesc of length %d", block.Height, tx.Txid, i, len(addrDesc))
				}
				continue
			}
			tao.AddrDesc = addrDesc
			if output.AssetInfo != nil {
				tao.AssetInfo = &bchain.AssetInfo{AssetGuid: output.AssetInfo.AssetGuid, ValueSat: new(big.Int).Set(output.AssetInfo.ValueSat)}
			}
			if d.chainParser.IsAddrDescIndexable(addrDesc) {
				strAddrDesc := string(addrDesc)
				balance, e := balances[strAddrDesc]
				if !e {
					balance, err = d.GetAddrDescBalance(addrDesc, bchain.AddressBalanceDetailUTXOIndexed)
					if err != nil {
						return err
					}
					if balance == nil {
						balance = &bchain.AddrBalance{}
					}
					balances[strAddrDesc] = balance
					d.cbs.balancesMiss++
				} else {
					d.cbs.balancesHit++
				}
				balance.BalanceSat.Add(&balance.BalanceSat, &output.ValueSat)
				balance.AddUtxo(&bchain.Utxo{
					BtxID:    btxID,
					Vout:     int32(i),
					Height:   block.Height,
					ValueSat: output.ValueSat,
					AssetInfo: tao.AssetInfo,
				})
				counted := addToAddressesMap(addresses, strAddrDesc, btxID, int32(i), mask)
				if !counted {
					balance.Txs++
				}
				if tao.AssetInfo != nil {
					if balance.AssetBalances == nil {
						balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
					}
					balanceAsset, ok := balance.AssetBalances[tao.AssetInfo.AssetGuid]
					if !ok {
						balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceSat: big.NewInt(0), SentSat: big.NewInt(0)}
						balance.AssetBalances[tao.AssetInfo.AssetGuid] = balanceAsset
					}
					err = d.ConnectAllocationOutput(&addrDesc, block.Height, balanceAsset, isActivate, tx.Version, btxID, tao.AssetInfo, assets, txAssets, blockTxAssetAddresses)
					if err != nil {
						return err
					}
				}
			} else if ((isAssetTx || isAssetSendTx) && asset == nil && addrDesc[0] == txscript.OP_RETURN) {
				asset, err = d.chainParser.GetAssetFromDesc(&addrDesc)
				if err != nil {
					return err
				}
			}
		}
		if asset != nil {
			err = d.ConnectAssetOutput(asset, isActivate, isAssetTx, isAssetSendTx, assets)
			if err != nil {
				return err
			}
		}
	}
	// process inputs
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		spendingTxid := blockTxIDs[txi]
		ta := blockTxAddresses[txi]
		ta.Inputs = make([]bchain.TxInput, len(tx.Vin))
		logged := false
		for i, input := range tx.Vin {
			tai := &ta.Inputs[i]
			btxID, err := d.chainParser.PackTxid(input.Txid)
			if err != nil {
				// do not process inputs without input txid
				if err == bchain.ErrTxidMissing {
					continue
				}
				return err
			}
			stxID := string(btxID)
			ita, e := txAddressesMap[stxID]
			if !e {
				ita, err = d.getTxAddresses(btxID)
				if err != nil {
					return err
				}
				if ita == nil {
					// allow parser to process unknown input, some coins may implement special handling, default is to log warning
					tai.AddrDesc = d.chainParser.GetAddrDescForUnknownInput(tx, i)
					continue
				}
				txAddressesMap[stxID] = ita
				d.cbs.txAddressesMiss++
			} else {
				d.cbs.txAddressesHit++
			}
			if len(ita.Outputs) <= int(input.Vout) {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is out of bounds of stored tx", block.Height, tx.Txid, input.Txid, input.Vout)
				continue
			}
			spentOutput := &ita.Outputs[int(input.Vout)]
			if spentOutput.Spent {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is double spend", block.Height, tx.Txid, input.Txid, input.Vout)
			}


			tai.AddrDesc = spentOutput.AddrDesc
			tai.ValueSat = spentOutput.ValueSat
			mask := bchain.BaseCoinMask
			if spentOutput.AssetInfo != nil {
				tai.AssetInfo = &bchain.AssetInfo{AssetGuid: spentOutput.AssetInfo.AssetGuid, ValueSat: new(big.Int).Set(spentOutput.AssetInfo.ValueSat)}
				mask = d.chainParser.GetAssetsMaskFromVersion(ita.Version)
			}
			// mark the output as spent in tx
			spentOutput.Spent = true
			if len(spentOutput.AddrDesc) == 0 {
				if !logged {
					glog.V(1).Infof("rocksdb: height %d, tx %v, input tx %v vout %v skipping empty address", block.Height, tx.Txid, input.Txid, input.Vout)
					logged = true
				}
				continue
			}
			if d.chainParser.IsAddrDescIndexable(spentOutput.AddrDesc) {
				strAddrDesc := string(spentOutput.AddrDesc)
				balance, e := balances[strAddrDesc]
				if !e {
					balance, err = d.GetAddrDescBalance(spentOutput.AddrDesc, bchain.AddressBalanceDetailUTXOIndexed)
					if err != nil {
						return err
					}
					if balance == nil {
						balance = &bchain.AddrBalance{}
					}
					balances[strAddrDesc] = balance
					d.cbs.balancesMiss++
				} else {
					d.cbs.balancesHit++
				}
				counted := addToAddressesMap(addresses, strAddrDesc, spendingTxid, ^int32(i), mask)
				if !counted {
					balance.Txs++
				}
				balance.BalanceSat.Sub(&balance.BalanceSat, &spentOutput.ValueSat)
				balance.MarkUtxoAsSpent(btxID, int32(input.Vout))
				if balance.BalanceSat.Sign() < 0 {
					d.resetValueSatToZero(&balance.BalanceSat, spentOutput.AddrDesc, "balance")
				}
				balance.SentSat.Add(&balance.SentSat, &spentOutput.ValueSat)
				if spentOutput.AssetInfo != nil {
					if balance.AssetBalances == nil {
						balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
					}
					balanceAsset, ok := balance.AssetBalances[spentOutput.AssetInfo.AssetGuid]
					if !ok {
						balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceSat: big.NewInt(0), SentSat: big.NewInt(0)}
						balance.AssetBalances[spentOutput.AssetInfo.AssetGuid] = balanceAsset
					}
					err := d.ConnectAllocationInput(&spentOutput.AddrDesc, balanceAsset, btxID, spentOutput.AssetInfo, blockTxAssetAddresses)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// addToAddressesMap maintains mapping between addresses and transactions in one block
// the method assumes that outputs in the block are processed before the inputs
// the return value is true if the tx was processed before, to not to count the tx multiple times
func addToAddressesMap(addresses bchain.AddressesMap, strAddrDesc string, btxID []byte, index int32, assetsMask bchain.AssetsMask) bool {
	// check that the address was already processed in this block
	// if not found, it has certainly not been counted
	at, found := addresses[strAddrDesc]
	if found {
		// if the tx is already in the slice, append the index to the array of indexes
		for i, t := range at {
			if bytes.Equal(btxID, t.BtxID) {
				at[i].Indexes = append(t.Indexes, index)
				// add the mask to the existing type incase there are multiple types in one transaction (ie: asset update + asset allocation send + syscoin send)
				at[i].Type |= assetsMask
				return true
			}
		}
	}

	addresses[strAddrDesc] = append(at, bchain.TxIndexes{
		Type:    assetsMask,
		BtxID:   btxID,
		Indexes: []int32{index},
	})
	return false
}

func (d *RocksDB) storeAddresses(wb *gorocksdb.WriteBatch, height uint32, addresses bchain.AddressesMap) error {
	for addrDesc, txi := range addresses {
		ba := bchain.AddressDescriptor(addrDesc)
		key := d.chainParser.PackAddressKey(ba, height)
		val := d.chainParser.PackTxIndexes(txi)
		wb.PutCF(d.cfh[cfAddresses], key, val)
	}
	return nil
}


func (d *RocksDB) storeTxAddresses(wb *gorocksdb.WriteBatch, am map[string]*bchain.TxAddresses) error {
	varBuf := make([]byte, d.chainParser.MaxPackedBigintBytes())
	buf := make([]byte, 1024)
	for txID, ta := range am {
		buf = d.chainParser.PackTxAddresses(ta, buf, varBuf)
		wb.PutCF(d.cfh[cfTxAddresses], []byte(txID), buf)
	}
	return nil
}

func (d *RocksDB) storeBalances(wb *gorocksdb.WriteBatch, abm map[string]*bchain.AddrBalance) error {
	// allocate buffer initial buffer
	buf := make([]byte, 1024)
	varBuf := make([]byte, d.chainParser.MaxPackedBigintBytes())
	for addrDesc, ab := range abm {
		// balance with 0 transactions is removed from db - happens on disconnect
		if ab == nil || ab.Txs <= 0 {
			glog.Warning("txs <= 0")
			wb.DeleteCF(d.cfh[cfAddressBalance], bchain.AddressDescriptor(addrDesc))
		} else {
			// asset transfers with 0 transactions are removed from db - happens on disconnect
			for key, value := range ab.AssetBalances {
				if value.Transfers <= 0 {
					delete(ab.AssetBalances, key)
				}
			}
			buf = d.chainParser.PackAddrBalance(ab, buf, varBuf)
			wb.PutCF(d.cfh[cfAddressBalance], bchain.AddressDescriptor(addrDesc), buf)
		}
	}
	return nil
}

func (d *RocksDB) cleanupBlockTxs(wb *gorocksdb.WriteBatch, block *bchain.Block) error {
	keep := d.chainParser.KeepBlockAddresses()
	// cleanup old block address
	if block.Height > uint32(keep) {
		for rh := block.Height - uint32(keep); rh > 0; rh-- {
			key := d.chainParser.PackUint(rh)
			val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], key)
			if err != nil {
				return err
			}
			// nil data means the key was not found in DB
			if val.Data() == nil {
				break
			}
			val.Free()
			d.db.DeleteCF(d.wo, d.cfh[cfBlockTxs], key)
		}
	}
	return nil
}

func (d *RocksDB) storeAndCleanupBlockTxs(wb *gorocksdb.WriteBatch, block *bchain.Block) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, pl*len(block.Txs))
	varBuf := make([]byte, vlq.MaxLen64)
	zeroTx := make([]byte, pl)
	for i := range block.Txs {
		tx := &block.Txs[i]
		o := make([]bchain.DbOutpoint, len(tx.Vin))
		for v := range tx.Vin {
			vin := &tx.Vin[v]
			btxID, err := d.chainParser.PackTxid(vin.Txid)
			if err != nil {
				// do not process inputs without input txid
				if err == bchain.ErrTxidMissing {
					btxID = zeroTx
				} else {
					return err
				}
			}
			o[v].BtxID = btxID
			o[v].Index = int32(vin.Vout)
		}
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		buf = append(buf, btxID...)
		l := d.chainParser.PackVaruint(uint(len(o)), varBuf)
		buf = append(buf, varBuf[:l]...)
		buf = append(buf, d.chainParser.PackOutpoints(o)...)
	}
	key := d.chainParser.PackUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	return d.cleanupBlockTxs(wb, block)
}

func (d *RocksDB) getBlockTxs(height uint32) ([]bchain.BlockTxs, error) {
	pl := d.chainParser.PackedTxidLen()
	val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], d.chainParser.PackUint(height))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	bt := make([]bchain.BlockTxs, 0, 8)
	for i := 0; i < len(buf); {
		if len(buf)-i < pl {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, errors.New("Inconsistent data in blockTxs")
		}
		txid := append([]byte(nil), buf[i:i+pl]...)
		i += pl
		o, ol, err := d.chainParser.UnpackNOutpoints(buf[i:])
		if err != nil {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, errors.New("Inconsistent data in blockTxs")
		}
		bt = append(bt, bchain.BlockTxs{
			BtxID:  txid,
			Inputs: o,
		})
		i += ol
	}
	return bt, nil
}

// GetAddrDescBalance returns AddrBalance for given addrDesc
func (d *RocksDB) GetAddrDescBalance(addrDesc bchain.AddressDescriptor, detail bchain.AddressBalanceDetail) (*bchain.AddrBalance, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfAddressBalance], addrDesc)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// 4 is minimum length of addrBalance - 1 byte txs, 1 byte sent, 1 byte balance, 1 byte assetinfo flag
	if len(buf) < 4 {
		return nil, nil
	}
	return d.chainParser.UnpackAddrBalance(buf, d.chainParser.PackedTxidLen(), detail)
}

// GetAddressBalance returns address balance for an address or nil if address not found
func (d *RocksDB) GetAddressBalance(address string, detail bchain.AddressBalanceDetail) (*bchain.AddrBalance, error) {
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	return d.GetAddrDescBalance(addrDesc, detail)
}

func (d *RocksDB) getTxAddresses(btxID []byte) (*bchain.TxAddresses, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfTxAddresses], btxID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// 2 is minimum length of addrBalance - 1 byte height, 1 byte inputs len, 1 byte outputs len
	if len(buf) < 3 {
		return nil, nil
	}
	return d.chainParser.UnpackTxAddresses(buf)
}

// GetTxAddresses returns TxAddresses for given txid or nil if not found
func (d *RocksDB) GetTxAddresses(txid string) (*bchain.TxAddresses, error) {
	btxID, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, err
	}
	return d.getTxAddresses(btxID)
}

// AddrDescForOutpoint is a function that returns address descriptor and value for given outpoint or nil if outpoint not found
func (d *RocksDB) AddrDescForOutpoint(outpoint bchain.Outpoint) (bchain.AddressDescriptor, *big.Int) {
	ta, err := d.GetTxAddresses(outpoint.Txid)
	if err != nil || ta == nil {
		return nil, nil
	}
	if outpoint.Vout < 0 {
		vin := ^outpoint.Vout
		if len(ta.Inputs) <= int(vin) {
			return nil, nil
		}
		return ta.Inputs[vin].AddrDesc, &ta.Inputs[vin].ValueSat
	}
	if len(ta.Outputs) <= int(outpoint.Vout) {
		return nil, nil
	}
	return ta.Outputs[outpoint.Vout].AddrDesc, &ta.Outputs[outpoint.Vout].ValueSat
}

// GetBestBlock returns the block hash of the block with highest height in the db
func (d *RocksDB) GetBestBlock() (uint32, string, error) {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfHeight])
	defer it.Close()
	if it.SeekToLast(); it.Valid() {
		bestHeight := d.chainParser.UnpackUint(it.Key().Data())
		info, err := d.chainParser.UnpackBlockInfo(it.Value().Data())
		if info != nil {
			if glog.V(1) {
				glog.Infof("rocksdb: bestblock %d %+v", bestHeight, info)
			}
			return bestHeight, info.Hash, err
		}
	}
	return 0, "", nil
}

// GetBlockHash returns block hash at given height or empty string if not found
func (d *RocksDB) GetBlockHash(height uint32) (string, error) {
	key := d.chainParser.PackUint(height)
	val, err := d.db.GetCF(d.ro, d.cfh[cfHeight], key)
	if err != nil {
		return "", err
	}
	defer val.Free()
	info, err := d.chainParser.UnpackBlockInfo(val.Data())
	if info == nil {
		return "", err
	}
	return info.Hash, nil
}

// GetBlockInfo returns block info stored in db
func (d *RocksDB) GetBlockInfo(height uint32) (*bchain.DbBlockInfo, error) {
	key := d.chainParser.PackUint(height)
	val, err := d.db.GetCF(d.ro, d.cfh[cfHeight], key)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	bi, err := d.chainParser.UnpackBlockInfo(val.Data())
	if err != nil || bi == nil {
		return nil, err
	}
	bi.Height = height
	return bi, err
}

func (d *RocksDB) writeHeightFromBlock(wb *gorocksdb.WriteBatch, block *bchain.Block, op int) error {
	return d.writeHeight(wb, block.Height, &bchain.DbBlockInfo{
		Hash:   block.Hash,
		Time:   block.Time,
		Txs:    uint32(len(block.Txs)),
		Size:   uint32(block.Size),
		Height: block.Height,
	}, op)
}

func (d *RocksDB) writeHeight(wb *gorocksdb.WriteBatch, height uint32, bi *bchain.DbBlockInfo, op int) error {
	key := d.chainParser.PackUint(height)
	switch op {
	case opInsert:
		val, err := d.chainParser.PackBlockInfo(bi)
		if err != nil {
			return err
		}
		wb.PutCF(d.cfh[cfHeight], key, val)
		d.is.UpdateBestHeight(height)
	case opDelete:
		wb.DeleteCF(d.cfh[cfHeight], key)
		d.is.UpdateBestHeight(height - 1)
	}
	return nil
}

// Disconnect blocks
func (d *RocksDB) disconnectTxAddressesInputs(btxID []byte, inputs []bchain.DbOutpoint, txa *bchain.TxAddresses, txAddressesToUpdate map[string]*bchain.TxAddresses,
	getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error),
	addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool,
	assetFoundInTx func(asset uint32, btxID []byte) bool,
	assets map[uint32]*bchain.Asset, blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	var err error
	for i, t := range txa.Inputs {
		if len(t.AddrDesc) > 0 {
			input := &inputs[i]
			exist := addressFoundInTx(t.AddrDesc, btxID)
			s := string(input.BtxID)
			sa, found := txAddressesToUpdate[s]
			if !found {
				sa, err = d.getTxAddresses(input.BtxID)
				if err != nil {
					return err
				}
				if sa != nil {
					txAddressesToUpdate[s] = sa
				}
			}
			var inputHeight uint32
			if sa != nil {
				sa.Outputs[input.Index].Spent = false
				inputHeight = sa.Height
			}
			if d.chainParser.IsAddrDescIndexable(t.AddrDesc) {
				balance, err := getAddressBalance(t.AddrDesc)
				if err != nil {
					return err
				}
				if balance != nil {
					// subtract number of txs only once
					if !exist {
						balance.Txs--
					}
					balance.SentSat.Sub(&balance.SentSat, &t.ValueSat)
					if balance.SentSat.Sign() < 0 {
						d.resetValueSatToZero(&balance.SentSat, t.AddrDesc, "sent amount")
					}
					balance.BalanceSat.Add(&balance.BalanceSat, &t.ValueSat)
					balance.AddUtxoInDisconnect(&bchain.Utxo{
						BtxID:    input.BtxID,
						Vout:     input.Index,
						Height:   inputHeight,
						ValueSat: t.ValueSat,
						AssetInfo: t.AssetInfo,
					})
					if t.AssetInfo != nil {
						if balance.AssetBalances == nil {
							return errors.New("DisconnectSyscoinInput asset balances was nil but not expected to be")
						}
						balanceAsset, ok := balance.AssetBalances[t.AssetInfo.AssetGuid]
						if !ok {
							return errors.New("DisconnectSyscoinInput asset balance not found")
						}
						err := d.DisconnectAllocationInput(&t.AddrDesc, balanceAsset, btxID, t.AssetInfo, blockTxAssetAddresses, assets, assetFoundInTx)
						if err != nil {
							return err
						}
					}
				} else {
					ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(t.AddrDesc)
					glog.Warningf("Balance for address %s (%s) not found", ad, t.AddrDesc)
				}
			}
		}
	}
	return nil
}

func (d *RocksDB) disconnectTxAssetOutputs(txa *bchain.TxAddresses,
	assets map[uint32]*bchain.Asset) error {
	var asset *bchain.Asset = nil
	isAssetTx := d.chainParser.IsAssetTx(txa.Version)
	isAssetSendTx := d.chainParser.IsAssetSendTx(txa.Version)
	if !isAssetTx && !isAssetSendTx {
		return nil
	}
	for _, t := range txa.Outputs {
		if len(t.AddrDesc) > 0 {
			if t.AddrDesc[0] == txscript.OP_RETURN {
				var err error
				asset, err = d.chainParser.GetAssetFromDesc(&t.AddrDesc)
				if err != nil {
					return err
				}
				break
			}
		}
	}
	if asset != nil {
		isActivate := d.chainParser.IsAssetActivateTx(txa.Version)
		err := d.DisconnectAssetOutput(asset, isActivate, isAssetSendTx, assets)
		if err != nil {
			return err
		}
	}
	return nil
}
func (d *RocksDB) disconnectTxAddressesOutputs(btxID []byte, txa *bchain.TxAddresses,
	getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error),
	addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool,
	blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	for i, t := range txa.Outputs {
		if len(t.AddrDesc) > 0 {
			exist := addressFoundInTx(t.AddrDesc, btxID)
			if d.chainParser.IsAddrDescIndexable(t.AddrDesc) {
				balance, err := getAddressBalance(t.AddrDesc)
				if err != nil {
					return err
				}
				if balance != nil {
					// subtract number of txs only once
					if !exist {
						balance.Txs--
					}
					balance.BalanceSat.Sub(&balance.BalanceSat, &t.ValueSat)
					if balance.BalanceSat.Sign() < 0 {
						d.resetValueSatToZero(&balance.BalanceSat, t.AddrDesc, "balance")
					}
					balance.MarkUtxoAsSpent(btxID, int32(i))
					if t.AssetInfo != nil {
						if balance.AssetBalances == nil {
							return errors.New("DisconnectSyscoinOutput asset balances was nil but not expected to be")
						}
						balanceAsset, ok := balance.AssetBalances[t.AssetInfo.AssetGuid]
						if !ok {
							return errors.New("DisconnectSyscoinOutput asset balance not found")
						}
						err := d.DisconnectAllocationOutput(&t.AddrDesc, balanceAsset, btxID, t.AssetInfo, blockTxAssetAddresses)
						if err != nil {
							return err
						}
					}
				} else {
					ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(t.AddrDesc)
					glog.Warningf("Balance for address %s (%s) not found", ad, t.AddrDesc)
				}
			}
		}
	}
	return nil
}
func (d *RocksDB) disconnectBlock(height uint32, blockTxs []bchain.BlockTxs) error {
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	txAddressesToUpdate := make(map[string]*bchain.TxAddresses)
	txAddresses := make([]*bchain.TxAddresses, len(blockTxs))
	txsToDelete := make(map[string]struct{})
	blockTxAssetAddresses := make(bchain.TxAssetAddressMap)
	balances := make(map[string]*bchain.AddrBalance)
	assets := make(map[uint32]*bchain.Asset)
	getAddressBalance := func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error) {
		var err error
		s := string(addrDesc)
		b, fb := balances[s]
		if !fb {
			b, err = d.GetAddrDescBalance(addrDesc, bchain.AddressBalanceDetailUTXOIndexed)
			if err != nil {
				return nil, err
			}
			balances[s] = b
		}
		return b, nil
	}

	// all addresses in the block are stored in blockAddressesTxs, together with a map of transactions where they appear
	blockAddressesTxs := make(map[string]map[string]struct{})
	// addressFoundInTx handles updates of the blockAddressesTxs map and returns true if the address+tx was already encountered
	addressFoundInTx := func(addrDesc bchain.AddressDescriptor, btxID []byte) bool {
		sAddrDesc := string(addrDesc)
		sBtxID := string(btxID)
		a, exist := blockAddressesTxs[sAddrDesc]
		if !exist {
			blockAddressesTxs[sAddrDesc] = map[string]struct{}{sBtxID: {}}
		} else {
			_, exist = a[sBtxID]
			if !exist {
				a[sBtxID] = struct{}{}
			}
		}
		return exist
	}
	// all assets in the block are stored in blockAssetsTxs, together with a map of transactions where they appear
	blockAssetsTxs := make(map[uint32]map[string]struct{})
	// assetFoundInTx handles updates of the blockAssetsTxs map and returns true if the asset+tx was already encountered
	assetFoundInTx := func(asset uint32, btxID []byte) bool {
		sBtxID := string(btxID)
		a, exist := blockAssetsTxs[asset]
		if !exist {
			blockAssetsTxs[asset] = map[string]struct{}{sBtxID: {}}
		} else {
			_, exist = a[sBtxID]
			if !exist {
				a[sBtxID] = struct{}{}
			}
		}
		return exist
	}
	glog.Info("Disconnecting block ", height, " containing ", len(blockTxs), " transactions")
	// when connecting block, outputs are processed first
	// when disconnecting, inputs must be reversed first
	for i := range blockTxs {
		btxID := blockTxs[i].BtxID
		s := string(btxID)
		txsToDelete[s] = struct{}{}
		txa, err := d.getTxAddresses(btxID)
		if err != nil {
			return err
		}
		if txa == nil {
			ut, _ := d.chainParser.UnpackTxid(btxID)
			glog.Warning("TxAddress for txid ", ut, " not found")
			continue
		}
		txAddresses[i] = txa
		if err := d.disconnectTxAddressesInputs(btxID, blockTxs[i].Inputs, txa, txAddressesToUpdate, getAddressBalance, addressFoundInTx, assetFoundInTx, assets, blockTxAssetAddresses); err != nil {
			return err
		}
	}
	for i := range blockTxs {
		btxID := blockTxs[i].BtxID
		txa := txAddresses[i]
		if txa == nil {
			continue
		}
		if err := d.disconnectTxAddressesOutputs(btxID, txa, getAddressBalance, addressFoundInTx, blockTxAssetAddresses); err != nil {
			return err
		}
		if err := d.disconnectTxAssetOutputs(txa, assets); err != nil {
			return err
		}
	}
	for a := range blockAddressesTxs {
		key := d.chainParser.PackAddressKey([]byte(a), height)
		wb.DeleteCF(d.cfh[cfAddresses], key)
		key = d.chainParser.PackAddressKey([]byte(a), height)
	}
	for a := range blockAssetsTxs {
		key := d.chainParser.PackAssetKey(a, height)
		wb.DeleteCF(d.cfh[cfTxAssets], key)
	}
	key := d.chainParser.PackUint(height)
	wb.DeleteCF(d.cfh[cfBlockTxs], key)
	wb.DeleteCF(d.cfh[cfHeight], key)
	d.storeTxAddresses(wb, txAddressesToUpdate)
	d.storeBalancesDisconnect(wb, balances)
	d.storeAssets(wb, assets)
	for s := range txsToDelete {
		b := []byte(s)
		wb.DeleteCF(d.cfh[cfTransactions], b)
		wb.DeleteCF(d.cfh[cfTxAddresses], b)
	}
	return d.db.Write(d.wo, wb)
}

// DisconnectBlockRangeBitcoinType removes all data belonging to blocks in range lower-higher
// it is able to disconnect only blocks for which there are data in the blockTxs column
func (d *RocksDB) DisconnectBlockRangeBitcoinType(lower uint32, higher uint32) error {
	blocks := make([][]bchain.BlockTxs, higher-lower+1)
	for height := lower; height <= higher; height++ {
		blockTxs, err := d.getBlockTxs(height)
		if err != nil {
			return err
		}
		if len(blockTxs) == 0 {
			return errors.Errorf("Cannot disconnect blocks with height %v and lower. It is necessary to rebuild index.", height)
		}
		blocks[height-lower] = blockTxs
	}
	for height := higher; height >= lower; height-- {
		err := d.disconnectBlock(height, blocks[height-lower])
		if err != nil {
			return err
		}
	}
	d.is.RemoveLastBlockTimes(int(higher-lower) + 1)
	glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	return nil
}

func (d *RocksDB) storeBalancesDisconnect(wb *gorocksdb.WriteBatch, balances map[string]*bchain.AddrBalance) {
	for _, b := range balances {
		if b != nil {
			// remove spent utxos
			us := make([]bchain.Utxo, 0, len(b.Utxos))
			for _, u := range b.Utxos {
				// remove utxos marked as spent
				if u.Vout >= 0 {
					us = append(us, u)
				}
			}
			b.Utxos = us
			// sort utxos by height
			sort.SliceStable(b.Utxos, func(i, j int) bool {
				return b.Utxos[i].Height < b.Utxos[j].Height
			})
		}
	}
	d.storeBalances(wb, balances)

}
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil {
			if !info.IsDir() {
				size += info.Size()
			}
		}
		return err
	})
	return size, err
}

// DatabaseSizeOnDisk returns size of the database in bytes
func (d *RocksDB) DatabaseSizeOnDisk() int64 {
	size, err := dirSize(d.path)
	if err != nil {
		glog.Warning("rocksdb: DatabaseSizeOnDisk: ", err)
		return 0
	}
	return size
}

// GetTx returns transaction stored in db and height of the block containing it
func (d *RocksDB) GetTx(txid string) (*bchain.Tx, uint32, error) {
	key, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, 0, err
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfTransactions], key)
	if err != nil {
		return nil, 0, err
	}
	defer val.Free()
	data := val.Data()
	if len(data) > 4 {
		return d.chainParser.UnpackTx(data)
	}
	return nil, 0, nil
}

// PutTx stores transactions in db
func (d *RocksDB) PutTx(tx *bchain.Tx, height uint32, blockTime int64) error {
	key, err := d.chainParser.PackTxid(tx.Txid)
	if err != nil {
		return nil
	}
	buf, err := d.chainParser.PackTx(tx, height, blockTime)
	if err != nil {
		return err
	}
	err = d.db.PutCF(d.wo, d.cfh[cfTransactions], key, buf)
	if err == nil {
		d.is.AddDBColumnStats(cfTransactions, 1, int64(len(key)), int64(len(buf)))
	}
	return err
}

// DeleteTx removes transactions from db
func (d *RocksDB) DeleteTx(txid string) error {
	key, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil
	}
	// use write batch so that this delete matches other deletes
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	d.internalDeleteTx(wb, key)
	return d.db.Write(d.wo, wb)
}

// internalDeleteTx checks if tx is cached and updates internal state accordingly
func (d *RocksDB) internalDeleteTx(wb *gorocksdb.WriteBatch, key []byte) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfTransactions], key)
	// ignore error, it is only for statistics
	if err == nil {
		l := len(val.Data())
		if l > 0 {
			d.is.AddDBColumnStats(cfTransactions, -1, int64(-len(key)), int64(-l))
		}
		defer val.Free()
	}
	wb.DeleteCF(d.cfh[cfTransactions], key)
}

// internal state
const internalStateKey = "internalState"

func (d *RocksDB) loadBlockTimes() ([]uint32, error) {
	var times []uint32
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfHeight])
	defer it.Close()
	counter := uint32(0)
	time := uint32(0)
	for it.SeekToFirst(); it.Valid(); it.Next() {
		height := d.chainParser.UnpackUint(it.Key().Data())
		if height > counter {
			glog.Warning("gap in cfHeight: expecting ", counter, ", got ", height)
			for ; counter < height; counter++ {
				times = append(times, time)
			}
		}
		counter++
		info, err := d.chainParser.UnpackBlockInfo(it.Value().Data())
		if err != nil {
			return nil, err
		}
		if info != nil {
			time = uint32(info.Time)
		}
		times = append(times, time)
	}
	glog.Info("loaded ", len(times), " block times")
	return times, nil
}

// LoadInternalState loads from db internal state or initializes a new one if not yet stored
func (d *RocksDB) LoadInternalState(rpcCoin string) (*common.InternalState, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(internalStateKey))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	var is *common.InternalState
	if len(data) == 0 {
		is = &common.InternalState{Coin: rpcCoin, UtxoChecked: true}
	} else {
		is, err = common.UnpackInternalState(data)
		if err != nil {
			return nil, err
		}
		// verify that the rpc coin matches DB coin
		// running it mismatched would corrupt the database
		if is.Coin == "" {
			is.Coin = rpcCoin
		} else if is.Coin != rpcCoin {
			return nil, errors.Errorf("Coins do not match. DB coin %v, RPC coin %v", is.Coin, rpcCoin)
		}
	}
	// make sure that column stats match the columns
	sc := is.DbColumns
	nc := make([]common.InternalStateColumn, len(cfNames))
	for i := 0; i < len(nc); i++ {
		nc[i].Name = cfNames[i]
		nc[i].Version = dbVersion
		for j := 0; j < len(sc); j++ {
			if sc[j].Name == nc[i].Name {
				// check the version of the column, if it does not match, the db is not compatible
				if sc[j].Version != dbVersion {
					return nil, errors.Errorf("DB version %v of column '%v' does not match the required version %v. DB is not compatible.", sc[j].Version, sc[j].Name, dbVersion)
				}
				nc[i].Rows = sc[j].Rows
				nc[i].KeyBytes = sc[j].KeyBytes
				nc[i].ValueBytes = sc[j].ValueBytes
				nc[i].Updated = sc[j].Updated
				break
			}
		}
	}
	is.DbColumns = nc
	is.BlockTimes, err = d.loadBlockTimes()
	if err != nil {
		return nil, err
	}
	// after load, reset the synchronization data
	is.IsSynchronized = false
	is.IsMempoolSynchronized = false
	var t time.Time
	is.LastMempoolSync = t
	is.SyncMode = false
	return is, nil
}

// SetInconsistentState sets the internal state to DbStateInconsistent or DbStateOpen based on inconsistent parameter
// db in left in DbStateInconsistent state cannot be used and must be recreated
func (d *RocksDB) SetInconsistentState(inconsistent bool) error {
	if d.is == nil {
		return errors.New("Internal state not created")
	}
	if inconsistent {
		d.is.DbState = common.DbStateInconsistent
	} else {
		d.is.DbState = common.DbStateOpen
	}
	return d.storeState(d.is)
}

// SetInternalState sets the InternalState to be used by db to collect internal state
func (d *RocksDB) SetInternalState(is *common.InternalState) {
	d.is = is
}

// StoreInternalState stores the internal state to db
func (d *RocksDB) StoreInternalState(is *common.InternalState) error {
	if d.metrics != nil {
		for c := 0; c < len(cfNames); c++ {
			rows, keyBytes, valueBytes := d.is.GetDBColumnStatValues(c)
			d.metrics.DbColumnRows.With(common.Labels{"column": cfNames[c]}).Set(float64(rows))
			d.metrics.DbColumnSize.With(common.Labels{"column": cfNames[c]}).Set(float64(keyBytes + valueBytes))
		}
	}
	return d.storeState(is)
}

func (d *RocksDB) storeState(is *common.InternalState) error {
	buf, err := is.Pack()
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(internalStateKey), buf)
}

func (d *RocksDB) computeColumnSize(col int, stopCompute chan os.Signal) (int64, int64, int64, error) {
	var rows, keysSum, valuesSum int64
	var seekKey []byte
	// do not use cache
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	for {
		var key []byte
		it := d.db.NewIteratorCF(ro, d.cfh[col])
		if rows == 0 {
			it.SeekToFirst()
		} else {
			glog.Info("db: Column ", cfNames[col], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum, ", in progress...")
			it.Seek(seekKey)
			it.Next()
		}
		for count := 0; it.Valid() && count < refreshIterator; it.Next() {
			select {
			case <-stopCompute:
				return 0, 0, 0, errors.New("Interrupted")
			default:
			}
			key = it.Key().Data()
			count++
			rows++
			keysSum += int64(len(key))
			valuesSum += int64(len(it.Value().Data()))
		}
		seekKey = append([]byte{}, key...)
		valid := it.Valid()
		it.Close()
		if !valid {
			break
		}
	}
	return rows, keysSum, valuesSum, nil
}

// ComputeInternalStateColumnStats computes stats of all db columns and sets them to internal state
// can be very slow operation
func (d *RocksDB) ComputeInternalStateColumnStats(stopCompute chan os.Signal) error {
	start := time.Now()
	glog.Info("db: ComputeInternalStateColumnStats start")
	for c := 0; c < len(cfNames); c++ {
		rows, keysSum, valuesSum, err := d.computeColumnSize(c, stopCompute)
		if err != nil {
			return err
		}
		d.is.SetDBColumnStats(c, rows, keysSum, valuesSum)
		glog.Info("db: Column ", cfNames[c], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum)
	}
	glog.Info("db: ComputeInternalStateColumnStats finished in ", time.Since(start))
	return nil
}

func reorderUtxo(utxos []bchain.Utxo, index int) {
	var from, to int
	for from = index; from >= 0; from-- {
		if !bytes.Equal(utxos[from].BtxID, utxos[index].BtxID) {
			break
		}
	}
	from++
	for to = index + 1; to < len(utxos); to++ {
		if !bytes.Equal(utxos[to].BtxID, utxos[index].BtxID) {
			break
		}
	}
	toSort := utxos[from:to]
	sort.SliceStable(toSort, func(i, j int) bool {
		return toSort[i].Vout < toSort[j].Vout
	})

}

func (d *RocksDB) fixUtxo(addrDesc bchain.AddressDescriptor, ba *bchain.AddrBalance) (bool, bool, error) {
	reorder := false
	var checksum big.Int
	var prevUtxo *bchain.Utxo
	for i := range ba.Utxos {
		utxo := &ba.Utxos[i]
		checksum.Add(&checksum, &utxo.ValueSat)
		if prevUtxo != nil {
			if prevUtxo.Vout > utxo.Vout && *(*int)(unsafe.Pointer(&utxo.BtxID[0])) == *(*int)(unsafe.Pointer(&prevUtxo.BtxID[0])) && bytes.Equal(utxo.BtxID, prevUtxo.BtxID) {
				reorderUtxo(ba.Utxos, i)
				reorder = true
			}
		}
		prevUtxo = utxo
	}
	if reorder {
		// get the checksum again after reorder
		checksum.SetInt64(0)
		for i := range ba.Utxos {
			utxo := &ba.Utxos[i]
			checksum.Add(&checksum, &utxo.ValueSat)
		}
	}
	if checksum.Cmp(&ba.BalanceSat) != 0 {
		var checksumFromTxs big.Int
		var utxos []bchain.Utxo
		err := d.GetAddrDescTransactions(addrDesc, 0, ^uint32(0), bchain.AllMask, func(txid string, height uint32, indexes []int32) error {
			var ta *bchain.TxAddresses
			var err error
			// sort the indexes so that the utxos are appended in the reverse order
			sort.Slice(indexes, func(i, j int) bool {
				return indexes[i] > indexes[j]
			})
			for _, index := range indexes {
				// take only outputs
				if index < 0 {
					break
				}
				if ta == nil {
					ta, err = d.GetTxAddresses(txid)
					if err != nil {
						return err
					}
				}
				if ta == nil {
					return errors.New("DB inconsistency:  tx " + txid + ": not found in txAddresses")
				}
				if len(ta.Outputs) <= int(index) {
					glog.Warning("DB inconsistency:  txAddresses " + txid + " does not have enough outputs")
				} else {
					tao := &ta.Outputs[index]
					if !tao.Spent {
						bTxid, _ := d.chainParser.PackTxid(txid)
						checksumFromTxs.Add(&checksumFromTxs, &tao.ValueSat)
						utxos = append(utxos, bchain.Utxo{AssetInfo: tao.AssetInfo, BtxID: bTxid, Height: height, Vout: index, ValueSat: tao.ValueSat})
						if checksumFromTxs.Cmp(&ba.BalanceSat) == 0 {
							return &StopIteration{}
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return false, false, err
		}
		fixed := false
		if checksumFromTxs.Cmp(&ba.BalanceSat) == 0 {
			// reverse the utxos as they are added in descending order by height
			for i := len(utxos)/2 - 1; i >= 0; i-- {
				opp := len(utxos) - 1 - i
				utxos[i], utxos[opp] = utxos[opp], utxos[i]
			}
			ba.Utxos = utxos
			wb := gorocksdb.NewWriteBatch()
			err = d.storeBalances(wb, map[string]*bchain.AddrBalance{string(addrDesc): ba})
			if err == nil {
				err = d.db.Write(d.wo, wb)
			}
			wb.Destroy()
			if err != nil {
				return false, false, errors.Errorf("balance %s, checksum %s, from txa %s, txs %d, error storing fixed utxos %v", ba.BalanceSat.String(), checksum.String(), checksumFromTxs.String(), ba.Txs, err)
			}
			fixed = true
		}
		return fixed, false, errors.Errorf("balance %s, checksum %s, from txa %s, txs %d", ba.BalanceSat.String(), checksum.String(), checksumFromTxs.String(), ba.Txs)
	} else if reorder {
		wb := gorocksdb.NewWriteBatch()
		err := d.storeBalances(wb, map[string]*bchain.AddrBalance{string(addrDesc): ba})
		if err == nil {
			err = d.db.Write(d.wo, wb)
		}
		wb.Destroy()
		if err != nil {
			return false, false, errors.Errorf("error storing reordered utxos %v", err)
		}
	}
	return false, reorder, nil
}

// FixUtxos checks and fixes possible
func (d *RocksDB) FixUtxos(stop chan os.Signal) error {
	if d.chainParser.GetChainType() != bchain.ChainBitcoinType {
		glog.Info("FixUtxos: applicable only for bitcoin type coins")
		return nil
	}
	glog.Info("FixUtxos: starting")
	var row, errorsCount, fixedCount int64
	var seekKey []byte
	// do not use cache
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	for {
		var addrDesc bchain.AddressDescriptor
		it := d.db.NewIteratorCF(ro, d.cfh[cfAddressBalance])
		if row == 0 {
			it.SeekToFirst()
		} else {
			glog.Info("FixUtxos: row ", row, ", errors ", errorsCount)
			it.Seek(seekKey)
			it.Next()
		}
		for count := 0; it.Valid() && count < refreshIterator; it.Next() {
			select {
			case <-stop:
				return errors.New("Interrupted")
			default:
			}
			addrDesc = it.Key().Data()
			buf := it.Value().Data()
			count++
			row++
			if len(buf) < 3 {
				glog.Error("FixUtxos: row ", row, ", addrDesc ", addrDesc, ", empty data")
				errorsCount++
				continue
			}
			ba, err := d.chainParser.UnpackAddrBalance(buf, d.chainParser.PackedTxidLen(), bchain.AddressBalanceDetailUTXO)
			if err != nil {
				glog.Error("FixUtxos: row ", row, ", addrDesc ", addrDesc, ", unpackAddrBalance error ", err)
				errorsCount++
				continue
			}
			fixed, reordered, err := d.fixUtxo(addrDesc, ba)
			if err != nil {
				errorsCount++
				glog.Error("FixUtxos: row ", row, ", addrDesc ", addrDesc, ", error ", err, ", fixed ", fixed)
				if fixed {
					fixedCount++
				}
			} else if reordered {
				glog.Error("FixUtxos: row ", row, ", addrDesc ", addrDesc, " reordered")
				fixedCount++
			}
		}
		seekKey = append([]byte{}, addrDesc...)
		valid := it.Valid()
		it.Close()
		if !valid {
			break
		}
	}
	glog.Info("FixUtxos: finished, scanned ", row, " rows, found ", errorsCount, " errors, fixed ", fixedCount)
	return nil
}
