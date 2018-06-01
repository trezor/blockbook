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
	Name      string `json:"name"`
	Version   uint32 `json:"version"`
	Rows      int64  `json:"rows"`
	KeysSum   int64  `json:"keysSum"`
	ValuesSum int64  `json:"valuesSum"`
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

func (is *InternalState) AddDBColumnStats(c int, rowsDiff int64, keysSumDiff int64, valuesSumDiff int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.DbColumns[c].Rows += rowsDiff
	is.DbColumns[c].KeysSum += keysSumDiff
	is.DbColumns[c].ValuesSum += valuesSumDiff
}

func (is *InternalState) SetDBColumnStats(c int, rowsDiff int64, keysSumDiff int64, valuesSumDiff int64) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.DbColumns[c].Rows = rowsDiff
	is.DbColumns[c].KeysSum = keysSumDiff
	is.DbColumns[c].ValuesSum = valuesSumDiff
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
