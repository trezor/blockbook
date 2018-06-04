package common

import (
	"encoding/json"
	"sync"
	"time"
)

const (
	// DbStateClosed means db was closed gracefully
	DbStateClosed = uint32(iota)
	// DbStateOpen means db is open or application died without closing the db
	DbStateOpen
)

// InternalStateColumn contains the data of a db column
type InternalStateColumn struct {
	Name       string `json:"name"`
	Version    uint32 `json:"version"`
	Rows       int64  `json:"rows"`
	KeyBytes   int64  `json:"keysSum"`
	ValueBytes int64  `json:"valuesSum"`
}

// InternalState contains the data of the internal state
type InternalState struct {
	mux sync.Mutex

	Coin string `json:"coin"`
	Host string `json:"host"`

	DbState uint32 `json:"dbState"`

	LastStore time.Time `json:"lastStore"`

	IsSynchronized bool      `json:"isSynchronized"`
	BestHeight     uint32    `json:"bestHeight"`
	LastSync       time.Time `json:"lastSync"`

	IsMempoolSynchronized bool      `json:"isMempoolSynchronized"`
	MempoolSize           int       `json:"mempoolSize"`
	LastMempoolSync       time.Time `json:"lastMempoolSync"`

	DbColumns []InternalStateColumn `json:"dbColumns"`
}

// StartedSync signals start of synchronization
func (is *InternalState) StartedSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = false
}

// FinishedSync marks end of synchronization, bestHeight specifies new best block height
func (is *InternalState) FinishedSync(bestHeight uint32) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = true
	is.BestHeight = bestHeight
	is.LastSync = time.Now()
}

// FinishedSyncNoChange marks end of synchronization in case no index update was necessary, it does not update lastSync time
func (is *InternalState) FinishedSyncNoChange() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = true
}

// GetSyncState gets the state of synchronization
func (is *InternalState) GetSyncState() (bool, uint32, time.Time) {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.IsSynchronized, is.BestHeight, is.LastSync
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
func (is *InternalState) GetMempoolSyncState() (bool, time.Time) {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.IsMempoolSynchronized, is.LastMempoolSync
}

func (is *InternalState) AddDBColumnStats(c int, rowsDiff int64, keyBytesDiff int64, valueBytesDiff int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.DbColumns[c].Rows += rowsDiff
	is.DbColumns[c].KeyBytes += keyBytesDiff
	is.DbColumns[c].ValueBytes += valueBytesDiff
}

func (is *InternalState) SetDBColumnStats(c int, rows int64, keyBytes int64, valueBytes int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.DbColumns[c].Rows = rows
	is.DbColumns[c].KeyBytes = keyBytes
	is.DbColumns[c].ValueBytes = valueBytes
}

func (is *InternalState) GetDBColumnStats(c int) (int64, int64, int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	if c < len(is.DbColumns) {
		return is.DbColumns[c].Rows, is.DbColumns[c].KeyBytes, is.DbColumns[c].ValueBytes
	}
	return 0, 0, 0
}

func (is *InternalState) DBSizeTotal() int64 {
	is.mux.Lock()
	defer is.mux.Unlock()
	total := int64(0)
	for _, c := range is.DbColumns {
		total += c.KeyBytes + c.ValueBytes
	}
	return total
}

func (is *InternalState) Pack() ([]byte, error) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.LastStore = time.Now()
	return json.Marshal(is)
}

func UnpackInternalState(buf []byte) (*InternalState, error) {
	var is InternalState
	if err := json.Unmarshal(buf, &is); err != nil {
		return nil, err
	}
	return &is, nil
}
