package db

import (
	"blockbook/bchain"
	"bytes"
	"strings"
	"math/big"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/tecbot/gorocksdb"
	"encoding/hex"
	"time"
)
var AssetCache map[uint32]bchain.Asset
var SetupAssetCacheFirstTime bool = true
// GetTxAssetsCallback is called by GetTransactions/GetTxAssets for each found tx
type GetTxAssetsCallback func(txids []string) error

func (d *RocksDB) ConnectAssetOutputHelper(isActivate bool, asset *bchain.AssetType, dBAsset *bchain.Asset) error {
	if !isActivate {
		if asset.Balance > 0 {
			valueTo := big.NewInt(asset.Balance)
			balanceDb := big.NewInt(dBAsset.AssetObj.Balance)
			balanceDb.Add(balanceDb, valueTo)
			supplyDb := big.NewInt(dBAsset.AssetObj.TotalSupply)
			supplyDb.Add(supplyDb, valueTo)
			dBAsset.AssetObj.Balance = balanceDb.Int64()
			dBAsset.AssetObj.TotalSupply = supplyDb.Int64()
		}
		// logic follows core CheckAssetInputs()
		if len(asset.PubData) > 0 {
			dBAsset.AssetObj.PubData = asset.PubData
		}
		if len(asset.Contract) > 0 {
			dBAsset.AssetObj.Contract = asset.Contract
		}
		if asset.UpdateFlags != dBAsset.AssetObj.UpdateFlags {
			dBAsset.AssetObj.UpdateFlags = asset.UpdateFlags
		}
	} else {
		dBAsset.AssetObj.TotalSupply = asset.Balance
	}	
	return nil
}

func (d *RocksDB) DisconnectAssetOutputHelper(asset *bchain.AssetType, dBAsset *bchain.Asset) error {
	if asset.Balance > 0 {
		valueTo := big.NewInt(asset.Balance)
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
	if len(asset.PubData) > 0 {
		dBAsset.AssetObj.PubData = asset.PrevPubData
	}
	if len(asset.Contract) > 0 {
		dBAsset.AssetObj.Contract = asset.PrevContract
	}
	if asset.UpdateFlags != dBAsset.AssetObj.UpdateFlags {
		dBAsset.AssetObj.UpdateFlags = asset.PrevUpdateFlags
	}
	return nil
}

func (d *RocksDB) ConnectAllocationInput(addrDesc* bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance, isActivate bool, btxID []byte, assetInfo* bchain.AssetInfo, assets map[uint32]*bchain.Asset, blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, assets)
	if !isActivate && err != nil {
		return err
	}
	counted := d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers++
	}
	if dBAsset != nil {
		assets[assetInfo.AssetGuid] = dBAsset
	}
	balanceAsset.BalanceSat.Sub(balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.BalanceSat.Sign() < 0 {
		balanceAsset.BalanceSat.SetInt64(0)
	}
	balanceAsset.SentSat.Add(balanceAsset.SentSat, assetInfo.ValueSat)
	return nil
}

func (d *RocksDB) ConnectAllocationOutput(addrDesc* bchain.AddressDescriptor, height uint32, balanceAsset *bchain.AssetBalance, isActivate bool, version int32, btxID []byte, assetInfo* bchain.AssetInfo, assets map[uint32]*bchain.Asset, txAssets bchain.TxAssetMap, blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, assets)
	if !isActivate && err != nil {
		return err
	}
	counted := d.addToAssetsMap(txAssets, assetInfo.AssetGuid, btxID, version, height)
	if !counted {
		if dBAsset != nil {
			dBAsset.Transactions++
		}
	}
	// asset guid + txid + address of output/input must match for counted to be true
	counted = d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers++
	}
	if dBAsset != nil {
		if d.chainParser.IsAssetSendTx(version) {
			balanceAssetSat := big.NewInt(dBAsset.AssetObj.Balance)
			balanceAssetSat.Sub(balanceAssetSat, assetInfo.ValueSat)
			dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
			if dBAsset.AssetObj.Balance < 0 {
				glog.Warningf("ConnectAssetOutput balance is negative %v, setting to 0...", dBAsset.AssetObj.Balance)
				dBAsset.AssetObj.Balance = 0
			}
		}
		assets[assetInfo.AssetGuid] = dBAsset
	} else {
		return errors.New("ConnectSyscoinOutput: asset not found")
	}
	balanceAsset.BalanceSat.Add(balanceAsset.BalanceSat, assetInfo.ValueSat)
	return nil
}

func (d *RocksDB) ConnectAssetOutput(addrDescData *bchain.AddressDescriptor, addrDesc *bchain.AddressDescriptor, isActivate bool, isAssetTx bool, assetGuid uint32, assets map[uint32]*bchain.Asset) error {
	script, err := d.chainParser.GetScriptFromAddrDesc(*addrDescData)
	if err != nil {
		return err
	}
	sptData := d.chainParser.TryGetOPReturn(script)
	if sptData == nil {
		return nil
	}
	asset, err := d.chainParser.GetAssetFromData(sptData)
	if err != nil {
		return err
	}	
	var dBAsset* bchain.Asset = nil
	if !isActivate {
		dBAsset, err = d.GetAsset(assetGuid, &assets)
		if  err != nil {
			return err
		}
	} else if isActivate {
		dBAsset = &bchain.Asset{Transactions: 1, AssetObj: *asset}
	}
	if dBAsset != nil {
		if isAssetTx {
			err = d.ConnectAssetOutputHelper(isActivate, asset, dBAsset)
			if err != nil {
				return err
			}
			dBAsset.AddrDesc = *addrDesc
		} 
		assets[assetGuid] = dBAsset
	} else {
		return errors.New("ConnectSyscoinOutput: asset not found")
	}
	return nil
}

func (d *RocksDB) DisconnectAllocationOutput(addrDesc *bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance, isActivate bool,  version int32, btxID []byte, assets map[uint32]*bchain.Asset,  assetInfo *bchain.AssetInfo, blockTxAssetAddresses bchain.TxAssetAddressMap, assetFoundInTx func(asset uint32, btxID []byte) bool) error {
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, assets)
	if dBAsset == nil || err != nil {
		if dbAsset == nil {
			return errors.New("DisconnectAllocationOutput could not read asset")
		}
		return err
	}
	
	balanceAsset.BalanceSat.Sub(balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.BalanceSat.Sign() < 0 {
		balanceAsset.BalanceSat.SetInt64(0)
	}

	if d.chainParser.IsAssetSendTx(version) {
		balanceAssetSat := big.NewInt(dBAsset.AssetObj.Balance)
		balanceAssetSat.Add(balanceAssetSat, assetInfo.ValueSat)
		dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
	} else if isActivate {
		// signals for removal from asset db
		dBAsset.AssetObj.TotalSupply = -1
	}
	// on activate we won't get here but its ok because DisconnectSyscoinInput will catch assetFoundInTx
	exists := assetFoundInTx(assetInfo.AssetGuid, btxID)
	if !exists {
		dBAsset.Transactions--
	}
	counted := d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers--
	}
	assets[assetInfo.AssetGuid] = dBAsset
	return nil
}
func (d *RocksDB) DisconnectAssetOutput(addrDesc *bchain.AddressDescriptor, isActivate bool, assets map[uint32]*bchain.Asset, assetGuid uint32) error {
	script, err := d.chainParser.GetScriptFromAddrDesc(*addrDesc)
	if err != nil {
		return err
	}
	sptData := d.chainParser.TryGetOPReturn(script)
	if sptData == nil {
		return nil
	}
	asset, err := d.chainParser.GetAssetFromData(sptData)
	if err != nil {
		return err
	}
	dBAsset, err := d.GetAsset(assetGuid, assets)
	if dBAsset == nil || err != nil {
		if dbAsset == nil {
			return errors.New("DisconnectAssetOutput could not read asset")
		}
		return err
	}
	if !isActivate {
		err = d.DisconnectAssetOutputHelper(asset, dBAsset)
		if err != nil {
			return err
		}
	}
	assets[assetGuid] = dBAsset
	return nil
}
func (d *RocksDB) DisconnectAllocationInput(addrDesc *bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance,  btxID []byte, assetInfo *bchain.AssetInfo, assets map[uint32]*bchain.Asset, blockTxAssetAddresses bchain.TxAssetAddressMap, assetFoundInTx func(asset uint32, btxID []byte) bool) error {
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, assets)
	if dBAsset == nil || err != nil {
		if dbAsset == nil {
			return errors.New("DisconnectAllocationInput could not read asset")
		}
		return err
	}
	balanceAsset.SentSat.Sub(balanceAsset.SentSat, assetInfo.ValueSat)
	balanceAsset.BalanceSat.Add(balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.SentSat.Sign() < 0 {
		balanceAsset.SentSat.SetInt64(0)
	}
	assetFoundInTx(assetInfo.AssetGuid, btxID)
	counted := d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers--
	}	
	assets[assetInfo.AssetGuid] = dBAsset
	return nil
}
func (d *RocksDB) DisconnectAssetInput(addrDesc *bchain.AddressDescriptor, assets map[uint32]*bchain.Asset, assetGuid uint32) error {
	dBAsset, err := d.GetAsset(assetGuid, assets)
	if dBAsset == nil || err != nil {
		if dbAsset == nil {
			return errors.New("DisconnectAssetInput could not read asset")
		}
		return err
	}
	dBAsset.AddrDesc = *addrDesc
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
		assetDb := d.chainParser.UnpackAsset(it.Value().Data())
		if assetDb == nil {
			return errors.New("SetupAssetCache: UnpackAsset failure")
		}
		AssetCache[assetKey] = *assetDb
	}
	glog.Info("SetupAssetCache finished in ", time.Since(start))
	return nil
}

// find assets from cache that contain filter
func (d *RocksDB) FindAssetsFromFilter(filter string) map[uint32]bchain.Asset {
	start := time.Now()
	if SetupAssetCacheFirstTime == true {
		if err := d.SetupAssetCache(); err != nil {
			glog.Error("storeAssets SetupAssetCache ", err)
			return nil
		}
		SetupAssetCacheFirstTime = false;
	}
	assets := map[uint32]bchain.Asset{}
	filterLower := strings.ToLower(filter)
	filterLower = strings.Replace(filterLower, "0x", "", -1)
	for guid, assetCached := range AssetCache {
		symbolLower := strings.ToLower(assetCached.AssetObj.Symbol)
		if strings.Contains(symbolLower, filterLower) {
			assets[guid] = assetCached
		} else if len(assetCached.AssetObj.Contract) > 0 && len(filterLower) > 5 {
			contractStr := hex.EncodeToString(assetCached.AssetObj.Contract)
			contractLower := strings.ToLower(contractStr)
			if strings.Contains(contractLower, filterLower) {
				assets[guid] = assetCached
			}
		}
	}
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
			buf := d.chainParser.PackAsset(asset)
			wb.PutCF(d.cfh[cfAssets], key, buf)
		}
	}
	return nil
}

func (d *RocksDB) GetAsset(guid uint32, assets map[uint32]*bchain.Asset) (*bchain.Asset, error) {
	var assetDb *bchain.Asset
	var assetL1 *bchain.Asset
	var ok bool
	if assets != nil {
		if assetL1, ok = assets[guid]; ok {
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
	assetDb = d.chainParser.UnpackAsset(buf)
	if assetDb == nil {
		return nil, errors.New("GetAsset: Could not unpack asset")
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
		wb.PutCF(d.cfh[cfTxAssets], []byte(key), buf)
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
				if (uint32(assetsBitMask) & mask) == mask {
					txids = append(txids, hex.EncodeToString(txIndex.BtxID))
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
func (d *RocksDB) addToAssetsMap(txassets bchain.TxAssetMap, assetGuid uint32, btxID []byte, version int32, height uint32) bool {
	// check that the asset was already processed in this block
	// if not found, it has certainly not been counted
	key := string(d.chainParser.PackAssetKey(assetGuid, height))
	at, found := txassets[key]
	if found {
		// if the tx is already in the slice
		for _, t := range at.Txs {
			if bytes.Equal(btxID, t.BtxID) {
				return true
			}
		}
	} else {
		at = &bchain.TxAsset{Txs: []*bchain.TxAssetIndex{}}
		txassets[key] = at
	}
	at.Txs = append(at.Txs, &bchain.TxAssetIndex{Type: d.chainParser.GetAssetsMaskFromVersion(version), BtxID: btxID})
	at.Height = height
	return false
}
// to control Transfer add/remove
func (d *RocksDB) addToAssetAddressMap(txassetAddresses bchain.TxAssetAddressMap, assetGuid uint32, btxID []byte, addrDesc *bchain.AddressDescriptor) bool {
	at, found := txassetAddresses[assetGuid]
	if found {
		// if the tx is already in the slice
		for _, t := range at.Txs {
			if bytes.Equal(btxID, t.BtxID) && bytes.Equal(*addrDesc, t.AddrDesc) {
				return true
			}
		}
	} else {
		at = &bchain.TxAssetAddress{Txs: []*bchain.TxAssetAddressIndex{}}
		txassetAddresses[assetGuid] = at
	}
	at.Txs = append(at.Txs, &bchain.TxAssetAddressIndex{AddrDesc: *addrDesc, BtxID: btxID})
	return false
}