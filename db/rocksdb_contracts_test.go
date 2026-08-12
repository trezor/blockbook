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

func TestRocksDB_storeHealedContractInfos(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	healContracts := func(contracts ...bchain.ContractInfo) {
		t.Helper()
		wb := grocksdb.NewWriteBatch()
		defer wb.Destroy()
		if err := d.storeHealedContractInfos(wb, contracts); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteBatch(wb); err != nil {
			t.Fatal(err)
		}
	}

	// A creation must fill CreatedInBlock into a row already enriched on demand - sync
	// stores no name/symbol, so overwriting the row would strip a contract that a client
	// queried before its creating block was healed.
	enriched := "0x" + dbtestdata.EthAddr4b
	if err := d.StoreContractInfo(&bchain.ContractInfo{
		Standard: bchain.ERC20TokenStandard,
		Type:     bchain.ERC20TokenStandard,
		Contract: enriched,
		Name:     "Enriched",
		Symbol:   "ENR",
		Decimals: 18,
	}); err != nil {
		t.Fatal(err)
	}
	healContracts(bchain.ContractInfo{Contract: enriched, CreatedInBlock: 1500})
	got, err := d.GetContractInfoForAddress(enriched)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.CreatedInBlock != 1500 || got.Name != "Enriched" || got.Symbol != "ENR" ||
		got.Standard != bchain.ERC20TokenStandard || got.Decimals != 18 {
		t.Fatalf("healed enriched row = %+v, want CreatedInBlock 1500 with the enrichment preserved", got)
	}

	// A healed block can carry a destruction whose creation was never indexed; recording
	// it keeps the contract from looking alive forever.
	destroyed := "0x" + dbtestdata.EthAddr20
	healContracts(bchain.ContractInfo{Contract: destroyed, DestructedInBlock: 2000})
	got, err = d.GetContractInfoForAddress(destroyed)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.DestructedInBlock != 2000 || got.CreatedInBlock != 0 {
		t.Fatalf("healed destruction = %+v, want DestructedInBlock 2000 and CreatedInBlock 0", got)
	}

	// One block can create and destroy the same contract. The records are applied in order,
	// so the second does not undo the first.
	shortLived := "0x" + dbtestdata.EthAddr55
	healContracts(
		bchain.ContractInfo{Contract: shortLived, CreatedInBlock: 3000},
		bchain.ContractInfo{Contract: shortLived, DestructedInBlock: 3000},
	)
	got, err = d.GetContractInfoForAddress(shortLived)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.CreatedInBlock != 3000 || got.DestructedInBlock != 3000 {
		t.Fatalf("healed create+destruct = %+v, want both heights 3000", got)
	}
}

// Healing runs out of block order - the queue is drained after the blocks around it are
// already indexed, and the backoff can even invert the order of two queued blocks - so a
// reused address (SELFDESTRUCT followed by CREATE2) must not have its lifecycle rewound.
// Every case states what a sequential sync leaves behind, which is what healing must match.
func TestRocksDB_storeHealedContractInfos_ReusedAddress(t *testing.T) {
	tests := []struct {
		name           string
		stored         *bchain.ContractInfo
		healed         []bchain.ContractInfo
		wantCreated    uint32
		wantDestructed uint32
	}{
		{
			// sync: the creation in block 200 overwrote the row, so block 100 is history
			name:        "a creation older than the stored one is dropped",
			stored:      &bchain.ContractInfo{CreatedInBlock: 200},
			healed:      []bchain.ContractInfo{{CreatedInBlock: 100}},
			wantCreated: 200,
		},
		{
			// sync: the destruction of the previous incarnation was wiped by the creation
			// in block 200; storing it would destruct the contract before it was created
			name:        "a destruction older than the stored creation is dropped",
			stored:      &bchain.ContractInfo{CreatedInBlock: 200},
			healed:      []bchain.ContractInfo{{DestructedInBlock: 150}},
			wantCreated: 200,
		},
		{
			name:           "a destruction older than the stored destruction is dropped",
			stored:         &bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 300},
			healed:         []bchain.ContractInfo{{DestructedInBlock: 200}},
			wantCreated:    100,
			wantDestructed: 300,
		},
		{
			// sync: the creation overwrites the whole row, so the contract is alive again
			name:        "a creation after the stored destruction revives the contract",
			stored:      &bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 200},
			healed:      []bchain.ContractInfo{{CreatedInBlock: 300}},
			wantCreated: 300,
		},
		{
			// a destruction in a later block must survive re-healing the creating block
			name:           "a newer destruction survives a creation healed again",
			stored:         &bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 150},
			healed:         []bchain.ContractInfo{{CreatedInBlock: 100}},
			wantCreated:    100,
			wantDestructed: 150,
		},
		{
			// sync: the destruction lands on the previous incarnation, then the creation
			// overwrites the row - the contract deployed in this block is alive
			name:        "a destruction followed by a creation in one block leaves it alive",
			stored:      &bchain.ContractInfo{CreatedInBlock: 100},
			healed:      []bchain.ContractInfo{{DestructedInBlock: 400}, {CreatedInBlock: 400}},
			wantCreated: 400,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := setupRocksDB(t, &testEthereumParser{
				EthereumParser: ethereumTestnetParser(),
			})
			defer closeAndDestroyRocksDB(t, d)

			address := "0x" + dbtestdata.EthAddrContract4a
			stored := *tt.stored
			stored.Contract = address
			stored.Standard = bchain.ERC20TokenStandard
			stored.Type = bchain.ERC20TokenStandard
			if err := d.StoreContractInfo(&stored); err != nil {
				t.Fatal(err)
			}

			wb := grocksdb.NewWriteBatch()
			defer wb.Destroy()
			healed := append([]bchain.ContractInfo(nil), tt.healed...)
			for i := range healed {
				healed[i].Contract = address
			}
			if err := d.storeHealedContractInfos(wb, healed); err != nil {
				t.Fatal(err)
			}
			if err := d.WriteBatch(wb); err != nil {
				t.Fatal(err)
			}

			got, err := d.GetContractInfoForAddress(address)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.CreatedInBlock != tt.wantCreated || got.DestructedInBlock != tt.wantDestructed {
				t.Errorf("healed record = %+v, want created %d, destructed %d", got, tt.wantCreated, tt.wantDestructed)
			}
			// the guards must never cost the enrichment a client already queried
			if got != nil && got.Standard != bchain.ERC20TokenStandard {
				t.Errorf("healed record standard = %s, want %s", got.Standard, bchain.ERC20TokenStandard)
			}
		})
	}
}

func Test_applyHealedLifecycle(t *testing.T) {
	tests := []struct {
		name           string
		contractInfo   bchain.ContractInfo
		creation       uint32
		destruction    uint32
		wantApplied    bool
		wantCreated    uint32
		wantDestructed uint32
	}{
		// the case the healer exists for: a row enriched on demand carries no creation
		{name: "creation fills an empty row", creation: 100, wantApplied: true, wantCreated: 100},
		{
			name: "creation is idempotent", contractInfo: bchain.ContractInfo{CreatedInBlock: 100},
			creation: 100, wantApplied: true, wantCreated: 100,
		},
		{
			name: "creation older than the stored one is dropped", contractInfo: bchain.ContractInfo{CreatedInBlock: 200},
			creation: 100, wantApplied: false, wantCreated: 200,
		},
		{
			name:         "creation clears a destruction of the previous incarnation",
			contractInfo: bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 200},
			creation:     300, wantApplied: true, wantCreated: 300,
		},
		{
			name:         "creation keeps a destruction that is newer",
			contractInfo: bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 300},
			creation:     100, wantApplied: true, wantCreated: 100, wantDestructed: 300,
		},
		{
			name: "destruction of a stored creation applies", contractInfo: bchain.ContractInfo{CreatedInBlock: 100},
			destruction: 150, wantApplied: true, wantCreated: 100, wantDestructed: 150,
		},
		// a destruction with no stored row is recorded, so a contract created before the
		// index knew about it does not look alive forever
		{name: "destruction of an unknown contract applies", destruction: 150, wantApplied: true, wantDestructed: 150},
		{
			name: "destruction before the stored creation is dropped", contractInfo: bchain.ContractInfo{CreatedInBlock: 200},
			destruction: 150, wantApplied: false, wantCreated: 200,
		},
		{
			name:         "destruction is idempotent",
			contractInfo: bchain.ContractInfo{CreatedInBlock: 100, DestructedInBlock: 150},
			destruction:  150, wantApplied: false, wantCreated: 100, wantDestructed: 150,
		},
		{
			name:         "destruction in the creating block applies",
			contractInfo: bchain.ContractInfo{CreatedInBlock: 100},
			destruction:  100, wantApplied: true, wantCreated: 100, wantDestructed: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contractInfo := tt.contractInfo
			var applied bool
			if tt.creation != 0 {
				applied = applyHealedCreation(&contractInfo, tt.creation)
			} else {
				applied = applyHealedDestruction(&contractInfo, tt.destruction)
			}
			if applied != tt.wantApplied {
				t.Errorf("applied = %v, want %v", applied, tt.wantApplied)
			}
			if contractInfo.CreatedInBlock != tt.wantCreated || contractInfo.DestructedInBlock != tt.wantDestructed {
				t.Errorf("record = %+v, want created %d, destructed %d", contractInfo, tt.wantCreated, tt.wantDestructed)
			}
		})
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
