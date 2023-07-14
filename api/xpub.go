package api

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/db"
)

const defaultAddressesGap = 20
const maxAddressesGap = 10000

const txInput = 1
const txOutput = 2

const xpubCacheExpirationSeconds = 3600

var cachedXpubs map[string]xpubData
var cachedXpubsMux sync.Mutex

const xpubLogPrefix = 30

type xpubTxid struct {
	txid        string
	height      uint32
	inputOutput byte
}

type xpubTxids []xpubTxid

func (a xpubTxids) Len() int      { return len(a) }
func (a xpubTxids) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a xpubTxids) Less(i, j int) bool {
	// if the heights are equal, make inputs less than outputs
	hi := a[i].height
	hj := a[j].height
	if hi == hj {
		return (a[i].inputOutput & txInput) >= (a[j].inputOutput & txInput)
	}
	return hi > hj
}

type xpubAddress struct {
	addrDesc  bchain.AddressDescriptor
	balance   *db.AddrBalance
	txs       uint32
	maxHeight uint32
	complete  bool
	txids     xpubTxids
}

type xpubData struct {
	descriptor      *bchain.XpubDescriptor
	gap             int
	accessed        int64
	basePath        string
	dataHeight      uint32
	dataHash        string
	txCountEstimate uint32
	sentSat         big.Int
	balanceSat      big.Int
	addresses       [][]xpubAddress
}

func (w *Worker) initXpubCache() {
	cachedXpubsMux.Lock()
	if cachedXpubs == nil {
		cachedXpubs = make(map[string]xpubData)
		go func() {
			for {
				time.Sleep(20 * time.Second)
				w.evictXpubCacheItems()
			}
		}()
	}
	cachedXpubsMux.Unlock()
}

func (w *Worker) evictXpubCacheItems() {
	cachedXpubsMux.Lock()
	defer cachedXpubsMux.Unlock()
	threshold := time.Now().Unix() - xpubCacheExpirationSeconds
	count := 0
	for k, v := range cachedXpubs {
		if v.accessed < threshold {
			delete(cachedXpubs, k)
			count++
		}
	}
	w.metrics.XPubCacheSize.Set(float64(len(cachedXpubs)))
	glog.Info("Evicted ", count, " items from xpub cache, cache size ", len(cachedXpubs))
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
		o, err := w.mempool.GetAddrDescTransactions(addrDesc)
		if err != nil {
			return nil, false, err
		}
		for _, m := range o {
			if l, found := uniqueTxs[m.Txid]; !found {
				l = len(txs)
				callback(m.Txid, 0, []int32{m.Vout})
				if len(txs) > l {
					uniqueTxs[m.Txid] = l
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

func (w *Worker) xpubCheckAndLoadTxids(ad *xpubAddress, filter *AddressFilter, maxHeight uint32, maxResults int) error {
	// skip if not used
	if ad.balance == nil {
		return nil
	}
	// if completely loaded, check if there are not some new txs and load if necessary
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
		return nil
	}
	// load all txids to get paging correctly
	newTxids, complete, err := w.xpubGetAddressTxids(ad.addrDesc, false, 0, maxHeight, maxInt)
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
	if ad.balance, err = w.db.GetAddrDescBalance(ad.addrDesc, db.AddressBalanceDetailUTXO); err != nil {
		return false, err
	}
	if ad.balance != nil {
		data.txCountEstimate += ad.balance.Txs
		data.sentSat.Add(&data.sentSat, &ad.balance.SentSat)
		data.balanceSat.Add(&data.balanceSat, &ad.balance.BalanceSat)
		return true, nil
	}
	return false, nil
}

func (w *Worker) xpubScanAddresses(xd *bchain.XpubDescriptor, data *xpubData, addresses []xpubAddress, gap int, change uint32, minDerivedIndex int, fork bool) (int, []xpubAddress, error) {
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
		descriptors, err := w.chainParser.DeriveAddressDescriptorsFromTo(xd, change, uint32(from), uint32(to))
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

func (w *Worker) tokenFromXpubAddress(data *xpubData, ad *xpubAddress, changeIndex int, index int, option AccountDetails) Token {
	a, _, _ := w.chainParser.GetAddressesFromAddrDesc(ad.addrDesc)
	var address string
	if len(a) > 0 {
		address = a[0]
	}
	var balance, totalReceived, totalSent *big.Int
	var transfers int
	if ad.balance != nil {
		transfers = int(ad.balance.Txs)
		if option >= AccountDetailsTokenBalances {
			balance = &ad.balance.BalanceSat
			totalSent = &ad.balance.SentSat
			totalReceived = ad.balance.ReceivedSat()
		}
	}
	return Token{
		Type:             bchain.XPUBAddressTokenType,
		Name:             address,
		Decimals:         w.chainParser.AmountDecimals(),
		BalanceSat:       (*Amount)(balance),
		TotalReceivedSat: (*Amount)(totalReceived),
		TotalSentSat:     (*Amount)(totalSent),
		Transfers:        transfers,
		Path:             fmt.Sprintf("%s/%d/%d", data.basePath, changeIndex, index),
	}
}

// returns true if addresses are "own", i.e. the address belongs to the xpub
func isOwnAddresses(xpubAddresses map[string]struct{}, addresses []string) bool {
	if len(addresses) == 1 {
		_, found := xpubAddresses[addresses[0]]
		return found
	}
	return false
}

func setIsOwnAddresses(txs []*Tx, xpubAddresses map[string]struct{}) {
	for i := range txs {
		tx := txs[i]
		for j := range tx.Vin {
			vin := &tx.Vin[j]
			if isOwnAddresses(xpubAddresses, vin.Addresses) {
				vin.IsOwn = true
			}
		}
		for j := range tx.Vout {
			vout := &tx.Vout[j]
			if isOwnAddresses(xpubAddresses, vout.Addresses) {
				vout.IsOwn = true
			}
		}
	}
}

func (w *Worker) getXpubData(xd *bchain.XpubDescriptor, page int, txsOnPage int, option AccountDetails, filter *AddressFilter, gap int) (*xpubData, uint32, bool, error) {
	if w.chainType != bchain.ChainBitcoinType {
		return nil, 0, false, ErrUnsupportedXpub
	}
	var (
		err        error
		bestheight uint32
		besthash   string
	)
	if gap <= 0 {
		gap = defaultAddressesGap
	} else if gap > maxAddressesGap {
		// limit the maximum gap to protect against unreasonably big values that could cause high load of the server
		gap = maxAddressesGap
	}
	// gap is increased one as there must be gap of empty addresses before the derivation is stopped
	gap++
	var processedHash string
	cachedXpubsMux.Lock()
	data, inCache := cachedXpubs[xd.XpubDescriptor]
	cachedXpubsMux.Unlock()
	// to load all data for xpub may take some time, do it in a loop to process a possible new block
	for {
		bestheight, besthash, err = w.db.GetBestBlock()
		if err != nil {
			return nil, 0, inCache, errors.Annotatef(err, "GetBestBlock")
		}
		if besthash == processedHash {
			break
		}
		fork := false
		if !inCache || data.gap != gap {
			data = xpubData{
				gap:       gap,
				addresses: make([][]xpubAddress, len(xd.ChangeIndexes)),
			}
			data.basePath, err = w.chainParser.DerivationBasePath(xd)
			if err != nil {
				return nil, 0, inCache, err
			}
		} else {
			hash, err := w.db.GetBlockHash(data.dataHeight)
			if err != nil {
				return nil, 0, inCache, err
			}
			if hash != data.dataHash {
				// in case of for reset all cached data
				fork = true
			}
		}
		processedHash = besthash
		if data.dataHeight < bestheight || fork {
			data.dataHeight = bestheight
			data.dataHash = besthash
			data.balanceSat = *new(big.Int)
			data.sentSat = *new(big.Int)
			data.txCountEstimate = 0
			var minDerivedIndex int
			for i, change := range xd.ChangeIndexes {
				minDerivedIndex, data.addresses[i], err = w.xpubScanAddresses(xd, &data, data.addresses[i], gap, change, minDerivedIndex, fork)
				if err != nil {
					return nil, 0, inCache, err
				}
			}
		}
		if option >= AccountDetailsTxidHistory {
			for _, da := range data.addresses {
				for i := range da {
					if err = w.xpubCheckAndLoadTxids(&da[i], filter, bestheight, (page+1)*txsOnPage); err != nil {
						return nil, 0, inCache, err
					}
				}
			}
		}
	}
	data.accessed = time.Now().Unix()
	cachedXpubsMux.Lock()
	cachedXpubs[xd.XpubDescriptor] = data
	cachedXpubsMux.Unlock()
	return &data, bestheight, inCache, nil
}

// GetXpubAddress computes address value and gets transactions for given address
func (w *Worker) GetXpubAddress(xpub string, page int, txsOnPage int, option AccountDetails, filter *AddressFilter, gap int, secondaryCoin string) (*Address, error) {
	start := time.Now()
	page--
	if page < 0 {
		page = 0
	}
	type mempoolMap struct {
		tx          *Tx
		inputOutput byte
	}
	var (
		txc            xpubTxids
		txmMap         map[string]*Tx
		txCount        int
		txs            []*Tx
		txids          []string
		pg             Paging
		filtered       bool
		uBalSat        big.Int
		unconfirmedTxs int
	)
	xd, err := w.chainParser.ParseXpub(xpub)
	if err != nil {
		return nil, err
	}
	data, bestheight, inCache, err := w.getXpubData(xd, page, txsOnPage, option, filter, gap)
	if err != nil {
		return nil, err
	}
	// setup filtering of txids
	var txidFilter func(txid *xpubTxid, ad *xpubAddress) bool
	if !(filter.FromHeight == 0 && filter.ToHeight == 0 && filter.Vout == AddressFilterVoutOff) {
		toHeight := maxUint32
		if filter.ToHeight != 0 {
			toHeight = filter.ToHeight
		}
		txidFilter = func(txid *xpubTxid, ad *xpubAddress) bool {
			if txid.height < filter.FromHeight || txid.height > toHeight {
				return false
			}
			if filter.Vout != AddressFilterVoutOff {
				if filter.Vout == AddressFilterVoutInputs && txid.inputOutput&txInput == 0 ||
					filter.Vout == AddressFilterVoutOutputs && txid.inputOutput&txOutput == 0 {
					return false
				}
			}
			return true
		}
		filtered = true
	}
	addresses := w.newAddressesMapForAliases()
	// process mempool, only if ToHeight is not specified
	if filter.ToHeight == 0 && !filter.OnlyConfirmed {
		txmMap = make(map[string]*Tx)
		mempoolEntries := make(bchain.MempoolTxidEntries, 0)
		for _, da := range data.addresses {
			for i := range da {
				ad := &da[i]
				newTxids, _, err := w.xpubGetAddressTxids(ad.addrDesc, true, 0, 0, maxInt)
				if err != nil {
					return nil, err
				}
				for _, txid := range newTxids {
					// the same tx can have multiple addresses from the same xpub, get it from backend it only once
					tx, foundTx := txmMap[txid.txid]
					if !foundTx {
						tx, err = w.getTransaction(txid.txid, false, true, addresses)
						// mempool transaction may fail
						if err != nil || tx == nil {
							glog.Warning("GetTransaction in mempool: ", err)
							continue
						}
						txmMap[txid.txid] = tx
					}
					// skip already confirmed txs, mempool may be out of sync
					if tx.Confirmations == 0 {
						if !foundTx {
							unconfirmedTxs++
						}
						uBalSat.Add(&uBalSat, tx.getAddrVoutValue(ad.addrDesc))
						uBalSat.Sub(&uBalSat, tx.getAddrVinValue(ad.addrDesc))
						// mempool txs are returned only on the first page, uniquely and filtered
						if page == 0 && !foundTx && (txidFilter == nil || txidFilter(&txid, ad)) {
							mempoolEntries = append(mempoolEntries, bchain.MempoolTxidEntry{Txid: txid.txid, Time: uint32(tx.Blocktime)})
						}
					}
				}
			}
		}
		// sort the entries by time descending
		sort.Sort(mempoolEntries)
		for _, entry := range mempoolEntries {
			if option == AccountDetailsTxidHistory {
				txids = append(txids, entry.Txid)
			} else if option >= AccountDetailsTxHistoryLight {
				txs = append(txs, txmMap[entry.Txid])
			}
		}
	}
	if option >= AccountDetailsTxidHistory {
		txcMap := make(map[string]bool)
		txc = make(xpubTxids, 0, 32)
		for _, da := range data.addresses {
			for i := range da {
				ad := &da[i]
				for _, txid := range ad.txids {
					added, foundTx := txcMap[txid.txid]
					// count txs regardless of filter but only once
					if !foundTx {
						txCount++
					}
					// add tx only once
					if !added {
						add := txidFilter == nil || txidFilter(&txid, ad)
						txcMap[txid.txid] = add
						if add {
							txc = append(txc, txid)
						}
					}
				}
			}
		}
		sort.Stable(txc)
		txCount = len(txcMap)
		totalResults := txCount
		if filtered {
			totalResults = -1
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
		// get confirmed transactions
		for i := from; i < to; i++ {
			xpubTxid := &txc[i]
			if option == AccountDetailsTxidHistory {
				txids = append(txids, xpubTxid.txid)
			} else {
				tx, err := w.txFromTxid(xpubTxid.txid, bestheight, option, nil, addresses)
				if err != nil {
					return nil, err
				}
				txs = append(txs, tx)
			}
		}
	} else {
		txCount = int(data.txCountEstimate)
	}
	addrTxCount := int(data.txCountEstimate)
	usedTokens := 0
	var tokens []Token
	var xpubAddresses map[string]struct{}
	if option > AccountDetailsBasic {
		tokens = make([]Token, 0, 4)
		xpubAddresses = make(map[string]struct{})
	}
	for ci, da := range data.addresses {
		for i := range da {
			ad := &da[i]
			if ad.balance != nil {
				usedTokens++
			}
			if option > AccountDetailsBasic {
				token := w.tokenFromXpubAddress(data, ad, ci, i, option)
				if filter.TokensToReturn == TokensToReturnDerived ||
					filter.TokensToReturn == TokensToReturnUsed && ad.balance != nil ||
					filter.TokensToReturn == TokensToReturnNonzeroBalance && ad.balance != nil && !IsZeroBigInt(&ad.balance.BalanceSat) {
					tokens = append(tokens, token)
				}
				xpubAddresses[token.Name] = struct{}{}
			}
		}
	}
	setIsOwnAddresses(txs, xpubAddresses)
	var totalReceived big.Int
	totalReceived.Add(&data.balanceSat, &data.sentSat)

	var secondaryValue float64
	if secondaryCoin != "" {
		ticker := w.fiatRates.GetCurrentTicker("", "")
		balance, err := strconv.ParseFloat((*Amount)(&data.balanceSat).DecimalString(w.chainParser.AmountDecimals()), 64)
		if ticker != nil && err == nil {
			r, found := ticker.Rates[secondaryCoin]
			if found {
				secondaryRate := float64(r)
				secondaryValue = secondaryRate * balance
			}
		}
	}

	addr := Address{
		Paging:                pg,
		AddrStr:               xpub,
		BalanceSat:            (*Amount)(&data.balanceSat),
		TotalReceivedSat:      (*Amount)(&totalReceived),
		TotalSentSat:          (*Amount)(&data.sentSat),
		Txs:                   txCount,
		AddrTxCount:           addrTxCount,
		UnconfirmedBalanceSat: (*Amount)(&uBalSat),
		UnconfirmedTxs:        unconfirmedTxs,
		Transactions:          txs,
		Txids:                 txids,
		UsedTokens:            usedTokens,
		Tokens:                tokens,
		SecondaryValue:        secondaryValue,
		XPubAddresses:         xpubAddresses,
		AddressAliases:        w.getAddressAliases(addresses),
	}
	glog.Info("GetXpubAddress ", xpub[:xpubLogPrefix], ", cache ", inCache, ", ", txCount, " txs, ", time.Since(start))
	return &addr, nil
}

// GetXpubUtxo returns unspent outputs for given xpub
func (w *Worker) GetXpubUtxo(xpub string, onlyConfirmed bool, gap int) (Utxos, error) {
	start := time.Now()
	xd, err := w.chainParser.ParseXpub(xpub)
	if err != nil {
		return nil, err
	}
	data, _, inCache, err := w.getXpubData(xd, 0, 1, AccountDetailsBasic, &AddressFilter{
		Vout:          AddressFilterVoutOff,
		OnlyConfirmed: onlyConfirmed,
	}, gap)
	if err != nil {
		return nil, err
	}
	r := make(Utxos, 0, 8)
	for ci, da := range data.addresses {
		for i := range da {
			ad := &da[i]
			onlyMempool := false
			if ad.balance == nil {
				if onlyConfirmed {
					continue
				}
				onlyMempool = true
			}
			utxos, err := w.getAddrDescUtxo(ad.addrDesc, ad.balance, onlyConfirmed, onlyMempool)
			if err != nil {
				return nil, err
			}
			if len(utxos) > 0 {
				t := w.tokenFromXpubAddress(data, ad, ci, i, AccountDetailsTokens)
				for j := range utxos {
					a := &utxos[j]
					a.Address = t.Name
					a.Path = t.Path
				}
				r = append(r, utxos...)
			}
		}
	}
	sort.Stable(r)
	glog.Info("GetXpubUtxo ", xpub[:xpubLogPrefix], ", cache ", inCache, ", ", len(r), " utxos,  ", time.Since(start))
	return r, nil
}

// GetXpubBalanceHistory returns history of balance for given xpub
func (w *Worker) GetXpubBalanceHistory(xpub string, fromTimestamp, toTimestamp int64, currencies []string, gap int, groupBy uint32) (BalanceHistories, error) {
	bhs := make(BalanceHistories, 0)
	start := time.Now()
	fromUnix, fromHeight, toUnix, toHeight := w.balanceHistoryHeightsFromTo(fromTimestamp, toTimestamp)
	if fromHeight >= toHeight {
		return bhs, nil
	}
	xd, err := w.chainParser.ParseXpub(xpub)
	if err != nil {
		return nil, err
	}
	data, _, inCache, err := w.getXpubData(xd, 0, 1, AccountDetailsTxidHistory, &AddressFilter{
		Vout:          AddressFilterVoutOff,
		OnlyConfirmed: true,
		FromHeight:    fromHeight,
		ToHeight:      toHeight,
	}, gap)
	if err != nil {
		return nil, err
	}
	selfAddrDesc := make(map[string]struct{})
	for _, da := range data.addresses {
		for i := range da {
			selfAddrDesc[string(da[i].addrDesc)] = struct{}{}
		}
	}
	for _, da := range data.addresses {
		for i := range da {
			ad := &da[i]
			txids := ad.txids
			for txi := len(txids) - 1; txi >= 0; txi-- {
				bh, err := w.balanceHistoryForTxid(ad.addrDesc, txids[txi].txid, fromUnix, toUnix, selfAddrDesc)
				if err != nil {
					return nil, err
				}
				if bh != nil {
					bhs = append(bhs, *bh)
				}
			}
		}
	}
	bha := bhs.SortAndAggregate(groupBy)
	err = w.setFiatRateToBalanceHistories(bha, currencies)
	if err != nil {
		return nil, err
	}
	glog.Info("GetUtxoBalanceHistory ", xpub[:xpubLogPrefix], ", cache ", inCache, ", blocks ", fromHeight, "-", toHeight, ", count ", len(bha), ",  ", time.Since(start))
	return bha, nil
}
