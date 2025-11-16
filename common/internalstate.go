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

// SetBackendInfo sets new BackendInfo
func (is *InternalState) SetBackendInfo(bi *BackendInfo) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.BackendInfo = *bi
}

// GetBackendInfo gets BackendInfo
func (is *InternalState) GetBackendInfo() BackendInfo {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.BackendInfo
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
