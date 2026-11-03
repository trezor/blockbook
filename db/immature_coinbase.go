package db

import (
	"math/big"
	"sync"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

// immatureCoinbase holds the outputs of coinbase txs younger than the coinbase maturity.
// Consensus forbids spending them, so they are all unspent and need no utxo scan to sum.
type immatureCoinbase struct {
	mux      sync.RWMutex
	valid    bool
	tip      uint32
	byHeight map[uint32][]immatureOutput
	byAddr   map[string][]immatureOutput
}

type immatureOutput struct {
	height   uint32
	addrDesc string
	valueSat big.Int
}

// coinbaseMaturity returns the number of confirmations a coinbase output needs, 0 if not tracked
func (d *RocksDB) coinbaseMaturity() uint32 {
	if d.chainParser.GetChainType() != bchain.ChainBitcoinType {
		return 0
	}
	if m := d.chainParser.MinimumCoinbaseConfirmations(); m > 1 {
		return uint32(m)
	}
	return 0
}

// IsCoinbaseTxAddresses matches the indexed form of a coinbase: one input without address and value
func IsCoinbaseTxAddresses(ta *TxAddresses) bool {
	return len(ta.Inputs) == 1 && len(ta.Inputs[0].AddrDesc) == 0 && ta.Inputs[0].ValueSat.Sign() == 0
}

func (ic *immatureCoinbase) add(height uint32, ta *TxAddresses) {
	for i := range ta.Outputs {
		o := &ta.Outputs[i]
		if len(o.AddrDesc) == 0 {
			continue
		}
		out := immatureOutput{height: height, addrDesc: string(o.AddrDesc)}
		out.valueSat.Set(&o.ValueSat)
		ic.byHeight[height] = append(ic.byHeight[height], out)
		ic.byAddr[out.addrDesc] = append(ic.byAddr[out.addrDesc], out)
	}
}

// prune drops the heights that are mature at tip
func (ic *immatureCoinbase) prune(maturity uint32) {
	for height, outs := range ic.byHeight {
		if ic.tip-height+1 < maturity {
			continue
		}
		delete(ic.byHeight, height)
		for i := range outs {
			kept := ic.byAddr[outs[i].addrDesc][:0]
			for _, o := range ic.byAddr[outs[i].addrDesc] {
				if o.height != height {
					kept = append(kept, o)
				}
			}
			if len(kept) == 0 {
				delete(ic.byAddr, outs[i].addrDesc)
			} else {
				ic.byAddr[outs[i].addrDesc] = kept
			}
		}
	}
}

// rebuildImmatureCoinbase loads the window from blockTxs and txAddresses; caller holds ic.mux
func (d *RocksDB) rebuildImmatureCoinbase(maturity uint32) error {
	ic := &d.immatureCoinbase
	ic.valid = false
	ic.byHeight = make(map[uint32][]immatureOutput)
	ic.byAddr = make(map[string][]immatureOutput)
	tip, _, err := d.GetBestBlock()
	if err != nil {
		return err
	}
	ic.tip = tip
	from := uint32(0)
	if tip+1 >= maturity {
		from = tip + 2 - maturity
	}
	missing := 0
	for height := from; height <= tip; height++ {
		ta, found, err := d.blockCoinbase(height)
		if err != nil {
			return err
		}
		if !found {
			missing++
		} else if ta != nil {
			ic.add(height, ta)
		}
	}
	if missing > 0 && tip > 0 {
		glog.Warningf("rocksdb: immature coinbase index is missing %d of the last %d blocks", missing, tip-from+1)
	}
	ic.valid = true
	return nil
}

// blockCoinbase returns the indexed coinbase of the block; found is false if the block txs are no longer kept
func (d *RocksDB) blockCoinbase(height uint32) (*TxAddresses, bool, error) {
	pl := d.chainParser.PackedTxidLen()
	val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], packUint(height))
	if err != nil {
		return nil, false, err
	}
	var first []byte
	if len(val.Data()) >= pl {
		first = append(first, val.Data()[:pl]...)
	}
	val.Free()
	if first == nil {
		return nil, false, nil
	}
	// fast path: consensus puts the coinbase first
	ta, err := d.getTxAddresses(first)
	if err != nil {
		return nil, true, err
	}
	if ta != nil && IsCoinbaseTxAddresses(ta) {
		return ta, true, nil
	}
	bt, err := d.getBlockTxs(height)
	if err != nil {
		return nil, true, err
	}
	for i := range bt {
		if len(bt[i].inputs) != 1 || !isZeroBytes(bt[i].inputs[0].btxID) {
			continue
		}
		if ta, err = d.getTxAddresses(bt[i].btxID); err != nil || ta == nil || IsCoinbaseTxAddresses(ta) {
			return ta, true, err
		}
	}
	return nil, true, nil
}

func isZeroBytes(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// connectImmatureCoinbase adds the coinbase of a block just written to the db
func (d *RocksDB) connectImmatureCoinbase(height uint32, coinbase *TxAddresses) {
	maturity := d.coinbaseMaturity()
	if maturity == 0 {
		return
	}
	ic := &d.immatureCoinbase
	ic.mux.Lock()
	defer ic.mux.Unlock()
	if ic.valid && height <= ic.tip {
		// a rebuild racing with this connect already read the block
		return
	}
	if !ic.valid || height != ic.tip+1 {
		if err := d.rebuildImmatureCoinbase(maturity); err != nil {
			glog.Error("rocksdb: immature coinbase rebuild ", err)
		}
		return
	}
	ic.tip = height
	if coinbase != nil && IsCoinbaseTxAddresses(coinbase) {
		ic.add(height, coinbase)
	}
	ic.prune(maturity)
}

// invalidateImmatureCoinbase forces a rebuild after the tip moved outside ConnectBlock
func (d *RocksDB) invalidateImmatureCoinbase() {
	d.immatureCoinbase.mux.Lock()
	d.immatureCoinbase.valid = false
	d.immatureCoinbase.mux.Unlock()
}

// GetImmatureCoinbaseBalance returns the part of the address balance in coinbase outputs that cannot be spent yet
func (d *RocksDB) GetImmatureCoinbaseBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	var sum big.Int
	maturity := d.coinbaseMaturity()
	if maturity == 0 {
		return &sum, nil
	}
	ic := &d.immatureCoinbase
	ic.mux.RLock()
	if !ic.valid {
		ic.mux.RUnlock()
		ic.mux.Lock()
		if !ic.valid {
			if err := d.rebuildImmatureCoinbase(maturity); err != nil {
				ic.mux.Unlock()
				return nil, err
			}
		}
		ic.mux.Unlock()
		ic.mux.RLock()
	}
	defer ic.mux.RUnlock()
	for i := range ic.byAddr[string(addrDesc)] {
		o := &ic.byAddr[string(addrDesc)][i]
		if ic.tip-o.height+1 < maturity {
			sum.Add(&sum, &o.valueSat)
		}
	}
	return &sum, nil
}
