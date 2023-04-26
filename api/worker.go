package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
)

// Worker is handle to api worker
type Worker struct {
	db                *db.RocksDB
	txCache           *db.TxCache
	chain             bchain.BlockChain
	chainParser       bchain.BlockChainParser
	chainType         bchain.ChainType
	useAddressAliases bool
	mempool           bchain.Mempool
	is                *common.InternalState
	fiatRates         *fiat.FiatRates
	metrics           *common.Metrics
}

// NewWorker creates new api worker
func NewWorker(db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState, fiatRates *fiat.FiatRates) (*Worker, error) {
	w := &Worker{
		db:                db,
		txCache:           txCache,
		chain:             chain,
		chainParser:       chain.GetChainParser(),
		chainType:         chain.GetChainParser().GetChainType(),
		useAddressAliases: chain.GetChainParser().UseAddressAliases(),
		mempool:           mempool,
		is:                is,
		fiatRates:         fiatRates,
		metrics:           metrics,
	}
	if w.chainType == bchain.ChainBitcoinType {
		w.initXpubCache()
	}
	return w, nil
}

func (w *Worker) getAddressesFromVout(vout *bchain.Vout) (bchain.AddressDescriptor, []string, bool, error) {
	addrDesc, err := w.chainParser.GetAddrDescFromVout(vout)
	if err != nil {
		return nil, nil, false, err
	}
	a, s, err := w.chainParser.GetAddressesFromAddrDesc(addrDesc)
	return addrDesc, a, s, err
}

// setSpendingTxToVout is helper function, that finds transaction that spent given output and sets it to the output
// there is no direct index for the operation, it must be found using addresses -> txaddresses -> tx
func (w *Worker) setSpendingTxToVout(vout *Vout, txid string, height uint32) error {
	err := w.db.GetAddrDescTransactions(vout.AddrDesc, height, maxUint32, func(t string, height uint32, indexes []int32) error {
		for _, index := range indexes {
			// take only inputs
			if index < 0 {
				index = ^index
				tsp, err := w.db.GetTxAddresses(t)
				if err != nil {
					return err
				} else if tsp == nil {
					glog.Warning("DB inconsistency:  tx ", t, ": not found in txAddresses")
				} else if len(tsp.Inputs) > int(index) {
					if tsp.Inputs[index].ValueSat.Cmp((*big.Int)(vout.ValueSat)) == 0 {
						spentTx, spentHeight, err := w.txCache.GetTransaction(t)
						if err != nil {
							glog.Warning("Tx ", t, ": not found")
						} else {
							if len(spentTx.Vin) > int(index) {
								if spentTx.Vin[index].Txid == txid {
									vout.SpentTxID = t
									vout.SpentHeight = int(spentHeight)
									vout.SpentIndex = int(index)
									return &db.StopIteration{}
								}
							}
						}
					}
				}
			}
		}
		return nil
	})
	return err
}

// GetSpendingTxid returns transaction id of transaction that spent given output
func (w *Worker) GetSpendingTxid(txid string, n int) (string, error) {
	if w.db.HasExtendedIndex() {
		tsp, err := w.db.GetTxAddresses(txid)
		if err != nil {
			return "", err
		} else if tsp == nil {
			glog.Warning("DB inconsistency:  tx ", txid, ": not found in txAddresses")
			return "", NewAPIError(fmt.Sprintf("Txid %v not found", txid), false)
		}
		if n >= len(tsp.Outputs) || n < 0 {
			return "", NewAPIError(fmt.Sprintf("Passed incorrect vout index %v for tx %v, len vout %v", n, txid, len(tsp.Outputs)), false)
		}
		return tsp.Outputs[n].SpentTxid, nil
	}
	start := time.Now()
	tx, err := w.getTransaction(txid, false, false, nil)
	if err != nil {
		return "", err
	}
	if n >= len(tx.Vout) || n < 0 {
		return "", NewAPIError(fmt.Sprintf("Passed incorrect vout index %v for tx %v, len vout %v", n, tx.Txid, len(tx.Vout)), false)
	}
	err = w.setSpendingTxToVout(&tx.Vout[n], tx.Txid, uint32(tx.Blockheight))
	if err != nil {
		return "", err
	}
	glog.Info("GetSpendingTxid ", txid, " ", n, ", ", time.Since(start))
	return tx.Vout[n].SpentTxID, nil
}

func aggregateAddress(m map[string]struct{}, a string) {
	if m != nil && len(a) > 0 {
		m[a] = struct{}{}
	}
}

func aggregateAddresses(m map[string]struct{}, addresses []string, isAddress bool) {
	if m != nil && isAddress {
		for _, a := range addresses {
			if len(a) > 0 {
				m[a] = struct{}{}
			}
		}
	}
}

func (w *Worker) newAddressesMapForAliases() map[string]struct{} {
	// return non nil map only if the chain supports address aliases
	if w.useAddressAliases {
		return make(map[string]struct{})
	}
	// returning nil disables the processing of the address aliases
	return nil
}

func (w *Worker) getAddressAliases(addresses map[string]struct{}) AddressAliasesMap {
	if len(addresses) > 0 {
		aliases := make(AddressAliasesMap)
		var t string
		if w.chainType == bchain.ChainEthereumType {
			t = "ENS"
		} else {
			t = "Alias"
		}
		for a := range addresses {
			if w.chainType == bchain.ChainEthereumType {
				ci, err := w.db.GetContractInfoForAddress(a)
				if err == nil && ci != nil && ci.Name != "" {
					aliases[a] = AddressAlias{Type: "Contract", Alias: ci.Name}
				}
			}
			n := w.db.GetAddressAlias(a)
			if len(n) > 0 {
				aliases[a] = AddressAlias{Type: t, Alias: n}
			}
		}
		return aliases
	}
	return nil
}

// GetTransaction reads transaction data from txid
func (w *Worker) GetTransaction(txid string, spendingTxs bool, specificJSON bool) (*Tx, error) {
	addresses := w.newAddressesMapForAliases()
	tx, err := w.getTransaction(txid, spendingTxs, specificJSON, addresses)
	if err != nil {
		return nil, err
	}
	tx.AddressAliases = w.getAddressAliases(addresses)
	return tx, nil
}

// getTransaction reads transaction data from txid
func (w *Worker) getTransaction(txid string, spendingTxs bool, specificJSON bool, addresses map[string]struct{}) (*Tx, error) {
	bchainTx, height, err := w.txCache.GetTransaction(txid)
	if err != nil {
		if err == bchain.ErrTxNotFound {
			return nil, NewAPIError(fmt.Sprintf("Transaction '%v' not found", txid), true)
		}
		return nil, NewAPIError(fmt.Sprintf("Transaction '%v' not found (%v)", txid, err), true)
	}
	return w.getTransactionFromBchainTx(bchainTx, height, spendingTxs, specificJSON, addresses)
}

func (w *Worker) getParsedEthereumInputData(data string) *bchain.EthereumParsedInputData {
	var err error
	var signatures *[]bchain.FourByteSignature
	fourBytes := eth.GetSignatureFromData(data)
	if fourBytes != 0 {
		signatures, err = w.db.GetFourByteSignatures(fourBytes)
		if err != nil {
			glog.Errorf("GetFourByteSignatures(%v) error %v", fourBytes, err)
			return nil
		}
		if signatures == nil {
			return nil
		}
	}
	return eth.ParseInputData(signatures, data)
}

// getConfirmationETA returns confirmation ETA in seconds and blocks
func (w *Worker) getConfirmationETA(tx *Tx) (int64, uint32) {
	var etaBlocks uint32
	var etaSeconds int64
	if w.chainType == bchain.ChainBitcoinType && tx.FeesSat != nil {
		_, _, mempoolSize := w.is.GetMempoolSyncState()
		// if there are a few transactions in the mempool, the estimate fee does not work well
		// and the tx is most probably going to be confirmed in the first block
		if mempoolSize < 32 {
			etaBlocks = 1
		} else {
			var txFeePerKB int64
			if tx.VSize > 0 {
				txFeePerKB = 1000 * tx.FeesSat.AsInt64() / int64(tx.VSize)
			} else if tx.Size > 0 {
				txFeePerKB = 1000 * tx.FeesSat.AsInt64() / int64(tx.Size)
			}
			if txFeePerKB > 0 {
				// binary search the estimate, split it to more common first 7 blocks and the rest up to 70 blocks
				var b int
				fee, _ := w.cachedEstimateFee(7, true)
				if fee.Int64() <= txFeePerKB {
					b = sort.Search(7, func(i int) bool {
						// fee is in sats/kB
						fee, _ := w.cachedEstimateFee(i+1, true)
						return fee.Int64() <= txFeePerKB
					})
					b += 1
				} else {
					b = sort.Search(63, func(i int) bool {
						fee, _ := w.cachedEstimateFee(i+7, true)
						return fee.Int64() <= txFeePerKB
					})
					b += 7
				}
				etaBlocks = uint32(b)
			}
		}
		etaSeconds = int64(etaBlocks * w.is.AvgBlockPeriod)
	}
	return etaSeconds, etaBlocks
}

// getTransactionFromBchainTx reads transaction data from txid
func (w *Worker) getTransactionFromBchainTx(bchainTx *bchain.Tx, height int, spendingTxs bool, specificJSON bool, addresses map[string]struct{}) (*Tx, error) {
	var err error
	var ta *db.TxAddresses
	var tokens []TokenTransfer
	var ethSpecific *EthereumSpecific
	var blockhash string
	if bchainTx.Confirmations > 0 {
		if w.chainType == bchain.ChainBitcoinType {
			ta, err = w.db.GetTxAddresses(bchainTx.Txid)
			if err != nil {
				return nil, errors.Annotatef(err, "GetTxAddresses %v", bchainTx.Txid)
			}
		}
		blockhash, err = w.db.GetBlockHash(uint32(height))
		if err != nil {
			return nil, errors.Annotatef(err, "GetBlockHash %v", height)
		}
	}
	var valInSat, valOutSat, feesSat big.Int
	var pValInSat *big.Int
	vins := make([]Vin, len(bchainTx.Vin))
	rbf := false
	for i := range bchainTx.Vin {
		bchainVin := &bchainTx.Vin[i]
		vin := &vins[i]
		vin.Txid = bchainVin.Txid
		vin.N = i
		vin.Vout = bchainVin.Vout
		vin.Sequence = int64(bchainVin.Sequence)
		// detect explicit Replace-by-Fee transactions as defined by BIP125
		if bchainTx.Confirmations == 0 && bchainVin.Sequence < 0xffffffff-1 {
			rbf = true
		}
		vin.Hex = bchainVin.ScriptSig.Hex
		vin.Coinbase = bchainVin.Coinbase
		if w.chainType == bchain.ChainBitcoinType {
			//  bchainVin.Txid=="" is coinbase transaction
			if bchainVin.Txid != "" {
				// load spending addresses from TxAddresses
				tas, err := w.db.GetTxAddresses(bchainVin.Txid)
				if err != nil {
					return nil, errors.Annotatef(err, "GetTxAddresses %v", bchainVin.Txid)
				}
				if tas == nil {
					// try to load from backend
					otx, _, err := w.txCache.GetTransaction(bchainVin.Txid)
					if err != nil {
						if err == bchain.ErrTxNotFound {
							// try to get AddrDesc using coin specific handling and continue processing the tx
							vin.AddrDesc = w.chainParser.GetAddrDescForUnknownInput(bchainTx, i)
							vin.Addresses, vin.IsAddress, err = w.chainParser.GetAddressesFromAddrDesc(vin.AddrDesc)
							if err != nil {
								glog.Warning("GetAddressesFromAddrDesc tx ", bchainVin.Txid, ", addrDesc ", vin.AddrDesc, ": ", err)
							}
							aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
							continue
						}
						return nil, errors.Annotatef(err, "txCache.GetTransaction %v", bchainVin.Txid)
					}
					// mempool transactions are not in TxAddresses but confirmed should be there, log a problem
					// ignore when Confirmations==1, it may be just a timing problem
					if bchainTx.Confirmations > 1 {
						glog.Warning("DB inconsistency:  tx ", bchainVin.Txid, ": not found in txAddresses, confirmations ", bchainTx.Confirmations)
					}
					if len(otx.Vout) > int(vin.Vout) {
						vout := &otx.Vout[vin.Vout]
						vin.ValueSat = (*Amount)(&vout.ValueSat)
						vin.AddrDesc, vin.Addresses, vin.IsAddress, err = w.getAddressesFromVout(vout)
						if err != nil {
							glog.Errorf("getAddressesFromVout error %v, vout %+v", err, vout)
						}
						aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
					}
				} else {
					if len(tas.Outputs) > int(vin.Vout) {
						output := &tas.Outputs[vin.Vout]
						vin.ValueSat = (*Amount)(&output.ValueSat)
						vin.AddrDesc = output.AddrDesc
						vin.Addresses, vin.IsAddress, err = output.Addresses(w.chainParser)
						if err != nil {
							glog.Errorf("output.Addresses error %v, tx %v, output %v", err, bchainVin.Txid, i)
						}
						aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
					}
				}
				if vin.ValueSat != nil {
					valInSat.Add(&valInSat, (*big.Int)(vin.ValueSat))
				}
			}
		} else if w.chainType == bchain.ChainEthereumType {
			if len(bchainVin.Addresses) > 0 {
				vin.AddrDesc, err = w.chainParser.GetAddrDescFromAddress(bchainVin.Addresses[0])
				if err != nil {
					glog.Errorf("GetAddrDescFromAddress error %v, tx %v, bchainVin %v", err, bchainTx.Txid, bchainVin)
				}
				vin.Addresses = bchainVin.Addresses
				vin.IsAddress = true
				aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
			}
		}
	}
	vouts := make([]Vout, len(bchainTx.Vout))
	for i := range bchainTx.Vout {
		bchainVout := &bchainTx.Vout[i]
		vout := &vouts[i]
		vout.N = i
		vout.ValueSat = (*Amount)(&bchainVout.ValueSat)
		valOutSat.Add(&valOutSat, &bchainVout.ValueSat)
		vout.Hex = bchainVout.ScriptPubKey.Hex
		vout.AddrDesc, vout.Addresses, vout.IsAddress, err = w.getAddressesFromVout(bchainVout)
		if err != nil {
			glog.V(2).Infof("getAddressesFromVout error %v, %v, output %v", err, bchainTx.Txid, bchainVout.N)
		}
		aggregateAddresses(addresses, vout.Addresses, vout.IsAddress)
		if ta != nil {
			vout.Spent = ta.Outputs[i].Spent
			if vout.Spent {
				if w.db.HasExtendedIndex() {
					vout.SpentTxID = ta.Outputs[i].SpentTxid
					vout.SpentIndex = int(ta.Outputs[i].SpentIndex)
					vout.SpentHeight = int(ta.Outputs[i].SpentHeight)
				} else if spendingTxs {
					err = w.setSpendingTxToVout(vout, bchainTx.Txid, uint32(height))
					if err != nil {
						glog.Errorf("setSpendingTxToVout error %v, %v, output %v", err, vout.AddrDesc, vout.N)
					}
				}
			}
		}
	}
	if w.chainType == bchain.ChainBitcoinType {
		// for coinbase transactions valIn is 0
		feesSat.Sub(&valInSat, &valOutSat)
		if feesSat.Sign() == -1 {
			feesSat.SetUint64(0)
		}
		pValInSat = &valInSat
	} else if w.chainType == bchain.ChainEthereumType {
		tokenTransfers, err := w.chainParser.EthereumTypeGetTokenTransfersFromTx(bchainTx)
		if err != nil {
			glog.Errorf("GetTokenTransfersFromTx error %v, %v", err, bchainTx)
		}
		tokens = w.getEthereumTokensTransfers(tokenTransfers, addresses)
		ethTxData := eth.GetEthereumTxData(bchainTx)

		var internalData *bchain.EthereumInternalData
		if eth.ProcessInternalTransactions {
			internalData, err = w.db.GetEthereumInternalData(bchainTx.Txid)
			if err != nil {
				return nil, err
			}
		}

		parsedInputData := w.getParsedEthereumInputData(ethTxData.Data)

		// mempool txs do not have fees yet
		if ethTxData.GasUsed != nil {
			feesSat.Mul(ethTxData.GasPrice, ethTxData.GasUsed)
		}
		if len(bchainTx.Vout) > 0 {
			valOutSat = bchainTx.Vout[0].ValueSat
		}
		ethSpecific = &EthereumSpecific{
			GasLimit:   ethTxData.GasLimit,
			GasPrice:   (*Amount)(ethTxData.GasPrice),
			GasUsed:    ethTxData.GasUsed,
			Nonce:      ethTxData.Nonce,
			Status:     ethTxData.Status,
			Data:       ethTxData.Data,
			ParsedData: parsedInputData,
		}
		if internalData != nil {
			ethSpecific.Type = internalData.Type
			ethSpecific.CreatedContract = internalData.Contract
			ethSpecific.Error = internalData.Error
			ethSpecific.InternalTransfers = make([]EthereumInternalTransfer, len(internalData.Transfers))
			for i := range internalData.Transfers {
				f := &internalData.Transfers[i]
				t := &ethSpecific.InternalTransfers[i]
				t.From = f.From
				aggregateAddress(addresses, t.From)
				t.To = f.To
				aggregateAddress(addresses, t.To)
				t.Type = f.Type
				t.Value = (*Amount)(&f.Value)
			}
		}

	}
	var sj json.RawMessage
	// return CoinSpecificData for all mempool transactions or if requested
	if specificJSON || bchainTx.Confirmations == 0 {
		sj, err = w.chain.GetTransactionSpecific(bchainTx)
		if err != nil {
			return nil, err
		}
	}
	r := &Tx{
		Blockhash:        blockhash,
		Blockheight:      height,
		Blocktime:        bchainTx.Blocktime,
		Confirmations:    bchainTx.Confirmations,
		FeesSat:          (*Amount)(&feesSat),
		Locktime:         bchainTx.LockTime,
		Txid:             bchainTx.Txid,
		ValueInSat:       (*Amount)(pValInSat),
		ValueOutSat:      (*Amount)(&valOutSat),
		Version:          bchainTx.Version,
		Size:             len(bchainTx.Hex) >> 1,
		VSize:            int(bchainTx.VSize),
		Hex:              bchainTx.Hex,
		Rbf:              rbf,
		Vin:              vins,
		Vout:             vouts,
		CoinSpecificData: sj,
		TokenTransfers:   tokens,
		EthereumSpecific: ethSpecific,
	}
	if bchainTx.Confirmations == 0 {
		r.Blocktime = int64(w.mempool.GetTransactionTime(bchainTx.Txid))
		r.ConfirmationETASeconds, r.ConfirmationETABlocks = w.getConfirmationETA(r)
	}
	return r, nil
}

// GetTransactionFromMempoolTx converts bchain.MempoolTx to Tx, with limited amount of data
// it is not doing any request to backend or to db
func (w *Worker) GetTransactionFromMempoolTx(mempoolTx *bchain.MempoolTx) (*Tx, error) {
	var err error
	var valInSat, valOutSat, feesSat big.Int
	var pValInSat *big.Int
	var tokens []TokenTransfer
	var ethSpecific *EthereumSpecific
	addresses := w.newAddressesMapForAliases()
	vins := make([]Vin, len(mempoolTx.Vin))
	rbf := false
	for i := range mempoolTx.Vin {
		bchainVin := &mempoolTx.Vin[i]
		vin := &vins[i]
		vin.Txid = bchainVin.Txid
		vin.N = i
		vin.Vout = bchainVin.Vout
		vin.Sequence = int64(bchainVin.Sequence)
		// detect explicit Replace-by-Fee transactions as defined by BIP125
		if bchainVin.Sequence < 0xffffffff-1 {
			rbf = true
		}
		vin.Hex = bchainVin.ScriptSig.Hex
		vin.Coinbase = bchainVin.Coinbase
		if w.chainType == bchain.ChainBitcoinType {
			//  bchainVin.Txid=="" is coinbase transaction
			if bchainVin.Txid != "" {
				vin.ValueSat = (*Amount)(&bchainVin.ValueSat)
				vin.AddrDesc = bchainVin.AddrDesc
				vin.Addresses, vin.IsAddress, _ = w.chainParser.GetAddressesFromAddrDesc(vin.AddrDesc)
				if vin.ValueSat != nil {
					valInSat.Add(&valInSat, (*big.Int)(vin.ValueSat))
				}
				aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
			}
		} else if w.chainType == bchain.ChainEthereumType {
			if len(bchainVin.Addresses) > 0 {
				vin.AddrDesc, err = w.chainParser.GetAddrDescFromAddress(bchainVin.Addresses[0])
				if err != nil {
					glog.Errorf("GetAddrDescFromAddress error %v, tx %v, bchainVin %v", err, mempoolTx.Txid, bchainVin)
				}
				vin.Addresses = bchainVin.Addresses
				vin.IsAddress = true
				aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
			}
		}
	}
	vouts := make([]Vout, len(mempoolTx.Vout))
	for i := range mempoolTx.Vout {
		bchainVout := &mempoolTx.Vout[i]
		vout := &vouts[i]
		vout.N = i
		vout.ValueSat = (*Amount)(&bchainVout.ValueSat)
		valOutSat.Add(&valOutSat, &bchainVout.ValueSat)
		vout.Hex = bchainVout.ScriptPubKey.Hex
		vout.AddrDesc, vout.Addresses, vout.IsAddress, err = w.getAddressesFromVout(bchainVout)
		if err != nil {
			glog.V(2).Infof("getAddressesFromVout error %v, %v, output %v", err, mempoolTx.Txid, bchainVout.N)
		}
		aggregateAddresses(addresses, vout.Addresses, vout.IsAddress)
	}
	if w.chainType == bchain.ChainBitcoinType {
		// for coinbase transactions valIn is 0
		feesSat.Sub(&valInSat, &valOutSat)
		if feesSat.Sign() == -1 {
			feesSat.SetUint64(0)
		}
		pValInSat = &valInSat
	} else if w.chainType == bchain.ChainEthereumType {
		if len(mempoolTx.Vout) > 0 {
			valOutSat = mempoolTx.Vout[0].ValueSat
		}
		tokens = w.getEthereumTokensTransfers(mempoolTx.TokenTransfers, addresses)
		ethTxData := eth.GetEthereumTxDataFromSpecificData(mempoolTx.CoinSpecificData)
		ethSpecific = &EthereumSpecific{
			GasLimit: ethTxData.GasLimit,
			GasPrice: (*Amount)(ethTxData.GasPrice),
			GasUsed:  ethTxData.GasUsed,
			Nonce:    ethTxData.Nonce,
			Status:   ethTxData.Status,
			Data:     ethTxData.Data,
		}
	}
	r := &Tx{
		Blocktime:        mempoolTx.Blocktime,
		FeesSat:          (*Amount)(&feesSat),
		Locktime:         mempoolTx.LockTime,
		Txid:             mempoolTx.Txid,
		ValueInSat:       (*Amount)(pValInSat),
		ValueOutSat:      (*Amount)(&valOutSat),
		Version:          mempoolTx.Version,
		Size:             len(mempoolTx.Hex) >> 1,
		VSize:            int(mempoolTx.VSize),
		Hex:              mempoolTx.Hex,
		Rbf:              rbf,
		Vin:              vins,
		Vout:             vouts,
		TokenTransfers:   tokens,
		EthereumSpecific: ethSpecific,
		AddressAliases:   w.getAddressAliases(addresses),
	}
	r.ConfirmationETASeconds, r.ConfirmationETABlocks = w.getConfirmationETA(r)
	return r, nil
}

func (w *Worker) getContractInfo(contract string, typeFromContext bchain.TokenTypeName) (*bchain.ContractInfo, bool, error) {
	cd, err := w.chainParser.GetAddrDescFromAddress(contract)
	if err != nil {
		return nil, false, err
	}
	return w.getContractDescriptorInfo(cd, typeFromContext)
}

func (w *Worker) getContractDescriptorInfo(cd bchain.AddressDescriptor, typeFromContext bchain.TokenTypeName) (*bchain.ContractInfo, bool, error) {
	var err error
	validContract := true
	contractInfo, err := w.db.GetContractInfo(cd, typeFromContext)
	if err != nil {
		return nil, false, err
	}
	if contractInfo == nil {
		// log warning only if the contract should have been known from processing of the internal data
		if eth.ProcessInternalTransactions {
			glog.Warningf("Contract %v %v not found in DB", cd, typeFromContext)
		}
		contractInfo, err = w.chain.GetContractInfo(cd)
		if err != nil {
			glog.Errorf("GetContractInfo from chain error %v, contract %v", err, cd)
		}
		if contractInfo == nil {
			contractInfo = &bchain.ContractInfo{Type: bchain.UnknownTokenType, Decimals: w.chainParser.AmountDecimals()}
			addresses, _, _ := w.chainParser.GetAddressesFromAddrDesc(cd)
			if len(addresses) > 0 {
				contractInfo.Contract = addresses[0]
			}

			validContract = false
		} else {
			if typeFromContext != bchain.UnknownTokenType && contractInfo.Type == bchain.UnknownTokenType {
				contractInfo.Type = typeFromContext
			}
			if err = w.db.StoreContractInfo(contractInfo); err != nil {
				glog.Errorf("StoreContractInfo error %v, contract %v", err, cd)
			}
		}
	} else if (len(contractInfo.Name) > 0 && contractInfo.Name[0] == 0) || (len(contractInfo.Symbol) > 0 && contractInfo.Symbol[0] == 0) {
		// fix contract name/symbol that was parsed as a string consisting of zeroes
		blockchainContractInfo, err := w.chain.GetContractInfo(cd)
		if err != nil {
			glog.Errorf("GetContractInfo from chain error %v, contract %v", err, cd)
		} else {
			if blockchainContractInfo != nil && len(blockchainContractInfo.Name) > 0 && blockchainContractInfo.Name[0] != 0 {
				contractInfo.Name = blockchainContractInfo.Name
			} else {
				contractInfo.Name = ""
			}
			if blockchainContractInfo != nil && len(blockchainContractInfo.Symbol) > 0 && blockchainContractInfo.Symbol[0] != 0 {
				contractInfo.Symbol = blockchainContractInfo.Symbol
			} else {
				contractInfo.Symbol = ""
			}
			if blockchainContractInfo != nil {
				contractInfo.Decimals = blockchainContractInfo.Decimals
			}
			if err = w.db.StoreContractInfo(contractInfo); err != nil {
				glog.Errorf("StoreContractInfo error %v, contract %v", err, cd)
			}
		}
	}
	return contractInfo, validContract, nil
}

func (w *Worker) getEthereumTokensTransfers(transfers bchain.TokenTransfers, addresses map[string]struct{}) []TokenTransfer {
	sort.Sort(transfers)
	tokens := make([]TokenTransfer, len(transfers))
	for i := range transfers {
		t := transfers[i]
		typeName := bchain.EthereumTokenTypeMap[t.Type]
		contractInfo, _, err := w.getContractInfo(t.Contract, typeName)
		if err != nil {
			glog.Errorf("getContractInfo error %v, contract %v", err, t.Contract)
			continue
		}
		var value *Amount
		var values []MultiTokenValue
		if t.Type == bchain.MultiToken {
			values = make([]MultiTokenValue, len(t.MultiTokenValues))
			for j := range values {
				values[j].Id = (*Amount)(&t.MultiTokenValues[j].Id)
				values[j].Value = (*Amount)(&t.MultiTokenValues[j].Value)
			}
		} else {
			value = (*Amount)(&t.Value)
		}
		aggregateAddress(addresses, t.From)
		aggregateAddress(addresses, t.To)
		tokens[i] = TokenTransfer{
			Type:             typeName,
			Contract:         t.Contract,
			From:             t.From,
			To:               t.To,
			Value:            value,
			MultiTokenValues: values,
			Decimals:         contractInfo.Decimals,
			Name:             contractInfo.Name,
			Symbol:           contractInfo.Symbol,
		}
	}
	return tokens
}

func (w *Worker) GetEthereumTokenURI(contract string, id string) (string, *bchain.ContractInfo, error) {
	cd, err := w.chainParser.GetAddrDescFromAddress(contract)
	if err != nil {
		return "", nil, err
	}
	tokenId, ok := new(big.Int).SetString(id, 10)
	if !ok {
		return "", nil, errors.New("Invalid token id")
	}
	uri, err := w.chain.GetTokenURI(cd, tokenId)
	if err != nil {
		return "", nil, err
	}
	ci, _, err := w.getContractDescriptorInfo(cd, bchain.UnknownTokenType)
	if err != nil {
		return "", nil, err
	}
	return uri, ci, nil
}

func (w *Worker) getAddressTxids(addrDesc bchain.AddressDescriptor, mempool bool, filter *AddressFilter, maxResults int) ([]string, error) {
	var err error
	txids := make([]string, 0, 4)
	var callback db.GetTransactionsCallback
	if filter.Vout == AddressFilterVoutOff {
		callback = func(txid string, height uint32, indexes []int32) error {
			txids = append(txids, txid)
			if len(txids) >= maxResults {
				return &db.StopIteration{}
			}
			return nil
		}
	} else {
		callback = func(txid string, height uint32, indexes []int32) error {
			for _, index := range indexes {
				vout := index
				if vout < 0 {
					vout = ^vout
				}
				if (filter.Vout == AddressFilterVoutInputs && index < 0) ||
					(filter.Vout == AddressFilterVoutOutputs && index >= 0) ||
					(vout == int32(filter.Vout)) {
					txids = append(txids, txid)
					if len(txids) >= maxResults {
						return &db.StopIteration{}
					}
					break
				}
			}
			return nil
		}
	}
	if mempool {
		uniqueTxs := make(map[string]struct{})
		o, err := w.mempool.GetAddrDescTransactions(addrDesc)
		if err != nil {
			return nil, err
		}
		for _, m := range o {
			if _, found := uniqueTxs[m.Txid]; !found {
				l := len(txids)
				callback(m.Txid, 0, []int32{m.Vout})
				if len(txids) > l {
					uniqueTxs[m.Txid] = struct{}{}
				}
			}
		}
	} else {
		to := filter.ToHeight
		if to == 0 {
			to = maxUint32
		}
		err = w.db.GetAddrDescTransactions(addrDesc, filter.FromHeight, to, callback)
		if err != nil {
			return nil, err
		}
	}
	return txids, nil
}

func (t *Tx) getAddrVoutValue(addrDesc bchain.AddressDescriptor) *big.Int {
	var val big.Int
	for _, vout := range t.Vout {
		if bytes.Equal(vout.AddrDesc, addrDesc) && vout.ValueSat != nil {
			val.Add(&val, (*big.Int)(vout.ValueSat))
		}
	}
	return &val
}
func (t *Tx) getAddrEthereumTypeMempoolInputValue(addrDesc bchain.AddressDescriptor) *big.Int {
	var val big.Int
	if len(t.Vin) > 0 && len(t.Vout) > 0 && bytes.Equal(t.Vin[0].AddrDesc, addrDesc) {
		val.Add(&val, (*big.Int)(t.Vout[0].ValueSat))
		// add maximum possible fee (the used value is not yet known)
		if t.EthereumSpecific != nil && t.EthereumSpecific.GasLimit != nil && t.EthereumSpecific.GasPrice != nil {
			var fees big.Int
			fees.Mul((*big.Int)(t.EthereumSpecific.GasPrice), t.EthereumSpecific.GasLimit)
			val.Add(&val, &fees)
		}
	}
	return &val
}

func (t *Tx) getAddrVinValue(addrDesc bchain.AddressDescriptor) *big.Int {
	var val big.Int
	for _, vin := range t.Vin {
		if bytes.Equal(vin.AddrDesc, addrDesc) && vin.ValueSat != nil {
			val.Add(&val, (*big.Int)(vin.ValueSat))
		}
	}
	return &val
}

// GetUniqueTxids removes duplicate transactions
func GetUniqueTxids(txids []string) []string {
	ut := make([]string, len(txids))
	txidsMap := make(map[string]struct{})
	i := 0
	for _, txid := range txids {
		_, e := txidsMap[txid]
		if !e {
			ut[i] = txid
			i++
			txidsMap[txid] = struct{}{}
		}
	}
	return ut[0:i]
}

func (w *Worker) txFromTxAddress(txid string, ta *db.TxAddresses, bi *db.BlockInfo, bestheight uint32, addresses map[string]struct{}) *Tx {
	var err error
	var valInSat, valOutSat, feesSat big.Int
	vins := make([]Vin, len(ta.Inputs))
	for i := range ta.Inputs {
		tai := &ta.Inputs[i]
		vin := &vins[i]
		vin.N = i
		vin.ValueSat = (*Amount)(&tai.ValueSat)
		valInSat.Add(&valInSat, &tai.ValueSat)
		vin.Addresses, vin.IsAddress, err = tai.Addresses(w.chainParser)
		if err != nil {
			glog.Errorf("tai.Addresses error %v, tx %v, input %v, tai %+v", err, txid, i, tai)
		}
		if w.db.HasExtendedIndex() {
			vin.Txid = tai.Txid
			vin.Vout = tai.Vout
		}
		aggregateAddresses(addresses, vin.Addresses, vin.IsAddress)
	}
	vouts := make([]Vout, len(ta.Outputs))
	for i := range ta.Outputs {
		tao := &ta.Outputs[i]
		vout := &vouts[i]
		vout.N = i
		vout.ValueSat = (*Amount)(&tao.ValueSat)
		valOutSat.Add(&valOutSat, &tao.ValueSat)
		vout.Addresses, vout.IsAddress, err = tao.Addresses(w.chainParser)
		if err != nil {
			glog.Errorf("tai.Addresses error %v, tx %v, output %v, tao %+v", err, txid, i, tao)
		}
		vout.Spent = tao.Spent
		if vout.Spent && w.db.HasExtendedIndex() {
			vout.SpentTxID = tao.SpentTxid
			vout.SpentIndex = int(tao.SpentIndex)
			vout.SpentHeight = int(tao.SpentHeight)
		}
		aggregateAddresses(addresses, vout.Addresses, vout.IsAddress)
	}
	// for coinbase transactions valIn is 0
	feesSat.Sub(&valInSat, &valOutSat)
	if feesSat.Sign() == -1 {
		feesSat.SetUint64(0)
	}
	r := &Tx{
		Blockhash:     bi.Hash,
		Blockheight:   int(ta.Height),
		Blocktime:     bi.Time,
		Confirmations: bestheight - ta.Height + 1,
		FeesSat:       (*Amount)(&feesSat),
		Txid:          txid,
		ValueInSat:    (*Amount)(&valInSat),
		ValueOutSat:   (*Amount)(&valOutSat),
		Vin:           vins,
		Vout:          vouts,
	}
	if w.chainParser.SupportsVSize() {
		r.VSize = int(ta.VSize)
	} else {
		r.Size = int(ta.VSize)
	}
	return r
}

func computePaging(count, page, itemsOnPage int) (Paging, int, int, int) {
	from := page * itemsOnPage
	totalPages := (count - 1) / itemsOnPage
	if totalPages < 0 {
		totalPages = 0
	}
	if from >= count {
		page = totalPages
	}
	from = page * itemsOnPage
	to := (page + 1) * itemsOnPage
	if to > count {
		to = count
	}
	return Paging{
		ItemsOnPage: itemsOnPage,
		Page:        page + 1,
		TotalPages:  totalPages + 1,
	}, from, to, page
}

func (w *Worker) getEthereumContractBalance(addrDesc bchain.AddressDescriptor, index int, c *db.AddrContract, details AccountDetails, ticker *common.CurrencyRatesTicker, secondaryCoin string) (*Token, error) {
	typeName := bchain.EthereumTokenTypeMap[c.Type]
	ci, validContract, err := w.getContractDescriptorInfo(c.Contract, typeName)
	if err != nil {
		return nil, errors.Annotatef(err, "getEthereumContractBalance %v", c.Contract)
	}
	t := Token{
		Contract:      ci.Contract,
		Name:          ci.Name,
		Symbol:        ci.Symbol,
		Type:          typeName,
		Transfers:     int(c.Txs),
		Decimals:      ci.Decimals,
		ContractIndex: strconv.Itoa(index),
	}
	// return contract balances/values only at or above AccountDetailsTokenBalances
	if details >= AccountDetailsTokenBalances && validContract {
		if c.Type == bchain.FungibleToken {
			// get Erc20 Contract Balance from blockchain, balance obtained from adding and subtracting transfers is not correct
			b, err := w.chain.EthereumTypeGetErc20ContractBalance(addrDesc, c.Contract)
			if err != nil {
				// return nil, nil, nil, errors.Annotatef(err, "EthereumTypeGetErc20ContractBalance %v %v", addrDesc, c.Contract)
				glog.Warningf("EthereumTypeGetErc20ContractBalance addr %v, contract %v, %v", addrDesc, c.Contract, err)
			} else {
				t.BalanceSat = (*Amount)(b)
				if secondaryCoin != "" {
					baseRate, found := w.GetContractBaseRate(ticker, t.Contract, 0)
					if found {
						value, err := strconv.ParseFloat(t.BalanceSat.DecimalString(t.Decimals), 64)
						if err == nil {
							t.BaseValue = value * baseRate
							if ticker != nil {
								secondaryRate, found := ticker.Rates[secondaryCoin]
								if found {
									t.SecondaryValue = t.BaseValue * float64(secondaryRate)
								}
							}
						}
					}
				}
			}
		} else {
			if len(c.Ids) > 0 {
				ids := make([]Amount, len(c.Ids))
				for j := range ids {
					ids[j] = (Amount)(c.Ids[j])
				}
				t.Ids = ids
			}
			if len(c.MultiTokenValues) > 0 {
				idValues := make([]MultiTokenValue, len(c.MultiTokenValues))
				for j := range idValues {
					idValues[j].Id = (*Amount)(&c.MultiTokenValues[j].Id)
					idValues[j].Value = (*Amount)(&c.MultiTokenValues[j].Value)
				}
				t.MultiTokenValues = idValues
			}
		}
	}

	return &t, nil
}

// a fallback method in case internal transactions are not processed and there is no indexed info about contract balance for an address
func (w *Worker) getEthereumContractBalanceFromBlockchain(addrDesc, contract bchain.AddressDescriptor, details AccountDetails) (*Token, error) {
	var b *big.Int
	ci, validContract, err := w.getContractDescriptorInfo(contract, bchain.UnknownTokenType)
	if err != nil {
		return nil, errors.Annotatef(err, "GetContractInfo %v", contract)
	}
	// do not read contract balances etc in case of Basic option
	if details >= AccountDetailsTokenBalances && validContract {
		b, err = w.chain.EthereumTypeGetErc20ContractBalance(addrDesc, contract)
		if err != nil {
			// return nil, nil, nil, errors.Annotatef(err, "EthereumTypeGetErc20ContractBalance %v %v", addrDesc, c.Contract)
			glog.Warningf("EthereumTypeGetErc20ContractBalance addr %v, contract %v, %v", addrDesc, contract, err)
		}
	} else {
		b = nil
	}
	return &Token{
		Type:          ci.Type,
		BalanceSat:    (*Amount)(b),
		Contract:      ci.Contract,
		Name:          ci.Name,
		Symbol:        ci.Symbol,
		Transfers:     0,
		Decimals:      ci.Decimals,
		ContractIndex: "0",
	}, nil
}

// GetContractBaseRate returns contract rate in base coin from the ticker or DB at the timestamp. Zero timestamp means now.
func (w *Worker) GetContractBaseRate(ticker *common.CurrencyRatesTicker, token string, timestamp int64) (float64, bool) {
	if ticker == nil {
		return 0, false
	}
	rate, found := ticker.GetTokenRate(token)
	if !found {
		if timestamp == 0 {
			ticker = w.fiatRates.GetCurrentTicker("", token)
		} else {
			tickers, err := w.fiatRates.GetTickersForTimestamps([]int64{timestamp}, "", token)
			if err != nil || tickers == nil || len(*tickers) == 0 {
				ticker = nil
			} else {
				ticker = (*tickers)[0]
			}
		}
		if ticker == nil {
			return 0, false
		}
		rate, found = ticker.GetTokenRate(token)
	}

	return float64(rate), found
}

type ethereumTypeAddressData struct {
	tokens               Tokens
	contractInfo         *bchain.ContractInfo
	nonce                string
	nonContractTxs       int
	internalTxs          int
	totalResults         int
	tokensBaseValue      float64
	tokensSecondaryValue float64
}

func (w *Worker) getEthereumTypeAddressBalances(addrDesc bchain.AddressDescriptor, details AccountDetails, filter *AddressFilter, secondaryCoin string) (*db.AddrBalance, *ethereumTypeAddressData, error) {
	var ba *db.AddrBalance
	var n uint64
	// unknown number of results for paging initially
	d := ethereumTypeAddressData{totalResults: -1}
	ca, err := w.db.GetAddrDescContracts(addrDesc)
	if err != nil {
		return nil, nil, NewAPIError(fmt.Sprintf("Address not found, %v", err), true)
	}
	b, err := w.chain.EthereumTypeGetBalance(addrDesc)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "EthereumTypeGetBalance %v", addrDesc)
	}
	var filterDesc bchain.AddressDescriptor
	if filter.Contract != "" {
		filterDesc, err = w.chainParser.GetAddrDescFromAddress(filter.Contract)
		if err != nil {
			return nil, nil, NewAPIError(fmt.Sprintf("Invalid contract filter, %v", err), true)
		}
	}
	if ca != nil {
		ba = &db.AddrBalance{
			Txs: uint32(ca.TotalTxs),
		}
		if b != nil {
			ba.BalanceSat = *b
		}
		n, err = w.chain.EthereumTypeGetNonce(addrDesc)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "EthereumTypeGetNonce %v", addrDesc)
		}
		ticker := w.fiatRates.GetCurrentTicker("", "")
		if details > AccountDetailsBasic {
			d.tokens = make([]Token, len(ca.Contracts))
			var j int
			for i := range ca.Contracts {
				c := &ca.Contracts[i]
				if len(filterDesc) > 0 {
					if !bytes.Equal(filterDesc, c.Contract) {
						continue
					}
					// filter only transactions of this contract
					filter.Vout = i + db.ContractIndexOffset
				}
				t, err := w.getEthereumContractBalance(addrDesc, i+db.ContractIndexOffset, c, details, ticker, secondaryCoin)
				if err != nil {
					return nil, nil, err
				}
				d.tokens[j] = *t
				d.tokensBaseValue += t.BaseValue
				d.tokensSecondaryValue += t.SecondaryValue
				j++
			}
			d.tokens = d.tokens[:j]
			sort.Sort(d.tokens)
		}
		d.contractInfo, err = w.db.GetContractInfo(addrDesc, "")
		if err != nil {
			return nil, nil, err
		}
		if filter.FromHeight == 0 && filter.ToHeight == 0 {
			// compute total results for paging
			if filter.Vout == AddressFilterVoutOff {
				d.totalResults = int(ca.TotalTxs)
			} else if filter.Vout == 0 {
				d.totalResults = int(ca.NonContractTxs)
			} else if filter.Vout == db.InternalTxIndexOffset {
				d.totalResults = int(ca.InternalTxs)
			} else if filter.Vout >= db.ContractIndexOffset && filter.Vout-db.ContractIndexOffset < len(ca.Contracts) {
				d.totalResults = int(ca.Contracts[filter.Vout-db.ContractIndexOffset].Txs)
			} else if filter.Vout == AddressFilterVoutQueryNotNecessary {
				d.totalResults = 0
			}
		}
		d.nonContractTxs = int(ca.NonContractTxs)
		d.internalTxs = int(ca.InternalTxs)
	} else {
		// addresses without any normal transactions can have internal transactions that were not processed and therefore balance
		if b != nil {
			ba = &db.AddrBalance{
				BalanceSat: *b,
			}
		}
	}
	// returns 0 for unknown address
	d.nonce = strconv.Itoa(int(n))
	// special handling if filtering for a contract, return the contract details even though the address had no transactions with it
	if len(d.tokens) == 0 && len(filterDesc) > 0 && details >= AccountDetailsTokens {
		t, err := w.getEthereumContractBalanceFromBlockchain(addrDesc, filterDesc, details)
		if err != nil {
			return nil, nil, err
		}
		d.tokens = []Token{*t}
		// switch off query for transactions, there are no transactions
		filter.Vout = AddressFilterVoutQueryNotNecessary
		d.totalResults = -1
	}
	return ba, &d, nil
}

func (w *Worker) txFromTxid(txid string, bestHeight uint32, option AccountDetails, blockInfo *db.BlockInfo, addresses map[string]struct{}) (*Tx, error) {
	var tx *Tx
	var err error
	// only ChainBitcoinType supports TxHistoryLight
	if option == AccountDetailsTxHistoryLight && w.chainType == bchain.ChainBitcoinType {
		ta, err := w.db.GetTxAddresses(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "GetTxAddresses %v", txid)
		}
		if ta == nil {
			glog.Warning("DB inconsistency:  tx ", txid, ": not found in txAddresses")
			// as fallback, get tx from backend
			tx, err = w.getTransaction(txid, false, false, addresses)
			if err != nil {
				return nil, errors.Annotatef(err, "getTransaction %v", txid)
			}
		} else {
			if blockInfo == nil {
				blockInfo, err = w.db.GetBlockInfo(ta.Height)
				if err != nil {
					return nil, errors.Annotatef(err, "GetBlockInfo %v", ta.Height)
				}
				if blockInfo == nil {
					glog.Warning("DB inconsistency:  block height ", ta.Height, ": not found in db")
					// provide empty BlockInfo to return the rest of tx data
					blockInfo = &db.BlockInfo{}
				}
			}
			tx = w.txFromTxAddress(txid, ta, blockInfo, bestHeight, addresses)
		}
	} else {
		tx, err = w.getTransaction(txid, false, false, addresses)
		if err != nil {
			return nil, errors.Annotatef(err, "getTransaction %v", txid)
		}
	}
	return tx, nil
}

func (w *Worker) getAddrDescAndNormalizeAddress(address string) (bchain.AddressDescriptor, string, error) {
	addrDesc, err := w.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		var errAd error
		// try if the address is not address descriptor converted to string
		addrDesc, errAd = bchain.AddressDescriptorFromString(address)
		if errAd != nil {
			return nil, "", NewAPIError(fmt.Sprintf("Invalid address, %v", err), true)
		}
	}
	// convert the address to the format defined by the parser
	addresses, _, err := w.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		glog.V(2).Infof("GetAddressesFromAddrDesc error %v, %v", err, addrDesc)
	}
	if len(addresses) == 1 {
		address = addresses[0]
	}
	return addrDesc, address, nil
}

func isOwnAddress(address string, addresses []string) bool {
	if len(addresses) == 1 {
		return address == addresses[0]
	}
	return false
}

func setIsOwnAddress(tx *Tx, address string) {
	for j := range tx.Vin {
		vin := &tx.Vin[j]
		if isOwnAddress(address, vin.Addresses) {
			vin.IsOwn = true
		}
	}
	for j := range tx.Vout {
		vout := &tx.Vout[j]
		if isOwnAddress(address, vout.Addresses) {
			vout.IsOwn = true
		}
	}
}

// GetAddress computes address value and gets transactions for given address
func (w *Worker) GetAddress(address string, page int, txsOnPage int, option AccountDetails, filter *AddressFilter, secondaryCoin string) (*Address, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	var (
		ba                       *db.AddrBalance
		txm                      []string
		txs                      []*Tx
		txids                    []string
		pg                       Paging
		uBalSat                  big.Int
		totalReceived, totalSent *big.Int
		unconfirmedTxs           int
		totalResults             int
	)
	ed := &ethereumTypeAddressData{}
	addrDesc, address, err := w.getAddrDescAndNormalizeAddress(address)
	if err != nil {
		return nil, err
	}
	if w.chainType == bchain.ChainEthereumType {
		ba, ed, err = w.getEthereumTypeAddressBalances(addrDesc, option, filter, secondaryCoin)
		if err != nil {
			return nil, err
		}
		totalResults = ed.totalResults
	} else {
		// ba can be nil if the address is only in mempool!
		ba, err = w.db.GetAddrDescBalance(addrDesc, db.AddressBalanceDetailNoUTXO)
		if err != nil {
			return nil, NewAPIError(fmt.Sprintf("Address not found, %v", err), true)
		}
		if ba != nil {
			// totalResults is known only if there is no filter
			if filter.Vout == AddressFilterVoutOff && filter.FromHeight == 0 && filter.ToHeight == 0 {
				totalResults = int(ba.Txs)
			} else {
				totalResults = -1
			}
		}
	}
	// if there are only unconfirmed transactions, there is no paging
	if ba == nil {
		ba = &db.AddrBalance{}
		page = 0
	}
	addresses := w.newAddressesMapForAliases()
	// process mempool, only if toHeight is not specified
	if filter.ToHeight == 0 && !filter.OnlyConfirmed {
		txm, err = w.getAddressTxids(addrDesc, true, filter, maxInt)
		if err != nil {
			return nil, errors.Annotatef(err, "getAddressTxids %v true", addrDesc)
		}
		for _, txid := range txm {
			tx, err := w.getTransaction(txid, false, true, addresses)
			// mempool transaction may fail
			if err != nil || tx == nil {
				glog.Warning("GetTransaction in mempool: ", err)
			} else {
				// skip already confirmed txs, mempool may be out of sync
				if tx.Confirmations == 0 {
					unconfirmedTxs++
					uBalSat.Add(&uBalSat, tx.getAddrVoutValue(addrDesc))
					// ethereum has a different logic - value not in input and add maximum possible fees
					if w.chainType == bchain.ChainEthereumType {
						uBalSat.Sub(&uBalSat, tx.getAddrEthereumTypeMempoolInputValue(addrDesc))
					} else {
						uBalSat.Sub(&uBalSat, tx.getAddrVinValue(addrDesc))
					}
					if page == 0 {
						if option == AccountDetailsTxidHistory {
							txids = append(txids, tx.Txid)
						} else if option >= AccountDetailsTxHistoryLight {
							setIsOwnAddress(tx, address)
							txs = append(txs, tx)
						}
					}
				}
			}
		}
	}
	// get tx history if requested by option or check mempool if there are some transactions for a new address
	if option >= AccountDetailsTxidHistory && filter.Vout != AddressFilterVoutQueryNotNecessary {
		txc, err := w.getAddressTxids(addrDesc, false, filter, (page+1)*txsOnPage)
		if err != nil {
			return nil, errors.Annotatef(err, "getAddressTxids %v false", addrDesc)
		}
		bestheight, _, err := w.db.GetBestBlock()
		if err != nil {
			return nil, errors.Annotatef(err, "GetBestBlock")
		}
		var from, to int
		pg, from, to, page = computePaging(len(txc), page, txsOnPage)
		if len(txc) >= txsOnPage {
			if totalResults < 0 {
				pg.TotalPages = -1
			} else {
				pg, _, _, _ = computePaging(totalResults, page, txsOnPage)
			}
		}
		for i := from; i < to; i++ {
			txid := txc[i]
			if option == AccountDetailsTxidHistory {
				txids = append(txids, txid)
			} else {
				tx, err := w.txFromTxid(txid, bestheight, option, nil, addresses)
				if err != nil {
					return nil, err
				}
				setIsOwnAddress(tx, address)
				txs = append(txs, tx)
			}
		}
	}
	if w.chainType == bchain.ChainBitcoinType {
		totalReceived = ba.ReceivedSat()
		totalSent = &ba.SentSat
	}
	var secondaryRate, totalSecondaryValue, totalBaseValue, secondaryValue float64
	if secondaryCoin != "" {
		ticker := w.fiatRates.GetCurrentTicker("", "")
		balance, err := strconv.ParseFloat((*Amount)(&ba.BalanceSat).DecimalString(w.chainParser.AmountDecimals()), 64)
		if ticker != nil && err == nil {
			r, found := ticker.Rates[secondaryCoin]
			if found {
				secondaryRate = float64(r)
			}
		}
		secondaryValue = secondaryRate * balance
		if w.chainType == bchain.ChainEthereumType {
			totalBaseValue += balance + ed.tokensBaseValue
			totalSecondaryValue = secondaryRate * totalBaseValue
		}
	}
	r := &Address{
		Paging:                pg,
		AddrStr:               address,
		BalanceSat:            (*Amount)(&ba.BalanceSat),
		TotalReceivedSat:      (*Amount)(totalReceived),
		TotalSentSat:          (*Amount)(totalSent),
		Txs:                   int(ba.Txs),
		NonTokenTxs:           ed.nonContractTxs,
		InternalTxs:           ed.internalTxs,
		UnconfirmedBalanceSat: (*Amount)(&uBalSat),
		UnconfirmedTxs:        unconfirmedTxs,
		Transactions:          txs,
		Txids:                 txids,
		Tokens:                ed.tokens,
		SecondaryValue:        secondaryValue,
		TokensBaseValue:       ed.tokensBaseValue,
		TokensSecondaryValue:  ed.tokensSecondaryValue,
		TotalBaseValue:        totalBaseValue,
		TotalSecondaryValue:   totalSecondaryValue,
		ContractInfo:          ed.contractInfo,
		Nonce:                 ed.nonce,
		AddressAliases:        w.getAddressAliases(addresses),
	}
	// keep address backward compatible, set deprecated Erc20Contract value if ERC20 token
	if ed.contractInfo != nil && ed.contractInfo.Type == bchain.ERC20TokenType {
		r.Erc20Contract = ed.contractInfo
	}
	glog.Info("GetAddress ", address, ", ", time.Since(start))
	return r, nil
}

func (w *Worker) balanceHistoryHeightsFromTo(fromTimestamp, toTimestamp int64) (uint32, uint32, uint32, uint32) {
	fromUnix := uint32(0)
	toUnix := maxUint32
	fromHeight := uint32(0)
	toHeight := maxUint32
	if fromTimestamp != 0 {
		fromUnix = uint32(fromTimestamp)
		fromHeight = w.is.GetBlockHeightOfTime(fromUnix)
	}
	if toTimestamp != 0 {
		toUnix = uint32(toTimestamp)
		toHeight = w.is.GetBlockHeightOfTime(toUnix)
	}
	return fromUnix, fromHeight, toUnix, toHeight
}

func (w *Worker) balanceHistoryForTxid(addrDesc bchain.AddressDescriptor, txid string, fromUnix, toUnix uint32, selfAddrDesc map[string]struct{}) (*BalanceHistory, error) {
	var time uint32
	var err error
	var ta *db.TxAddresses
	var bchainTx *bchain.Tx
	var height uint32
	if w.chainType == bchain.ChainBitcoinType {
		ta, err = w.db.GetTxAddresses(txid)
		if err != nil {
			return nil, err
		}
		if ta == nil {
			glog.Warning("DB inconsistency:  tx ", txid, ": not found in txAddresses")
			return nil, nil
		}
		height = ta.Height
	} else if w.chainType == bchain.ChainEthereumType {
		var h int
		bchainTx, h, err = w.txCache.GetTransaction(txid)
		if err != nil {
			return nil, err
		}
		if bchainTx == nil {
			glog.Warning("Inconsistency:  tx ", txid, ": not found in the blockchain")
			return nil, nil
		}
		height = uint32(h)
	}
	time = w.is.GetBlockTime(height)
	if time < fromUnix || time >= toUnix {
		return nil, nil
	}
	bh := BalanceHistory{
		Time:          time,
		Txs:           1,
		ReceivedSat:   &Amount{},
		SentSat:       &Amount{},
		SentToSelfSat: &Amount{},
		Txid:          txid,
	}
	countSentToSelf := false
	if w.chainType == bchain.ChainBitcoinType {
		// detect if this input is the first of selfAddrDesc
		// to not to count sentToSelf multiple times if counting multiple xpub addresses
		ownInputIndex := -1
		for i := range ta.Inputs {
			tai := &ta.Inputs[i]
			if _, found := selfAddrDesc[string(tai.AddrDesc)]; found {
				if ownInputIndex < 0 {
					ownInputIndex = i
				}
			}
			if bytes.Equal(addrDesc, tai.AddrDesc) {
				(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &tai.ValueSat)
				if ownInputIndex == i {
					countSentToSelf = true
				}
			}
		}
		for i := range ta.Outputs {
			tao := &ta.Outputs[i]
			if bytes.Equal(addrDesc, tao.AddrDesc) {
				(*big.Int)(bh.ReceivedSat).Add((*big.Int)(bh.ReceivedSat), &tao.ValueSat)
			}
			if countSentToSelf {
				if _, found := selfAddrDesc[string(tao.AddrDesc)]; found {
					(*big.Int)(bh.SentToSelfSat).Add((*big.Int)(bh.SentToSelfSat), &tao.ValueSat)
				}
			}
		}
	} else if w.chainType == bchain.ChainEthereumType {
		var value big.Int
		ethTxData := eth.GetEthereumTxData(bchainTx)
		// add received amount only for OK or unknown status (old) transactions
		if ethTxData.Status == eth.TxStatusOK || ethTxData.Status == eth.TxStatusUnknown {
			if len(bchainTx.Vout) > 0 {
				bchainVout := &bchainTx.Vout[0]
				value = bchainVout.ValueSat
				if len(bchainVout.ScriptPubKey.Addresses) > 0 {
					txAddrDesc, err := w.chainParser.GetAddrDescFromAddress(bchainVout.ScriptPubKey.Addresses[0])
					if err != nil {
						return nil, err
					}
					if bytes.Equal(addrDesc, txAddrDesc) {
						(*big.Int)(bh.ReceivedSat).Add((*big.Int)(bh.ReceivedSat), &value)
					}
					if _, found := selfAddrDesc[string(txAddrDesc)]; found {
						countSentToSelf = true
					}
				}
			}
			// process internal transactions
			if eth.ProcessInternalTransactions {
				internalData, err := w.db.GetEthereumInternalData(txid)
				if err != nil {
					return nil, err
				}
				if internalData != nil {
					for i := range internalData.Transfers {
						f := &internalData.Transfers[i]
						txAddrDesc, err := w.chainParser.GetAddrDescFromAddress(f.From)
						if err != nil {
							return nil, err
						}
						if bytes.Equal(addrDesc, txAddrDesc) {
							(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &f.Value)
							if f.From == f.To {
								(*big.Int)(bh.SentToSelfSat).Add((*big.Int)(bh.SentToSelfSat), &f.Value)
							}
						}
						txAddrDesc, err = w.chainParser.GetAddrDescFromAddress(f.To)
						if err != nil {
							return nil, err
						}
						if bytes.Equal(addrDesc, txAddrDesc) {
							(*big.Int)(bh.ReceivedSat).Add((*big.Int)(bh.ReceivedSat), &f.Value)
						}
					}
				}
			}
		}
		for i := range bchainTx.Vin {
			bchainVin := &bchainTx.Vin[i]
			if len(bchainVin.Addresses) > 0 {
				txAddrDesc, err := w.chainParser.GetAddrDescFromAddress(bchainVin.Addresses[0])
				if err != nil {
					return nil, err
				}
				if bytes.Equal(addrDesc, txAddrDesc) {
					// add received amount only for OK or unknown status (old) transactions, fees always
					if ethTxData.Status == eth.TxStatusOK || ethTxData.Status == eth.TxStatusUnknown {
						(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &value)
						if countSentToSelf {
							if _, found := selfAddrDesc[string(txAddrDesc)]; found {
								(*big.Int)(bh.SentToSelfSat).Add((*big.Int)(bh.SentToSelfSat), &value)
							}
						}
					}
					var feesSat big.Int
					// mempool txs do not have fees yet
					if ethTxData.GasUsed != nil {
						feesSat.Mul(ethTxData.GasPrice, ethTxData.GasUsed)
					}
					(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &feesSat)
				}
			}
		}
	}
	return &bh, nil
}

func (w *Worker) setFiatRateToBalanceHistories(histories BalanceHistories, currencies []string) error {
	for i := range histories {
		bh := &histories[i]
		tickers, err := w.fiatRates.GetTickersForTimestamps([]int64{int64(bh.Time)}, "", "")
		if err != nil || tickers == nil || len(*tickers) == 0 {
			glog.Errorf("Error finding ticker by date %v. Error: %v", bh.Time, err)
			continue
		}
		ticker := (*tickers)[0]
		if ticker == nil {
			continue
		}
		if len(currencies) == 0 {
			bh.FiatRates = ticker.Rates
		} else {
			rates := make(map[string]float32)
			for _, currency := range currencies {
				currency = strings.ToLower(currency)
				if rate, found := ticker.Rates[currency]; found {
					rates[currency] = rate
				} else {
					rates[currency] = -1
				}
			}
			bh.FiatRates = rates
		}
	}
	return nil
}

// GetBalanceHistory returns history of balance for given address
func (w *Worker) GetBalanceHistory(address string, fromTimestamp, toTimestamp int64, currencies []string, groupBy uint32) (BalanceHistories, error) {
	currencies = removeEmpty(currencies)
	bhs := make(BalanceHistories, 0)
	start := time.Now()
	addrDesc, _, err := w.getAddrDescAndNormalizeAddress(address)
	if err != nil {
		return nil, err
	}
	fromUnix, fromHeight, toUnix, toHeight := w.balanceHistoryHeightsFromTo(fromTimestamp, toTimestamp)
	if fromHeight >= toHeight {
		return bhs, nil
	}
	txs, err := w.getAddressTxids(addrDesc, false, &AddressFilter{Vout: AddressFilterVoutOff, FromHeight: fromHeight, ToHeight: toHeight}, maxInt)
	if err != nil {
		return nil, err
	}
	selfAddrDesc := map[string]struct{}{string(addrDesc): {}}
	for txi := len(txs) - 1; txi >= 0; txi-- {
		bh, err := w.balanceHistoryForTxid(addrDesc, txs[txi], fromUnix, toUnix, selfAddrDesc)
		if err != nil {
			return nil, err
		}
		if bh != nil {
			bhs = append(bhs, *bh)
		}
	}
	bha := bhs.SortAndAggregate(groupBy)
	err = w.setFiatRateToBalanceHistories(bha, currencies)
	if err != nil {
		return nil, err
	}
	glog.Info("GetBalanceHistory ", address, ", blocks ", fromHeight, "-", toHeight, ", count ", len(bha), ", ", time.Since(start))
	return bha, nil
}

func (w *Worker) waitForBackendSync() {
	// wait a short time if blockbook is synchronizing with backend
	inSync, _, _, _ := w.is.GetSyncState()
	count := 30
	for !inSync && count > 0 {
		time.Sleep(time.Millisecond * 100)
		count--
		inSync, _, _, _ = w.is.GetSyncState()
	}
}

func (w *Worker) getAddrDescUtxo(addrDesc bchain.AddressDescriptor, ba *db.AddrBalance, onlyConfirmed bool, onlyMempool bool) (Utxos, error) {
	w.waitForBackendSync()
	var err error
	utxos := make(Utxos, 0, 8)
	// store txids from mempool so that they are not added twice in case of import of new block while processing utxos, issue #275
	inMempool := make(map[string]struct{})
	// outputs could be spent in mempool, record and check mempool spends
	spentInMempool := make(map[string]struct{})
	if !onlyConfirmed {
		// get utxo from mempool
		txm, err := w.getAddressTxids(addrDesc, true, &AddressFilter{Vout: AddressFilterVoutOff}, maxInt)
		if err != nil {
			return nil, err
		}
		if len(txm) > 0 {
			mc := make([]*bchain.Tx, len(txm))
			for i, txid := range txm {
				// get mempool txs and process their inputs to detect spends between mempool txs
				bchainTx, _, err := w.txCache.GetTransaction(txid)
				// mempool transaction may fail
				if err != nil {
					glog.Error("GetTransaction in mempool ", txid, ": ", err)
				} else {
					mc[i] = bchainTx
					// get outputs spent by the mempool tx
					for i := range bchainTx.Vin {
						vin := &bchainTx.Vin[i]
						spentInMempool[vin.Txid+strconv.Itoa(int(vin.Vout))] = struct{}{}
					}
				}
			}
			for _, bchainTx := range mc {
				if bchainTx != nil {
					for i := range bchainTx.Vout {
						vout := &bchainTx.Vout[i]
						vad, err := w.chainParser.GetAddrDescFromVout(vout)
						if err == nil && bytes.Equal(addrDesc, vad) {
							// report only outpoints that are not spent in mempool
							_, e := spentInMempool[bchainTx.Txid+strconv.Itoa(i)]
							if !e {
								coinbase := false
								if len(bchainTx.Vin) == 1 && len(bchainTx.Vin[0].Coinbase) > 0 {
									coinbase = true
								}
								utxos = append(utxos, Utxo{
									Txid:      bchainTx.Txid,
									Vout:      int32(i),
									AmountSat: (*Amount)(&vout.ValueSat),
									Locktime:  bchainTx.LockTime,
									Coinbase:  coinbase,
								})
								inMempool[bchainTx.Txid] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
	if !onlyMempool {
		// get utxo from index
		if ba == nil {
			ba, err = w.db.GetAddrDescBalance(addrDesc, db.AddressBalanceDetailUTXO)
			if err != nil {
				return nil, NewAPIError(fmt.Sprintf("Address not found, %v", err), true)
			}
		}
		// ba can be nil if the address is only in mempool!
		if ba != nil && len(ba.Utxos) > 0 {
			b, _, err := w.db.GetBestBlock()
			if err != nil {
				return nil, err
			}
			bestheight := int(b)
			var checksum big.Int
			checksum.Set(&ba.BalanceSat)
			// go backwards to get the newest first
			for i := len(ba.Utxos) - 1; i >= 0; i-- {
				utxo := &ba.Utxos[i]
				txid, err := w.chainParser.UnpackTxid(utxo.BtxID)
				if err != nil {
					return nil, err
				}
				_, e := spentInMempool[txid+strconv.Itoa(int(utxo.Vout))]
				if !e {
					confirmations := bestheight - int(utxo.Height) + 1
					coinbase := false
					// for performance reasons, check coinbase transactions only in minimum confirmantion range
					if confirmations < w.chainParser.MinimumCoinbaseConfirmations() {
						ta, err := w.db.GetTxAddresses(txid)
						if err != nil {
							return nil, err
						}
						if len(ta.Inputs) == 1 && len(ta.Inputs[0].AddrDesc) == 0 && IsZeroBigInt(&ta.Inputs[0].ValueSat) {
							coinbase = true
						}
					}
					_, e = inMempool[txid]
					if !e {
						utxos = append(utxos, Utxo{
							Txid:          txid,
							Vout:          utxo.Vout,
							AmountSat:     (*Amount)(&utxo.ValueSat),
							Height:        int(utxo.Height),
							Confirmations: confirmations,
							Coinbase:      coinbase,
						})
					}
				}
				checksum.Sub(&checksum, &utxo.ValueSat)
			}
			if checksum.Uint64() != 0 {
				glog.Warning("DB inconsistency:  ", addrDesc, ": checksum is not zero, checksum=", checksum.Int64())
			}
		}
	}
	return utxos, nil
}

// GetAddressUtxo returns unspent outputs for given address
func (w *Worker) GetAddressUtxo(address string, onlyConfirmed bool) (Utxos, error) {
	if w.chainType != bchain.ChainBitcoinType {
		return nil, NewAPIError("Not supported", true)
	}
	start := time.Now()
	addrDesc, err := w.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Invalid address '%v', %v", address, err), true)
	}
	r, err := w.getAddrDescUtxo(addrDesc, nil, onlyConfirmed, false)
	if err != nil {
		return nil, err
	}
	glog.Info("GetAddressUtxo ", address, ", ", len(r), " utxos, ", time.Since(start))
	return r, nil
}

// GetBlocks returns BlockInfo for blocks on given page
func (w *Worker) GetBlocks(page int, blocksOnPage int) (*Blocks, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	b, _, err := w.db.GetBestBlock()
	bestheight := int(b)
	if err != nil {
		return nil, errors.Annotatef(err, "GetBestBlock")
	}
	pg, from, to, page := computePaging(bestheight+1, page, blocksOnPage)
	r := &Blocks{Paging: pg}
	r.Blocks = make([]db.BlockInfo, to-from)
	for i := from; i < to; i++ {
		bi, err := w.db.GetBlockInfo(uint32(bestheight - i))
		if err != nil {
			return nil, err
		}
		if bi == nil {
			r.Blocks = r.Blocks[:i]
			break
		}
		r.Blocks[i-from] = *bi
	}
	glog.Info("GetBlocks page ", page, ", ", time.Since(start))
	return r, nil
}

// removeEmpty removes empty strings from a slice
func removeEmpty(stringSlice []string) []string {
	var ret []string
	for _, str := range stringSlice {
		if str != "" {
			ret = append(ret, str)
		}
	}
	return ret
}

// getFiatRatesResult checks if CurrencyRatesTicker contains all necessary data and returns formatted result
func (w *Worker) getFiatRatesResult(currencies []string, ticker *common.CurrencyRatesTicker, token string) (*FiatTicker, error) {
	if token != "" {
		rates := make(map[string]float32)
		if len(currencies) == 0 {
			for currency := range ticker.Rates {
				currency = strings.ToLower(currency)
				rate := ticker.TokenRateInCurrency(token, currency)
				if rate <= 0 {
					rate = -1
				}
				rates[currency] = rate
			}
		} else {
			for _, currency := range currencies {
				currency = strings.ToLower(currency)
				rate := ticker.TokenRateInCurrency(token, currency)
				if rate <= 0 {
					rate = -1
				}
				rates[currency] = rate
			}
		}
		return &FiatTicker{
			Timestamp: ticker.Timestamp.UTC().Unix(),
			Rates:     rates,
		}, nil
	}
	if len(currencies) == 0 {
		// Return all available ticker rates
		return &FiatTicker{
			Timestamp: ticker.Timestamp.UTC().Unix(),
			Rates:     ticker.Rates,
		}, nil
	}
	// Check if currencies from the list are available in the ticker rates
	rates := make(map[string]float32)
	for _, currency := range currencies {
		currency = strings.ToLower(currency)
		if rate, found := ticker.Rates[currency]; found {
			rates[currency] = rate
		} else {
			rates[currency] = -1
		}
	}
	return &FiatTicker{
		Timestamp: ticker.Timestamp.UTC().Unix(),
		Rates:     rates,
	}, nil
}

// GetCurrentFiatRates returns last available fiat rates
func (w *Worker) GetCurrentFiatRates(currencies []string, token string) (*FiatTicker, error) {
	vsCurrency := ""
	currencies = removeEmpty(currencies)
	if len(currencies) == 1 {
		vsCurrency = currencies[0]
	}
	ticker := w.fiatRates.GetCurrentTicker(vsCurrency, token)
	var err error
	if ticker == nil {
		ticker, err = w.db.FiatRatesFindLastTicker(vsCurrency, token)
		if err != nil {
			return nil, NewAPIError(fmt.Sprintf("Error finding ticker: %v", err), false)
		} else if ticker == nil {
			return nil, NewAPIError("No tickers found!", true)
		}
	}
	result, err := w.getFiatRatesResult(currencies, ticker, token)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// makeErrorRates returns a map of currencies, with each value equal to -1
// used when there was an error finding ticker
func makeErrorRates(currencies []string) map[string]float32 {
	rates := make(map[string]float32)
	for _, currency := range currencies {
		rates[strings.ToLower(currency)] = -1
	}
	return rates
}

// GetFiatRatesForTimestamps returns fiat rates for each of the provided dates
func (w *Worker) GetFiatRatesForTimestamps(timestamps []int64, currencies []string, token string) (*FiatTickers, error) {
	if len(timestamps) == 0 {
		return nil, NewAPIError("No timestamps provided", true)
	}
	vsCurrency := ""
	currencies = removeEmpty(currencies)
	if len(currencies) == 1 {
		vsCurrency = currencies[0]
	}
	tickers, err := w.fiatRates.GetTickersForTimestamps(timestamps, vsCurrency, token)
	if err != nil {
		return nil, err
	}
	if tickers == nil {
		return nil, NewAPIError("No tickers found", true)
	}
	if len(*tickers) != len(timestamps) {
		glog.Error("GetFiatRatesForTimestamps: number of tickers does not match timestamps ", len(*tickers), ", ", len(timestamps))
		return nil, NewAPIError("No tickers found", false)
	}
	fiatTickers := make([]FiatTicker, len(*tickers))
	for i, t := range *tickers {
		if t == nil {
			fiatTickers[i] = FiatTicker{Timestamp: timestamps[i], Rates: makeErrorRates(currencies)}
			continue
		}
		result, err := w.getFiatRatesResult(currencies, t, token)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				if apiErr.Public {
					return nil, err
				}
			}
			fiatTickers[i] = FiatTicker{Timestamp: timestamps[i], Rates: makeErrorRates(currencies)}
			continue
		}
		fiatTickers[i] = *result
	}
	return &FiatTickers{Tickers: fiatTickers}, nil
}

// GetFiatRatesForBlockID returns fiat rates for block height or block hash
func (w *Worker) GetFiatRatesForBlockID(blockID string, currencies []string, token string) (*FiatTicker, error) {
	bi, err := w.getBlockInfoFromBlockID(blockID)
	if err != nil {
		if err == bchain.ErrBlockNotFound {
			return nil, NewAPIError(fmt.Sprintf("Block %v not found", blockID), true)
		}
		return nil, NewAPIError(fmt.Sprintf("Block %v not found, error: %v", blockID, err), false)
	}
	tickers, err := w.GetFiatRatesForTimestamps([]int64{bi.Time}, currencies, token)
	if err != nil || tickers == nil || len(tickers.Tickers) == 0 {
		return nil, err
	}
	return &tickers.Tickers[0], nil
}

// GetAvailableVsCurrencies returns the list of available versus currencies for exchange rates
func (w *Worker) GetAvailableVsCurrencies(timestamp int64, token string) (*AvailableVsCurrencies, error) {
	tickers, err := w.fiatRates.GetTickersForTimestamps([]int64{timestamp}, "", token)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Error finding ticker: %v", err), false)
	}
	if tickers == nil || len(*tickers) == 0 {
		return nil, NewAPIError("No tickers found", true)
	}
	ticker := (*tickers)[0]
	keys := make([]string, 0, len(ticker.Rates))
	for k := range ticker.Rates {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort to get deterministic results

	return &AvailableVsCurrencies{
		Timestamp: ticker.Timestamp.Unix(),
		Tickers:   keys,
	}, nil
}

// getBlockHashBlockID returns block hash from block height or block hash
func (w *Worker) getBlockHashBlockID(bid string) string {
	// try to decide if passed string (bid) is block height or block hash
	// if it's a number, must be less than int32
	var hash string
	height, err := strconv.Atoi(bid)
	if err == nil && height < int(maxUint32) {
		hash, err = w.db.GetBlockHash(uint32(height))
		if err != nil {
			hash = bid
		}
	} else {
		hash = bid
	}
	return hash
}

// getBlockInfoFromBlockID returns block info from block height or block hash
func (w *Worker) getBlockInfoFromBlockID(bid string) (*bchain.BlockInfo, error) {
	hash := w.getBlockHashBlockID(bid)
	if hash == "" {
		return nil, NewAPIError("Block not found", true)
	}
	bi, err := w.chain.GetBlockInfo(hash)
	return bi, err
}

// GetFeeStats returns statistics about block fees
func (w *Worker) GetFeeStats(bid string) (*FeeStats, error) {
	// txSpecific extends Tx with an additional Size and Vsize info
	type txSpecific struct {
		*bchain.Tx
		Vsize int `json:"vsize,omitempty"`
		Size  int `json:"size,omitempty"`
	}

	start := time.Now()
	bi, err := w.getBlockInfoFromBlockID(bid)
	if err != nil {
		if err == bchain.ErrBlockNotFound {
			return nil, NewAPIError("Block not found", true)
		}
		return nil, NewAPIError(fmt.Sprintf("Block not found, %v", err), true)
	}

	feesPerKb := make([]int64, 0, len(bi.Txids))
	totalFeesSat := big.NewInt(0)
	averageFeePerKb := int64(0)

	for _, txid := range bi.Txids {
		// Get a raw JSON with transaction details, including size, vsize, hex
		txSpecificJSON, err := w.chain.GetTransactionSpecific(&bchain.Tx{Txid: txid})
		if err != nil {
			return nil, errors.Annotatef(err, "GetTransactionSpecific")
		}

		// Serialize the raw JSON into TxSpecific struct
		var txSpec txSpecific
		err = json.Unmarshal(txSpecificJSON, &txSpec)
		if err != nil {
			return nil, errors.Annotatef(err, "Unmarshal")
		}

		// Calculate the TX size in bytes
		txSize := 0
		if txSpec.Vsize > 0 {
			txSize = txSpec.Vsize
		} else if txSpec.Size > 0 {
			txSize = txSpec.Size
		} else if txSpec.Hex != "" {
			txSize = len(txSpec.Hex) / 2
		} else {
			errMsg := "Cannot determine the transaction size from neither Vsize, Size nor Hex! Txid: " + txid
			return nil, NewAPIError(errMsg, true)
		}

		// Get values of TX inputs and outputs
		txAddresses, err := w.db.GetTxAddresses(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "GetTxAddresses")
		}

		// Calculate total fees in Satoshis
		feeSat := big.NewInt(0)
		for _, input := range txAddresses.Inputs {
			feeSat = feeSat.Add(&input.ValueSat, feeSat)
		}

		// Zero inputs means it's a Coinbase TX - skip it
		if feeSat.Cmp(big.NewInt(0)) == 0 {
			continue
		}

		for _, output := range txAddresses.Outputs {
			feeSat = feeSat.Sub(feeSat, &output.ValueSat)
		}
		totalFeesSat.Add(totalFeesSat, feeSat)

		// Convert feeSat to fee per kilobyte and add to an array for decile calculation
		feePerKb := int64(float64(feeSat.Int64()) / float64(txSize) * 1000)
		averageFeePerKb += feePerKb
		feesPerKb = append(feesPerKb, feePerKb)
	}

	var deciles [11]int64
	n := len(feesPerKb)

	if n > 0 {
		averageFeePerKb /= int64(n)

		// Sort fees and calculate the deciles
		sort.Slice(feesPerKb, func(i, j int) bool { return feesPerKb[i] < feesPerKb[j] })
		for k := 0; k <= 10; k++ {
			index := int(math.Floor(0.5+float64(k)*float64(n+1)/10)) - 1
			if index < 0 {
				index = 0
			} else if index >= n {
				index = n - 1
			}
			deciles[k] = feesPerKb[index]
		}
	}

	glog.Info("GetFeeStats ", bid, " (", len(feesPerKb), " txs), ", time.Since(start))

	return &FeeStats{
		TxCount:         len(feesPerKb),
		AverageFeePerKb: averageFeePerKb,
		TotalFeesSat:    (*Amount)(totalFeesSat),
		DecilesFeePerKb: deciles,
	}, nil
}

// GetBlock returns paged data about block
func (w *Worker) GetBlock(bid string, page int, txsOnPage int) (*Block, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	bi, err := w.getBlockInfoFromBlockID(bid)
	if err != nil {
		if err == bchain.ErrBlockNotFound {
			return nil, NewAPIError("Block not found", true)
		}
		return nil, NewAPIError(fmt.Sprintf("Block not found, %v", err), true)
	}
	dbi := &db.BlockInfo{
		Hash:   bi.Hash,
		Height: bi.Height,
		Time:   bi.Time,
	}
	txCount := len(bi.Txids)
	bestheight, _, err := w.db.GetBestBlock()
	if err != nil {
		return nil, errors.Annotatef(err, "GetBestBlock")
	}
	pg, from, to, page := computePaging(txCount, page, txsOnPage)
	txs := make([]*Tx, to-from)
	txi := 0
	addresses := w.newAddressesMapForAliases()
	for i := from; i < to; i++ {
		txs[txi], err = w.txFromTxid(bi.Txids[i], bestheight, AccountDetailsTxHistoryLight, dbi, addresses)
		if err != nil {
			return nil, err
		}
		txi++
	}
	if bi.Prev == "" && bi.Height != 0 {
		bi.Prev, _ = w.db.GetBlockHash(bi.Height - 1)
	}
	if bi.Next == "" && bi.Height != bestheight {
		bi.Next, _ = w.db.GetBlockHash(bi.Height + 1)
	}
	txs = txs[:txi]
	bi.Txids = nil
	glog.Info("GetBlock ", bid, ", page ", page, ", ", time.Since(start))
	return &Block{
		Paging: pg,
		BlockInfo: BlockInfo{
			Hash:          bi.Hash,
			Prev:          bi.Prev,
			Next:          bi.Next,
			Height:        bi.Height,
			Confirmations: bi.Confirmations,
			Size:          bi.Size,
			Time:          bi.Time,
			Bits:          bi.Bits,
			Difficulty:    string(bi.Difficulty),
			MerkleRoot:    bi.MerkleRoot,
			Nonce:         string(bi.Nonce),
			Txids:         bi.Txids,
			Version:       bi.Version,
		},
		TxCount:        txCount,
		Transactions:   txs,
		AddressAliases: w.getAddressAliases(addresses),
	}, nil
}

// GetBlock returns paged data about block
func (w *Worker) GetBlockRaw(bid string) (*BlockRaw, error) {
	hash := w.getBlockHashBlockID(bid)
	if hash == "" {
		return nil, NewAPIError("Block not found", true)
	}
	hex, err := w.chain.GetBlockRaw(hash)
	if err != nil {
		if err == bchain.ErrBlockNotFound {
			return nil, NewAPIError("Block not found", true)
		}
		return nil, err
	}
	return &BlockRaw{Hex: hex}, err
}

// ComputeFeeStats computes fee distribution in defined blocks and logs them to log
func (w *Worker) ComputeFeeStats(blockFrom, blockTo int, stopCompute chan os.Signal) error {
	bestheight, _, err := w.db.GetBestBlock()
	if err != nil {
		return errors.Annotatef(err, "GetBestBlock")
	}
	for block := blockFrom; block <= blockTo; block++ {
		hash, err := w.db.GetBlockHash(uint32(block))
		if err != nil {
			return err
		}
		bi, err := w.chain.GetBlockInfo(hash)
		if err != nil {
			return err
		}
		// process only blocks with enough transactions
		if len(bi.Txids) > 20 {
			dbi := &db.BlockInfo{
				Hash:   bi.Hash,
				Height: bi.Height,
				Time:   bi.Time,
			}
			txids := bi.Txids
			if w.chainType == bchain.ChainBitcoinType {
				// skip the coinbase transaction
				txids = txids[1:]
			}
			fees := make([]int64, len(txids))
			sum := int64(0)
			for i, txid := range txids {
				select {
				case <-stopCompute:
					glog.Info("ComputeFeeStats interrupted at height ", block)
					return db.ErrOperationInterrupted
				default:
					tx, err := w.txFromTxid(txid, bestheight, AccountDetailsTxHistoryLight, dbi, nil)
					if err != nil {
						return err
					}
					fee := tx.FeesSat.AsInt64()
					fees[i] = fee
					sum += fee
				}
			}
			sort.Slice(fees, func(i, j int) bool { return fees[i] < fees[j] })
			step := float64(len(fees)) / 10
			percentils := ""
			for i := float64(0); i < float64(len(fees)+1); i += step {
				ii := int(math.Round(i))
				if ii >= len(fees) {
					ii = len(fees) - 1
				}
				percentils += "," + strconv.FormatInt(fees[ii], 10)
			}
			glog.Info(block, ",", time.Unix(bi.Time, 0).Format(time.RFC3339), ",", len(bi.Txids), ",", sum, ",", float64(sum)/float64(len(bi.Txids)), percentils)
		}
	}
	return nil
}

func nonZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// GetSystemInfo returns information about system
func (w *Worker) GetSystemInfo(internal bool) (*SystemInfo, error) {
	start := time.Now().UTC()
	vi := common.GetVersionInfo()
	inSync, bestHeight, lastBlockTime, startSync := w.is.GetSyncState()
	if !inSync && !w.is.InitialSync {
		// if less than 5 seconds into syncing, return inSync=true to avoid short time not in sync reports that confuse monitoring
		if startSync.Add(5 * time.Second).After(start) {
			inSync = true
		}
	}
	inSyncMempool, lastMempoolTime, mempoolSize := w.is.GetMempoolSyncState()
	ci, err := w.chain.GetChainInfo()
	var backendError string
	if err != nil {
		glog.Error("GetChainInfo error ", err)
		backendError = errors.Annotatef(err, "GetChainInfo").Error()
		ci = &bchain.ChainInfo{}
		// set not in sync in case of backend error
		inSync = false
		inSyncMempool = false
	}
	var columnStats []common.InternalStateColumn
	var internalDBSize int64
	if internal {
		columnStats = w.is.GetAllDBColumnStats()
		internalDBSize = w.is.DBSizeTotal()
	}
	var currentFiatRatesTime time.Time
	ct := w.fiatRates.GetCurrentTicker("", "")
	if ct != nil {
		currentFiatRatesTime = ct.Timestamp
	}
	blockbookInfo := &BlockbookInfo{
		Coin:                         w.is.Coin,
		Host:                         w.is.Host,
		Version:                      vi.Version,
		GitCommit:                    vi.GitCommit,
		BuildTime:                    vi.BuildTime,
		SyncMode:                     w.is.SyncMode,
		InitialSync:                  w.is.InitialSync,
		InSync:                       inSync,
		BestHeight:                   bestHeight,
		LastBlockTime:                lastBlockTime,
		InSyncMempool:                inSyncMempool,
		LastMempoolTime:              lastMempoolTime,
		MempoolSize:                  mempoolSize,
		Decimals:                     w.chainParser.AmountDecimals(),
		HasFiatRates:                 w.is.HasFiatRates,
		HasTokenFiatRates:            w.is.HasTokenFiatRates,
		CurrentFiatRatesTime:         nonZeroTime(currentFiatRatesTime),
		HistoricalFiatRatesTime:      nonZeroTime(w.is.HistoricalFiatRatesTime),
		HistoricalTokenFiatRatesTime: nonZeroTime(w.is.HistoricalTokenFiatRatesTime),
		DbSize:                       w.db.DatabaseSizeOnDisk(),
		DbSizeFromColumns:            internalDBSize,
		DbColumns:                    columnStats,
		About:                        Text.BlockbookAbout,
	}
	backendInfo := &common.BackendInfo{
		BackendError:     backendError,
		BestBlockHash:    ci.Bestblockhash,
		Blocks:           ci.Blocks,
		Chain:            ci.Chain,
		Difficulty:       ci.Difficulty,
		Headers:          ci.Headers,
		ProtocolVersion:  ci.ProtocolVersion,
		SizeOnDisk:       ci.SizeOnDisk,
		Subversion:       ci.Subversion,
		Timeoffset:       ci.Timeoffset,
		Version:          ci.Version,
		Warnings:         ci.Warnings,
		ConsensusVersion: ci.ConsensusVersion,
		Consensus:        ci.Consensus,
	}
	w.is.SetBackendInfo(backendInfo)
	glog.Info("GetSystemInfo, ", time.Since(start))
	return &SystemInfo{blockbookInfo, backendInfo}, nil
}

// GetMempool returns a page of mempool txids
func (w *Worker) GetMempool(page int, itemsOnPage int) (*MempoolTxids, error) {
	page--
	if page < 0 {
		page = 0
	}
	entries := w.mempool.GetAllEntries()
	pg, from, to, _ := computePaging(len(entries), page, itemsOnPage)
	r := &MempoolTxids{
		Paging:      pg,
		MempoolSize: len(entries),
	}
	r.Mempool = make([]MempoolTxid, to-from)
	for i := from; i < to; i++ {
		entry := &entries[i]
		r.Mempool[i-from] = MempoolTxid{
			Txid: entry.Txid,
			Time: int64(entry.Time),
		}
	}
	return r, nil
}

type bitcoinTypeEstimatedFee struct {
	timestamp int64
	fee       big.Int
	lock      sync.Mutex
}

const estimatedFeeCacheSize = 300

var estimatedFeeCache [estimatedFeeCacheSize]bitcoinTypeEstimatedFee
var estimatedFeeConservativeCache [estimatedFeeCacheSize]bitcoinTypeEstimatedFee

func (w *Worker) cachedEstimateFee(blocks int, conservative bool) (big.Int, error) {
	var s *bitcoinTypeEstimatedFee
	if conservative {
		s = &estimatedFeeConservativeCache[blocks]
	} else {
		s = &estimatedFeeCache[blocks]
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	// 10 seconds cache
	threshold := time.Now().Unix() - 10
	if s.timestamp >= threshold {
		return s.fee, nil
	}
	fee, err := w.chain.EstimateSmartFee(blocks, conservative)
	if err == nil {
		s.timestamp = time.Now().Unix()
		s.fee = fee
		// store metrics for the first 32 block estimates
		if blocks < 33 {
			w.metrics.EstimatedFee.With(common.Labels{
				"blocks":       strconv.Itoa(blocks),
				"conservative": strconv.FormatBool(conservative),
			}).Set(float64(fee.Int64()))
		}
	}
	return fee, err
}

// EstimateFee returns a fee estimation for given number of blocks
// it uses 10 second cache to reduce calls to the backend
func (w *Worker) EstimateFee(blocks int, conservative bool) (big.Int, error) {
	if blocks >= estimatedFeeCacheSize {
		return w.chain.EstimateSmartFee(blocks, conservative)
	}
	return w.cachedEstimateFee(blocks, conservative)
}
