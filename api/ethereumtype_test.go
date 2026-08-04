//go:build unittest

package api

import (
	"testing"
	"time"
)

// resetInternalDataRefetchState clears the package-level refetch state so a test cannot
// leave the barrier armed for the rest of the test binary.
func resetInternalDataRefetchState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		refetchInternalDataMux.Lock()
		refetchInternalDataStopped = false
		refetchingInternalData = false
		refetchInternalDataMux.Unlock()
	})
}

// The healing pass writes to RocksDB from a goroutine nothing else awaits, so shutdown
// has to hold the database open until the pass lets go.
func TestStopInternalDataRefetchWaitsForPass(t *testing.T) {
	resetInternalDataRefetchState(t)

	// stand in for a pass started by RefetchInternalData
	refetchInternalDataMux.Lock()
	refetchingInternalData = true
	refetchInternalDataWG.Add(1)
	refetchInternalDataMux.Unlock()

	returned := make(chan bool, 1)
	go func() { returned <- StopInternalDataRefetch() }()

	select {
	case <-returned:
		t.Fatal("StopInternalDataRefetch returned while a pass was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	// the pass observes the stop flag and finishes
	if !internalDataRefetchStopping() {
		t.Error("internalDataRefetchStopping() = false, want true once the barrier is waiting")
	}
	refetchInternalDataMux.Lock()
	refetchingInternalData = false
	refetchInternalDataMux.Unlock()
	refetchInternalDataWG.Done()

	select {
	case finished := <-returned:
		if !finished {
			t.Error("StopInternalDataRefetch() = false, want true when the pass finished in time")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopInternalDataRefetch did not return after the pass finished")
	}
}

// Once the barrier has run, a late admin click must not start a pass that would write
// into a database that is being closed.
func TestRefetchInternalDataDoesNotStartAfterStop(t *testing.T) {
	resetInternalDataRefetchState(t)

	if finished := StopInternalDataRefetch(); !finished {
		t.Fatal("StopInternalDataRefetch() = false with no pass running, want true")
	}

	w := &Worker{}
	if err := w.RefetchInternalData(true); err != nil {
		t.Fatalf("RefetchInternalData() error = %v", err)
	}
	// a started pass would have set the flag and nil-dereferenced w.db
	refetchInternalDataMux.Lock()
	started := refetchingInternalData
	refetchInternalDataMux.Unlock()
	if started {
		t.Error("RefetchInternalData started a pass after StopInternalDataRefetch")
	}
}
