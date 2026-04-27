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
