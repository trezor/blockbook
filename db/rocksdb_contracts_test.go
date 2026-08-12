//go:build unittest

package db

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestRocksDB_ListContractInfos(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// ordered by address descriptor: 0x20… < 0x4b… < 0x55…
	addresses := []string{"0x" + dbtestdata.EthAddr20, "0x" + dbtestdata.EthAddr4b, "0x" + dbtestdata.EthAddr55}
	for i, a := range addresses {
		if err := d.StoreContractInfo(&bchain.ContractInfo{
			Standard: bchain.ERC20TokenStandard,
			Type:     bchain.ERC20TokenStandard,
			Contract: a,
			Name:     "Contract " + strconv.Itoa(i),
			Decimals: 18,
		}); err != nil {
			t.Fatal(err)
		}
	}

	contracts, next, err := d.ListContractInfos("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 3 || next != "" {
		t.Fatalf("ListContractInfos() = %d rows, next %q, want 3 rows and no next", len(contracts), next)
	}
	for i, c := range contracts {
		if !strings.EqualFold(c.Contract, addresses[i]) {
			t.Errorf("row %d = %s, want %s", i, c.Contract, addresses[i])
		}
	}

	// paging: a full first page and a next cursor pointing at the third row
	contracts, next, err = d.ListContractInfos("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 || !strings.EqualFold(next, addresses[2]) {
		t.Fatalf("ListContractInfos(limit 2) = %d rows, next %q, want 2 rows and next %s", len(contracts), next, addresses[2])
	}
	contracts, next, err = d.ListContractInfos(next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || next != "" || !strings.EqualFold(contracts[0].Contract, addresses[2]) {
		t.Fatalf("ListContractInfos(from next) = %+v next %q, want only the third row", contracts, next)
	}

	if _, _, err = d.ListContractInfos("not-an-address", 2); err == nil {
		t.Error("ListContractInfos() with invalid from: expected error")
	}
}

func TestRocksDB_DeleteContractInfoForAddress(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	address := "0x" + dbtestdata.EthAddr20
	ci := &bchain.ContractInfo{
		Standard:       bchain.ERC20TokenStandard,
		Type:           bchain.ERC20TokenStandard,
		Contract:       address,
		Name:           "Test contract",
		Symbol:         "TCT",
		Decimals:       18,
		CreatedInBlock: 1234567,
	}
	if err := d.StoreContractInfo(ci); err != nil {
		t.Fatal(err)
	}
	// The get populates the in-memory cache, so a successful delete below also
	// proves the cache entry is purged along with the DB row.
	got, err := d.GetContractInfoForAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != ci.Name {
		t.Fatalf("GetContractInfoForAddress() = %+v, want stored contract", got)
	}

	genBefore := d.protocolGen.Load()
	purged, err := d.DeleteContractInfoForAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if purged == nil || purged.Name != ci.Name || purged.CreatedInBlock != ci.CreatedInBlock {
		t.Errorf("DeleteContractInfoForAddress() = %+v, want the stored record", purged)
	}
	// The generation bump protects against a concurrent GetContractInfo
	// re-inserting the deleted row into the cache (see SetErcProtocol).
	if d.protocolGen.Load() != genBefore+1 {
		t.Error("DeleteContractInfoForAddress() did not bump protocolGen")
	}
	got, err = d.GetContractInfoForAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("GetContractInfoForAddress() after delete = %+v, want nil", got)
	}

	// Idempotent: deleting a missing row is not an error and does not bump
	// the generation (nothing a concurrent reader could re-insert).
	genBefore = d.protocolGen.Load()
	purged, err = d.DeleteContractInfoForAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if purged != nil {
		t.Errorf("DeleteContractInfoForAddress() = %+v, want nil for a missing row", purged)
	}
	if d.protocolGen.Load() != genBefore {
		t.Error("DeleteContractInfoForAddress() of a missing row must not bump protocolGen")
	}

	if _, err = d.DeleteContractInfoForAddress("not-an-address"); err == nil {
		t.Error("DeleteContractInfoForAddress() with invalid address: expected error")
	}
}

func TestStoreContractInfo_MergesSameBatchCreateAndDestroy(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	ephemeral := "0x" + dbtestdata.EthAddr20
	destroyedOnly := "0x" + dbtestdata.EthAddr4b

	// an ephemeral contract emits creation and destruction in one block; the
	// destruction can see the uncommitted creation only through the in-batch map
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	inBatch := make(map[string]*bchain.ContractInfo)
	if err := d.storeContractInfo(wb, &bchain.ContractInfo{
		Contract:       ephemeral,
		Standard:       bchain.UnhandledTokenStandard,
		CreatedInBlock: 500,
	}, inBatch); err != nil {
		t.Fatal(err)
	}
	if err := d.storeContractInfo(wb, &bchain.ContractInfo{
		Contract:          ephemeral,
		DestructedInBlock: 500,
	}, inBatch); err != nil {
		t.Fatal(err)
	}
	// a destruction of a contract known neither in the batch nor in the DB
	// stays dropped
	if err := d.storeContractInfo(wb, &bchain.ContractInfo{
		Contract:          destroyedOnly,
		DestructedInBlock: 500,
	}, inBatch); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}

	got, err := d.GetContractInfoForAddress(ephemeral)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.CreatedInBlock != 500 || got.DestructedInBlock != 500 {
		t.Errorf("GetContractInfoForAddress() = %+v, want CreatedInBlock 500 and DestructedInBlock 500", got)
	}
	got, err = d.GetContractInfoForAddress(destroyedOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("GetContractInfoForAddress() = %+v, want nil for an unknown destroyed contract", got)
	}
}

// packContractInfo only carries the sync-owned core fields. ERC4626 detection
// data lives in the cfErcProtocols column family and is exercised
// separately in rocksdb_protocols_test.go.
func Test_packUnpackContractInfo(t *testing.T) {
	tests := []struct {
		name         string
		contractInfo bchain.ContractInfo
	}{
		{
			name:         "empty",
			contractInfo: bchain.ContractInfo{},
		},
		{
			name: "unknown",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.UnknownTokenStandard,
				Standard:          bchain.UnknownTokenStandard,
				Name:              "Test contract",
				Symbol:            "TCT",
				Decimals:          18,
				CreatedInBlock:    1234567,
				DestructedInBlock: 234567890,
			},
		},
		{
			name: "ERC20",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.ERC20TokenStandard,
				Standard:          bchain.ERC20TokenStandard,
				Name:              "GreenContract🟢",
				Symbol:            "🟢",
				Decimals:          0,
				CreatedInBlock:    1,
				DestructedInBlock: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := packContractInfo(&tt.contractInfo)
			got, err := unpackContractInfo(buf)
			if err != nil {
				t.Fatalf("unpackContractInfo() err = %v", err)
			}
			if !reflect.DeepEqual(*got, tt.contractInfo) {
				t.Errorf("packUnpackContractInfo() = %+v, want %+v", *got, tt.contractInfo)
			}
		})
	}
}
