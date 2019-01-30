package api

import (
	"blockbook/bchain"
	"blockbook/db"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

const xpubLen = 111
const derivedAddressesBlock = 20

var cachedXpubs = make(map[string]*xpubData)
var cachedXpubsMux sync.Mutex

type txHeight struct {
	txid      string
	height    uint32
	addrIndex uint32
}

type xpubAddress struct {
	addrDesc     bchain.AddressDescriptor
	balance      *db.AddrBalance
	bottomHeight uint32
}

type xpubData struct {
	dataHeight      uint32
	dataHash        string
	txs             uint32
	sentSat         big.Int
	balanceSat      big.Int
	addresses       []xpubAddress
	changeAddresses []xpubAddress
	txids           []txHeight
}

func (w *Worker) getAddressTxHeights(addrDesc bchain.AddressDescriptor, addrIndex uint32, mempool bool, filter *AddressFilter, maxResults int) ([]txHeight, error) {
	var err error
	txHeights := make([]txHeight, 0, 4)
	var callback db.GetTransactionsCallback
	if filter.Vout == AddressFilterVoutOff {
		callback = func(txid string, height uint32, indexes []int32) error {
			txHeights = append(txHeights, txHeight{txid, height, addrIndex})
			// take all txs in the last found block even if it exceeds maxResults
			if len(txHeights) >= maxResults && txHeights[len(txHeights)-1].height != height {
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
					txHeights = append(txHeights, txHeight{txid, height, addrIndex})
					if len(txHeights) >= maxResults {
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
		o, err := w.chain.GetMempoolTransactionsForAddrDesc(addrDesc)
		if err != nil {
			return nil, err
		}
		for _, m := range o {
			if _, found := uniqueTxs[m.Txid]; !found {
				l := len(txHeights)
				callback(m.Txid, 0, []int32{m.Vout})
				if len(txHeights) > l {
					uniqueTxs[m.Txid] = struct{}{}
				}
			}
		}
	} else {
		to := filter.ToHeight
		if to == 0 {
			to = ^uint32(0)
		}
		err = w.db.GetAddrDescTransactions(addrDesc, filter.FromHeight, to, callback)
		if err != nil {
			return nil, err
		}
	}
	return txHeights, nil
}

func (w *Worker) derivedAddressBalance(data *xpubData, ad *xpubAddress) (bool, error) {
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
func (w *Worker) GetAddressForXpub(xpub string, page int, txsOnPage int, option GetAddressOption, filter *AddressFilter) (*Address, error) {
	if w.chainType != bchain.ChainBitcoinType || len(xpub) != xpubLen {
		return nil, ErrUnsupportedXpub
	}
	start := time.Now()
	var processedHash string
	cachedXpubsMux.Lock()
	data, found := cachedXpubs[xpub]
	cachedXpubsMux.Unlock()
	// to load all data for xpub may take some time, perform it in a loop to process a possible new block
	for {
		bestheight, besthash, err := w.db.GetBestBlock()
		if err != nil {
			return nil, errors.Annotatef(err, "GetBestBlock")
		}
		if besthash == processedHash {
			break
		}
		fork := false
		if !found {
			data = &xpubData{}
		} else {
			hash, err := w.db.GetBlockHash(data.dataHeight)
			if err != nil {
				return nil, err
			}
			if hash != data.dataHash {
				// in case of for reset all cached txids
				fork = true
				data.txids = nil
			}
		}
		processedHash = besthash
		if data.dataHeight < bestheight {
			data.dataHeight = bestheight
			data.dataHash = besthash
			// rescan known addresses
			lastUsed := 0
			for i := range data.addresses {
				ad := &data.addresses[i]
				if fork {
					ad.bottomHeight = 0
				}
				used, err := w.derivedAddressBalance(data, ad)
				if err != nil {
					return nil, err
				}
				if used {
					lastUsed = i
				}
			}
			// derive new addresses as necessary
			missing := len(data.addresses) - lastUsed
			for missing < derivedAddressesBlock {
				from := len(data.addresses)
				descriptors, err := w.chainParser.DeriveAddressDescriptorsFromTo(xpub, 0, uint32(from), uint32(from+derivedAddressesBlock-missing))
				if err != nil {
					return nil, err
				}
				for i, a := range descriptors {
					ad := xpubAddress{addrDesc: a}
					used, err := w.derivedAddressBalance(data, &ad)
					if err != nil {
						return nil, err
					}
					if used {
						lastUsed = i + from
					}
					data.addresses = append(data.addresses, ad)
				}
				missing = len(data.addresses) - lastUsed
			}
			// check and generate change addresses
			ca := data.changeAddresses
			data.changeAddresses = make([]xpubAddress, len(data.addresses))
			copy(data.changeAddresses, ca)
			changeIndexes := []uint32{}
			for i, ad := range data.addresses {
				if ad.balance != nil {
					if data.changeAddresses[i].addrDesc == nil {
						changeIndexes = append(changeIndexes, uint32(i))
					} else {
						_, err := w.derivedAddressBalance(data, &ad)
						if err != nil {
							return nil, err
						}
					}
				}
			}
			if len(changeIndexes) > 0 {
				descriptors, err := w.chainParser.DeriveAddressDescriptors(xpub, 1, changeIndexes)
				if err != nil {
					return nil, err
				}
				for i, a := range descriptors {
					ad := &data.changeAddresses[changeIndexes[i]]
					ad.addrDesc = a
					_, err := w.derivedAddressBalance(data, ad)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}
	cachedXpubsMux.Lock()
	cachedXpubs[xpub] = data
	cachedXpubsMux.Unlock()
	tokens := make([]Token, 0, 4)
	for i, ad := range data.addresses {
		if ad.balance != nil {
			tokens = append(tokens, w.tokenFromXpubAddress(&ad, 0, i))
		}
		if data.changeAddresses[i].balance != nil {
			tokens = append(tokens, w.tokenFromXpubAddress(&data.changeAddresses[i], 1, i))
		}
	}
	var totalReceived big.Int
	totalReceived.Add(&data.balanceSat, &data.sentSat)
	addr := Address{
		// Paging:                pg,
		AddrStr:          xpub,
		BalanceSat:       (*Amount)(&data.balanceSat),
		TotalReceivedSat: (*Amount)(&totalReceived),
		TotalSentSat:     (*Amount)(&data.sentSat),
		Txs:              int(data.txs),
		// UnconfirmedBalanceSat: (*Amount)(&uBalSat),
		// UnconfirmedTxs:        len(txm),
		// Transactions:          txs,
		// Txids:                 txids,
		Tokens: tokens,
		// Erc20Contract:         erc20c,
		// Nonce:                 nonce,
	}
	glog.Info("GetAddressForXpub ", xpub[:10], ", ", len(data.addresses), " derived addresses, ", data.txs, " total txs finished in ", time.Since(start))
	return &addr, nil
}
