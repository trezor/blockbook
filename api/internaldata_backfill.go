package api

import (
	"bytes"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// InternalDataHealingRoutine keeps the internal data of the index complete. It
// combines two mechanisms: a periodic, always-on retry of the internal data error
// queue (blocks whose internal data fetch failed during sync or healing), and a
// downward sweep that backfills blocks synced before processInternalTransactions was
// enabled, without a full reindex. The sweep runs only when an operator seeded the
// watermark (the -healinternaldata flag, see RocksDB.ResolveInternalDataFrom) or when
// a previous sweep was interrupted; its range is the internal state watermark, and it
// sweeps downward - healing the most recent history first and moving the watermark
// after every block. Progress is persisted with the periodic internal state store, so
// an interrupted run resumes after restart; re-healing a block is safe - transactions
// whose stored internal data already match the recomputed data are skipped.
func (w *Worker) InternalDataHealingRoutine() {
	if w.chainType != bchain.ChainEthereumType || !bchain.ProcessInternalTransactions {
		return
	}
	// blocks whose internal data fetch failed - during sync or during the healing
	// sweep below - wait in the internal data error queue; retry them periodically
	// so that gaps close without operator action
	go w.internalDataErrorRetryLoop()
	from, resolved := w.is.GetInternalDataFrom()
	if !resolved || from == 0 {
		return
	}
	glog.Infof("internalDataHealing: index is missing internal data below height %d, backfilling from the backend", from)
	start := time.Now()
	var healed, errorCount uint64
	for h := from; h > 0; {
		if common.IsInShutdown() {
			glog.Infof("internalDataHealing: shutdown at height %d, healing resumes after restart", h)
			return
		}
		// do not compete with the sync for the backend while catching up
		if synced, _, _, _ := w.is.GetSyncState(); !synced {
			time.Sleep(10 * time.Second)
			continue
		}
		h--
		if err := w.backfillBlockInternalData(h); err != nil {
			errorCount++
			glog.Errorf("internalDataHealing: height %d: %v", h, err)
			w.storeInternalDataError(h, err)
		}
		w.is.SetInternalDataFrom(h)
		healed++
		if healed%10000 == 0 {
			rate := float64(healed) / time.Since(start).Seconds()
			glog.Infof("internalDataHealing: %d blocks healed, at height %d, %.1f blocks/s, %d errors", healed, h, rate, errorCount)
		}
	}
	glog.Infof("internalDataHealing: finished, %d blocks healed in %v, %d errors", healed, time.Since(start), errorCount)
}

// storeInternalDataError puts a block that failed to heal into the same internal
// data error queue that sync-time failures use, so it is retried by the periodic
// refetch and visible on the internal data errors admin page. A successful
// reconnect removes the entry from the queue.
func (w *Worker) storeInternalDataError(height uint32, healErr error) {
	hash, err := w.db.GetBlockHash(height)
	if err != nil || hash == "" {
		return
	}
	w.storeBlockInternalDataError(hash, height, healErr.Error(), 0)
}

const internalDataErrorRetryPeriod = time.Hour

// internalDataErrorRetryLoop periodically retries the blocks in the internal data
// error queue. The underlying refetch routine runs at most once at a time, is a
// no-op on an empty queue and gives up on a block after its retry limit, so the
// periodic trigger is safe. The first pass runs shortly after startup to pick up
// failures from the previous run while the backend cache is warm.
func (w *Worker) internalDataErrorRetryLoop() {
	period := time.Minute
	for {
		time.Sleep(period)
		period = internalDataErrorRetryPeriod
		if common.IsInShutdown() {
			return
		}
		// periodic auto-retry keeps the retry cap so it never hammers a permanently
		// unfetchable block; an operator can force a reset via the admin page
		if err := w.RefetchInternalData(false); err != nil {
			glog.Errorf("internalDataErrorRetryLoop: %v", err)
		}
	}
}

func (w *Worker) backfillBlockInternalData(height uint32) error {
	hash, err := w.db.GetBlockHash(height)
	if err != nil {
		return err
	}
	if hash == "" {
		return errors.New("block not found in index")
	}
	block, err := w.getBlockRetryInternalData(hash, height)
	if err != nil {
		return err
	}
	if block == nil {
		return errors.New("block not found on backend")
	}
	blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
	if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
		return errors.New(blockSpecificData.InternalDataError)
	}
	// guard against a reorg between reading the indexed hash and the backend fetch
	if currentHash, err := w.db.GetBlockHash(height); err != nil || currentHash != hash {
		return errors.New("block hash changed, possible reorg, skipping")
	}
	w.filterAlreadyReconnectedInternalData(block)
	if err := w.db.ReconnectInternalDataToBlockEthereumType(block); err != nil {
		return err
	}
	if blockSpecificData != nil {
		w.storeBackfilledContracts(blockSpecificData)
	}
	return nil
}

// filterAlreadyReconnectedInternalData clears the internal data of transactions that
// need no backfill - either there is nothing to store or the DB already holds matching
// data (from sync or a previous backfill run). ReconnectInternalDataToBlockEthereumType
// skips transactions with nil internal data; without this filter a repeated run over
// the same block would increment the contract transaction counters again.
func (w *Worker) filterAlreadyReconnectedInternalData(block *bchain.Block) {
	for i := range block.Txs {
		tx := &block.Txs[i]
		eid, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
		if !ok || eid.InternalData == nil {
			continue
		}
		drop := emptyInternalData(eid.InternalData)
		if !drop {
			stored, err := w.db.GetEthereumInternalData(tx.Txid)
			drop = err == nil && stored != nil && internalDataEqual(w.chainParser, stored, eid.InternalData)
		}
		if drop {
			eid.InternalData = nil
			tx.CoinSpecificData = eid
		}
	}
}

func emptyInternalData(id *bchain.EthereumInternalData) bool {
	return id.Type == bchain.CALL && len(id.Transfers) == 0 && id.Error == ""
}

// internalDataEqual compares stored internal data with freshly computed data.
// The stored form is lossy: the top-level type keeps only the CALL|CREATE bit
// (SELFDESTRUCT unpacks as CALL) and the error message is transformed on unpack,
// so only the CREATE distinction is compared and errors are ignored. Addresses
// are compared as address descriptors - the stored side unpacks to the chain's
// canonical form (EIP55 checksummed hex on eth) while the computed side may
// keep the backend's casing (the trace's Contract is raw lowercase hex).
func internalDataEqual(parser bchain.BlockChainParser, stored, computed *bchain.EthereumInternalData) bool {
	if (stored.Type == bchain.CREATE) != (computed.Type == bchain.CREATE) {
		return false
	}
	if stored.Type == bchain.CREATE && !addressesEqual(parser, stored.Contract, computed.Contract) {
		return false
	}
	if len(stored.Transfers) != len(computed.Transfers) {
		return false
	}
	for i := range stored.Transfers {
		s, c := &stored.Transfers[i], &computed.Transfers[i]
		if s.Type != c.Type || !addressesEqual(parser, s.From, c.From) || !addressesEqual(parser, s.To, c.To) || s.Value.Cmp(&c.Value) != 0 {
			return false
		}
	}
	return true
}

// addressesEqual reports whether two address strings denote the same address,
// comparing their address descriptors when the strings differ. An address that
// does not parse never equals anything but its exact string form. The packed
// DB form stores a missing address as the zero address, so an empty address
// equals the chain's representation of the zero address.
func addressesEqual(parser bchain.BlockChainParser, a, b string) bool {
	if a == b {
		return true
	}
	if a == "" {
		return isZeroAddress(parser, b)
	}
	if b == "" {
		return isZeroAddress(parser, a)
	}
	da, err := parser.GetAddrDescFromAddress(a)
	if err != nil {
		return false
	}
	db, err := parser.GetAddrDescFromAddress(b)
	if err != nil {
		return false
	}
	return bytes.Equal(da, db)
}

func isZeroAddress(parser bchain.BlockChainParser, a string) bool {
	d, err := parser.GetAddrDescFromAddress(a)
	if err != nil {
		return false
	}
	for _, b := range d {
		if b != 0 {
			return false
		}
	}
	return true
}

// storeBackfilledContracts persists contract registry entries discovered during the
// backfill; ReconnectInternalDataToBlockEthereumType does not store them. Creations
// are stored only when the registry has no entry yet, so that contract info enriched
// after sync is not overwritten. Destructions reuse the StoreContractInfo merge,
// which is idempotent and a no-op for unknown contracts.
func (w *Worker) storeBackfilledContracts(blockSpecificData *bchain.EthereumBlockSpecificData) {
	store := func(ci *bchain.ContractInfo) {
		if err := w.db.StoreContractInfo(ci); err != nil {
			glog.Errorf("storeBackfilledContracts: StoreContractInfo %s: %v", ci.Contract, err)
		}
	}
	for i := range blockSpecificData.Contracts {
		ci := &blockSpecificData.Contracts[i]
		if ci.CreatedInBlock == 0 && ci.DestructedInBlock != 0 {
			store(ci)
			continue
		}
		existing, err := w.db.GetContractInfoForAddress(ci.Contract)
		if err != nil {
			glog.Errorf("storeBackfilledContracts: GetContractInfoForAddress %s: %v", ci.Contract, err)
			continue
		}
		if existing == nil {
			store(ci)
		}
	}
}
