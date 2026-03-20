package bchain

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

type chanInputPayload struct {
	tx    *MempoolTx
	index int
}

type txPayload struct {
	txid string
	tx   *Tx
}

type resyncOutpointCache struct {
	mu      sync.RWMutex
	entries map[Outpoint]outpointInfo
	// hits/misses track cache effectiveness without impacting read paths with extra locks.
	hits   uint64
	misses uint64
}

type outpointInfo struct {
	addrDesc AddressDescriptor
	value    *big.Int
}

func newResyncOutpointCache(sizeHint int) *resyncOutpointCache {
	return &resyncOutpointCache{entries: make(map[Outpoint]outpointInfo, sizeHint)}
}

func (c *resyncOutpointCache) get(outpoint Outpoint) (AddressDescriptor, *big.Int, bool) {
	c.mu.RLock()
	entry, ok := c.entries[outpoint]
	c.mu.RUnlock()
	if !ok {
		// Use atomics to avoid lock contention on hot lookup paths.
		atomic.AddUint64(&c.misses, 1)
		return nil, nil, false
	}
	atomic.AddUint64(&c.hits, 1)
	return entry.addrDesc, entry.value, true
}

func (c *resyncOutpointCache) set(outpoint Outpoint, addrDesc AddressDescriptor, value *big.Int) {
	if len(addrDesc) == 0 || value == nil {
		return
	}
	// Copy to keep cached values independent of the transaction object lifetime.
	valueCopy := new(big.Int).Set(value)
	addrCopy := append(AddressDescriptor(nil), addrDesc...)
	c.mu.Lock()
	c.entries[outpoint] = outpointInfo{addrDesc: addrCopy, value: valueCopy}
	c.mu.Unlock()
}

func (c *resyncOutpointCache) len() int {
	c.mu.RLock()
	n := len(c.entries)
	c.mu.RUnlock()
	return n
}

func (c *resyncOutpointCache) stats() (uint64, uint64) {
	return atomic.LoadUint64(&c.hits), atomic.LoadUint64(&c.misses)
}

// MempoolBitcoinType is mempool handle.
type MempoolBitcoinType struct {
	BaseMempool
	chanTx              chan txPayload
	chanAddrIndex       chan txidio
	AddrDescForOutpoint AddrDescForOutpointFunc
	golombFilterP       uint8
	filterScripts       string
	useZeroedKey        bool
	resyncBatchSize     int
	// resyncBatchWorkers controls how many batch RPCs can be in flight during resync.
	resyncBatchWorkers int
	// resyncOutpoints caches mempool outputs during resync to avoid extra RPC lookups for parents.
	resyncOutpoints atomic.Value
}

// NewMempoolBitcoinType creates new mempool handler.
// For now there is no cleanup of sync routines, the expectation is that the mempool is created only once per process
func NewMempoolBitcoinType(chain BlockChain, workers int, subworkers int, golombFilterP uint8, filterScripts string, useZeroedKey bool, resyncBatchSize int) *MempoolBitcoinType {
	if resyncBatchSize < 1 {
		resyncBatchSize = 1
	}
	if workers < 1 {
		workers = 1
	}
	m := &MempoolBitcoinType{
		BaseMempool: BaseMempool{
			chain:        chain,
			txEntries:    make(map[string]txEntry),
			addrDescToTx: make(map[string][]Outpoint),
		},
		chanTx:             make(chan txPayload, 1),
		chanAddrIndex:      make(chan txidio, 1),
		golombFilterP:      golombFilterP,
		filterScripts:      filterScripts,
		useZeroedKey:       useZeroedKey,
		resyncBatchSize:    resyncBatchSize,
		resyncBatchWorkers: workers,
	}
	m.resyncOutpoints.Store((*resyncOutpointCache)(nil))
	for i := 0; i < workers; i++ {
		go func(i int) {
			chanInput := make(chan chanInputPayload, 1)
			chanResult := make(chan *addrIndex, 1)
			for j := 0; j < subworkers; j++ {
				go func(j int) {
					for payload := range chanInput {
						ai := m.getInputAddress(&payload)
						chanResult <- ai
					}
				}(j)
			}
			for payload := range m.chanTx {
				io, golombFilter, ok := m.getTxAddrs(payload.txid, payload.tx, chanInput, chanResult)
				if !ok {
					io = []addrIndex{}
				}
				m.chanAddrIndex <- txidio{payload.txid, io, golombFilter}
			}
		}(i)
	}
	glog.Info("mempool: starting with ", workers, "*", subworkers, " sync workers")
	return m
}

func (m *MempoolBitcoinType) getResyncOutpointCache() *resyncOutpointCache {
	cache, _ := m.resyncOutpoints.Load().(*resyncOutpointCache)
	return cache
}

func roundDuration(d time.Duration, unit time.Duration) time.Duration {
	if unit <= 0 {
		return d
	}
	return d.Round(unit)
}

func (m *MempoolBitcoinType) getInputAddress(payload *chanInputPayload) *addrIndex {
	var addrDesc AddressDescriptor
	var value *big.Int
	vin := &payload.tx.Vin[payload.index]
	if vin.Txid == "" {
		// cannot get address from empty input txid (for example in Litecoin mweb)
		return nil
	}
	outpoint := Outpoint{vin.Txid, int32(vin.Vout)}
	cache := m.getResyncOutpointCache()
	if m.AddrDescForOutpoint != nil {
		addrDesc, value = m.AddrDescForOutpoint(outpoint)
	}
	if addrDesc == nil {
		if cache != nil {
			if cachedDesc, cachedValue, ok := cache.get(outpoint); ok {
				addrDesc = cachedDesc
				value = cachedValue
			}
		}
	}
	if addrDesc == nil {
		itx, err := m.chain.GetTransactionForMempool(vin.Txid)
		if err != nil {
			glog.Error("cannot get transaction ", vin.Txid, ": ", err)
			return nil
		}
		if int(vin.Vout) >= len(itx.Vout) {
			glog.Error("Vout len in transaction ", vin.Txid, " ", len(itx.Vout), " input.Vout=", vin.Vout)
			return nil
		}
		parser := m.chain.GetChainParser()
		if cache != nil {
			// Cache all outputs for this parent so other inputs can skip another RPC.
			found := false
			for i := range itx.Vout {
				output := &itx.Vout[i]
				outDesc, outErr := parser.GetAddrDescFromVout(output)
				if outErr != nil {
					if output.N == vin.Vout {
						glog.Error("error in addrDesc in ", vin.Txid, " ", vin.Vout, ": ", outErr)
						return nil
					}
					continue
				}
				cache.set(Outpoint{vin.Txid, int32(output.N)}, outDesc, &output.ValueSat)
				if output.N == vin.Vout {
					found = true
					addrDesc = outDesc
					value = &output.ValueSat
				}
			}
			if !found {
				glog.Error("Vout not found in transaction ", vin.Txid, " input.Vout=", vin.Vout)
				return nil
			}
		} else {
			addrDesc, err = parser.GetAddrDescFromVout(&itx.Vout[vin.Vout])
			if err != nil {
				glog.Error("error in addrDesc in ", vin.Txid, " ", vin.Vout, ": ", err)
				return nil
			}
			value = &itx.Vout[vin.Vout].ValueSat
		}
	}
	vin.AddrDesc = addrDesc
	vin.ValueSat = *value
	return &addrIndex{string(addrDesc), ^int32(vin.Vout)}

}

func (m *MempoolBitcoinType) computeGolombFilter(mtx *MempoolTx, tx *Tx) string {
	gf, _ := NewGolombFilter(m.golombFilterP, m.filterScripts, mtx.Txid, m.useZeroedKey)
	if gf == nil || !gf.Enabled {
		return ""
	}
	for _, vin := range mtx.Vin {
		gf.AddAddrDesc(vin.AddrDesc, tx)
	}
	for _, vout := range mtx.Vout {
		b, err := hex.DecodeString(vout.ScriptPubKey.Hex)
		if err == nil {
			gf.AddAddrDesc(b, tx)
		}
	}
	fb := gf.Compute()
	return hex.EncodeToString(fb)
}

func (m *MempoolBitcoinType) getTxAddrs(txid string, tx *Tx, chanInput chan chanInputPayload, chanResult chan *addrIndex) ([]addrIndex, string, bool) {
	if tx == nil {
		var err error
		tx, err = m.chain.GetTransactionForMempool(txid)
		if err != nil {
			glog.Error("cannot get transaction ", txid, ": ", err)
			return nil, "", false
		}
	}
	glog.V(2).Info("mempool: gettxaddrs ", txid, ", ", len(tx.Vin), " inputs")
	mtx := m.txToMempoolTx(tx)
	io := make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
	cache := m.getResyncOutpointCache()
	for _, output := range tx.Vout {
		addrDesc, err := m.chain.GetChainParser().GetAddrDescFromVout(&output)
		if err != nil {
			glog.Error("error in addrDesc in ", txid, " ", output.N, ": ", err)
			continue
		}
		if cache != nil {
			cache.set(Outpoint{txid, int32(output.N)}, addrDesc, &output.ValueSat)
		}
		if len(addrDesc) > 0 {
			io = append(io, addrIndex{string(addrDesc), int32(output.N)})
		}
		if m.OnNewTxAddr != nil {
			m.OnNewTxAddr(tx, addrDesc)
		}
	}
	dispatched := 0
	for i := range tx.Vin {
		input := &tx.Vin[i]
		if input.Coinbase != "" {
			continue
		}
		payload := chanInputPayload{mtx, i}
	loop:
		for {
			select {
			// store as many processed results as possible
			case ai := <-chanResult:
				if ai != nil {
					io = append(io, *ai)
				}
				dispatched--
			// send input to be processed
			case chanInput <- payload:
				dispatched++
				break loop
			}
		}
	}
	for i := 0; i < dispatched; i++ {
		ai := <-chanResult
		if ai != nil {
			io = append(io, *ai)
		}
	}
	var golombFilter string
	if m.golombFilterP > 0 {
		golombFilter = m.computeGolombFilter(mtx, tx)
	}
	if m.OnNewTx != nil {
		m.OnNewTx(mtx)
	}
	return io, golombFilter, true
}

func (m *MempoolBitcoinType) dispatchResyncPayloads(txids []string, cache map[string]*Tx, txTime uint32, onNewEntry func(txid string, entry txEntry)) {
	dispatched := 0
	for _, txid := range txids {
		var tx *Tx
		if cache != nil {
			tx = cache[txid]
		}
	sendLoop:
		for {
			select {
			// store as many processed transactions as possible
			case tio := <-m.chanAddrIndex:
				onNewEntry(tio.txid, txEntry{tio.io, txTime, tio.filter})
				dispatched--
			// send transaction to be processed
			case m.chanTx <- txPayload{txid: txid, tx: tx}:
				dispatched++
				break sendLoop
			}
		}
	}
	for i := 0; i < dispatched; i++ {
		tio := <-m.chanAddrIndex
		onNewEntry(tio.txid, txEntry{tio.io, txTime, tio.filter})
	}
}

func (m *MempoolBitcoinType) resyncBatchedMissing(missing []string, batcher MempoolBatcher, batchSize int, txTime uint32, onNewEntry func(txid string, entry txEntry)) (int, error) {
	if len(missing) == 0 {
		return 0, nil
	}
	type batchResult struct {
		txids []string
		cache map[string]*Tx
		err   error
	}
	batchCount := (len(missing) + batchSize - 1) / batchSize
	batchWorkers := m.resyncBatchWorkers
	if batchWorkers < 1 {
		batchWorkers = 1
	}
	if batchWorkers > batchCount {
		batchWorkers = batchCount
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batchJobs := make(chan []string)
	// Buffer results so up to batchWorkers RPC calls can run in parallel.
	batchResults := make(chan batchResult, batchWorkers)
	var wg sync.WaitGroup
	for i := 0; i < batchWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case batch, ok := <-batchJobs:
					if !ok {
						return
					}
					cache, err := batcher.GetRawTransactionsForMempoolBatch(batch)
					select {
					case <-ctx.Done():
						return
					case batchResults <- batchResult{txids: batch, cache: cache, err: err}:
					}
					if err != nil {
						return
					}
				}
			}
		}()
	}

	go func() {
		defer close(batchJobs)
		for start := 0; start < len(missing); start += batchSize {
			end := start + batchSize
			if end > len(missing) {
				end = len(missing)
			}
			batch := missing[start:end]
			select {
			case <-ctx.Done():
				return
			case batchJobs <- batch:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(batchResults)
	}()

	var batchErr error
	for batch := range batchResults {
		if batch.err != nil {
			if batchErr == nil {
				// Fail fast to avoid mixing partial batch results with per-tx fetches.
				batchErr = batch.err
				cancel()
			}
			continue
		}
		if batchErr != nil {
			// Drain remaining results after failure to let fetchers exit cleanly.
			continue
		}
		m.dispatchResyncPayloads(batch.txids, batch.cache, txTime, onNewEntry)
	}
	if batchErr != nil {
		return batchWorkers, batchErr
	}
	return batchWorkers, nil
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *MempoolBitcoinType) Resync() (count int, err error) {
	start := time.Now()
	var (
		mempoolSize          int
		missingCount         int
		outpointCacheEntries int
		batchSize            int
		batchWorkers         int
		listDuration         time.Duration
		processDuration      time.Duration
		processStart         time.Time
	)
	// Log metrics on every exit path to make bottlenecks visible even on errors.
	defer func() {
		if !processStart.IsZero() && processDuration == 0 {
			processDuration = time.Since(processStart)
		}
		totalDuration := time.Since(start)
		avgPerTx := time.Duration(0)
		if mempoolSize > 0 {
			avgPerTx = totalDuration / time.Duration(mempoolSize)
		}
		throughput := 0.0
		if seconds := totalDuration.Seconds(); seconds > 0 {
			throughput = float64(mempoolSize) / seconds
		}
		var cacheHits uint64
		var cacheMisses uint64
		var cacheHitRate float64
		if cache := m.getResyncOutpointCache(); cache != nil {
			outpointCacheEntries = cache.len()
			cacheHits, cacheMisses = cache.stats()
			total := cacheHits + cacheMisses
			if total > 0 {
				cacheHitRate = float64(cacheHits) / float64(total)
			}
		}
		listDurationRounded := roundDuration(listDuration, time.Millisecond)
		processDurationRounded := roundDuration(processDuration, time.Millisecond)
		totalDurationRounded := roundDuration(totalDuration, time.Millisecond)
		avgPerTxRounded := roundDuration(avgPerTx, time.Microsecond)
		hitRateText := fmt.Sprintf("%.3f", cacheHitRate)
		throughputText := fmt.Sprintf("%.3f", throughput)
		if err != nil {
			glog.Warning("mempool: resync failed size=", mempoolSize, " missing=", missingCount, " outpoint_cache_entries=", outpointCacheEntries, " outpoint_cache_hits=", cacheHits, " outpoint_cache_misses=", cacheMisses, " outpoint_cache_hit_rate=", hitRateText, " batch_size=", batchSize, " batch_workers=", batchWorkers, " list_duration=", listDurationRounded, " process_duration=", processDurationRounded, " duration=", totalDurationRounded, " avg_per_tx=", avgPerTxRounded, " throughput_txs_per_second=", throughputText, " err=", err)
		} else {
			glog.Info("mempool: resync finished size=", mempoolSize, " missing=", missingCount, " outpoint_cache_entries=", outpointCacheEntries, " outpoint_cache_hits=", cacheHits, " outpoint_cache_misses=", cacheMisses, " outpoint_cache_hit_rate=", hitRateText, " batch_size=", batchSize, " batch_workers=", batchWorkers, " list_duration=", listDurationRounded, " process_duration=", processDurationRounded, " duration=", totalDurationRounded, " avg_per_tx=", avgPerTxRounded, " throughput_txs_per_second=", throughputText)
		}
		m.resyncOutpoints.Store((*resyncOutpointCache)(nil))
	}()

	glog.V(1).Info("mempool: resync")
	listStart := time.Now()
	txs, err := m.chain.GetMempoolTransactions()
	listDuration = time.Since(listStart)
	if err != nil {
		return 0, err
	}
	mempoolSize = len(txs)
	m.resyncOutpoints.Store(newResyncOutpointCache(mempoolSize))
	glog.V(2).Info("mempool: resync ", len(txs), " txs")
	onNewEntry := func(txid string, entry txEntry) {
		if len(entry.addrIndexes) > 0 {
			m.mux.Lock()
			m.txEntries[txid] = entry
			for _, si := range entry.addrIndexes {
				m.addrDescToTx[si.addrDesc] = append(m.addrDescToTx[si.addrDesc], Outpoint{txid, si.n})
			}
			m.mux.Unlock()
		}
	}
	txsMap := make(map[string]struct{}, len(txs))
	txTime := uint32(time.Now().Unix())
	missing := make([]string, 0, len(txs))
	for _, txid := range txs {
		txsMap[txid] = struct{}{}
		_, exists := m.txEntries[txid]
		if !exists {
			missing = append(missing, txid)
		}
	}
	missingCount = len(missing)

	batchSize = m.resyncBatchSize
	if batchSize < 1 {
		batchSize = 1
	}
	var batcher MempoolBatcher
	if batchSize > 1 {
		var ok bool
		batcher, ok = m.chain.(MempoolBatcher)
		if !ok {
			// Fail fast so operators notice unsupported batch backends early.
			return 0, errors.New("mempool: batch resync requested but backend does not support batch fetch")
		}
	}

	processStart = time.Now()
	if batchSize == 1 {
		// get transaction in parallel using goroutines created in NewUTXOMempool
		m.dispatchResyncPayloads(missing, nil, txTime, onNewEntry)
	} else {
		var batchErr error
		batchWorkers, batchErr = m.resyncBatchedMissing(missing, batcher, batchSize, txTime, onNewEntry)
		if batchErr != nil {
			return 0, batchErr
		}
	}

	for txid, entry := range m.txEntries {
		if _, exists := txsMap[txid]; !exists {
			m.mux.Lock()
			m.removeEntryFromMempool(txid, entry)
			m.mux.Unlock()
		}
	}
	processDuration = time.Since(processStart)
	count = len(m.txEntries)
	return count, nil
}

// GetTxidFilterEntries returns all mempool entries with golomb filter from
func (m *MempoolBitcoinType) GetTxidFilterEntries(filterScripts string, fromTimestamp uint32) (MempoolTxidFilterEntries, error) {
	if m.filterScripts != filterScripts {
		return MempoolTxidFilterEntries{}, errors.Errorf("Unsupported script filter %s", filterScripts)
	}
	m.mux.Lock()
	entries := make(map[string]string)
	for txid, entry := range m.txEntries {
		if entry.filter != "" && entry.time >= fromTimestamp {
			entries[txid] = entry.filter
		}
	}
	m.mux.Unlock()
	return MempoolTxidFilterEntries{entries, m.useZeroedKey}, nil
}
