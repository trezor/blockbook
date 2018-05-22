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

	DbState uint32 `json:"dbState"`

	LastStore time.Time `json:"lastStore"`

	IsSynchronized bool      `json:"isSynchronized"`
	BestHeight     uint32    `json:"bestHeight"`
	LastSync       time.Time `json:"lastSync"`

	IsMempoolSynchronized bool      `json:"isMempoolSynchronized"`
	LastMempoolSync       time.Time `json:"lastMempoolSync"`

	DbColumns []InternalStateColumn `json:"dbColumns"`
}

// IS is a singleton holding internal state of the application
var IS *InternalState

func (is *InternalState) StartedSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = false
}

func (is *InternalState) FinishedSync(bestHeight uint32) {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsSynchronized = true
	is.BestHeight = bestHeight
	is.LastSync = time.Now()
}

func (is *InternalState) GetSyncState() (bool, uint32, time.Time) {
	is.mux.Lock()
	defer is.mux.Unlock()
	return is.IsSynchronized, is.BestHeight, is.LastSync
}

func (is *InternalState) StartedMempoolSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsMempoolSynchronized = false
}

func (is *InternalState) FinishedMempoolSync() {
	is.mux.Lock()
	defer is.mux.Unlock()
	is.IsMempoolSynchronized = true
	is.LastMempoolSync = time.Now()
}

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
