//go:build unittest

package db

import (
	"math/big"
	"testing"

	"github.com/linxGnu/grocksdb"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/bchain/coins/tron"
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

func Test_internalDataEqual(t *testing.T) {
	transfer := func(t bchain.EthereumInternalTransactionType, from, to string, value int64) bchain.EthereumInternalTransfer {
		return bchain.EthereumInternalTransfer{Type: t, From: from, To: to, Value: *big.NewInt(value)}
	}
	tronParser := tron.NewTronParser(1, false)
	ethParser := eth.NewEthereumParser(1, false)
	tests := []struct {
		name     string
		parser   bchain.BlockChainParser
		stored   bchain.EthereumInternalData
		computed bchain.EthereumInternalData
		want     bool
	}{
		{
			name: "identical CALL with transfers",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 700000),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 700000),
			}},
			want: true,
		},
		{
			// the type is packed as a single CALL|CREATE bit, so SELFDESTRUCT unpacks as CALL
			name: "stored lossy CALL equals computed SELFDESTRUCT",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", 5),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.SELFDESTRUCT, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", 5),
			}},
			want: true,
		},
		{
			// processInternalData demotes CREATE to CALL when the contract does not
			// parse; the computed side must demote too, or it is re-counted every retry
			name:     "computed failed CREATE with empty contract equals stored CALL",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: ""},
			want:     true,
		},
		{
			name: "computed failed CREATE with transfer equals stored demoted CALL",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CREATE, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", 5),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "", Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CREATE, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "", 5),
			}},
			want: true,
		},
		{
			name:     "CREATE does not equal CALL",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     false,
		},
		{
			name:     "CREATE with different contract",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     false,
		},
		{
			name:     "CREATE with same contract",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     true,
		},
		{
			name:   "different transfer count",
			stored: bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 1),
			}},
			want: false,
		},
		{
			name: "different transfer value",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 1),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 2),
			}},
			want: false,
		},
		{
			// the stored message is transformed on unpack; comparing it would
			// reprocess failed transactions forever
			name:     "error is ignored",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL, Error: "Error(REVERT)", Transfers: []bchain.EthereumInternalTransfer{transfer(bchain.CALL, "a", "b", 1)}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Error: "REVERT", Transfers: []bchain.EthereumInternalTransfer{transfer(bchain.CALL, "a", "b", 1)}},
			want:     true,
		},
		{
			// stored unpacks EIP55 checksummed, the trace returns lowercase
			name:     "eth CREATE contract differing only in case",
			parser:   ethParser,
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5C43B1eD97e52d009611D89b74fA829FE4ac56b1"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5c43b1ed97e52d009611d89b74fa829fe4ac56b1"},
			want:     true,
		},
		{
			name:   "eth transfer addresses differing only in case",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x8eB187136a5B4D3110A275c00918884F9BEcffFC", 42),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "0x8eb187136a5b4d3110a275c00918884f9becfffc", 42),
			}},
			want: true,
		},
		{
			name:     "eth CREATE with different contract",
			parser:   ethParser,
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5C43B1eD97e52d009611D89b74fA829FE4ac56b1"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x8eb187136a5b4d3110a275c00918884f9becfffc"},
			want:     false,
		},
		{
			// the tron parser accepts both the base58 and the raw hex form
			name:     "tron CREATE contract in mixed base58 and hex form",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"},
			want:     true,
		},
		{
			// the packed form stores a missing transfer address as the zero address
			name:   "eth stored zero address equals computed empty address",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x0000000000000000000000000000000000000000", 0),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "", 0),
			}},
			want: true,
		},
		{
			name: "tron stored zero address equals computed empty address",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", 0),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "", 0),
			}},
			want: true,
		},
		{
			name:   "empty address does not equal a non-zero address",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x8eB187136a5B4D3110A275c00918884F9BEcffFC", 1),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "", 1),
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := tt.parser
			if parser == nil {
				parser = tronParser
			}
			if got := internalDataEqual(parser, &tt.stored, &tt.computed); got != tt.want {
				t.Errorf("internalDataEqual() = %v, want %v", got, tt.want)
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
