//go:build unittest

package db

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// blockWithFailedInternalData returns the test block the way a failed internal data fetch
// leaves it: no per-tx internal data and no contracts - the trace is what would have
// produced both - only the error marker and the aliases that came from the logs.
func blockWithFailedInternalData(block *bchain.Block) *bchain.Block {
	for i := range block.Txs {
		csd, _ := block.Txs[i].CoinSpecificData.(bchain.EthereumSpecificData)
		csd.InternalData = nil
		block.Txs[i].CoinSpecificData = csd
	}
	block.CoinSpecificData = &bchain.EthereumBlockSpecificData{
		InternalDataError:   dbtestdata.Block2SpecificData.InternalDataError,
		AddressAliasRecords: dbtestdata.Block2SpecificData.AddressAliasRecords,
	}
	return block
}

// blockWithHealedInternalData returns the test block as a successful refetch delivers it -
// internal data present, no error - so it can be reconnected to the indexed block.
func blockWithHealedInternalData(block *bchain.Block) *bchain.Block {
	block.CoinSpecificData = &bchain.EthereumBlockSpecificData{
		AddressAliasRecords: dbtestdata.Block2SpecificData.AddressAliasRecords,
		Contracts:           dbtestdata.Block2SpecificData.Contracts,
	}
	return block
}

// columnDump returns a column family as a hex key -> hex value map.
func columnDump(t *testing.T, d *RocksDB, col int) map[string]string {
	t.Helper()
	dump := map[string]string{}
	it := d.db.NewIteratorCF(d.ro, d.cfh[col])
	defer it.Close()
	for it.SeekToFirst(); it.Valid(); it.Next() {
		dump[hex.EncodeToString(it.Key().Data())] = hex.EncodeToString(it.Value().Data())
	}
	return dump
}

// addressTxIndexes returns every indexed transaction of every address with its index list
// sorted. The heal appends the internal data indexes to rows the failed sync already
// wrote, so they land in a different order than a sync that never failed produces. The
// entries are the same, and the order within one transaction only decides which index a
// filter matches first, so the comparison is order-insensitive here and exact everywhere
// else.
func addressTxIndexes(t *testing.T, d *RocksDB) map[string][]string {
	t.Helper()
	entries := map[string][]string{}
	for key := range columnDump(t, d, cfAddresses) {
		raw, err := hex.DecodeString(key)
		if err != nil {
			t.Fatal(err)
		}
		addrDesc, _, err := unpackAddressKey(raw)
		if err != nil {
			t.Fatal(err)
		}
		strAddrDesc := hex.EncodeToString(addrDesc)
		if _, done := entries[strAddrDesc]; done {
			continue
		}
		txs := []string{}
		if err := d.GetAddrDescTransactions(addrDesc, 0, math.MaxUint32, func(txid string, height uint32, indexes []int32) error {
			sorted := append([]int32(nil), indexes...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			txs = append(txs, fmt.Sprintf("%d %s %v", height, txid, sorted))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sort.Strings(txs)
		entries[strAddrDesc] = txs
	}
	return entries
}

func assertSameIndex(t *testing.T, healed, synced *RocksDB) {
	t.Helper()
	for col := range cfNames {
		if col == cfAddresses {
			continue
		}
		if got, want := columnDump(t, healed, col), columnDump(t, synced, col); !reflect.DeepEqual(got, want) {
			t.Errorf("column %s of the healed index differs\n got %v\nwant %v", cfNames[col], got, want)
		}
	}
	if got, want := addressTxIndexes(t, healed), addressTxIndexes(t, synced); !reflect.DeepEqual(got, want) {
		t.Errorf("addresses of the healed index differ\n got %v\nwant %v", got, want)
	}
}

// A healed block must be indistinguishable from one that synced without a trace failure:
// the same internal data, the same address counters, and the contract it destroyed
// recorded without losing the metadata block 1 stored for it.
func TestRocksDB_ReconnectInternalDataToBlockEthereumType_HealsLikeSync(t *testing.T) {
	healed := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, healed)
	synced := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, synced)

	for _, d := range []*RocksDB{healed, synced} {
		if err := d.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)); err != nil {
			t.Fatal(err)
		}
	}
	// the reference index: block 2 synced with its internal data
	if err := synced.ConnectBlock(blockWithHealedInternalData(dbtestdata.GetTestEthereumTypeBlock2(synced.chainParser))); err != nil {
		t.Fatal(err)
	}

	// the healed index: block 2 connected the way a failed fetch leaves it, then healed
	if err := healed.ConnectBlock(blockWithFailedInternalData(dbtestdata.GetTestEthereumTypeBlock2(healed.chainParser))); err != nil {
		t.Fatal(err)
	}
	if internalDataErrorAtHeight(t, healed, 4321001) == nil {
		t.Fatal("block 2 was not queued for healing")
	}
	if err := healed.ReconnectInternalDataToBlockEthereumType(blockWithHealedInternalData(dbtestdata.GetTestEthereumTypeBlock2(healed.chainParser))); err != nil {
		t.Fatal(err)
	}
	assertSameIndex(t, healed, synced)

	// Healing the same block again must change nothing. The address counters are
	// cumulative, so a second pass over data that are already stored would inflate them
	// with no way to repair it.
	if err := healed.ReconnectInternalDataToBlockEthereumType(blockWithHealedInternalData(dbtestdata.GetTestEthereumTypeBlock2(healed.chainParser))); err != nil {
		t.Fatal(err)
	}
	assertSameIndex(t, healed, synced)
}

func TestRocksDB_ReconnectInternalDataToBlockEthereumType_ReorgGuard(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	if err := d.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)); err != nil {
		t.Fatal(err)
	}

	// The height still holds this hash, so the reconnect proceeds. Without this case a
	// hash compared in the wrong format would reject every heal and silently disable
	// healing altogether, while the reorg case below would still pass.
	if err := d.ReconnectInternalDataToBlockEthereumType(dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)); err != nil {
		t.Fatalf("reconnect of the indexed block: %v", err)
	}

	// A reorg replaced the block at this height: the orphan's transactions are no longer
	// indexed, so its internal data must not be written back.
	orphan := dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)
	orphan.Hash = "0x" + strings.Repeat("ab", 32)
	err := d.ReconnectInternalDataToBlockEthereumType(orphan)
	if err == nil {
		t.Fatal("reconnect of an orphaned block succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "possible reorg") {
		t.Fatalf("reconnect of an orphaned block: %v, want a reorg error", err)
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
	queue, err := d.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		t.Fatal(err)
	}
	for i := range queue {
		if queue[i].Height == height {
			return &queue[i]
		}
	}
	return nil
}

// A healing pass reports a failure from a snapshot of the queue taken before it started,
// so the update must not resurrect an entry a rollback removed, nor overwrite the entry of
// a different block that took over the height in the meantime.
func TestRocksDB_UpdateBlockInternalDataErrorEthereumType(t *testing.T) {
	const (
		height  = uint32(4321)
		hashA   = "0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee"
		hashB   = "0x9f8d4c2e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e611"
		queued  = "trace failed"
		refetch = "Block not found"
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
