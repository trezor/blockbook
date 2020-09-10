package db

import (
	"blockbook/bchain"
	"bytes"
	"strings"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/tecbot/gorocksdb"
	"encoding/hex"
	"time"
	"fmt"
	"github.com/syscoin/btcd/wire"
)
var AssetCache map[uint32]bchain.Asset
var SetupAssetCacheFirstTime bool = true
// GetTxAssetsCallback is called by GetTransactions/GetTxAssets for each found tx
type GetTxAssetsCallback func(txids []string) error

func (d *RocksDB) ConnectAssetOutputHelper(isActivate bool, asset *bchain.Asset, dBAsset *bchain.Asset) error {
	if !isActivate {
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_SUPPLY) != 0 {
			dBAsset.AssetObj.Balance += asset.AssetObj.Balance
			dBAsset.AssetObj.TotalSupply += asset.AssetObj.Balance
		}
		// logic follows core CheckAssetInputs()
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_DATA) != 0 {
			dBAsset.AssetObj.PubData = asset.AssetObj.PubData
			if len(dBAsset.AssetObj.PubData) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_DATA
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_DATA
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_CONTRACT) != 0 {
			dBAsset.AssetObj.Contract = asset.AssetObj.Contract
			if len(dBAsset.AssetObj.Contract) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_CONTRACT
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_CONTRACT
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_NOTARY_KEY) != 0 {
			dBAsset.AssetObj.NotaryKeyID = asset.AssetObj.NotaryKeyID
			if len(dBAsset.AssetObj.NotaryKeyID) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_NOTARY_KEY
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_NOTARY_KEY
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_NOTARY_DETAILS) != 0 {
			dBAsset.AssetObj.NotaryDetails = asset.AssetObj.NotaryDetails
			if len(dBAsset.AssetObj.NotaryDetails.EndPoint) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_NOTARY_DETAILS
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_NOTARY_DETAILS
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_AUXFEE_KEY) != 0 {
			dBAsset.AssetObj.AuxFeeKeyID = asset.AssetObj.AuxFeeKeyID
			if len(dBAsset.AssetObj.AuxFeeKeyID) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_AUXFEE_KEY
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_AUXFEE_KEY
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_AUXFEE_DETAILS) != 0 {
			dBAsset.AssetObj.AuxFeeDetails = asset.AssetObj.AuxFeeDetails
			if len(dBAsset.AssetObj.AuxFeeDetails.AuxFees) > 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_AUXFEE_DETAILS
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_AUXFEE_DETAILS
			}
		}
		if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_CAPABILITYFLAGS) != 0 {
			dBAsset.AssetObj.UpdateCapabilityFlags = asset.AssetObj.UpdateCapabilityFlags
			if dBAsset.AssetObj.UpdateCapabilityFlags != 0 {
				dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_CAPABILITYFLAGS
			} else {
				dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_CAPABILITYFLAGS
			}
		}
	} else {
		dBAsset.AssetObj.TotalSupply = asset.AssetObj.Balance
		dBAsset.AssetObj.UpdateFlags = asset.AssetObj.UpdateFlags
	}
	return nil
}

func (d *RocksDB) DisconnectAssetOutputHelper(asset *bchain.Asset, dBAsset *bchain.Asset) error {
	// nothing was updated
	if asset.AssetObj.UpdateFlags == 0 {
		return nil
	}
	if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_SUPPLY) != 0 {
		dBAsset.AssetObj.Balance -= asset.AssetObj.Balance
		if dBAsset.AssetObj.Balance < 0 {
			glog.Warningf("DisconnectAssetOutput balance is negative %v, setting to 0...", dBAsset.AssetObj.Balance)
			dBAsset.AssetObj.Balance = 0
		}
		dBAsset.AssetObj.TotalSupply -= asset.AssetObj.Balance
		if dBAsset.AssetObj.TotalSupply < 0 {
			glog.Warningf("DisconnectAssetOutput total supply is negative %v, setting to 0...", dBAsset.AssetObj.TotalSupply)
			dBAsset.AssetObj.TotalSupply = 0
		}
	}
	// logic follows core CheckAssetInputs()
	// undo data fields from last update
	// if fields changed then undo them using prev fields
	dBAsset.AssetObj.UpdateFlags = wire.ASSET_UPDATE_SUPPLY
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_DATA) != 0 {
		dBAsset.AssetObj.PubData = asset.AssetObj.PrevPubData
		if len(dBAsset.AssetObj.PubData) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_DATA
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_DATA
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_CONTRACT) != 0 {
		dBAsset.AssetObj.Contract = asset.AssetObj.PrevContract
		if len(dBAsset.AssetObj.Contract) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_CONTRACT
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_CONTRACT
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_NOTARY_KEY) != 0 {
		dBAsset.AssetObj.NotaryKeyID = asset.AssetObj.PrevNotaryKeyID
		if len(dBAsset.AssetObj.NotaryKeyID) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_NOTARY_KEY
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_NOTARY_KEY
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_NOTARY_DETAILS) != 0 {
		dBAsset.AssetObj.NotaryDetails = asset.AssetObj.PrevNotaryDetails
		if len(dBAsset.AssetObj.NotaryDetails.EndPoint) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_NOTARY_DETAILS
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_NOTARY_DETAILS
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_AUXFEE_KEY) != 0 {
		dBAsset.AssetObj.AuxFeeKeyID = asset.AssetObj.PrevAuxFeeKeyID
		if len(dBAsset.AssetObj.AuxFeeKeyID) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_AUXFEE_KEY
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_AUXFEE_KEY
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_AUXFEE_DETAILS) != 0 {
		dBAsset.AssetObj.AuxFeeDetails = asset.AssetObj.PrevAuxFeeDetails
		if len(dBAsset.AssetObj.AuxFeeDetails.AuxFees) > 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_AUXFEE_DETAILS
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_AUXFEE_DETAILS
		}
    }
    if (asset.AssetObj.UpdateFlags & wire.ASSET_UPDATE_CAPABILITYFLAGS) != 0 {
		dBAsset.AssetObj.UpdateCapabilityFlags = asset.AssetObj.PrevUpdateCapabilityFlags
		if dBAsset.AssetObj.UpdateCapabilityFlags != 0 {
			dBAsset.AssetObj.UpdateFlags |= wire.ASSET_UPDATE_CAPABILITYFLAGS
		} else {
			dBAsset.AssetObj.UpdateFlags &= ^wire.ASSET_UPDATE_CAPABILITYFLAGS
		}
	}

	return nil
}

func (d *RocksDB) ConnectAllocationInput(addrDesc* bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance, btxID []byte, assetInfo* bchain.AssetInfo, blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	counted := d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers++
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
	if !isActivate && dBAsset == nil {
		return errors.New(fmt.Sprint("ConnectAllocationOutput could not read asset " , assetInfo.AssetGuid))
	}
	counted := d.addToAssetsMap(txAssets, assetInfo.AssetGuid, btxID, version, height)
	if !counted {
		if dBAsset != nil {
			dBAsset.Transactions++
			assets[assetInfo.AssetGuid] = dBAsset
		}
	}
	// asset guid + txid + address of output/input must match for counted to be true
	counted = d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers++
	}
	balanceAsset.BalanceSat.Add(balanceAsset.BalanceSat, assetInfo.ValueSat)
	return nil
}

func (d *RocksDB) ConnectAssetOutput(asset *bchain.Asset, isActivate bool, isAssetTx bool, isAssetSendTx bool, assets map[uint32]*bchain.Asset) error {
	var dBAsset* bchain.Asset = nil
	var err error
	assetGuid := asset.AssetObj.Allocation.VoutAssets[0].AssetGuid
	if !isActivate {
		dBAsset, err = d.GetAsset(assetGuid, assets)
		if  err != nil {
			return err
		}
		if dBAsset == nil {
			return errors.New(fmt.Sprint("ConnectAssetOutput could not read asset " , assetGuid))
		}
	} else {
		dBAsset = &bchain.Asset{Transactions: 1, AssetObj: asset.AssetObj}
	}
	if dBAsset != nil {
		if isAssetTx {
			err = d.ConnectAssetOutputHelper(isActivate, asset, dBAsset)
			if err != nil {
				return err
			}
		} else if isAssetSendTx {
			// tally total amount and subtract from asset
			valueSat := int64(0)
			for _, v := range asset.AssetObj.Allocation.VoutAssets[0].Values {
				valueSat += v.ValueSat
			}
			dBAsset.AssetObj.Balance -= valueSat
			if dBAsset.AssetObj.Balance < 0 {
				glog.Warningf("ConnectAssetOutput balance is negative %v, setting to 0...", dBAsset.AssetObj.Balance)
				dBAsset.AssetObj.Balance = 0
			}
		} 
		assets[assetGuid] = dBAsset
	} else {
		return errors.New("ConnectSyscoinOutput: asset not found")
	}
	return nil
}

func (d *RocksDB) DisconnectAllocationOutput(addrDesc *bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance,  btxID []byte,  assetInfo *bchain.AssetInfo, blockTxAssetAddresses bchain.TxAssetAddressMap) error {
	balanceAsset.BalanceSat.Sub(balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.BalanceSat.Sign() < 0 {
		balanceAsset.BalanceSat.SetInt64(0)
	}

	counted := d.addToAssetAddressMap(blockTxAssetAddresses, assetInfo.AssetGuid, btxID, addrDesc)
	if !counted {
		balanceAsset.Transfers--
	}
	return nil
}
func (d *RocksDB) DisconnectAssetOutput(asset *bchain.Asset, isActivate bool, isAssetSendTx bool, assets map[uint32]*bchain.Asset) error {
	assetGuid := asset.AssetObj.Allocation.VoutAssets[0].AssetGuid
	dBAsset, err := d.GetAsset(assetGuid, assets)
	if dBAsset == nil || err != nil {
		if dBAsset == nil {
			return errors.New(fmt.Sprint("DisconnectAssetOutput could not read asset " , assetGuid))
		}
		return err
	}
	if !isActivate {
		if isAssetSendTx {
			// tally total amount and add to asset
			valueSat := int64(0)
			for _, v := range asset.AssetObj.Allocation.VoutAssets[0].Values {
				valueSat += v.ValueSat
			}
			dBAsset.AssetObj.Balance += valueSat
		} else {
			err = d.DisconnectAssetOutputHelper(asset, dBAsset)
			if err != nil {
				return err
			}
		}
	} else {
		// signals for removal from asset db
		dBAsset.AssetObj.TotalSupply = -1
	}
	assets[assetGuid] = dBAsset
	return nil
}
func (d *RocksDB) DisconnectAllocationInput(addrDesc *bchain.AddressDescriptor, balanceAsset *bchain.AssetBalance,  btxID []byte, assetInfo *bchain.AssetInfo, blockTxAssetAddresses bchain.TxAssetAddressMap, assets map[uint32]*bchain.Asset, assetFoundInTx func(asset uint32, btxID []byte) bool) error {
	dBAsset, err := d.GetAsset(assetInfo.AssetGuid, assets)
	if dBAsset == nil || err != nil {
		if dBAsset == nil {
			return errors.New(fmt.Sprint("DisconnectAllocationInput could not read asset " , assetInfo.AssetGuid))
		}
		return err
	}
	balanceAsset.SentSat.Sub(balanceAsset.SentSat, assetInfo.ValueSat)
	balanceAsset.BalanceSat.Add(balanceAsset.BalanceSat, assetInfo.ValueSat)
	if balanceAsset.SentSat.Sign() < 0 {
		balanceAsset.SentSat.SetInt64(0)
	}
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
			return err
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
			buf, err := d.chainParser.PackAsset(asset)
			if err != nil {
				return err
			}
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
		return nil, errors.New("GetAsset: key not found in asset db")
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, errors.New("GetAsset: empty value in asset db")
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
				if (assetsBitMask == bchain.AllMask) || ((uint32(assetsBitMask) & mask) == mask) {
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