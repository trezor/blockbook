//go:build unittest

package db

import (
	"testing"

	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

func Test_emptyInternalData(t *testing.T) {
	tests := []struct {
		name string
		data bchain.EthereumInternalData
		want bool
	}{
		{
			name: "empty CALL",
			data: bchain.EthereumInternalData{Type: bchain.CALL},
			want: true,
		},
		{
			name: "CREATE without transfers",
			data: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want: false,
		},
		{
			name: "CALL with transfer",
			data: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{{}}},
			want: false,
		},
		{
			name: "CALL with error",
			data: bchain.EthereumInternalData{Type: bchain.CALL, Error: "REVERT"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emptyInternalData(&tt.data); got != tt.want {
				t.Errorf("emptyInternalData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func storeInternalDataError(t *testing.T, d *RocksDB, height uint32, hash, message string, retries uint8) {
	t.Helper()
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.StoreBlockInternalDataErrorEthereumType(wb, &bchain.Block{
		BlockHeader: bchain.BlockHeader{Hash: hash, Height: height},
	}, message, retries); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}
}

func internalDataErrorAtHeight(t *testing.T, d *RocksDB, height uint32) *BlockInternalDataError {
	t.Helper()
	errors, err := d.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		t.Fatal(err)
	}
	for i := range errors {
		if errors[i].Height == height {
			return &errors[i]
		}
	}
	return nil
}

// A healing pass reports a failure from a snapshot of the queue taken before it started,
// so the update must not resurrect an entry a rollback removed, nor overwrite the entry
// of a different block that took over the height in the meantime.
func TestRocksDB_UpdateBlockInternalDataErrorEthereumType(t *testing.T) {
	const (
		height  = uint32(4321)
		hashA   = "0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee"
		hashB   = "0x9f8d4c2e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e611"
		queued  = "trace failed"
		refetch = "block not found"
	)

	t.Run("updates the queued block", func(t *testing.T) {
		d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
		defer closeAndDestroyRocksDB(t, d)
		storeInternalDataError(t, d, height, hashA, queued, 7)

		if err := d.UpdateBlockInternalDataErrorEthereumType(height, hashA, refetch, 8); err != nil {
			t.Fatal(err)
		}
		got := internalDataErrorAtHeight(t, d, height)
		if got == nil {
			t.Fatal("entry disappeared")
		}
		if got.Retries != 8 || got.ErrorMessage != refetch || got.Hash != hashA {
			t.Errorf("got %d retries, message %q, hash %s; want 8, %q, %s", got.Retries, got.ErrorMessage, got.Hash, refetch, hashA)
		}
	})

	t.Run("does not resurrect a removed entry", func(t *testing.T) {
		d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
		defer closeAndDestroyRocksDB(t, d)

		if err := d.UpdateBlockInternalDataErrorEthereumType(height, hashA, refetch, 1); err != nil {
			t.Fatal(err)
		}
		if got := internalDataErrorAtHeight(t, d, height); got != nil {
			t.Errorf("entry resurrected: %+v", got)
		}
	})

	t.Run("does not overwrite another block at the same height", func(t *testing.T) {
		d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
		defer closeAndDestroyRocksDB(t, d)
		storeInternalDataError(t, d, height, hashB, queued, 0)

		if err := d.UpdateBlockInternalDataErrorEthereumType(height, hashA, refetch, 25); err != nil {
			t.Fatal(err)
		}
		got := internalDataErrorAtHeight(t, d, height)
		if got == nil {
			t.Fatal("entry disappeared")
		}
		if got.Hash != hashB || got.Retries != 0 || got.ErrorMessage != queued {
			t.Errorf("got hash %s, %d retries, message %q; want %s, 0, %q", got.Hash, got.Retries, got.ErrorMessage, hashB, queued)
		}
	})
}
