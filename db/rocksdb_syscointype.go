package db

import (
	"blockbook/bchain"
	"bytes"
	"strconv"
	"strings"
	"sort"
	"math/big"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/tecbot/gorocksdb"
	"encoding/json"
	"encoding/hex"
	"github.com/syscoin/btcd/wire"
	"github.com/martinboehm/btcutil/txscript"
	"time"
)
var AssetCache map[uint32]bchain.Asset
var SetupAssetCacheFirstTime bool = true
// GetTxAssetsCallback is called by GetTransactions/GetTxAssets for each found tx
type GetTxAssetsCallback func(txids []string) error

func (d *RocksDB) GetAuxFeeAddr(pubData []byte) bchain.AddressDescriptor {
	f := bchain.AuxFees{}
	var err error
	var addrDesc bchain.AddressDescriptor
	// cannot unmarshal, likely no auxfees defined
	err = json.Unmarshal(pubData, &f)
	if err != nil {
		return nil
	}
	// no auxfees defined
	if len(f.Aux_fees.Address) == 0 {
		return nil
	}
	addrDesc, err = d.chainParser.GetAddrDescFromAddress(f.Aux_fees.Address)
	if err != nil {
		return nil
	}
	return addrDesc

}

func (d *RocksDB) ConnectAssetOutput(version int32, asset *bchain.Asset, dbAsset *bchain.Asset) error {
	// deduct the output value from the asset balance
	if d.chainParser.IsAssetSendTx(version) {
		balanceAssetSat = big.NewInt(dBAsset.AssetObj.Balance)
		balanceAssetSat.Sub(balanceAssetSat, assetInfo.ValueSat)
		dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
		if dBAsset.AssetObj.Balance < 0 {
			glog.Warningf("ConnectAssetOutput balance is negative %v, setting to 0...", dBAsset.AssetObj.Balance)
			dBAsset.AssetObj.Balance = 0
		}
	} else if !d.chainParser.IsAssetActivateTx(version) {
		if asset.AssetObj.Balance > 0 {
			valueTo := big.NewInt(asset.AssetObj.Balance)
			balanceDb := big.NewInt(dBAsset.AssetObj.Balance)
			balanceDb.Add(balanceDb, valueTo)
			supplyDb := big.NewInt(dBAsset.AssetObj.TotalSupply)
			supplyDb.Add(supplyDb, valueTo)
			dBAsset.AssetObj.Balance = balanceDb.Int64()
			dBAsset.AssetObj.TotalSupply = supplyDb.Int64()
		}
		// logic follows core CheckAssetInputs()
		if len(asset.AssetObj.PubData) > 0 {
			dBAsset.AssetObj.PubData = asset.AssetObj.PubData
		}
		if len(asset.AssetObj.Contract) > 0 {
			dBAsset.AssetObj.Contract = asset.AssetObj.Contract
		}
		if asset.AssetObj.UpdateFlags != dBAsset.AssetObj.UpdateFlags {
			dBAsset.AssetObj.UpdateFlags = asset.AssetObj.UpdateFlags
		}
	} else {
		asset.AssetObj.TotalSupply = asset.AssetObj.Balance
	}	
	return nil
}

func (d *RocksDB) DisconnectAssetOutput(version int32, asset *bchain.Asset, dbAsset *bchain.Asset) error {
	// add the output value to the asset balance
	if d.chainParser.IsAssetSendTx(version) {
		balanceAssetSat = big.NewInt(dBAsset.AssetObj.Balance)
		balanceAssetSat.Add(balanceAssetSat, assetInfo.ValueSat)
		dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
	} else if !d.chainParser.IsAssetActivateTx(version) {
		if asset.AssetObj.Balance > 0 {
			valueTo := big.NewInt(asset.AssetObj.Balance)
			balanceDb := big.NewInt(dBAsset.AssetObj.Balance)
			balanceDb.Sub(balanceDb, valueTo)
			supplyDb := big.NewInt(dBAsset.AssetObj.TotalSupply)
			supplyDb.Sub(supplyDb, valueTo)
			dBAsset.AssetObj.Balance = balanceDb.Int64()
			if dBAsset.AssetObj.Balance < 0 {
				glog.Warningf("DisconnectAssetOutput balance is negative %v, setting to 0...", dBAsset.AssetObj.Balance)
				dBAsset.AssetObj.Balance = 0
			}
			dBAsset.AssetObj.TotalSupply = supplyDb.Int64()
			if dBAsset.AssetObj.TotalSupply < 0 {
				glog.Warningf("DisconnectAssetOutput total supply is negative %v, setting to 0...", dBAsset.AssetObj.TotalSupply)
				dBAsset.AssetObj.TotalSupply = 0
			}
		}
		// logic follows core CheckAssetInputs()
		// prev data is enforced to be correct (previous value) if value exists in the tx data
		if len(asset.AssetObj.PubData) > 0 {
			dBAsset.AssetObj.PubData = asset.AssetObj.PrevPubData
		}
		if len(asset.AssetObj.Contract) > 0 {
			dBAsset.AssetObj.Contract = asset.AssetObj.PrevContract
		}
		if asset.AssetObj.UpdateFlags != dBAsset.AssetObj.UpdateFlags {
			dBAsset.AssetObj.UpdateFlags = asset.AssetObj.PrevUpdateFlags
		}
	}
	return nil
}

func (d *RocksDB) ConnectSyscoinInput(height uint32, balanceAsset *bchain.AddrBalance, version int32, btxID []byte, assetInfo* bchain.AssetInfo, assets map[uint32]*bchain.Asset, txAssets bchain.TxAssetMap) error {
	dBAsset, err := d.GetAsset(assetGuid, &assets)
	if !d.chainParser.IsAssetActivateTx(version) && err != nil {
		return err
	}
	if dBAsset != nil {
		assetInfo.Details = &bchain.AssetInfoDetails{Symbol: dBAsset.Symbol, Decimals: dBAsset.Precision}
		counted := addToAssetsMap(txAssets, assetGuid, btxID, version, height)
		if !counted {
			balanceAsset.Transfers++
		}
		assets[assetGuid] = dBAsset
	}
	balanceAsset.BalanceSat.Sub(&balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.BalanceSat.Sign() < 0 {
		balanceAsset.BalanceSat.SetInt64(0)
	}
	balanceAsset.SentSat.Add(&balanceAsset.SentSat, assetInfo.ValueSat)
	return nil
}

func (d *RocksDB) ConnectSyscoinOutput(addrDesc bchain.AddressDescriptor, height uint32, balanceAsset *bchain.AddrBalance, version int32, btxID []byte, utxo* bchain.Utxo, assetInfo* bchain.AssetInfo, assets map[uint32]*bchain.Asset, txAssets bchain.TxAssetMap) error {
	isActivate := d.chainParser.IsAssetActivateTx(version)
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, &assets)
	if !isActivate && err != nil {
		return err
	}
	if dBAsset != nil || isActivate {
		if d.chainParser.IsAssetTx(version) {
			asset, err := d.chainParser.GetAssetFromTx(tx)
			if err != nil {
				return err
			}
			if isActivate {
				dBAsset = &asset
			}
			err = d.ConnectAssetOutput(version, &asset, dBAsset)
			if err != nil {
				return err
			}
			// in an asset tx, the output of 0 value is the destination for the new ownership of the asset
			if assetInfo.ValueSat.AsInt64() == 0 {
				dBAsset.AddrDesc = addrDesc
			}
		} 
		utxo.AssetInfo.Details = &bchain.AssetInfoDetails{Symbol: dBAsset.Symbol, Decimals: dBAsset.Precision}
		assetInfo.Details = utxo.AssetInfo.Details
		counted := addToAssetsMap(txAssets, assetGuid, btxID, version, height)
		if !counted {
			// only count asset tx on output because inputs must have the same assets as outputs
			dBAsset.Transactions++
			balanceAsset.Transfers++
		}
		assets[assetGuid] = dBAsset
	} else {
		return errors.New("ConnectSyscoinOutput: asset not found")
	}
	balanceAsset.BalanceSat.Add(&balanceAsset.BalanceSat, assetInfo.ValueSat)
	return nil
}

func (d *RocksDB) DisconnectSyscoinOutput(assetBalances map[uint32]*AssetBalance, version int32, btxID []byte, assets map[uint32]*bchain.Asset,  assetInfo *bchain.AssetInfo, assetFoundInTx func(asset uint32, btxID []byte) bool) error {
	balanceAsset, ok := balance.AssetBalances[assetInfo.AssetGuid]
	if !ok {
		return errors.New("DisconnectSyscoinOutput asset balance not found")
	}
	isActivate := d.chainParser.IsAssetActivateTx(version)
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, &assets)
	if dBAsset == nil || err != nil {
		return err
	}
	if isActivate {
		// vout AssetGuid should be set to 0 so it won't serialize asset info or use asset info anywhere in API
		assetInfo.AssetGuid = 0
		delete(assetBalances, assetInfo.AssetGuid)
		// signals for removal from asset db
		dBAsset.AssetObj.TotalSupply = -1
		assetFoundInTx(assetGuid, btxID)
		assets[assetGuid] = dBAsset
		return nil
	} else if d.chainParser.IsAssetTx(version) {
		asset, err := d.chainParser.GetAssetFromTx(tx)
		if err != nil {
			return err
		}
		err = d.DisconnectAssetOutput(version, asset, dBAsset)
		if err != nil {
			return err
		}
	} 
	// on activate we won't get here but its ok because DisconnectSyscoinInput will catch assetFoundInTx
	exists := assetFoundInTx(assetGuid, btxID)
	if !exists {
		dBAsset.Transactions--
		balanceAsset.Transfers--
	}
	

	balanceAsset.BalanceSat.Sub(&balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.BalanceSat.Sign() < 0 {
		balanceAsset.BalanceSat.SetInt64(0)
	}
	
	isAssetSend := d.chainParser.IsAssetSendTx(version)
	if isAssetSend {
		balanceAssetSat = big.NewInt(dBAsset.AssetObj.Balance)
		balanceAssetSat.Add(balanceAssetSat, assetInfo.ValueSat)
		dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
	} 
	assets[assetGuid] = dBAsset
	return nil
}

func (d *RocksDB) DisconnectSyscoinInput(addrDesc bchain.AddressDescriptor, version int32, balanceAsset *bchain.AddrBalance,  btxID []byte, assetInfo *bchain.AssetInfo, utxo *bchain.utxo, assets map[uint32]*bchain.Asset, assetFoundInTx func(asset uint32, btxID []byte) bool) error {
	isActivate := d.chainParser.IsAssetActivateTx(version)
	dBAsset, err := d.GetAsset(assetGuid, &assets)
	if dBAsset == nil || err != nil {
		return err
	}
	if isActivate {
		assetInfo.AssetGuid = 0
		utxo.AssetInfo.AssetGuid = 0
		return nil
	} else if d.chainParser.IsAssetTx(version) {
		// set the asset to be owned by the asset of 0 value
		if assetInfo.ValueSat.AsInt64() == 0 {
			dBAsset.AddrDesc = addrDesc
		}
	} 
	exists := assetFoundInTx(assetGuid, btxID)
	if !exists {
		balanceAsset.Transfers--
	}
	
	balanceAsset.SentSat.Sub(&balanceAsset.SentSat, assetInfo.ValueSat)
	balanceAsset.BalanceSat.Add(&balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.SentSat.Sign() < 0 {
		balanceAsset.SentSat.SetInt64(0)
	}
	utxo.AssetInfo = assetInfo
	assets[assetGuid] = dBAsset
	return nil
}
func (d *RocksDB) SetupAssetCache() error {
	start := time.Now()
	if AssetCache == nil {
		AssetCache = map[uint32]bchain.Asset{}
	}
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfAssets])
	defer it.Close()
	for it.SeekToFirst(); it.Valid(); it.Next() {
		assetKey := d.chainParser.UnpackUint(it.Key().Data())
		assetDb, err := d.chainParser.UnpackAsset(it.Value().Data())
		if err != nil {
			glog.Info("SetupAssetCache: UnpackAsset failure ", assetKey, " err ", err)
			return err
		}
		AssetCache[assetKey] = *assetDb
	}
	glog.Info("SetupAssetCache finished in ", time.Since(start))
	return nil
}

// find assets from cache that contain filter
func (d *RocksDB) FindAssetsFromFilter(filter string) bchain.Assets {
	start := time.Now()
	if SetupAssetCacheFirstTime == true {
		if err := d.SetupAssetCache(); err != nil {
			glog.Error("storeAssets SetupAssetCache ", err)
			return nil
		}
		SetupAssetCacheFirstTime = false;
	}
	assets := make(bchain.Assets, 0)
	filterLower := strings.ToLower(filter)
	filterLower = strings.Replace(filterLower, "0x", "", -1)
	for _, assetCached := range AssetCache {
		symbolLower := strings.ToLower(assetCached.AssetObj.Symbol)
		if strings.Contains(symbolLower, filterLower) {
			assets = append(assets, assetCached)
		} else if len(assetCached.AssetObj.Contract) > 0 && len(filterLower) > 5 {
			contractStr := hex.EncodeToString(assetCached.AssetObj.Contract)
			contractLower := strings.ToLower(contractStr)
			if strings.Contains(contractLower, filterLower) {
				assets = append(assets, assetCached)
			}
		}
	}
	sort.Sort(assets)
	glog.Info("FindAssetsFromFilter finished in ", time.Since(start))
	return assets
}

func (d *RocksDB) storeAssets(wb *gorocksdb.WriteBatch, assets map[uint32]*bchain.Asset) error {
	if assets == nil {
		return nil
	}
	if AssetCache == nil {
		AssetCache = map[uint32]bchain.Asset{}
	}
	for guid, asset := range assets {
		AssetCache[guid] = *asset
		key := d.chainParser.PackUint(guid)
		// total supply of -1 signals asset to be removed from db - happens on disconnect of new asset
		if asset.AssetObj.TotalSupply == -1 {
			delete(AssetCache, guid)
			wb.DeleteCF(d.cfh[cfAssets], key)
		} else {
			buf, err := d.chainParser.PackAsset(asset)
			if err != nil {
				return err
			}
			wb.PutCF(d.cfh[cfAssets], key, buf)
		}
	}
	return nil
}

func (d *RocksDB) GetAsset(guid uint32, assets *map[uint32]*bchain.Asset) (*bchain.Asset, error) {
	var assetDb *bchain.Asset
	var assetL1 *bchain.Asset
	var ok bool
	if assets != nil {
		if assetL1, ok = (*assets)[guid]; ok {
			return assetL1, nil
		}
	}
	if AssetCache == nil {
		AssetCache = map[uint32]bchain.Asset{}
		// so it will store later in cache
		ok = false
	} else {
		var assetDbCache, ok = AssetCache[guid]
		if ok {
			return &assetDbCache, nil
		}
	}
	key := d.chainParser.PackUint(guid)
	val, err := d.db.GetCF(d.ro, d.cfh[cfAssets], key)
	if err != nil {
		return nil, err
	}
	// nil data means the key was not found in DB
	if val.Data() == nil {
		return nil, nil
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	assetDb, err = d.chainParser.UnpackAsset(buf)
	if err != nil {
		return nil, err
	}
	// cache miss, add it, we also add it on storeAsset but on API queries we should not have to wait until a block
	// with this asset to store it in cache
	if !ok {
		AssetCache[guid] = *assetDb
	}
	return assetDb, nil
}

func (d *RocksDB) storeTxAssets(wb *gorocksdb.WriteBatch, txassets bchain.TxAssetMap) error {
	for key, txAsset := range txassets {
		buf := d.chainParser.PackAssetTxIndex(txAsset)
		wb.PutCF(d.cfh[cfTxAssets], key, buf)
	}
	return nil
}

// GetTxAssets finds all asset transactions for each asset
// Transaction are passed to callback function in the order from newest block to the oldest
func (d *RocksDB) GetTxAssets(assetGuid uint32, lower uint32, higher uint32, assetsBitMask bchain.AssetsMask, fn GetTxAssetsCallback) (err error) {
	startKey := d.chainParser.PackAssetKey(assetGuid, higher)
	stopKey := d.chainParser.PackAssetKey(assetGuid, lower)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfTxAssets])
	defer it.Close()
	for it.Seek(startKey); it.Valid(); it.Next() {
		key := it.Key().Data()
		val := it.Value().Data()
		if bytes.Compare(key, stopKey) > 0 {
			break
		}
		txIndexes := d.chainParser.UnpackAssetTxIndex(val)
		if txIndexes != nil {
			txids := []string{}
			for _, txIndex := range txIndexes {
				mask := uint32(txIndex.Type)
				if (assetsBitMask == bchain.AllMask || (uint32(assetsBitMask) & mask) == mask) {
					txids = append(txids, hex.EncodeToString(txIndex.Txid))
				}
			}
			if len(txids) > 0 {
				if err := fn(txids); err != nil {
					if _, ok := err.(*StopIteration); ok {
						return nil
					}
					return err
				}
			}
		}
	}
	return nil
}

// addToAssetsMap maintains mapping between assets and transactions in one block
// the return value is true if the tx was processed before, to not to count the tx multiple times
func addToAssetsMap(txassets bchain.TxAssetMap, assetGuid uint32, btxID []byte, version int32, height int32) bool {
	// check that the asset was already processed in this block
	// if not found, it has certainly not been counted
	key := d.chainParser.PackAssetKey(assetGuid, height)
	at, found := txassets[key]
	if found {
		// if the tx is already in the slice
		for i, t := range at.Txs {
			if bytes.Equal(btxID, t.BtxID) {
				return true
			}
		}
	} else {
		at = &bchain.TxAsset{Txs: []*bchain.TxAssetIndex{}}
		txassets[blockHash] = at
	}
	at.Txs = append(txAsset.Txs, &bchain.TxAssetIndex{Type: d.chainParser.GetAssetsMaskFromVersion(version), BtxID: btxID})
	at.Height = height
	return false
}