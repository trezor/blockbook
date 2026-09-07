package bchain

import (
	"reflect"
	"testing"
	"time"
)

func TestMempoolEthereumType_removeTransactionsMissingFromBackend(t *testing.T) {
	snapshotTime := uint32(time.Now().Unix())
	m := &MempoolEthereumType{
		BaseMempool: BaseMempool{
			txEntries: map[string]txEntry{
				"kept": {
					addrIndexes: []addrIndex{{addrDesc: "addr1"}},
					time:        snapshotTime - 1,
				},
				"removed": {
					addrIndexes: []addrIndex{{addrDesc: "addr1"}, {addrDesc: "addr2"}},
					time:        snapshotTime - 1,
				},
				"new": {
					addrIndexes: []addrIndex{{addrDesc: "addr2"}},
					time:        snapshotTime,
				},
			},
			addrDescToTx: map[string][]Outpoint{
				"addr1": {{Txid: "kept"}, {Txid: "removed"}},
				"addr2": {{Txid: "removed"}, {Txid: "new"}},
			},
		},
	}

	removed := m.removeTransactionsMissingFromBackend(map[string]struct{}{"kept": {}}, snapshotTime)
	if removed != 1 {
		t.Fatalf("removeTransactionsMissingFromBackend() = %d, want 1", removed)
	}
	if _, found := m.txEntries["removed"]; found {
		t.Fatal("expected tx missing from backend snapshot to be removed")
	}
	if _, found := m.txEntries["kept"]; !found {
		t.Fatal("expected backend tx to remain in mempool")
	}
	if _, found := m.txEntries["new"]; !found {
		t.Fatal("expected tx added at snapshot time to remain in mempool")
	}

	wantAddrDescToTx := map[string][]Outpoint{
		"addr1": {{Txid: "kept"}},
		"addr2": {{Txid: "new"}},
	}
	if !reflect.DeepEqual(m.addrDescToTx, wantAddrDescToTx) {
		t.Fatalf("addrDescToTx = %+v, want %+v", m.addrDescToTx, wantAddrDescToTx)
	}
}

func TestNewMempoolEthereumTypeUsesDuration(t *testing.T) {
	m := NewMempoolEthereumType(nil, 10*time.Minute, false)
	if m.mempoolTimeoutTime != 10*time.Minute {
		t.Fatalf("mempoolTimeoutTime = %s, want %s", m.mempoolTimeoutTime, 10*time.Minute)
	}
}

// A restamped entry ages from the newest add, so the timeout sweep cannot drop it while the
// alternative-provider cache - which also ages from the newest send - still serves it as pending.
func TestMempoolEthereumTypeAddOrRefreshRestampsExistingEntry(t *testing.T) {
	m := NewMempoolEthereumType(nil, 10*time.Minute, false)
	old := uint32(time.Now().Add(-time.Hour).Unix())
	m.txEntries["tx1"] = txEntry{addrIndexes: []addrIndex{{addrDesc: "addr1"}}, time: old}

	if !m.AddOrRefreshTransactionInMempool("tx1") {
		t.Fatal("AddOrRefreshTransactionInMempool() = false, want true")
	}
	if got := m.txEntries["tx1"].time; got == old {
		t.Fatalf("entry time = %d, want restamped past %d", got, old)
	}
	if got := m.txEntries["tx1"].addrIndexes; len(got) != 1 {
		t.Fatalf("addrIndexes = %+v, want the original index kept", got)
	}
}
