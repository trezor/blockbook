package common

import (
	"encoding/json"
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
	Name       string    `json:"name"`
	Version    uint32    `json:"version"`
	Rows       int64     `json:"rows"`
	KeyBytes   int64     `json:"keyBytes"`
	ValueBytes int64     `json:"valueBytes"`
	Updated    time.Time `json:"updated"`
}

// BackendInfo is used to get information about blockchain
type BackendInfo struct {
	BackendError     string      `json:"error,omitempty"`
	Chain            string      `json:"chain,omitempty"`
	Blocks           int         `json:"blocks,omitempty"`
	Headers          int         `json:"headers,omitempty"`
	BestBlockHash    string      `json:"bestBlockHash,omitempty"`
	Difficulty       string      `json:"difficulty,omitempty"`
	SizeOnDisk       int64       `json:"sizeOnDisk,omitempty"`
	Version          string      `json:"version,omitempty"`
	Subversion       string      `json:"subversion,omitempty"`
	ProtocolVersion  string      `json:"protocolVersion,omitempty"`
	Timeoffset       float64     `json:"timeOffset,omitempty"`
	Warnings         string      `json:"warnings,omitempty"`
	ConsensusVersion string      `json:"consensus_version,omitempty"`
	Consensus        interface{} `json:"consensus,omitempty"`
}

// InternalState contains the data of the internal state
type InternalState struct {
	mux sync.Mutex

	Coin         string `json:"coin"`
	CoinShortcut string `json:"coinShortcut"`
	CoinLabel    string `json:"coinLabel"`
	Host         string `json:"host"`

	DbState       uint32 `json:"dbState"`
	ExtendedIndex bool   `json:"extendedIndex"`

	LastStore time.Time `json:"lastStore"`

	// true if application is with flag --sync
	SyncMode bool `json:"syncMode"`

	InitialSync    bool      `json:"initialSync"`
	IsSynchronized bool      `json:"isSynchronized"`
	BestHeight     uint32    `json:"bestHeight"`
	StartSync      time.Time `json:"-"`
	LastSync       time.Time `json:"lastSync"`
	BlockTimes     []uint32  `json:"-"`
	AvgBlockPeriod uint32    `json:"-"`

	IsMempoolSynchronized bool      `json:"isMempoolSynchronized"`
	MempoolSize           int       `json:"mempoolSize"`
	LastMempoolSync       time.Time `json:"lastMempoolSync"`

	DbColumns []InternalStateColumn `json:"dbColumns"`

	HasFiatRates                 bool      `json:"-"`
	HasTokenFiatRates            bool      `json:"-"`
	HistoricalFiatRatesTime      time.Time `json:"historicalFiatRatesTime"`
	HistoricalTokenFiatRatesTime time.Time `json:"historicalTokenFiatRatesTime"`

	EnableSubNewTx bool `json:"-"`

	BackendInfo BackendInfo `json:"-"`

	// database migrations
	UtxoChecked            bool `json:"utxoChecked"`
	SortedAddressContracts bool `json:"sortedAddressContracts"`

	// golomb filter settings
	BlockGolombFilterP      uint8  `json:"block_golomb_filter_p"`
	BlockFilterScripts      string `json:"block_filter_scripts"`
	BlockFilterUseZeroedKey bool   `json:"block_filter_use_zeroed_key"`

	// allowed number of fetched accounts over websocket
	WsGetAccountInfoLimit int            `json:"-"`
	WsLimitExceedingIPs   map[string]int `json:"-"`
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
	return append(is.DbColumns[:0:0], is.DbColumns...)
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
	is.BlockTimes = blockTimes
	is.computeAvgBlockPeriod()
	glog.Info("set ", len(is.BlockTimes), " block times, average block period ", is.AvgBlockPeriod, "s")
	return is.AvgBlockPeriod
}

// AppendBlockTime appends block time to BlockTimes, returns AvgBlockPeriod
func (is *InternalState) AppendBlockTime(time uint32) uint32 {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.BlockTimes = append(is.BlockTimes, time)
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
