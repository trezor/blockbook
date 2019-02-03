package api

import (
	"blockbook/bchain"
	"blockbook/db"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

const xpubLen = 111
const defaultAddressesGap = 20

const txInput = 1
const txOutput = 2

var cachedXpubs = make(map[string]*xpubData)
var cachedXpubsMux sync.Mutex

type xpubTxid struct {
	txid        string
	height      uint32
	inputOutput byte
}

type xpubTxids []xpubTxid

func (a xpubTxids) Len() int           { return len(a) }
func (a xpubTxids) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a xpubTxids) Less(i, j int) bool { return a[i].height >= a[j].height }

type xpubAddress struct {
	addrDesc  bchain.AddressDescriptor
	balance   *db.AddrBalance
	txs       uint32
	maxHeight uint32
	complete  bool
	txids     xpubTxids
}

type xpubData struct {
	gap             int
	dataHeight      uint32
	dataHash        string
	txs             uint32
	sentSat         big.Int
	balanceSat      big.Int
	addresses       []xpubAddress
	changeAddresses []xpubAddress
}

func (w *Worker) xpubGetAddressTxids(addrDesc bchain.AddressDescriptor, mempool bool, fromHeight, toHeight uint32, maxResults int) ([]xpubTxid, bool, error) {
	var err error
	complete := true
	txs := make([]xpubTxid, 0, 4)
	var callback db.GetTransactionsCallback
	callback = func(txid string, height uint32, indexes []int32) error {
		// take all txs in the last found block even if it exceeds maxResults
		if len(txs) >= maxResults && txs[len(txs)-1].height != height {
			complete = false
			return &db.StopIteration{}
		}
		inputOutput := byte(0)
		for _, index := range indexes {
			if index < 0 {
				inputOutput |= txInput
			} else {
				inputOutput |= txOutput
			}
		}
		txs = append(txs, xpubTxid{txid, height, inputOutput})
		return nil
	}
	if mempool {
		uniqueTxs := make(map[string]int)
		o, err := w.chain.GetMempoolTransactionsForAddrDesc(addrDesc)
		if err != nil {
			return nil, false, err
		}
		for _, m := range o {
			if l, found := uniqueTxs[m.Txid]; !found {
				l = len(txs)
				callback(m.Txid, 0, []int32{m.Vout})
				if len(txs) > l {
					uniqueTxs[m.Txid] = l - 1
				}
			} else {
				if m.Vout < 0 {
					txs[l].inputOutput |= txInput
				} else {
					txs[l].inputOutput |= txOutput
				}
			}
		}
	} else {
		err = w.db.GetAddrDescTransactions(addrDesc, fromHeight, toHeight, callback)
		if err != nil {
			return nil, false, err
		}
	}
	return txs, complete, nil
}

func (w *Worker) xpubCheckAndLoadTxids(ad *xpubAddress, filter *AddressFilter, maxHeight uint32, pageSize int) error {
	// skip if not discovered
	if ad.balance == nil {
		return nil
	}
	// if completely read, check if there are not some new txs and load if necessary
	if ad.complete {
		if ad.balance.Txs != ad.txs {
			newTxids, _, err := w.xpubGetAddressTxids(ad.addrDesc, false, ad.maxHeight+1, maxHeight, maxInt)
			if err == nil {
				ad.txids = append(newTxids, ad.txids...)
				ad.maxHeight = maxHeight
				ad.txs = uint32(len(ad.txids))
				if ad.txs != ad.balance.Txs {
					glog.Warning("xpubCheckAndLoadTxids inconsistency ", ad.addrDesc, ", ad.txs=", ad.txs, ", ad.balance.Txs=", ad.balance.Txs)
				}
			}
			return err
		}
	}
	// unless the filter is completely off, load all txids
	if filter.FromHeight != 0 || filter.ToHeight != 0 || filter.Vout != AddressFilterVoutOff {
		pageSize = maxInt
	}
	newTxids, complete, err := w.xpubGetAddressTxids(ad.addrDesc, false, 0, maxHeight, pageSize)
	if err != nil {
		return err
	}
	ad.txids = newTxids
	ad.complete = complete
	ad.maxHeight = maxHeight
	if complete {
		ad.txs = uint32(len(ad.txids))
		if ad.txs != ad.balance.Txs {
			glog.Warning("xpubCheckAndLoadTxids inconsistency ", ad.addrDesc, ", ad.txs=", ad.txs, ", ad.balance.Txs=", ad.balance.Txs)
		}
	}
	return nil
}

func (w *Worker) xpubDerivedAddressBalance(data *xpubData, ad *xpubAddress) (bool, error) {
	var err error
	if ad.balance, err = w.db.GetAddrDescBalance(ad.addrDesc); err != nil {
		return false, err
	}
	if ad.balance != nil {
		data.txs += ad.balance.Txs
		data.sentSat.Add(&data.sentSat, &ad.balance.SentSat)
		data.balanceSat.Add(&data.balanceSat, &ad.balance.BalanceSat)
		return true, nil
	}
	return false, nil
}

func (w *Worker) xpubScanAddresses(xpub string, data *xpubData, addresses []xpubAddress, gap int, change int, minDerivedIndex int, fork bool) (int, []xpubAddress, error) {
	// rescan known addresses
	lastUsed := 0
	for i := range addresses {
		ad := &addresses[i]
		if fork {
			// reset the cached data
			ad.txs = 0
			ad.maxHeight = 0
			ad.complete = false
			ad.txids = nil
		}
		used, err := w.xpubDerivedAddressBalance(data, ad)
		if err != nil {
			return 0, nil, err
		}
		if used {
			lastUsed = i
		}
	}
	// derive new addresses as necessary
	missing := len(addresses) - lastUsed
	for missing < gap {
		from := len(addresses)
		to := from + gap - missing
		if to < minDerivedIndex {
			to = minDerivedIndex
		}
		descriptors, err := w.chainParser.DeriveAddressDescriptorsFromTo(xpub, uint32(change), uint32(from), uint32(to))
		if err != nil {
			return 0, nil, err
		}
		for i, a := range descriptors {
			ad := xpubAddress{addrDesc: a}
			used, err := w.xpubDerivedAddressBalance(data, &ad)
			if err != nil {
				return 0, nil, err
			}
			if used {
				lastUsed = i + from
			}
			addresses = append(addresses, ad)
		}
		missing = len(addresses) - lastUsed
	}
	return lastUsed, addresses, nil
}

func (w *Worker) tokenFromXpubAddress(ad *xpubAddress, changeIndex int, index int) Token {
	a, _, _ := w.chainParser.GetAddressesFromAddrDesc(ad.addrDesc)
	var address string
	if len(a) > 0 {
		address = a[0]
	}
	return Token{
		Type:       XPUBAddressTokenType,
		Name:       address,
		Decimals:   w.chainParser.AmountDecimals(),
		BalanceSat: (*Amount)(&ad.balance.BalanceSat),
		Transfers:  int(ad.balance.Txs),
		Contract:   fmt.Sprintf("%d/%d", changeIndex, index),
	}
}

// GetAddressForXpub computes address value and gets transactions for given address
func (w *Worker) GetAddressForXpub(xpub string, page int, txsOnPage int, option GetAddressOption, filter *AddressFilter, gap int) (*Address, error) {
	if w.chainType != bchain.ChainBitcoinType || len(xpub) != xpubLen {
		return nil, ErrUnsupportedXpub
	}
	start := time.Now()
	if gap <= 0 {
		gap = defaultAddressesGap
	}
	// gap is increased one as there must be gap of empty addresses before the derivation is stopped
	gap++
	var processedHash string
	cachedXpubsMux.Lock()
	data, found := cachedXpubs[xpub]
	cachedXpubsMux.Unlock()
	var (
		txm          []string
		txs          []*Tx
		txids        []string
		pg           Paging
		totalResults int
		err          error
		bestheight   uint32
		besthash     string
	)
	// to load all data for xpub may take some time, do it in a loop to process a possible new block
	for {
		bestheight, besthash, err = w.db.GetBestBlock()
		if err != nil {
			return nil, errors.Annotatef(err, "GetBestBlock")
		}
		if besthash == processedHash {
			break
		}
		fork := false
		if !found || data.gap != gap {
			data = &xpubData{gap: gap}
		} else {
			hash, err := w.db.GetBlockHash(data.dataHeight)
			if err != nil {
				return nil, err
			}
			if hash != data.dataHash {
				// in case of for reset all cached data
				fork = true
			}
		}
		processedHash = besthash
		if data.dataHeight < bestheight {
			data.dataHeight = bestheight
			data.dataHash = besthash
			var lastUsedIndex int
			lastUsedIndex, data.addresses, err = w.xpubScanAddresses(xpub, data, data.addresses, gap, 0, 0, fork)
			if err != nil {
				return nil, err
			}
			_, data.changeAddresses, err = w.xpubScanAddresses(xpub, data, data.changeAddresses, gap, 1, lastUsedIndex, fork)
			if err != nil {
				return nil, err
			}
		}
		if option >= TxidHistory {
			for i := range data.addresses {
				if err = w.xpubCheckAndLoadTxids(&data.addresses[i], filter, bestheight, txsOnPage); err != nil {
					return nil, err
				}
			}
			for i := range data.changeAddresses {
				if err = w.xpubCheckAndLoadTxids(&data.changeAddresses[i], filter, bestheight, txsOnPage); err != nil {
					return nil, err
				}
			}
		}
	}
	cachedXpubsMux.Lock()
	cachedXpubs[xpub] = data
	cachedXpubsMux.Unlock()
	// TODO mempool
	if option >= TxidHistory {
		txc := make(xpubTxids, 0, 32)
		var addTxids func(ad *xpubAddress)
		if filter.FromHeight != 0 || filter.ToHeight != 0 || filter.Vout != AddressFilterVoutOff {
			addTxids = func(ad *xpubAddress) {
				txc = append(txc, ad.txids...)
			}
			totalResults = int(data.txs)
		} else {
			toHeight := maxUint32
			if filter.ToHeight != 0 {
				toHeight = filter.ToHeight
			}
			addTxids = func(ad *xpubAddress) {
				for _, txid := range ad.txids {
					if txid.height < filter.FromHeight || txid.height > toHeight {
						continue
					}
					if filter.Vout != AddressFilterVoutOff {
						if filter.Vout == AddressFilterVoutInputs && txid.inputOutput&txInput == 0 ||
							filter.Vout == AddressFilterVoutOutputs && txid.inputOutput&txOutput == 0 {
							continue
						}
					}
					txc = append(txc, txid)
				}
			}
			totalResults = -1
		}
		for i := range data.addresses {
			addTxids(&data.addresses[i])
		}
		for i := range data.changeAddresses {
			addTxids(&data.changeAddresses[i])
		}
		sort.Stable(txc)
		var from, to int
		pg, from, to, page = computePaging(len(txc), page, txsOnPage)
		if len(txc) >= txsOnPage {
			if totalResults < 0 {
				pg.TotalPages = -1
			} else {
				pg, _, _, _ = computePaging(totalResults, page, txsOnPage)
			}
		}
		if option == TxidHistory {
			txids = make([]string, len(txm)+to-from)
		} else {
			txs = make([]*Tx, len(txm)+to-from)
		}
		txi := 0
		// get confirmed transactions
		for i := from; i < to; i++ {
			xpubTxid := &txc[i]
			if option == TxidHistory {
				txids[txi] = xpubTxid.txid
			} else {
				if txs[txi], err = w.txFromTxid(xpubTxid.txid, bestheight, option); err != nil {
					return nil, err
				}
			}
			txi++
		}
		if option == TxidHistory {
			txids = txids[:txi]
		} else if option >= TxHistoryLight {
			txs = txs[:txi]
		}
	}
	totalTokens := 0
	tokens := make([]Token, 0, 4)
	for i := range data.addresses {
		ad := &data.addresses[i]
		if ad.balance != nil {
			totalTokens++
			if filter.AllTokens || !IsZeroBigInt(&ad.balance.BalanceSat) {
				tokens = append(tokens, w.tokenFromXpubAddress(ad, 0, i))
			}
		}
	}
	for i := range data.changeAddresses {
		ad := &data.changeAddresses[i]
		if ad.balance != nil {
			totalTokens++
			if filter.AllTokens || !IsZeroBigInt(&ad.balance.BalanceSat) {
				tokens = append(tokens, w.tokenFromXpubAddress(ad, 1, i))
			}
		}
	}
	var totalReceived big.Int
	totalReceived.Add(&data.balanceSat, &data.sentSat)
	addr := Address{
		Paging:           pg,
		AddrStr:          xpub,
		BalanceSat:       (*Amount)(&data.balanceSat),
		TotalReceivedSat: (*Amount)(&totalReceived),
		TotalSentSat:     (*Amount)(&data.sentSat),
		Txs:              int(data.txs),
		// UnconfirmedBalanceSat: (*Amount)(&uBalSat),
		// UnconfirmedTxs:        len(txm),
		Transactions: txs,
		Txids:        txids,
		TotalTokens:  totalTokens,
		Tokens:       tokens,
	}
	glog.Info("GetAddressForXpub ", xpub[:16], ", ", len(data.addresses)+len(data.changeAddresses), " derived addresses, ", data.txs, " total txs finished in ", time.Since(start))
	return &addr, nil
}
