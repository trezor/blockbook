//go:build unittest

package db

import (
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

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
