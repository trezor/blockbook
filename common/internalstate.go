package common

import (
	"encoding/json"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

const (
	// DbStateClosed means db was closed gracefully
	DbStateClosed = uint32(iota)
	// DbStateOpen means db is open or application died without closing the db
	DbStateOpen
	// DbStateInconsistent means db is in inconsistent state and cannot be used
	DbStateInconsistent
)

var inShutdown int32

// InternalStateColumn contains the data of a db column
type InternalStateColumn struct {
	Name       string    `json:"name" ts_doc:"Name of the database column."`
	Version    uint32    `json:"version" ts_doc:"Version or schema version of the column."`
	Rows       int64     `json:"rows" ts_doc:"Number of rows stored in this column."`
	KeyBytes   int64     `json:"keyBytes" ts_doc:"Total size (in bytes) of keys stored in this column."`
	ValueBytes int64     `json:"valueBytes" ts_doc:"Total size (in bytes) of values stored in this column."`
	Updated    time.Time `json:"updated" ts_doc:"Timestamp of the last update to this column."`
}

// BackendInfo is used to get information about blockchain
type BackendInfo struct {
	BackendError     string      `json:"error,omitempty" ts_doc:"Error message if something went wrong in the backend."`
	Chain            string      `json:"chain,omitempty" ts_doc:"Name of the chain - e.g. 'main'."`
	Blocks           int         `json:"blocks,omitempty" ts_doc:"Number of fully verified blocks in the chain."`
	Headers          int         `json:"headers,omitempty" ts_doc:"Number of block headers in the chain."`
	BestBlockHash    string      `json:"bestBlockHash,omitempty" ts_doc:"Hash of the best block in hex."`
	Difficulty       string      `json:"difficulty,omitempty" ts_doc:"Current difficulty of the network."`
	SizeOnDisk       int64       `json:"sizeOnDisk,omitempty" ts_doc:"Size of the blockchain data on disk in bytes."`
	Version          string      `json:"version,omitempty" ts_doc:"Version of the blockchain backend - e.g. '280000'."`
	Subversion       string      `json:"subversion,omitempty" ts_doc:"Subversion of the blockchain backend - e.g. '/Satoshi:28.0.0/'."`
	ProtocolVersion  string      `json:"protocolVersion,omitempty" ts_doc:"Protocol version of the blockchain backend - e.g. '70016'."`
	Timeoffset       float64     `json:"timeOffset,omitempty" ts_doc:"Time offset (in seconds) reported by the backend."`
	Warnings         string      `json:"warnings,omitempty" ts_doc:"Any warnings given by the backend regarding the chain state."`
	ConsensusVersion string      `json:"consensus_version,omitempty" ts_doc:"Version or details of the consensus protocol in use."`
	Consensus        interface{} `json:"consensus,omitempty" ts_doc:"Additional chain-specific consensus data."`
}

// InternalState contains the data of the internal state
type InternalState struct {
	mux sync.Mutex `ts_doc:"Mutex for synchronized access to the internal state."`

	Coin         string `json:"coin" ts_doc:"Coin name (e.g. 'Bitcoin')."`
	CoinShortcut string `json:"coinShortcut" ts_doc:"Short code for the coin (e.g. 'BTC')."`
	CoinLabel    string `json:"coinLabel" ts_doc:"Human-readable label for the coin (e.g. 'Bitcoin main')."`
	Host         string `json:"host" ts_doc:"Hostname of the node or backend."`
	Network      string `json:"network,omitempty" ts_doc:"Network name if different from CoinShortcut (e.g. 'testnet')."`

	DbState       uint32 `json:"dbState" ts_doc:"State of the database (closed=0, open=1, inconsistent=2)."`
	ExtendedIndex bool   `json:"extendedIndex" ts_doc:"Indicates if an extended indexing strategy is used."`

	LastStore time.Time `json:"lastStore" ts_doc:"Time when the internal state was last stored/persisted."`

	// true if application is with flag --sync
	SyncMode bool `json:"syncMode" ts_doc:"Flag indicating if the node is in sync mode."`

	InitialSync    bool      `json:"initialSync" ts_doc:"If true, the system is in the initial sync phase."`
	IsSynchronized bool      `json:"isSynchronized" ts_doc:"If true, the main index is fully synced to BestHeight."`
	BestHeight     uint32    `json:"bestHeight" ts_doc:"Current best block height known to the indexer."`
	StartSync      time.Time `json:"-" ts_doc:"Timestamp when sync started (not exposed via JSON)."`
	LastSync       time.Time `json:"lastSync" ts_doc:"Timestamp of the last successful sync."`
	BlockTimes     []uint32  `json:"-" ts_doc:"List of block timestamps (per height) for calculating historical stats (not exposed via JSON)."`
	AvgBlockPeriod uint32    `json:"-" ts_doc:"Average time (in seconds) per block for the last 100 blocks (not exposed via JSON)."`

	IsMempoolSynchronized bool      `json:"isMempoolSynchronized" ts_doc:"If true, mempool data is in sync."`
	MempoolSize           int       `json:"mempoolSize" ts_doc:"Number of transactions in the current mempool."`
	LastMempoolSync       time.Time `json:"lastMempoolSync" ts_doc:"Timestamp of the last mempool sync."`

	DbColumns []InternalStateColumn `json:"dbColumns" ts_doc:"List of database column statistics."`

	HasFiatRates                 bool      `json:"-" ts_doc:"True if fiat rates are supported (not exposed via JSON)."`
	HasTokenFiatRates            bool      `json:"-" ts_doc:"True if token fiat rates are supported (not exposed via JSON)."`
	HistoricalFiatRatesTime      time.Time `json:"historicalFiatRatesTime" ts_doc:"Timestamp of the last historical fiat rates update."`
	HistoricalTokenFiatRatesTime time.Time `json:"historicalTokenFiatRatesTime" ts_doc:"Timestamp of the last historical token fiat rates update."`

	EnableSubNewTx bool `json:"-" ts_doc:"Internal flag controlling subscription to new transactions (not exposed)."`

	BackendInfo BackendInfo `json:"-" ts_doc:"Information about the connected blockchain backend (not exposed in JSON)."`

	BackendTipLastAdvance time.Time `json:"-" ts_doc:"Wall-clock time when BackendInfo.Blocks was last observed to advance (not exposed in JSON)."`

	// database migrations
	UtxoChecked            bool `json:"utxoChecked" ts_doc:"Indicates if UTXO consistency checks have been performed."`
	SortedAddressContracts bool `json:"sortedAddressContracts" ts_doc:"Indicates if address/contract sorting has been completed."`

	// golomb filter settings
	BlockGolombFilterP      uint8  `json:"block_golomb_filter_p" ts_doc:"Parameter P for building Golomb-Rice filters for blocks."`
	BlockFilterScripts      string `json:"block_filter_scripts" ts_doc:"Scripts included in block filters (e.g., 'p2pkh,p2sh')."`
	BlockFilterUseZeroedKey bool   `json:"block_filter_use_zeroed_key" ts_doc:"If true, uses a zeroed key for building block filters."`

	// allowed number of fetched accounts over websocket
	WsGetAccountInfoLimit int            `json:"-" ts_doc:"Limit of how many getAccountInfo calls can be made via WS (not exposed)."`
	WsLimitExceedingIPs   map[string]int `json:"-" ts_doc:"Tracks IP addresses exceeding the WS limit (not exposed)."`

	// BalanceHistoryMaxTxsWS / BalanceHistoryMaxTxsREST cap how many transactions a
	// single balance-history request (address or xpub) may aggregate; each costs a DB
	// read, so this bounds a cheap-to-send DoS and errors past the cap (0 = unlimited).
	// WS is generous (Trezor Suite's full-history graph); REST is tighter (open surface).
	BalanceHistoryMaxTxsWS   int `json:"-" ts_doc:"Max transactions aggregated per WS balance-history request, 0 = unlimited (not exposed)."`
	BalanceHistoryMaxTxsREST int `json:"-" ts_doc:"Max transactions aggregated per REST balance-history request, 0 = unlimited (not exposed)."`

	// websocket IP blocklist: keys (IPv4 address or full IPv6 /128) blocked from
	// opening new connections after flooding a single connection past the message-rate
	// limit. Keyed on the /128 (not the limiter's /64) so a block cannot take out a
	// shared /64. Guarded by its own mutex (consulted on every connection attempt).
	wsBlockMux   sync.Mutex
	wsBlockedIPs map[string]*WsBlockedIP

	// rpcCallAllowlists is the effective websocket rpcCall allowlist snapshot,
	// resolved from the DB runtime-setting overrides and the environment
	// defaults (see server initRpcCallAllowlists). Unexported so it is never
	// serialized by Pack; replaced wholesale via the accessors below, giving
	// the rpcCall hot path a lock-free, consistent view.
	rpcCallAllowlists atomic.Pointer[RpcCallAllowlists]
}

// Sources of a runtime setting value, reported by the /admin runtime-settings
// interface.
const (
	RuntimeSettingSourceUnset = "unset"
	RuntimeSettingSourceEnv   = "env"
	RuntimeSettingSourceDB    = "db"
)

// RpcCallAllowlists is an immutable snapshot of the websocket rpcCall
// allowlists. Readers must not mutate the maps; writers build a new snapshot
// and replace it via SetRpcCallAllowlists.
type RpcCallAllowlists struct {
	// To and Methods are the parsed allowlists; a nil map means that dimension
	// is unconfigured. With both nil, rpcCall is unrestricted.
	//
	// Each dimension's key format must match what the rpcCall check looks up
	// (server rpcCallAllowed), and the two deliberately differ:
	//   - To keys are the configured entries trimmed and lowercased verbatim —
	//     in practice 0x-prefixed hex addresses, because they are matched
	//     against the lowercased `to` field of the request as sent by clients.
	//   - Methods keys are 4-byte selectors as 8 lowercase hex characters
	//     without the 0x prefix, because they are matched against selectors
	//     hex-decoded from the request calldata (server evmCallSelector).
	// A future dimension should document its key format here and keep the
	// parser (server runtimeSettingDefs) and the rpcCall lookup in lockstep.
	To      map[string]struct{}
	Methods map[string]struct{}
	// Raw comma-separated values and their sources (unset/env/db), kept for
	// the admin interface and logging.
	ToValue       string
	MethodsValue  string
	ToSource      string
	MethodsSource string
}

// GetRpcCallAllowlists returns the current rpcCall allowlist snapshot, nil
// when not yet initialized.
func (is *InternalState) GetRpcCallAllowlists() *RpcCallAllowlists {
	return is.rpcCallAllowlists.Load()
}

// SetRpcCallAllowlists atomically replaces the rpcCall allowlist snapshot.
func (is *InternalState) SetRpcCallAllowlists(a *RpcCallAllowlists) {
	is.rpcCallAllowlists.Store(a)
}

// InitRpcCallAllowlists publishes the initial snapshot only when none exists
// yet and reports whether it did. The compare-and-swap keeps a snapshot that
// was already published — possibly updated by the admin interface in the
// meantime — intact when several server constructors initialize in any order.
func (is *InternalState) InitRpcCallAllowlists(a *RpcCallAllowlists) bool {
	return is.rpcCallAllowlists.CompareAndSwap(nil, a)
}

// WsBlockedIP records a websocket client key (IPv4 address or full IPv6 /128)
// that is temporarily blocked from opening new websocket connections.
type WsBlockedIP struct {
	Key       string    `ts_doc:"Blocked client key: an IPv4 address or a full IPv6 /128 address."`
	BlockedAt time.Time `ts_doc:"Time the key was first blocked in the current block."`
	Until     time.Time `ts_doc:"Time the block expires."`
	Breaches  int       `ts_doc:"How many times this key tripped the per-connection message rate limit."`
	Rejected  int       `ts_doc:"How many new connections were rejected while the key was blocked."`
}

// StartedSync signals start of synchronization
func (is *InternalState) StartedSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.StartSync = time.Now().UTC()
	is.IsSynchronized = false
}

// FinishedSync marks end of synchronization, bestHeight specifies new best block height
func (is *InternalState) FinishedSync(bestHeight uint32) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = true
	is.BestHeight = bestHeight
	is.LastSync = time.Now().UTC()
}

// UpdateBestHeight sets new best height, without changing IsSynchronized flag
func (is *InternalState) UpdateBestHeight(bestHeight uint32) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.BestHeight = bestHeight
	is.LastSync = time.Now().UTC()
}

// FinishedSyncNoChange marks end of synchronization in case no index update was necessary, it does not update lastSync time
func (is *InternalState) FinishedSyncNoChange() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = true
}

// GetSyncState gets the state of synchronization
func (is *InternalState) GetSyncState() (bool, uint32, time.Time, time.Time) {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.IsSynchronized, is.BestHeight, is.LastSync, is.StartSync
}

// StartedMempoolSync signals start of mempool synchronization
func (is *InternalState) StartedMempoolSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsMempoolSynchronized = false
}

// FinishedMempoolSync marks end of mempool synchronization
func (is *InternalState) FinishedMempoolSync(mempoolSize int) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsMempoolSynchronized = true
	is.MempoolSize = mempoolSize
	is.LastMempoolSync = time.Now()
}

// GetMempoolSyncState gets the state of mempool synchronization
func (is *InternalState) GetMempoolSyncState() (bool, time.Time, int) {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.IsMempoolSynchronized, is.LastMempoolSync, is.MempoolSize
}

// AddDBColumnStats adds differences in column statistics to column stats
func (is *InternalState) AddDBColumnStats(c int, rowsDiff int64, keyBytesDiff int64, valueBytesDiff int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	dc := &is.DbColumns[c]
	dc.Rows += rowsDiff
	dc.KeyBytes += keyBytesDiff
	dc.ValueBytes += valueBytesDiff
	dc.Updated = time.Now()
}

// SetDBColumnStats sets new values of column stats
func (is *InternalState) SetDBColumnStats(c int, rows int64, keyBytes int64, valueBytes int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	dc := &is.DbColumns[c]
	dc.Rows = rows
	dc.KeyBytes = keyBytes
	dc.ValueBytes = valueBytes
	dc.Updated = time.Now()
}

// GetDBColumnStatValues gets stat values for given column
func (is *InternalState) GetDBColumnStatValues(c int) (int64, int64, int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	if c < len(is.DbColumns) {
		return is.DbColumns[c].Rows, is.DbColumns[c].KeyBytes, is.DbColumns[c].ValueBytes
	}
	return 0, 0, 0
}

// GetAllDBColumnStats returns stats for all columns
func (is *InternalState) GetAllDBColumnStats() []InternalStateColumn {
	is.mux.Lock()
	defer is.mux.Unlock()
	return slices.Clone(is.DbColumns)
}

// DBSizeTotal sums the computed sizes of all columns
func (is *InternalState) DBSizeTotal() int64 {
	is.mux.Lock()
	defer is.mux.Unlock()
	total := int64(0)
	for _, c := range is.DbColumns {
		total += c.KeyBytes + c.ValueBytes
	}
	return total
}

// GetBlockTime returns block time if block found or 0
func (is *InternalState) GetBlockTime(height uint32) uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	if int(height) < len(is.BlockTimes) {
		return is.BlockTimes[height]
	}
	return 0
}

// GetLastBlockTime returns time of the last block
func (is *InternalState) GetLastBlockTime() uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	if len(is.BlockTimes) > 0 {
		return is.BlockTimes[len(is.BlockTimes)-1]
	}
	return 0
}

// SetBlockTimes initializes BlockTimes array, returns AvgBlockPeriod
func (is *InternalState) SetBlockTimes(blockTimes []uint32) uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	if len(is.BlockTimes) < len(blockTimes) {
		// no new block was set
		is.BlockTimes = blockTimes
	} else {
		copy(is.BlockTimes, blockTimes)
	}
	is.computeAvgBlockPeriod()
	glog.Info("set ", len(is.BlockTimes), " block times, average block period ", is.AvgBlockPeriod, "s")
	return is.AvgBlockPeriod
}

// SetBlockTime sets block time to BlockTimes, allocating the slice as necessary, returns AvgBlockPeriod
func (is *InternalState) SetBlockTime(height uint32, time uint32) uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	if int(height) >= len(is.BlockTimes) {
		extend := int(height) - len(is.BlockTimes) + 1
		for i := 0; i < extend; i++ {
			is.BlockTimes = append(is.BlockTimes, time)
		}
	} else {
		is.BlockTimes[height] = time
	}
	is.computeAvgBlockPeriod()
	return is.AvgBlockPeriod
}

// RemoveLastBlockTimes removes last times from BlockTimes
func (is *InternalState) RemoveLastBlockTimes(count int) {
	is.mux.Lock()
	defer is.mux.Unlock()
	if len(is.BlockTimes) < count {
		count = len(is.BlockTimes)
	}
	is.BlockTimes = is.BlockTimes[:len(is.BlockTimes)-count]
	is.computeAvgBlockPeriod()
}

// GetBlockHeightOfTime returns block height of the first block with time greater or equal to the given time or MaxUint32 if no such block
func (is *InternalState) GetBlockHeightOfTime(time uint32) uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	height := sort.Search(len(is.BlockTimes), func(i int) bool { return time <= is.BlockTimes[i] })
	if height == len(is.BlockTimes) {
		return ^uint32(0)
	}
	// as the block times can sometimes be out of order try 20 blocks lower to locate a block with the time greater or equal to the given time
	max, height := height, height-20
	if height < 0 {
		height = 0
	}
	for ; height <= max; height++ {
		if time <= is.BlockTimes[height] {
			break
		}
	}
	return uint32(height)
}

const avgBlockPeriodSample = 100

// Avg100BlocksPeriod returns average period of the last 100 blocks in seconds
func (is *InternalState) GetAvgBlockPeriod() uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.AvgBlockPeriod
}

// computeAvgBlockPeriod returns computes average of the last 100 blocks in seconds
func (is *InternalState) computeAvgBlockPeriod() {
	last := len(is.BlockTimes) - 1
	first := last - avgBlockPeriodSample - 1
	if first < 0 {
		return
	}
	is.AvgBlockPeriod = (is.BlockTimes[last] - is.BlockTimes[first]) / avgBlockPeriodSample
}

// GetNetwork returns network. If not set returns the same value as CoinShortcut
func (is *InternalState) GetNetwork() string {
	network := is.Network
	if network == "" {
		return is.CoinShortcut
	}
	return network
}

// SetBackendInfo sets new BackendInfo and records the time when Blocks advances.
// On the first observation the advance time is seeded to now so the
// derived tip-age metric reads a meaningful value instead of "since epoch."
func (is *InternalState) SetBackendInfo(bi *BackendInfo) {
	is.mux.Lock()
	defer is.mux.Unlock()
	if bi.Blocks > is.BackendInfo.Blocks || is.BackendTipLastAdvance.IsZero() {
		is.BackendTipLastAdvance = time.Now()
	}
	is.BackendInfo = *bi
}

// GetBackendInfo gets BackendInfo
func (is *InternalState) GetBackendInfo() BackendInfo {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.BackendInfo
}

// GetBackendTipLastAdvance returns the wall-clock time when the backend's
// Blocks height was last observed to advance. BackendTipLastAdvance is not
// persisted, so on startup (before the first SetBackendInfo) it is zero; seed
// it to now on first read so tip-age metrics don't report a bogus huge age.
func (is *InternalState) GetBackendTipLastAdvance() time.Time {
	is.mux.Lock()
	defer is.mux.Unlock()
	if is.BackendTipLastAdvance.IsZero() {
		is.BackendTipLastAdvance = time.Now()
	}
	return is.BackendTipLastAdvance
}

// Pack marshals internal state to json
func (is *InternalState) Pack() ([]byte, error) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.LastStore = time.Now()
	return json.Marshal(is)
}

// UnpackInternalState unmarshals internal state from json
func UnpackInternalState(buf []byte) (*InternalState, error) {
	var is InternalState
	if err := json.Unmarshal(buf, &is); err != nil {
		return nil, err
	}
	return &is, nil
}

// SetInShutdown sets the internal state to in shutdown state
func SetInShutdown() {
	atomic.StoreInt32(&inShutdown, 1)
}

// IsInShutdown returns true if in application shutdown state
func IsInShutdown() bool {
	return atomic.LoadInt32(&inShutdown) != 0
}

func (is *InternalState) AddWsLimitExceedingIP(ip string) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.WsLimitExceedingIPs[ip] = is.WsLimitExceedingIPs[ip] + 1
}

func (is *InternalState) ResetWsLimitExceedingIPs() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.WsLimitExceedingIPs = make(map[string]int)
}

// WsLimitExceedingIPsSnapshot returns a copy of the limit-exceeding counters.
// The map is mutated under is.mux by AddWsLimitExceedingIP, so readers (the
// admin page) must not range over it directly.
func (is *InternalState) WsLimitExceedingIPsSnapshot() map[string]int {
	is.mux.Lock()
	defer is.mux.Unlock()
	out := make(map[string]int, len(is.WsLimitExceedingIPs))
	for k, v := range is.WsLimitExceedingIPs {
		out[k] = v
	}
	return out
}

// BlockWsIP flags a websocket client key (an IPv4 address or full IPv6 /128) as
// blocked until the given time. If the key is already blocked the block is
// extended to the later of the two expirations and the breach counter is
// incremented; otherwise a new entry is created. now is passed in so callers can
// inject a clock in tests.
func (is *InternalState) BlockWsIP(key string, until, now time.Time) {
	is.wsBlockMux.Lock()
	defer is.wsBlockMux.Unlock()
	if is.wsBlockedIPs == nil {
		is.wsBlockedIPs = make(map[string]*WsBlockedIP)
	}
	e := is.wsBlockedIPs[key]
	if e == nil || !now.Before(e.Until) {
		// new block, or the previous one had already expired: reset the window
		e = &WsBlockedIP{Key: key, BlockedAt: now}
		is.wsBlockedIPs[key] = e
	}
	e.Breaches++
	if until.After(e.Until) {
		e.Until = until
	}
}

// IsWsIPBlocked reports whether the key is currently blocked. When blocked it
// records a rejected connection so the admin page can show how much traffic the
// block is shedding, and returns the updated count so the caller can throttle
// per-attempt logging. Expired entries are treated as not blocked (the periodic
// SweepWsBlockedIPs removes them).
func (is *InternalState) IsWsIPBlocked(key string, now time.Time) (bool, int) {
	is.wsBlockMux.Lock()
	defer is.wsBlockMux.Unlock()
	e := is.wsBlockedIPs[key]
	if e == nil || now.Before(e.BlockedAt) {
		return false, 0
	}
	if !now.Before(e.Until) {
		return false, 0
	}
	e.Rejected++
	return true, e.Rejected
}

// SweepWsBlockedIPs removes expired entries and returns the number of keys that
// remain blocked, for the websocket_blocked_ips gauge.
func (is *InternalState) SweepWsBlockedIPs(now time.Time) int {
	is.wsBlockMux.Lock()
	defer is.wsBlockMux.Unlock()
	for key, e := range is.wsBlockedIPs {
		if !now.Before(e.Until) {
			delete(is.wsBlockedIPs, key)
		}
	}
	return len(is.wsBlockedIPs)
}

// WsBlockedIPsSnapshot returns a copy of the currently blocked entries, newest
// expiry first, skipping any that have already expired.
func (is *InternalState) WsBlockedIPsSnapshot(now time.Time) []WsBlockedIP {
	is.wsBlockMux.Lock()
	defer is.wsBlockMux.Unlock()
	out := make([]WsBlockedIP, 0, len(is.wsBlockedIPs))
	for _, e := range is.wsBlockedIPs {
		if !now.Before(e.Until) {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Until.After(out[j].Until)
	})
	return out
}

// ResetWsBlockedIPs clears the websocket IP blocklist.
func (is *InternalState) ResetWsBlockedIPs() {
	is.wsBlockMux.Lock()
	defer is.wsBlockMux.Unlock()
	is.wsBlockedIPs = make(map[string]*WsBlockedIP)
}
