package bchain

import (
	reflect "reflect"
	"strconv"
	"testing"
)

func generateAddIndexes(count int) []addrIndex {
	rv := make([]addrIndex, count)
	for i := range count {
		rv[i] = addrIndex{
			addrDesc: "ad" + strconv.Itoa(i),
		}
	}
	return rv
}

func generateTxEntries(count int, skipTx int) map[string]txEntry {
	rv := make(map[string]txEntry)
	for i := range count {
		if i != skipTx {
			tx := "tx" + strconv.Itoa(i)
			rv[tx] = txEntry{
				addrIndexes: generateAddIndexes(count),
			}
		}
	}
	return rv
}

func generateAddrDescToTx(count int, skipTx int) map[string][]Outpoint {
	rv := make(map[string][]Outpoint)
	for i := range count {
		ad := "ad" + strconv.Itoa(i)
		op := []Outpoint{}
		for j := range count {
			if j != skipTx {
				tx := "tx" + strconv.Itoa(j)
				op = append(op, Outpoint{
					Txid: tx,
				})
			}
		}
		if len(op) > 0 {
			rv[ad] = op
		}
	}
	return rv
}

func TestBaseMempool_removeEntryFromMempool(t *testing.T) {
	tests := []struct {
		name  string
		m     *BaseMempool
		want  *BaseMempool
		txid  string
		entry txEntry
	}{
		{
			name: "test1",
			m: &BaseMempool{
				txEntries: map[string]txEntry{
					"tx1": {
						addrIndexes: []addrIndex{{addrDesc: "ad1", n: 0}, {addrDesc: "ad1", n: 1}},
					},
					"tx2": {
						addrIndexes: []addrIndex{{addrDesc: "ad1"}},
					},
				},
				addrDescToTx: map[string][]Outpoint{
					"ad1": {
						{Txid: "tx1", Vout: 0},
						{Txid: "tx1", Vout: 1},
						{Txid: "tx2"},
					},
				},
			},
			want: &BaseMempool{
				txEntries: map[string]txEntry{
					"tx2": {
						addrIndexes: []addrIndex{{addrDesc: "ad1"}},
					},
				},
				addrDescToTx: map[string][]Outpoint{
					"ad1": {{Txid: "tx2"}}},
			},
			txid: "tx1",
			entry: txEntry{
				addrIndexes: []addrIndex{
					{addrDesc: "ad1"},
					{addrDesc: "ad2"},
				},
			},
		},
		{
			name: "test2",
			m: &BaseMempool{
				txEntries: map[string]txEntry{
					"tx1": {
						addrIndexes: []addrIndex{{addrDesc: "ad1"}, {addrDesc: "ad1", n: 1}},
					},
				},
				addrDescToTx: map[string][]Outpoint{
					"ad1": {
						{Txid: "tx1", Vout: 0},
						{Txid: "tx1", Vout: 1},
					},
				},
			},
			want: &BaseMempool{
				txEntries:    map[string]txEntry{},
				addrDescToTx: map[string][]Outpoint{},
			},
			txid: "tx1",
			entry: txEntry{
				addrIndexes: []addrIndex{
					{addrDesc: "ad1"},
				},
			},
		},
		{
			name: "generated1",
			m: &BaseMempool{
				txEntries:    generateTxEntries(1, -1),
				addrDescToTx: generateAddrDescToTx(1, -1),
			},
			want: &BaseMempool{
				txEntries:    generateTxEntries(1, 0),
				addrDescToTx: generateAddrDescToTx(1, 0),
			},
			txid: "tx0",
			entry: txEntry{
				addrIndexes: generateAddIndexes(1),
			},
		},
		{
			name: "generated2",
			m: &BaseMempool{
				txEntries:    generateTxEntries(2, -1),
				addrDescToTx: generateAddrDescToTx(2, -1),
			},
			want: &BaseMempool{
				txEntries:    generateTxEntries(2, 1),
				addrDescToTx: generateAddrDescToTx(2, 1),
			},
			txid: "tx1",
			entry: txEntry{
				addrIndexes: generateAddIndexes(2),
			},
		},
		{
			name: "generated5000",
			m: &BaseMempool{
				txEntries:    generateTxEntries(5000, -1),
				addrDescToTx: generateAddrDescToTx(5000, -1),
			},
			want: &BaseMempool{
				txEntries:    generateTxEntries(5000, 2),
				addrDescToTx: generateAddrDescToTx(5000, 2),
			},
			txid: "tx2",
			entry: txEntry{
				addrIndexes: generateAddIndexes(5000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.removeEntryFromMempool(tt.txid, tt.entry)
			if !reflect.DeepEqual(tt.m, tt.want) {
				t.Errorf("removeEntryFromMempool() got = %+v, want %+v", tt.m, tt.want)
			}
		})
	}
}
