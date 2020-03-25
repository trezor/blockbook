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

func (d *RocksDB) ConnectAssetOutput(sptData []byte, balances map[string]*bchain.AddrBalance, version int32, addresses bchain.AddressesMap, btxID []byte, txAddresses* bchain.TxAddresses, assets map[uint32]*bchain.Asset) (uint32, error) {
	r := bytes.NewReader(sptData)
	var asset bchain.Asset
	var dBAsset *bchain.Asset
	err := asset.AssetObj.Deserialize(r)
	if err != nil {
		return 0, err
	}
	assetGuid := asset.AssetObj.Asset
	dBAsset, err = d.GetAsset(assetGuid, &assets)
	if err != nil || dBAsset == nil {
		if !d.chainParser.IsAssetActivateTx(version) {
			if err != nil {
				return assetGuid, err
			} else {
				glog.Warningf("ConnectAssetOutput asset %v was empty, skipping transaction...", assetGuid)
				return assetGuid, nil
			}
		} else {
			dBAsset = &asset
		}
	}
	dBAsset.Transactions++
	strAssetGuid := strconv.FormatUint(uint64(assetGuid), 10)
	senderAddress := asset.AssetObj.WitnessAddress.ToString("sys")
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress(senderAddress)
	if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("ConnectAssetOutput sender with asset %v (%v) could not be decoded error %v", assetGuid, string(assetSenderAddrDesc), err)
			}
		} else {
			glog.Warningf("ConnectAssetOutput sender with asset %v (%v) has invalid length: %d", assetGuid, string(assetSenderAddrDesc), len(assetSenderAddrDesc))
		}
		return assetGuid, errors.New("ConnectAssetOutput Skipping asset tx")
	}
	senderStr := string(assetSenderAddrDesc)
	balance, e := balances[senderStr]
	if !e {
		balance, err = d.GetAddrDescBalance(assetSenderAddrDesc, bchain.AddressBalanceDetailUTXOIndexed)
		if err != nil {
			return assetGuid, err
		}
		if balance == nil {
			balance = &bchain.AddrBalance{}
		}
		balances[senderStr] = balance
		d.cbs.balancesMiss++
	} else {
		d.cbs.balancesHit++
	}

	if len(asset.AssetObj.WitnessAddressTransfer.WitnessProgram) > 0 {
		receiverAddress := asset.AssetObj.WitnessAddressTransfer.ToString("sys")
		assetTransferWitnessAddrDesc, err := d.chainParser.GetAddrDescFromAddress(receiverAddress)
		if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("ConnectAssetOutput transferee with asset %v (%v) could not be decoded error %v", assetGuid, string(assetTransferWitnessAddrDesc), err)
				}
			} else {
				glog.Warningf("ConnectAssetOutput transferee with asset %v (%v) has invalid length: %d", assetGuid, string(assetTransferWitnessAddrDesc), len(assetTransferWitnessAddrDesc))
			}
			return assetGuid, errors.New("ConnectAssetOutput Skipping asset transfer tx")
		}
		transferStr := string(assetTransferWitnessAddrDesc)
		balanceTransfer, e1 := balances[transferStr]
		if !e1 {
			balanceTransfer, err = d.GetAddrDescBalance(assetTransferWitnessAddrDesc, bchain.AddressBalanceDetailUTXOIndexed)
			if err != nil {
				return assetGuid, err
			}
			if balanceTransfer == nil {
				balanceTransfer = &bchain.AddrBalance{}
			}
			balances[transferStr] = balanceTransfer
			d.cbs.balancesMiss++
		} else {
			d.cbs.balancesHit++
		}
		counted := addToAddressesMap(addresses, transferStr, btxID, int32(assetGuid))
		if !counted {
			balanceTransfer.Txs++
		}
		// transfer balance from old address to transfered address
		if balanceTransfer.AssetBalances == nil{
			balanceTransfer.AssetBalances = map[uint32]*bchain.AssetBalance{}
		}
		balanceAssetTransfer, ok := balanceTransfer.AssetBalances[assetGuid]
		if !ok {
			balanceAssetTransfer = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
			balanceTransfer.AssetBalances[assetGuid] = balanceAssetTransfer
		}
		balanceAssetTransfer.Transfers++
		balanceAsset, ok := balance.AssetBalances[assetGuid]
		if !ok {
			balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
			balance.AssetBalances[assetGuid] = balanceAsset
		}
		balanceAsset.Transfers++
		// transfer balance to new receiver
		totalSupplyDb := big.NewInt(dBAsset.AssetObj.TotalSupply)
		txAddresses.TokenTransferSummary = &bchain.TokenTransferSummary {
			Type:     d.chainParser.GetAssetTypeFromVersion(version),
			Token:    strAssetGuid,
			From:     senderAddress,
			To:       receiverAddress,
			Value:    (*bchain.Amount)(totalSupplyDb),
			Decimals: int(dBAsset.AssetObj.Precision),
			Symbol:   string(dBAsset.AssetObj.Symbol),
			Fee:      (*bchain.Amount)(big.NewInt(0)),
		}
		dBAsset.AssetObj.WitnessAddress = asset.AssetObj.WitnessAddressTransfer
		assets[assetGuid] = dBAsset
	} else {
		if balance.AssetBalances == nil{
			balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
		}
		balanceAsset, ok := balance.AssetBalances[assetGuid]
		if !ok {
			balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
			balance.AssetBalances[assetGuid] = balanceAsset
		}
		balanceAsset.Transfers++
		valueTo := big.NewInt(asset.AssetObj.Balance)
		if !d.chainParser.IsAssetActivateTx(version) {
			balanceDb := big.NewInt(dBAsset.AssetObj.Balance)
			balanceDb.Add(balanceDb, valueTo)
			supplyDb := big.NewInt(dBAsset.AssetObj.TotalSupply)
			supplyDb.Add(supplyDb, valueTo)
			dBAsset.AssetObj.Balance = balanceDb.Int64()
			dBAsset.AssetObj.TotalSupply = supplyDb.Int64()
			// logic follows core CheckAssetInputs()
			if len(asset.AssetObj.PubData) > 0 {
				dBAsset.AssetObj.PubData = asset.AssetObj.PubData
				dBAsset.AuxFeesAddr = d.GetAuxFeeAddr(asset.AssetObj.PubData)
			}
			if len(asset.AssetObj.Contract) > 0 {
				dBAsset.AssetObj.Contract = asset.AssetObj.Contract
			}
			if asset.AssetObj.UpdateFlags != dBAsset.AssetObj.UpdateFlags {
				dBAsset.AssetObj.UpdateFlags = asset.AssetObj.UpdateFlags
			}
			assets[assetGuid] = dBAsset
		} else {
			asset.AssetObj.TotalSupply = asset.AssetObj.Balance
			asset.AuxFeesAddr = d.GetAuxFeeAddr(asset.AssetObj.PubData)
			asset.Transactions = 1
			assets[assetGuid] = &asset
		}
		txAddresses.TokenTransferSummary = &bchain.TokenTransferSummary {
			Type:     d.chainParser.GetAssetTypeFromVersion(version),
			Token:    strAssetGuid,
			From:     senderAddress,
			Value:    (*bchain.Amount)(valueTo),
			Decimals: int(dBAsset.AssetObj.Precision),
			Symbol:   string(dBAsset.AssetObj.Symbol),
			Fee:       (*bchain.Amount)(big.NewInt(0)),
		}
		counted := addToAddressesMap(addresses, senderStr, btxID, ^int32(assetGuid))
		if !counted {
			balance.Txs++
		}
	}
	return assetGuid, nil
}

func (d *RocksDB) ConnectAssetAllocationOutput(sptData []byte, balances map[string]*bchain.AddrBalance, version int32, addresses bchain.AddressesMap, btxID []byte, txAddresses* bchain.TxAddresses, assets map[uint32]*bchain.Asset) (uint32, error) {
	r := bytes.NewReader(sptData)
	var assetAllocation wire.AssetAllocationType
	var dBAsset *bchain.Asset
	err := assetAllocation.Deserialize(r, version)
	if err != nil {
		return 0, err
	}
	totalAssetSentValue := big.NewInt(0)
	totalFeeValue := big.NewInt(0)
	assetGuid := assetAllocation.AssetAllocationTuple.Asset
	dBAsset, err = d.GetAsset(assetGuid, &assets)
	if err != nil || dBAsset == nil {
		if err == nil{
			return assetGuid, errors.New("ConnectAssetAllocationOutput Asset not found")
		}
		return assetGuid, err
	}
	dBAsset.Transactions++
	strAssetGuid := strconv.FormatUint(uint64(assetGuid), 10)
	senderAddress := assetAllocation.AssetAllocationTuple.WitnessAddress.ToString("sys")
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress(senderAddress)
	if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("ConnectAssetAllocationOutput sender with asset %v (%v) could not be decoded error %v", assetGuid, assetAllocation.AssetAllocationTuple.WitnessAddress.ToString("sys"), err)
			}
		} else {
			glog.Warningf("ConnectAssetAllocationOutput sender with asset %v (%v) has invalid length: %d", assetGuid, assetAllocation.AssetAllocationTuple.WitnessAddress.ToString("sys"), len(assetSenderAddrDesc))
		}
		return assetGuid, errors.New("ConnectAssetAllocationOutput Skipping asset allocation tx")
	}
	txAddresses.TokenTransferSummary = &bchain.TokenTransferSummary {
		Type:     d.chainParser.GetAssetTypeFromVersion(version),
		Token:    strAssetGuid,
		From:     senderAddress,
		Decimals: int(dBAsset.AssetObj.Precision),
		Symbol:   string(dBAsset.AssetObj.Symbol),
		Fee:       (*bchain.Amount)(big.NewInt(0)),
	}
	if d.chainParser.IsAssetSendTx(version) {
		txAddresses.TokenTransferSummary.Type = bchain.SPTAssetSendType
	}
	txAddresses.TokenTransferSummary.Recipients = make([]*bchain.TokenTransferRecipient, len(assetAllocation.ListSendingAllocationAmounts))
	for i, allocation := range assetAllocation.ListSendingAllocationAmounts {
		receiverAddress := allocation.WitnessAddress.ToString("sys")
		addrDesc, err := d.chainParser.GetAddrDescFromAddress(receiverAddress)
		if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("ConnectAssetAllocationOutput receiver with asset %v (%v) could not be decoded error %v", assetGuid, allocation.WitnessAddress.ToString("sys"), err)
				}
			} else {
				glog.Warningf("ConnectAssetAllocationOutput receiver with asset %v (%v) has invalid length: %d", assetGuid, allocation.WitnessAddress.ToString("sys"), len(addrDesc))
			}
			continue
		}
		receiverStr := string(addrDesc)
		balance, e := balances[receiverStr]
		if !e {
			balance, err = d.GetAddrDescBalance(addrDesc, bchain.AddressBalanceDetailUTXOIndexed)
			if err != nil {
				return assetGuid, err
			}
			if balance == nil {
				balance = &bchain.AddrBalance{}
			}
			balances[receiverStr] = balance
			d.cbs.balancesMiss++
		} else {
			d.cbs.balancesHit++
		}

		// for each address returned, add it to map
		counted := addToAddressesMap(addresses, receiverStr, btxID, int32(assetGuid))
		if !counted {
			balance.Txs++
		}

		if balance.AssetBalances == nil {
			balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
		}
		balanceAsset, ok := balance.AssetBalances[assetGuid]
		if !ok {
			balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
			balance.AssetBalances[assetGuid] = balanceAsset
		}
		balanceAsset.Transfers++
		amount := big.NewInt(allocation.ValueSat)
		balanceAsset.BalanceAssetSat.Add(balanceAsset.BalanceAssetSat, amount)
		totalAssetSentValue.Add(totalAssetSentValue, amount)
		// if receiver is aux fees address for this asset, add fee for summary
		if bytes.Equal(dBAsset.AuxFeesAddr, addrDesc) {
			totalFeeValue.Add(totalFeeValue, amount)
		}
		txAddresses.TokenTransferSummary.Recipients[i] = &bchain.TokenTransferRecipient {
			To:       receiverAddress,
			Value:    (*bchain.Amount)(amount),
		}
	}
	txAddresses.TokenTransferSummary.Value = (*bchain.Amount)(totalAssetSentValue)
	if totalFeeValue.Int64() > 0 {
		txAddresses.TokenTransferSummary.Fee = (*bchain.Amount)(totalFeeValue)
	}
	return assetGuid, d.ConnectAssetAllocationInput(btxID, assetGuid, version, totalAssetSentValue, assetSenderAddrDesc, balances, addresses, dBAsset, assets)
}

func (d *RocksDB) DisconnectAssetAllocationOutput(sptData []byte, version int32, assets map[uint32]*bchain.Asset, btxID []byte, getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error), addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool) (uint32, error) {
	r := bytes.NewReader(sptData)
	var assetAllocation wire.AssetAllocationType
	err := assetAllocation.Deserialize(r, version)
	if err != nil {
		return 0, err
	}
	totalAssetSentValue := big.NewInt(0)
	assetGuid := assetAllocation.AssetAllocationTuple.Asset
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress(assetAllocation.AssetAllocationTuple.WitnessAddress.ToString("sys"))
	if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("DisconnectAssetAllocationOutput sender with asset %v (%v) could not be decoded error %v", assetGuid, string(assetSenderAddrDesc), err)
			}
		} else {
			glog.Warningf("DisconnectAssetAllocationOutput sender with asset %v (%v) has invalid length: %d", assetGuid, string(assetSenderAddrDesc), len(assetSenderAddrDesc))
		}
		return assetGuid, errors.New("DisconnectAssetAllocationOutput Skipping disconnect asset allocation tx")
	}
	for _, allocation := range assetAllocation.ListSendingAllocationAmounts {
		addrDesc, err := d.chainParser.GetAddrDescFromAddress(allocation.WitnessAddress.ToString("sys"))
		if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("DisconnectAssetAllocationOutput receiver with asset %v (%v) could not be decoded error %v", assetGuid, string(addrDesc), err)
				}
			} else {
				glog.Warningf("DisconnectAssetAllocationOutput receiver with asset %v (%v) has invalid length: %d", assetGuid, string(addrDesc), len(addrDesc))
			}
			continue
		}
		exist := addressFoundInTx(addrDesc, btxID)
		balance, err := getAddressBalance(addrDesc)
		if err != nil {
			return assetGuid, err
		}
		if balance != nil {
			// subtract number of txs only once
			if !exist {
				balance.Txs--
			}
		} else {
			ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(addrDesc)
			glog.Warningf("DisconnectAssetAllocationOutput Balance for asset address %v (%v) not found", ad, addrDesc)
		}

		if balance.AssetBalances != nil{
			balanceAsset := balance.AssetBalances[assetGuid]
			balanceAsset.Transfers--
			amount := big.NewInt(allocation.ValueSat)
			balanceAsset.BalanceAssetSat.Sub(balanceAsset.BalanceAssetSat, amount)
			if balanceAsset.BalanceAssetSat.Sign() < 0 {
				d.resetValueSatToZero(balanceAsset.BalanceAssetSat, addrDesc, "balance")
			}
			totalAssetSentValue.Add(totalAssetSentValue, amount)
		} else {
			ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(addrDesc)
			glog.Warningf("DisconnectAssetAllocationOutput Asset Balance for asset address %v (%v) not found", ad, addrDesc)
		}
	}
	return assetGuid, d.DisconnectAssetAllocationInput(assetGuid, version, totalAssetSentValue, assetSenderAddrDesc, assets, getAddressBalance)
}

func (d *RocksDB) ConnectAssetAllocationInput(btxID []byte, assetGuid uint32, version int32, totalAssetSentValue *big.Int, assetSenderAddrDesc bchain.AddressDescriptor, balances map[string]*bchain.AddrBalance, addresses bchain.AddressesMap, dBAsset *bchain.Asset, assets map[uint32]*bchain.Asset) error {
	if totalAssetSentValue == nil {
		return errors.New("totalAssetSentValue was nil cannot connect allocation input")
	}
	assetStrSenderAddrDesc := string(assetSenderAddrDesc)
	balance, e := balances[assetStrSenderAddrDesc]
	if !e {
		var err error
		balance, err = d.GetAddrDescBalance(assetSenderAddrDesc, bchain.AddressBalanceDetailUTXOIndexed)
		if err != nil {
			return err
		}
		if balance == nil {
			balance = &bchain.AddrBalance{}
		}
		balances[assetStrSenderAddrDesc] = balance
		d.cbs.balancesMiss++
	} else {
		d.cbs.balancesHit++
	}

	if balance.AssetBalances == nil {
		balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
	}
	balanceAsset, ok := balance.AssetBalances[assetGuid]
	if !ok {
		balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
		balance.AssetBalances[assetGuid] = balanceAsset
	}
	balanceAsset.Transfers++
	var balanceAssetSat *big.Int
	isAssetSend := d.chainParser.IsAssetSendTx(version)
	if isAssetSend {
		balanceAssetSat = big.NewInt(dBAsset.AssetObj.Balance)
	} else {
		balanceAssetSat = balanceAsset.BalanceAssetSat
	}
	balanceAsset.SentAssetSat.Add(balanceAsset.SentAssetSat, totalAssetSentValue)
	balanceAssetSat.Sub(balanceAssetSat, totalAssetSentValue)
	if balanceAssetSat.Sign() < 0 {
		d.resetValueSatToZero(balanceAssetSat, assetSenderAddrDesc, "balance")
	}
	if isAssetSend {
		dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
	}
	assets[assetGuid] = dBAsset
	counted := addToAddressesMap(addresses, assetStrSenderAddrDesc, btxID, ^int32(assetGuid))
	if !counted {
		balance.Txs++
	}
	return nil

}

func (d *RocksDB) DisconnectAssetOutput(sptData []byte, version int32, assets map[uint32]*bchain.Asset, btxID []byte, getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error), addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool) (uint32, error) {
	r := bytes.NewReader(sptData)
	var asset bchain.Asset
	var dBAsset *bchain.Asset
	err := asset.AssetObj.Deserialize(r)
	if err != nil {
		return 0, err
	}
	assetGuid := asset.AssetObj.Asset
	dBAsset, err = d.GetAsset(assetGuid, &assets)
	if err != nil || dBAsset == nil {
		if err == nil{
			return assetGuid, errors.New("DisconnectAssetOutput Asset not found")
		}
		return assetGuid, err
	}
	dBAsset.Transactions--
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress(asset.AssetObj.WitnessAddress.ToString("sys"))
	addressFoundInTx(assetSenderAddrDesc, btxID)
	balance, err := getAddressBalance(assetSenderAddrDesc)
	if err != nil {
		return assetGuid, err
	}
	if balance == nil {
		ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(assetSenderAddrDesc)
		glog.Warningf("DisconnectAssetOutput Balance for asset address %s (%s) not found", ad, assetSenderAddrDesc)
	}
	if len(asset.AssetObj.WitnessAddressTransfer.WitnessProgram) > 0 {
		assetTransferWitnessAddrDesc, err := d.chainParser.GetAddrDescFromAddress(asset.AssetObj.WitnessAddressTransfer.ToString("sys"))
		exist := addressFoundInTx(assetTransferWitnessAddrDesc, btxID)
		balanceTransfer, err := getAddressBalance(assetTransferWitnessAddrDesc)
		if err != nil {
			return assetGuid, err
		}
		if balanceTransfer != nil {
			// subtract number of txs only once
			if !exist {
				balanceTransfer.Txs--
			}
		} else {
			ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(assetTransferWitnessAddrDesc)
			glog.Warningf("DisconnectAssetOutput Balance for transfer asset address %s (%s) not found", ad, assetTransferWitnessAddrDesc)
		}

		balanceAsset := balance.AssetBalances[assetGuid]
		balanceAsset.Transfers--
		balanceTransferAsset := balanceTransfer.AssetBalances[assetGuid]
		balanceTransferAsset.Transfers--
		// reset owner back to original asset sender
		dBAsset.AssetObj.WitnessAddress = asset.AssetObj.WitnessAddress
		assets[assetGuid] = dBAsset
	} else if balance.AssetBalances != nil {
		balanceAsset := balance.AssetBalances[assetGuid]
		balanceAsset.Transfers--
		if !d.chainParser.IsAssetActivateTx(version) {
			balanceDb := big.NewInt(dBAsset.AssetObj.Balance)
			valueTo := big.NewInt(asset.AssetObj.Balance)
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
			assets[assetGuid] = dBAsset
		} else {
			// flag to erase asset
			asset.AssetObj.TotalSupply = -1
			assets[assetGuid] = &asset
		}
	} else {
		glog.Warningf("DisconnectAssetOutput: Asset Sent balance not found guid %v (%v)", assetGuid, string(assetSenderAddrDesc))
	}
	return assetGuid, nil

}

func (d *RocksDB) DisconnectAssetAllocationInput(assetGuid uint32, version int32, totalAssetSentValue *big.Int, assetSenderAddrDesc bchain.AddressDescriptor, assets map[uint32]*bchain.Asset, getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error)) error {
	balance, err := getAddressBalance(assetSenderAddrDesc)
	var dBAsset *bchain.Asset
	dBAsset, err = d.GetAsset(assetGuid, &assets)
	if err != nil || dBAsset == nil {
		if err == nil{
			return errors.New("DisconnectAssetAllocationInput Asset not found")
		}
		return err
	}
	dBAsset.Transactions--
	if balance.AssetBalances != nil {
		balanceAsset := balance.AssetBalances[assetGuid]
		balanceAsset.Transfers--
		var balanceAssetSat *big.Int
		isAssetSend := d.chainParser.IsAssetSendTx(version)
		if isAssetSend {
			balanceAssetSat = big.NewInt(dBAsset.AssetObj.Balance)
		} else {
			balanceAssetSat = balanceAsset.BalanceAssetSat
		}
		balanceAsset.SentAssetSat.Sub(balanceAsset.SentAssetSat, totalAssetSentValue)
		if balanceAsset.SentAssetSat.Sign() < 0 {
			d.resetValueSatToZero(balanceAsset.SentAssetSat, assetSenderAddrDesc, "balance")
		}
		balanceAssetSat.Add(balanceAssetSat, totalAssetSentValue)
		if isAssetSend {
			dBAsset.AssetObj.Balance = balanceAssetSat.Int64()
		}

	} else {
		glog.Warningf("DisconnectAssetAllocationInput: Asset Sent balance not found guid %v (%v)", assetGuid, string(assetSenderAddrDesc))
	}
	assets[assetGuid] = dBAsset
	return nil

}

func (d *RocksDB) ConnectMintAssetOutput(sptData []byte, balances map[string]*bchain.AddrBalance, version int32, addresses bchain.AddressesMap, btxID []byte, txAddresses* bchain.TxAddresses, assets map[uint32]*bchain.Asset) (uint32, error) {
	r := bytes.NewReader(sptData)
	var mintasset wire.MintSyscoinType
	var dBAsset *bchain.Asset
	err := mintasset.Deserialize(r)
	if err != nil {
		return 0, err
	}
	assetGuid := mintasset.AssetAllocationTuple.Asset
	dBAsset, err = d.GetAsset(assetGuid, &assets)
	if err != nil || dBAsset == nil {
		if err == nil{
			return assetGuid, errors.New("ConnectMintAssetOutput Asset not found")
		}
		return assetGuid, err
	}
	dBAsset.Transactions++
	strAssetGuid := strconv.FormatUint(uint64(assetGuid), 10)
	senderAddress := "burn"
	receiverAddress := mintasset.AssetAllocationTuple.WitnessAddress.ToString("sys")
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress(senderAddress)
	if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("ConnectMintAssetOutput sender with asset %v (%v) could not be decoded error %v", assetGuid, receiverAddress, err)
			}
		} else {
			glog.Warningf("ConnectMintAssetOutput sender with asset %v (%v) has invalid length: %d", assetGuid, receiverAddress, len(assetSenderAddrDesc))
		}
		return assetGuid, errors.New("ConnectMintAssetOutput Skipping asset mint tx")
	}
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(receiverAddress)
	if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("ConnectMintAssetOutput receiver with asset %v (%v) could not be decoded error %v", assetGuid, receiverAddress, err)
			}
		} else {
			glog.Warningf("ConnectMintAssetOutput receiver with asset %v (%v) has invalid length: %d", assetGuid, receiverAddress, len(addrDesc))
		}
		return assetGuid, errors.New("ConnectMintAssetOutput Skipping asset mint tx")
	}
	receiverStr := string(addrDesc)
	balance, e := balances[receiverStr]
	if !e {
		balance, err = d.GetAddrDescBalance(addrDesc, bchain.AddressBalanceDetailUTXOIndexed)
		if err != nil {
			return assetGuid, err
		}
		if balance == nil {
			balance = &bchain.AddrBalance{}
		}
		balances[receiverStr] = balance
		d.cbs.balancesMiss++
	} else {
		d.cbs.balancesHit++
	}

	// for each address returned, add it to map
	counted := addToAddressesMap(addresses, receiverStr, btxID, int32(assetGuid))
	if !counted {
		balance.Txs++
	}

	if balance.AssetBalances == nil {
		balance.AssetBalances = map[uint32]*bchain.AssetBalance{}
	}
	balanceAsset, ok := balance.AssetBalances[assetGuid]
	if !ok {
		balanceAsset = &bchain.AssetBalance{Transfers: 0, BalanceAssetSat: big.NewInt(0), SentAssetSat: big.NewInt(0)}
		balance.AssetBalances[assetGuid] = balanceAsset
	}
	balanceAsset.Transfers++
	amount := big.NewInt(mintasset.ValueAsset)
	balanceAsset.BalanceAssetSat.Add(balanceAsset.BalanceAssetSat, amount)
	txAddresses.TokenTransferSummary = &bchain.TokenTransferSummary {
		Type:     d.chainParser.GetAssetTypeFromVersion(version),
		Token:    strAssetGuid,
		From:     senderAddress,
		Value:    (*bchain.Amount)(amount),
		Decimals: int(dBAsset.AssetObj.Precision),
		Symbol:   string(dBAsset.AssetObj.Symbol),
		Fee:       (*bchain.Amount)(big.NewInt(0)),
	}
	txAddresses.TokenTransferSummary.Recipients = make([]*bchain.TokenTransferRecipient, 1)
	txAddresses.TokenTransferSummary.Recipients[0] = &bchain.TokenTransferRecipient{
		Value: (*bchain.Amount)(amount), 
		To: receiverAddress,
	}
	return assetGuid, d.ConnectAssetAllocationInput(btxID, assetGuid, version, amount, assetSenderAddrDesc, balances, addresses, dBAsset, assets)
}

func (d *RocksDB) DisconnectMintAssetOutput(sptData []byte, version int32, assets map[uint32]*bchain.Asset, btxID []byte, getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error), addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool) (uint32, error) {
	r := bytes.NewReader(sptData)
	var mintasset wire.MintSyscoinType
	err := mintasset.Deserialize(r)
	if err != nil {
		return 0, err
	}
	assetGuid := mintasset.AssetAllocationTuple.Asset
	assetSenderAddrDesc, err := d.chainParser.GetAddrDescFromAddress("burn")
	if err != nil || len(assetSenderAddrDesc) == 0 || len(assetSenderAddrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("DisconnectMintAssetOutput sender with asset %v (%v) could not be decoded error %v", assetGuid, string(assetSenderAddrDesc), err)
			}
		} else {
			glog.Warningf("DisconnectMintAssetOutput sender with asset %v (%v) has invalid length: %d", assetGuid, string(assetSenderAddrDesc), len(assetSenderAddrDesc))
		}
		return assetGuid, errors.New("DisconnectMintAssetOutput Skipping disconnect asset mint tx")
	}
	
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(mintasset.AssetAllocationTuple.WitnessAddress.ToString("sys"))
	if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("DisconnectMintAssetOutput receiver with asset %v (%v) could not be decoded error %v", assetGuid, string(addrDesc), err)
			}
		} else {
			glog.Warningf("DisconnectMintAssetOutput receiver with asset %v (%v) has invalid length: %d", assetGuid, string(addrDesc), len(addrDesc))
		}
		return assetGuid, errors.New("DisconnectMintAssetOutput Skipping disconnect asset mint tx")
	}
	exist := addressFoundInTx(addrDesc, btxID)
	balance, err := getAddressBalance(addrDesc)
	if err != nil {
		return assetGuid, err
	}
	if balance != nil {
		// subtract number of txs only once
		if !exist {
			balance.Txs--
		}
	} else {
		ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(addrDesc)
		glog.Warningf("DisconnectMintAssetOutput Balance for asset address %v (%v) not found", ad, addrDesc)
	}
	var totalAssetSentValue *big.Int
	if balance.AssetBalances != nil{
		balanceAsset := balance.AssetBalances[assetGuid]
		balanceAsset.Transfers--
		totalAssetSentValue := big.NewInt(mintasset.ValueAsset)
		balanceAsset.BalanceAssetSat.Sub(balanceAsset.BalanceAssetSat, totalAssetSentValue)
		if balanceAsset.BalanceAssetSat.Sign() < 0 {
			d.resetValueSatToZero(balanceAsset.BalanceAssetSat, addrDesc, "balance")
		}
	} else {
		ad, _, _ := d.chainParser.GetAddressesFromAddrDesc(addrDesc)
		glog.Warningf("DisconnectMintAssetOutput Asset Balance for asset address %v (%v) not found", ad, addrDesc)
	}
	return assetGuid, d.DisconnectAssetAllocationInput(assetGuid, version, totalAssetSentValue, assetSenderAddrDesc, assets, getAddressBalance)
}

func (d *RocksDB) ConnectSyscoinOutputs(height uint32, blockHash string, addrDesc bchain.AddressDescriptor, balances map[string]*bchain.AddrBalance, version int32, addresses bchain.AddressesMap, btxID []byte,  txAddresses* bchain.TxAddresses, assets map[uint32]*bchain.Asset, txAssets map[string]*bchain.TxAsset) error {
	script, err := d.chainParser.GetScriptFromAddrDesc(addrDesc)
	if err != nil {
		return err
	}
	sptData := d.chainParser.TryGetOPReturn(script, version)
	if sptData == nil {
		return nil
	}
	var assetGuid uint32
	if d.chainParser.IsAssetAllocationTx(version) {
		assetGuid, err = d.ConnectAssetAllocationOutput(sptData, balances, version, addresses, btxID, txAddresses, assets)
	} else if d.chainParser.IsAssetTx(version) {
		assetGuid, err = d.ConnectAssetOutput(sptData, balances, version, addresses, btxID, txAddresses, assets)
	} else if d.chainParser.IsSyscoinMintTx(version) {
		assetGuid, err = d.ConnectMintAssetOutput(sptData, balances, version, addresses, btxID, txAddresses, assets)
	}
	if height > 0 && assetGuid > 0 && err == nil {
		txAsset, ok := txAssets[blockHash]
		if !ok {
			txAsset = &bchain.TxAsset{Txs: []*bchain.TxAssetIndex{}, AssetGuid: assetGuid}
			txAssets[blockHash] = txAsset
		}
		txAsset.Txs = append(txAsset.Txs, &bchain.TxAssetIndex{Type: d.chainParser.GetAssetsMaskFromVersion(version), Txid: btxID})
		txAsset.Height = height
	}	
	return err
}

func (d *RocksDB) DisconnectSyscoinOutputs(height uint32, btxID []byte, addrDesc bchain.AddressDescriptor, version int32, assets map[uint32]*bchain.Asset, txAssets []*bchain.TxAsset, getAddressBalance func(addrDesc bchain.AddressDescriptor) (*bchain.AddrBalance, error), addressFoundInTx func(addrDesc bchain.AddressDescriptor, btxID []byte) bool) error {
	script, err := d.chainParser.GetScriptFromAddrDesc(addrDesc)
	if err != nil {
		return err
	}
	sptData := d.chainParser.TryGetOPReturn(script, version)
	if sptData == nil {
		return nil
	}
	var assetGuid uint32
	if d.chainParser.IsAssetAllocationTx(version) {
		assetGuid, err = d.DisconnectAssetAllocationOutput(sptData, version, assets, btxID, getAddressBalance, addressFoundInTx)
	} else if d.chainParser.IsAssetTx(version) {
		assetGuid, err = d.DisconnectAssetOutput(sptData, version, assets, btxID, getAddressBalance, addressFoundInTx)
	} else if d.chainParser.IsSyscoinMintTx(version) {
		assetGuid, err = d.DisconnectMintAssetOutput(sptData, version, assets, btxID, getAddressBalance, addressFoundInTx)
	}
	if assetGuid > 0 && err == nil {
		txAssets = append(txAssets, &bchain.TxAsset{AssetGuid: assetGuid, Height: height})
	}
	return err
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

func (d *RocksDB) storeTxAssets(wb *gorocksdb.WriteBatch, txassets map[string]*bchain.TxAsset) error {
	for _, txAsset := range txassets {
		key := d.chainParser.PackAssetKey(txAsset.AssetGuid, txAsset.Height)
		buf := d.chainParser.PackAssetTxIndex(txAsset)
		wb.PutCF(d.cfh[cfTxAssets], key, buf)
	}
	return nil
}

func (d *RocksDB) removeTxAssets(wb *gorocksdb.WriteBatch, txassets []*bchain.TxAsset) error {
	for _, txAsset := range txassets {
		key := d.chainParser.PackAssetKey(txAsset.AssetGuid, txAsset.Height)
		wb.DeleteCF(d.cfh[cfAddresses], key)
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
				if (assetsBitMask == bchain.AssetAllMask || (uint32(assetsBitMask) & mask) == mask) {
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

func (d *RocksDB) GetTokenTransferSummaryFromTx(tx *bchain.Tx) ([]*bchain.TokenTransferSummary, error) {
	assets := make(map[uint32]*bchain.Asset)
	txAssets := make(map[string]*bchain.TxAsset, 0)
	balances := make(map[string]*bchain.AddrBalance)
	addresses := make(bchain.AddressesMap)
	btxID, err := d.chainParser.PackTxid(tx.Txid)
	if err != nil {
		return nil, err
	}
	ta := bchain.TxAddresses{Version: tx.Version, Height: 0}
	isSyscoinTx := d.chainParser.IsSyscoinTx(tx.Version)
	maxAddrDescLen := d.chainParser.GetMaxAddrLength()
	for i, output := range tx.Vout {
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
		} else if isSyscoinTx && addrDesc[0] == txscript.OP_RETURN {
			err := d.ConnectSyscoinOutputs(0, "", addrDesc, balances, tx.Version, addresses, btxID, &ta, assets, txAssets)
			if err != nil {
				glog.Warningf("rocksdb: ConnectSyscoinOutputs: height %d, tx %v, output %v, error %v", block.Height, tx.Txid, output, err)
				return nil, err
			}
		}
	}
	return ta.TokenTransferSummary, nil
}