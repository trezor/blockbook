package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// Worker is handle to api worker
type Worker struct {
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	is          *common.InternalState
}

// NewWorker creates new api worker
func NewWorker(db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, is *common.InternalState) (*Worker, error) {
	w := &Worker{
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		is:          is,
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
	err := w.db.GetAddrDescTransactions(vout.ScriptPubKey.AddrDesc, height, ^uint32(0), func(t string, index uint32, isOutput bool) error {
		if isOutput == false {
			tsp, err := w.db.GetTxAddresses(t)
			if err != nil {
				return err
			} else if tsp == nil {
				glog.Warning("DB inconsistency:  tx ", t, ": not found in txAddresses")
			} else if len(tsp.Inputs) > int(index) {
				if tsp.Inputs[index].ValueSat.Cmp(&vout.ValueSat) == 0 {
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
		return nil
	})
	return err
}

// GetSpendingTxid returns transaction id of transaction that spent given output
func (w *Worker) GetSpendingTxid(txid string, n int) (string, error) {
	start := time.Now()
	tx, err := w.GetTransaction(txid, false)
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
	glog.Info("GetSpendingTxid ", txid, " ", n, " finished in ", time.Since(start))
	return tx.Vout[n].SpentTxID, nil
}

// GetTransaction reads transaction data from txid
func (w *Worker) GetTransaction(txid string, spendingTxs bool) (*Tx, error) {
	start := time.Now()
	bchainTx, height, err := w.txCache.GetTransaction(txid)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Tx not found, %v", err), true)
	}
	var ta *db.TxAddresses
	var blockhash string
	if bchainTx.Confirmations > 0 {
		ta, err = w.db.GetTxAddresses(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "GetTxAddresses %v", txid)
		}
		blockhash, err = w.db.GetBlockHash(height)
		if err != nil {
			return nil, errors.Annotatef(err, "GetBlockHash %v", height)
		}
	}
	var valInSat, valOutSat, feesSat big.Int
	vins := make([]Vin, len(bchainTx.Vin))
	for i := range bchainTx.Vin {
		bchainVin := &bchainTx.Vin[i]
		vin := &vins[i]
		vin.Txid = bchainVin.Txid
		vin.N = i
		vin.Vout = bchainVin.Vout
		vin.Sequence = int64(bchainVin.Sequence)
		vin.ScriptSig.Hex = bchainVin.ScriptSig.Hex
		//  bchainVin.Txid=="" is coinbase transaction
		if bchainVin.Txid != "" {
			// load spending addresses from TxAddresses
			tas, err := w.db.GetTxAddresses(bchainVin.Txid)
			if err != nil {
				return nil, errors.Annotatef(err, "GetTxAddresses %v", bchainVin.Txid)
			}
			if tas == nil {
				// mempool transactions are not in TxAddresses but confirmed should be there, log a problem
				if bchainTx.Confirmations > 0 {
					glog.Warning("DB inconsistency:  tx ", bchainVin.Txid, ": not found in txAddresses")
				}
				// try to load from backend
				otx, _, err := w.txCache.GetTransaction(bchainVin.Txid)
				if err != nil {
					return nil, errors.Annotatef(err, "txCache.GetTransaction %v", bchainVin.Txid)
				}
				if len(otx.Vout) > int(vin.Vout) {
					vout := &otx.Vout[vin.Vout]
					vin.ValueSat = vout.ValueSat
					vin.AddrDesc, vin.Addresses, vin.Searchable, err = w.getAddressesFromVout(vout)
					if err != nil {
						glog.Errorf("getAddressesFromVout error %v, vout %+v", err, vout)
					}
				}
			} else {
				if len(tas.Outputs) > int(vin.Vout) {
					output := &tas.Outputs[vin.Vout]
					vin.ValueSat = output.ValueSat
					vin.Value = w.chainParser.AmountToDecimalString(&vin.ValueSat)
					vin.AddrDesc = output.AddrDesc
					vin.Addresses, vin.Searchable, err = output.Addresses(w.chainParser)
					if err != nil {
						glog.Errorf("output.Addresses error %v, tx %v, output %v", err, bchainVin.Txid, i)
					}
				}
			}
			vin.Value = w.chainParser.AmountToDecimalString(&vin.ValueSat)
			valInSat.Add(&valInSat, &vin.ValueSat)
		}
	}
	vouts := make([]Vout, len(bchainTx.Vout))
	for i := range bchainTx.Vout {
		bchainVout := &bchainTx.Vout[i]
		vout := &vouts[i]
		vout.N = i
		vout.ValueSat = bchainVout.ValueSat
		vout.Value = w.chainParser.AmountToDecimalString(&bchainVout.ValueSat)
		valOutSat.Add(&valOutSat, &bchainVout.ValueSat)
		vout.ScriptPubKey.Hex = bchainVout.ScriptPubKey.Hex
		vout.ScriptPubKey.AddrDesc, vout.ScriptPubKey.Addresses, vout.ScriptPubKey.Searchable, err = w.getAddressesFromVout(bchainVout)
		if err != nil {
			glog.V(2).Infof("getAddressesFromVout error %v, %v, output %v", err, bchainTx.Txid, bchainVout.N)
		}
		if ta != nil {
			vout.Spent = ta.Outputs[i].Spent
			if spendingTxs && vout.Spent {
				err = w.setSpendingTxToVout(vout, bchainTx.Txid, height)
				if err != nil {
					glog.Errorf("setSpendingTxToVout error %v, %v, output %v", err, vout.ScriptPubKey.AddrDesc, vout.N)
				}
			}
		}
	}
	// for coinbase transactions valIn is 0
	feesSat.Sub(&valInSat, &valOutSat)
	if feesSat.Sign() == -1 {
		feesSat.SetUint64(0)
	}
	// for now do not return size, we would have to compute vsize of segwit transactions
	// size:=len(bchainTx.Hex) / 2
	r := &Tx{
		Blockhash:     blockhash,
		Blockheight:   int(height),
		Blocktime:     bchainTx.Blocktime,
		Confirmations: bchainTx.Confirmations,
		Fees:          w.chainParser.AmountToDecimalString(&feesSat),
		FeesSat:       feesSat,
		Locktime:      bchainTx.LockTime,
		Time:          bchainTx.Time,
		Txid:          bchainTx.Txid,
		ValueIn:       w.chainParser.AmountToDecimalString(&valInSat),
		ValueInSat:    valInSat,
		ValueOut:      w.chainParser.AmountToDecimalString(&valOutSat),
		ValueOutSat:   valOutSat,
		Version:       bchainTx.Version,
		Hex:           bchainTx.Hex,
		Vin:           vins,
		Vout:          vouts,
	}
	if spendingTxs {
		glog.Info("GetTransaction ", txid, " finished in ", time.Since(start))
	}
	return r, nil
}

func (w *Worker) getAddressTxids(addrDesc bchain.AddressDescriptor, mempool bool) ([]string, error) {
	var err error
	txids := make([]string, 0, 4)
	if !mempool {
		err = w.db.GetAddrDescTransactions(addrDesc, 0, ^uint32(0), func(txid string, vout uint32, isOutput bool) error {
			txids = append(txids, txid)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		m, err := w.chain.GetMempoolTransactionsForAddrDesc(addrDesc)
		if err != nil {
			return nil, err
		}
		txids = append(txids, m...)
	}
	return txids, nil
}

func (t *Tx) getAddrVoutValue(addrDesc bchain.AddressDescriptor) *big.Int {
	var val big.Int
	for _, vout := range t.Vout {
		if bytes.Equal(vout.ScriptPubKey.AddrDesc, addrDesc) {
			val.Add(&val, &vout.ValueSat)
		}
	}
	return &val
}

func (t *Tx) getAddrVinValue(addrDesc bchain.AddressDescriptor) *big.Int {
	var val big.Int
	for _, vin := range t.Vin {
		if bytes.Equal(vin.AddrDesc, addrDesc) {
			val.Add(&val, &vin.ValueSat)
		}
	}
	return &val
}

// UniqueTxidsInReverse reverts the order of transactions (so that newest are first) and removes duplicate transactions
func UniqueTxidsInReverse(txids []string) []string {
	i := len(txids)
	ut := make([]string, i)
	txidsMap := make(map[string]struct{})
	for _, txid := range txids {
		_, e := txidsMap[txid]
		if !e {
			i--
			ut[i] = txid
			txidsMap[txid] = struct{}{}
		}
	}
	return ut[i:]
}

func (w *Worker) txFromTxAddress(txid string, ta *db.TxAddresses, bi *db.BlockInfo, bestheight uint32) *Tx {
	var err error
	var valInSat, valOutSat, feesSat big.Int
	vins := make([]Vin, len(ta.Inputs))
	for i := range ta.Inputs {
		tai := &ta.Inputs[i]
		vin := &vins[i]
		vin.N = i
		vin.ValueSat = tai.ValueSat
		vin.Value = w.chainParser.AmountToDecimalString(&vin.ValueSat)
		valInSat.Add(&valInSat, &vin.ValueSat)
		vin.Addresses, vin.Searchable, err = tai.Addresses(w.chainParser)
		if err != nil {
			glog.Errorf("tai.Addresses error %v, tx %v, input %v, tai %+v", err, txid, i, tai)
		}
	}
	vouts := make([]Vout, len(ta.Outputs))
	for i := range ta.Outputs {
		tao := &ta.Outputs[i]
		vout := &vouts[i]
		vout.N = i
		vout.ValueSat = tao.ValueSat
		vout.Value = w.chainParser.AmountToDecimalString(&vout.ValueSat)
		valOutSat.Add(&valOutSat, &vout.ValueSat)
		vout.ScriptPubKey.Addresses, vout.ScriptPubKey.Searchable, err = tao.Addresses(w.chainParser)
		if err != nil {
			glog.Errorf("tai.Addresses error %v, tx %v, output %v, tao %+v", err, txid, i, tao)
		}
		vout.Spent = tao.Spent
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
		Fees:          w.chainParser.AmountToDecimalString(&feesSat),
		Time:          bi.Time,
		Txid:          txid,
		ValueIn:       w.chainParser.AmountToDecimalString(&valInSat),
		ValueOut:      w.chainParser.AmountToDecimalString(&valOutSat),
		Vin:           vins,
		Vout:          vouts,
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

// GetAddress computes address value and gets transactions for given address
func (w *Worker) GetAddress(address string, page int, txsOnPage int, onlyTxids bool) (*Address, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	addrDesc, err := w.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Invalid address, %v", err), true)
	}
	// ba can be nil if the address is only in mempool!
	ba, err := w.db.GetAddrDescBalance(addrDesc)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Address not found, %v", err), true)
	}
	// convert the address to the format defined by the parser
	addresses, _, err := w.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		glog.V(2).Infof("GetAddressesFromAddrDesc error %v, %v", err, addrDesc)
	}
	if len(addresses) == 1 {
		address = addresses[0]
	}
	txc, err := w.getAddressTxids(addrDesc, false)
	if err != nil {
		return nil, errors.Annotatef(err, "getAddressTxids %v false", address)
	}
	txc = UniqueTxidsInReverse(txc)
	var txm []string
	// if there are only unconfirmed transactions, ba is nil
	if ba == nil {
		ba = &db.AddrBalance{}
		page = 0
	}
	txm, err = w.getAddressTxids(addrDesc, true)
	if err != nil {
		return nil, errors.Annotatef(err, "getAddressTxids %v true", address)
	}
	txm = UniqueTxidsInReverse(txm)
	// check if the address exist
	if len(txc)+len(txm) == 0 {
		return &Address{
			AddrStr: address,
		}, nil
	}
	bestheight, _, err := w.db.GetBestBlock()
	if err != nil {
		return nil, errors.Annotatef(err, "GetBestBlock")
	}
	pg, from, to, page := computePaging(len(txc), page, txsOnPage)
	var txs []*Tx
	var txids []string
	if onlyTxids {
		txids = make([]string, len(txm)+to-from)
	} else {
		txs = make([]*Tx, len(txm)+to-from)
	}
	txi := 0
	// load mempool transactions
	var uBalSat big.Int
	for _, tx := range txm {
		tx, err := w.GetTransaction(tx, false)
		// mempool transaction may fail
		if err != nil {
			glog.Error("GetTransaction in mempool ", tx, ": ", err)
		} else {
			uBalSat.Add(&uBalSat, tx.getAddrVoutValue(addrDesc))
			uBalSat.Sub(&uBalSat, tx.getAddrVinValue(addrDesc))
			if page == 0 {
				if onlyTxids {
					txids[txi] = tx.Txid
				} else {
					txs[txi] = tx
				}
				txi++
			}
		}
	}
	if len(txc) != int(ba.Txs) {
		glog.Warning("DB inconsistency for address ", address, ": number of txs from column addresses ", len(txc), ", from addressBalance ", ba.Txs)
	}
	for i := from; i < to; i++ {
		txid := txc[i]
		if onlyTxids {
			txids[txi] = txid
		} else {
			ta, err := w.db.GetTxAddresses(txid)
			if err != nil {
				return nil, errors.Annotatef(err, "GetTxAddresses %v", txid)
			}
			if ta == nil {
				glog.Warning("DB inconsistency:  tx ", txid, ": not found in txAddresses")
				continue
			}
			bi, err := w.db.GetBlockInfo(ta.Height)
			if err != nil {
				return nil, errors.Annotatef(err, "GetBlockInfo %v", ta.Height)
			}
			if bi == nil {
				glog.Warning("DB inconsistency:  block height ", ta.Height, ": not found in db")
				continue
			}
			txs[txi] = w.txFromTxAddress(txid, ta, bi, bestheight)
		}
		txi++
	}
	if onlyTxids {
		txids = txids[:txi]
	} else {
		txs = txs[:txi]
	}
	r := &Address{
		Paging:                  pg,
		AddrStr:                 address,
		Balance:                 w.chainParser.AmountToDecimalString(&ba.BalanceSat),
		TotalReceived:           w.chainParser.AmountToDecimalString(ba.ReceivedSat()),
		TotalSent:               w.chainParser.AmountToDecimalString(&ba.SentSat),
		TxApperances:            len(txc),
		UnconfirmedBalance:      w.chainParser.AmountToDecimalString(&uBalSat),
		UnconfirmedTxApperances: len(txm),
		Transactions:            txs,
		Txids:                   txids,
	}
	glog.Info("GetAddress ", address, " finished in ", time.Since(start))
	return r, nil
}

// GetAddressUtxo returns unspent outputs for given address
func (w *Worker) GetAddressUtxo(address string, onlyConfirmed bool) ([]AddressUtxo, error) {
	start := time.Now()
	addrDesc, err := w.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Invalid address, %v", err), true)
	}
	spentInMempool := make(map[string]struct{})
	r := make([]AddressUtxo, 0, 8)
	if !onlyConfirmed {
		// get utxo from mempool
		txm, err := w.getAddressTxids(addrDesc, true)
		if err != nil {
			return nil, errors.Annotatef(err, "getAddressTxids %v true", address)
		}
		txm = UniqueTxidsInReverse(txm)
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
							r = append(r, AddressUtxo{
								Txid:      bchainTx.Txid,
								Vout:      uint32(i),
								AmountSat: vout.ValueSat,
								Amount:    w.chainParser.AmountToDecimalString(&vout.ValueSat),
							})
						}
					}
				}
			}
		}
	}
	// get utxo from index
	ba, err := w.db.GetAddrDescBalance(addrDesc)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Address not found, %v", err), true)
	}
	var checksum big.Int
	// ba can be nil if the address is only in mempool!
	if ba != nil && ba.BalanceSat.Uint64() > 0 {
		type outpoint struct {
			txid string
			vout uint32
		}
		outpoints := make([]outpoint, 0, 8)
		err = w.db.GetAddrDescTransactions(addrDesc, 0, ^uint32(0), func(txid string, vout uint32, isOutput bool) error {
			if isOutput {
				outpoints = append(outpoints, outpoint{txid, vout})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		var lastTxid string
		var ta *db.TxAddresses
		checksum = ba.BalanceSat
		b, _, err := w.db.GetBestBlock()
		if err != nil {
			return nil, err
		}
		bestheight := int(b)
		for i := len(outpoints) - 1; i >= 0 && checksum.Int64() > 0; i-- {
			o := outpoints[i]
			if lastTxid != o.txid {
				ta, err = w.db.GetTxAddresses(o.txid)
				if err != nil {
					return nil, err
				}
				lastTxid = o.txid
			}
			if ta == nil {
				glog.Warning("DB inconsistency:  tx ", o.txid, ": not found in txAddresses")
			} else {
				if len(ta.Outputs) <= int(o.vout) {
					glog.Warning("DB inconsistency:  txAddresses ", o.txid, " does not have enough outputs")
				} else {
					if !ta.Outputs[o.vout].Spent {
						v := ta.Outputs[o.vout].ValueSat
						// report only outpoints that are not spent in mempool
						_, e := spentInMempool[o.txid+strconv.Itoa(int(o.vout))]
						if !e {
							r = append(r, AddressUtxo{
								Txid:          o.txid,
								Vout:          o.vout,
								AmountSat:     v,
								Amount:        w.chainParser.AmountToDecimalString(&v),
								Height:        int(ta.Height),
								Confirmations: bestheight - int(ta.Height) + 1,
							})
						}
						checksum.Sub(&checksum, &v)
					}
				}
			}
		}
	}
	if checksum.Uint64() != 0 {
		glog.Warning("DB inconsistency:  ", address, ": checksum is not zero")
	}
	glog.Info("GetAddressUtxo ", address, ", ", len(r), " utxos, finished in ", time.Since(start))
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
	glog.Info("GetBlocks page ", page, " finished in ", time.Since(start))
	return r, nil
}

// GetBlock returns paged data about block
func (w *Worker) GetBlock(bid string, page int, txsOnPage int) (*Block, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	// try to decide if passed string (bid) is block height or block hash
	// if it's a number, must be less than int32
	var hash string
	height, err := strconv.Atoi(bid)
	if err == nil && height < int(^uint32(0)) {
		hash, err = w.db.GetBlockHash(uint32(height))
		if err != nil {
			hash = bid
		}
	} else {
		hash = bid
	}
	bi, err := w.chain.GetBlockInfo(hash)
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
	glog.Info("GetBlock ", bid, ", page ", page, " finished in ", time.Since(start))
	txs := make([]*Tx, to-from)
	txi := 0
	for i := from; i < to; i++ {
		txid := bi.Txids[i]
		ta, err := w.db.GetTxAddresses(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "GetTxAddresses %v", txid)
		}
		if ta == nil {
			glog.Warning("DB inconsistency:  tx ", txid, ": not found in txAddresses")
			continue
		}
		txs[txi] = w.txFromTxAddress(txid, ta, dbi, bestheight)
		txi++
	}
	txs = txs[:txi]
	bi.Txids = nil
	return &Block{
		Paging:       pg,
		BlockInfo:    *bi,
		TxCount:      txCount,
		Transactions: txs,
	}, nil
}

// GetSystemInfo returns information about system
func (w *Worker) GetSystemInfo(internal bool) (*SystemInfo, error) {
	start := time.Now()
	ci, err := w.chain.GetChainInfo()
	if err != nil {
		return nil, errors.Annotatef(err, "GetChainInfo")
	}
	vi := common.GetVersionInfo()
	ss, bh, st := w.is.GetSyncState()
	ms, mt, msz := w.is.GetMempoolSyncState()
	var dbc []common.InternalStateColumn
	var dbs int64
	if internal {
		dbc = w.is.GetAllDBColumnStats()
		dbs = w.is.DBSizeTotal()
	}
	bi := &BlockbookInfo{
		Coin:              w.is.Coin,
		Host:              w.is.Host,
		Version:           vi.Version,
		GitCommit:         vi.GitCommit,
		BuildTime:         vi.BuildTime,
		SyncMode:          w.is.SyncMode,
		InitialSync:       w.is.InitialSync,
		InSync:            ss,
		BestHeight:        bh,
		LastBlockTime:     st,
		InSyncMempool:     ms,
		LastMempoolTime:   mt,
		MempoolSize:       msz,
		DbSize:            w.db.DatabaseSizeOnDisk(),
		DbSizeFromColumns: dbs,
		DbColumns:         dbc,
		About:             Text.BlockbookAbout,
	}
	glog.Info("GetSystemInfo finished in ", time.Since(start))
	return &SystemInfo{bi, ci}, nil
}
