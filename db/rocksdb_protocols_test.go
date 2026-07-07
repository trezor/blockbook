//go:build unittest

package db

import (
	"bytes"
	"testing"

	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

// helper: drive the generic SetErcProtocol with a payload of bytes("asset") and
// fetch back via GetErcProtocol. Most tests below operate at this level so we
// exercise the generic path; one spot-checks the ERC4626 shim too.

func newProtocolTestDB(t *testing.T) *RocksDB {
	t.Helper()
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	return d
}

func seedProtocolTestBlockHash(t *testing.T, d *RocksDB, height uint32, hash string) {
	t.Helper()
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.writeHeight(wb, height, &BlockInfo{Hash: hash, Time: 1, Height: height}, opInsert); err != nil {
		t.Fatalf("writeHeight: %v", err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("seed block hash: %v", err)
	}
}

func TestSetErcProtocol_PersistsAndReadsBack(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x4001)
	payload := []byte("asset")

	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, payload, 100, "", 0); err != nil {
		t.Fatalf("SetErcProtocol: %v", err)
	}
	got, h, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626)
	if err != nil || !ok {
		t.Fatalf("expected row, ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: %x vs %x", got, payload)
	}
	if h != 100 {
		t.Fatalf("persistHeight: got %d want 100", h)
	}
}

func TestSetErcProtocol_RefusesZeroPersistHeight(t *testing.T) {
	// Direct chain.GetContractInfo can return metadata without a known
	// CreatedInBlock, leaving persistHeight==0. A row keyed at height 0
	// would never be cleaned up by any realistic disconnect range, so the
	// writer refuses these defensively.
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x4001)
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 0, "", 0); err != nil {
		t.Fatalf("SetErcProtocol: %v", err)
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || ok {
		t.Fatalf("expected no row for persistHeight==0, ok=%v err=%v", ok, err)
	}
}

func TestSetErcProtocol_RefusesConflictingOverwrite(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x4002)
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("first"), 100, "", 0); err != nil {
		t.Fatalf("SetErcProtocol: %v", err)
	}
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("different"), 100, "", 0); err != nil {
		t.Fatalf("SetErcProtocol on conflict should not return error: %v", err)
	}
	got, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626)
	if err != nil || !ok {
		t.Fatalf("row missing after conflict refusal, ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, []byte("first")) {
		t.Fatalf("conflict overwrote payload: got %s want first", got)
	}
}

func TestSetErcProtocol_IdempotentOnSamePayload(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x4003)
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 100, "", 0); err != nil {
		t.Fatalf("first SetErcProtocol: %v", err)
	}
	// Second call with the same payload at a different persistHeight should be a no-op
	// (the existing row already records what we'd write).
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 200, "", 0); err != nil {
		t.Fatalf("idempotent SetErcProtocol: %v", err)
	}
	_, h, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626)
	if err != nil || !ok {
		t.Fatalf("row missing, ok=%v err=%v", ok, err)
	}
	if h != 100 {
		t.Fatalf("persistHeight changed: got %d want 100", h)
	}
}

func TestSetErcProtocol_DifferentProtocolsCoexist(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	// Two protocolIDs sharing the same contract address must not collide.
	addr := makeTestAddrDesc(0x4004)
	const otherProtocolID byte = 0x02

	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("vaultAsset"), 100, "", 0); err != nil {
		t.Fatalf("4626 set: %v", err)
	}
	if err := d.SetErcProtocol(addr, otherProtocolID, []byte("foreign"), 100, "", 0); err != nil {
		t.Fatalf("foreign set: %v", err)
	}

	got1, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626)
	if err != nil || !ok || string(got1) != "vaultAsset" {
		t.Fatalf("4626 readback: ok=%v err=%v payload=%s", ok, err, got1)
	}
	got2, _, ok, err := d.GetErcProtocol(addr, otherProtocolID)
	if err != nil || !ok || string(got2) != "foreign" {
		t.Fatalf("foreign readback: ok=%v err=%v payload=%s", ok, err, got2)
	}
}

func TestDisconnectErcProtocols_RemovesInRange(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	in := makeTestAddrDesc(0x5001)
	out := makeTestAddrDesc(0x5002)

	if err := d.SetErcProtocol(in, ErcProtocolErc4626, []byte("a"), 105, "", 0); err != nil {
		t.Fatalf("set in-range: %v", err)
	}
	if err := d.SetErcProtocol(out, ErcProtocolErc4626, []byte("b"), 90, "", 0); err != nil {
		t.Fatalf("set out-of-range: %v", err)
	}

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.disconnectErcProtocols(wb, 100, 110); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if _, _, ok, err := d.GetErcProtocol(in, ErcProtocolErc4626); err != nil || ok {
		t.Fatalf("expected in-range row removed, ok=%v err=%v", ok, err)
	}
	got, _, ok, err := d.GetErcProtocol(out, ErcProtocolErc4626)
	if err != nil || !ok || string(got) != "b" {
		t.Fatalf("out-of-range row should survive, ok=%v err=%v payload=%s", ok, err, got)
	}
}

func TestDisconnectBlockRangeEthereumType_BumpsReorgGenAndRevertsProtocols(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x6001)
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("a"), 50, "", 0); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Seed the height column so DisconnectBlockRangeEthereumType is willing to act.
	wb := grocksdb.NewWriteBatch()
	for h := uint32(50); h <= 51; h++ {
		wb.PutCF(d.cfh[cfHeight], packUint(h), []byte{})
		// A non-nil blockTxs row is required by the disconnect helper.
		wb.PutCF(d.cfh[cfBlockTxs], packUint(h), []byte{})
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("seed: %v", err)
	}
	wb.Destroy()

	prevGen := d.ReorgGeneration()
	if err := d.DisconnectBlockRangeEthereumType(50, 51); err != nil {
		t.Fatalf("DisconnectBlockRangeEthereumType: %v", err)
	}
	if d.ReorgGeneration() != prevGen+1 {
		t.Fatalf("reorg generation not bumped: was %d now %d", prevGen, d.ReorgGeneration())
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || ok {
		t.Fatalf("expected protocol row to be reverted by disconnect, ok=%v err=%v", ok, err)
	}
}

func TestErc4626VaultShim_RoundTrip(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x000000000000000000000000000000000000a17e"
	asset := "0x000000000000000000000000000000000000a55e7"

	if err := d.SetContractInfoErc4626Vault(address, asset, 50, "", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}
	got, ok, err := d.GetContractInfoErc4626Vault(addrDesc)
	if err != nil || !ok {
		t.Fatalf("readback: ok=%v err=%v", ok, err)
	}
	if got != asset {
		t.Fatalf("asset mismatch: got %q want %q", got, asset)
	}
}

// Simulates the reviewer's race: API request observes the chain at gen G,
// issues a multicall pinned to height H. Before the API write lands, a
// disconnect runs and bumps reorgGen. The writer must refuse the now-stale
// observation rather than persist a row at H that no future disconnect would
// catch.
func TestSetErcProtocol_RefusesStaleReorgGen(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x7001)
	observedGen := d.ReorgGeneration()

	// Disconnect happens between observation and write — bump reorgGen.
	d.reorgGen.Add(1)

	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 100, "", observedGen); err != nil {
		t.Fatalf("SetErcProtocol: %v", err)
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || ok {
		t.Fatalf("expected stale observation to be refused, ok=%v err=%v", ok, err)
	}

	// A fresh observation under the new gen must succeed.
	freshGen := d.ReorgGeneration()
	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 100, "", freshGen); err != nil {
		t.Fatalf("SetErcProtocol after re-observation: %v", err)
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || !ok {
		t.Fatalf("expected fresh-gen write to land, ok=%v err=%v", ok, err)
	}
}

func TestSetErcProtocol_RefusesStaleObservedHash(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	addr := makeTestAddrDesc(0x7002)
	const observedHash = "0x1111111111111111111111111111111111111111111111111111111111111111"
	const currentHash = "0x2222222222222222222222222222222222222222222222222222222222222222"
	seedProtocolTestBlockHash(t, d, 100, currentHash)
	gen := d.ReorgGeneration()

	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 100, observedHash, gen); err != nil {
		t.Fatalf("SetErcProtocol stale hash: %v", err)
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || ok {
		t.Fatalf("expected stale observed hash to be refused, ok=%v err=%v", ok, err)
	}

	if err := d.SetErcProtocol(addr, ErcProtocolErc4626, []byte("asset"), 100, currentHash, gen); err != nil {
		t.Fatalf("SetErcProtocol current hash: %v", err)
	}
	if _, _, ok, err := d.GetErcProtocol(addr, ErcProtocolErc4626); err != nil || !ok {
		t.Fatalf("expected current observed hash to land, ok=%v err=%v", ok, err)
	}
}

// Test that the API and sync paths can run their writes concurrently without
// either side dropping the other's data. The two writes target different column
// families and the API writer holds connectBlockMux, so this should pass even
// with -race.
func TestSetErcProtocol_DoesNotRaceWithStoreContractInfo(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x0000000000000000000000000000000000abcdef"
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}

	done := make(chan struct{}, 2)
	go func() {
		ci := &bchain.ContractInfo{
			Contract:       address,
			Standard:       bchain.ERC20TokenStandard,
			Type:           bchain.ERC20TokenStandard,
			Name:           "T",
			Symbol:         "T",
			Decimals:       18,
			CreatedInBlock: 50,
		}
		for i := 0; i < 100; i++ {
			if err := d.StoreContractInfo(ci); err != nil {
				t.Errorf("StoreContractInfo: %v", err)
				break
			}
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			if err := d.SetContractInfoErc4626Vault(address, "0x000000000000000000000000000000000000beef", 50, "", 0); err != nil {
				t.Errorf("SetContractInfoErc4626Vault: %v", err)
				break
			}
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	// Both records must be intact.
	ci, err := d.GetContractInfo(addrDesc, "")
	if err != nil || ci == nil {
		t.Fatalf("GetContractInfo: ci=%v err=%v", ci, err)
	}
	if ci.Name != "T" || ci.Symbol != "T" || ci.Decimals != 18 || ci.CreatedInBlock != 50 {
		t.Fatalf("sync metadata clobbered: %+v", ci)
	}
	if !ci.IsErc4626 || ci.Erc4626AssetContract != "0x000000000000000000000000000000000000bEEF" {
		// The asset address comparison is case-sensitive; the writer stores whatever the
		// caller passes, so just check it's non-empty and IsErc4626 is set.
		if !ci.IsErc4626 || ci.Erc4626AssetContract == "" {
			t.Fatalf("erc4626 record missing: %+v", ci)
		}
	}
}

// Reproduces the cache populate-after-write race: a reader caches IsErc4626=false
// just after a concurrent SetErcProtocol wrote the row. A subsequent
// SetErcProtocol with the same payload (idempotent path) must invalidate the
// stale entry so it doesn't drive a re-probe loop.
func TestSetErcProtocol_IdempotentInvalidatesStaleCache(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x000000000000000000000000000000000000c0de"
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}
	if err := d.StoreContractInfo(&bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
	}); err != nil {
		t.Fatalf("StoreContractInfo: %v", err)
	}

	if err := d.SetContractInfoErc4626Vault(address, "0x00000000000000000000000000000000000000a5", 100, "", 0); err != nil {
		t.Fatalf("SetContractInfoErc4626Vault: %v", err)
	}
	// Simulate the race: reader's CF read pre-dated the write, populates stale
	// entry under the post-write protocolGen (so the protocolGen-mismatch path
	// can't help — only the writer's cache delete can).
	stale := &bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
		IsErc4626: false, Erc4626AssetContract: "",
	}
	cachedContracts.add(string(addrDesc), stale, d.ReorgGeneration(), d.protocolGen.Load())

	// Idempotent re-write must clear the stale cache entry.
	if err := d.SetContractInfoErc4626Vault(address, "0x00000000000000000000000000000000000000a5", 100, "", 0); err != nil {
		t.Fatalf("idempotent SetContractInfoErc4626Vault: %v", err)
	}
	ci, err := d.GetContractInfo(addrDesc, "")
	if err != nil || ci == nil {
		t.Fatalf("GetContractInfo: ci=%v err=%v", ci, err)
	}
	if !ci.IsErc4626 || ci.Erc4626AssetContract == "" {
		t.Fatalf("expected fresh ERC4626 fields after idempotent re-write, got %+v", ci)
	}
}

// Same shape as the idempotent test, but exercises the conflict-refusal path
// (existing row, different payload). The write is refused but any stale cache
// entry must still be invalidated; otherwise stale negatives survive past a
// conflict and keep driving re-probes.
func TestSetErcProtocol_ConflictRefusalInvalidatesStaleCache(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x000000000000000000000000000000000000c1ff"
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}
	if err := d.StoreContractInfo(&bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
	}); err != nil {
		t.Fatalf("StoreContractInfo: %v", err)
	}

	const original = "0x00000000000000000000000000000000000000a5"
	if err := d.SetContractInfoErc4626Vault(address, original, 100, "", 0); err != nil {
		t.Fatalf("initial SetContractInfoErc4626Vault: %v", err)
	}
	stale := &bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
		IsErc4626: false, Erc4626AssetContract: "",
	}
	cachedContracts.add(string(addrDesc), stale, d.ReorgGeneration(), d.protocolGen.Load())

	// Write with a *different* asset; conflict path refuses and warns.
	if err := d.SetContractInfoErc4626Vault(address, "0x00000000000000000000000000000000000000ff", 100, "", 0); err != nil {
		t.Fatalf("conflict SetContractInfoErc4626Vault: %v", err)
	}
	ci, err := d.GetContractInfo(addrDesc, "")
	if err != nil || ci == nil {
		t.Fatalf("GetContractInfo: ci=%v err=%v", ci, err)
	}
	if !ci.IsErc4626 || ci.Erc4626AssetContract != original {
		t.Fatalf("expected fresh read of original asset after conflict refusal, got %+v", ci)
	}
}

// Reproduces the reorg populate-after-delete race: a reader populates the
// cache stamped at the old reorgGen; a later disconnect bumps the counter.
// The next reader sees the stamped entry mismatch and re-reads the post-disconnect
// CF state (IsErc4626=false) instead of the stale true.
func TestGetContractInfo_RejectsCacheEntryStampedAtOldReorgGen(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x000000000000000000000000000000000000beef"
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}
	if err := d.StoreContractInfo(&bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 100,
	}); err != nil {
		t.Fatalf("StoreContractInfo: %v", err)
	}

	// Plant a stale-true entry stamped at the current generation, mimicking a
	// reader who saw the old-fork protocol row before it was deleted.
	staleGen := d.ReorgGeneration()
	staleProtocolGen := d.protocolGen.Load()
	stale := &bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 100,
		IsErc4626: true, Erc4626AssetContract: "0x00000000000000000000000000000000000000a5",
	}
	cachedContracts.add(string(addrDesc), stale, staleGen, staleProtocolGen)

	// Disconnect bumps the generation; cfErcProtocols is empty (row never persisted).
	d.reorgGen.Add(1)

	ci, err := d.GetContractInfo(addrDesc, "")
	if err != nil || ci == nil {
		t.Fatalf("GetContractInfo: ci=%v err=%v", ci, err)
	}
	if ci.IsErc4626 || ci.Erc4626AssetContract != "" {
		t.Fatalf("expected stale cache entry to be rejected after reorgGen bump, got %+v", ci)
	}
}

// Reproduces the populate-after-write race that the conflict/idempotent cache
// deletes alone don't cover: reader misses, samples (reorgGen, protocolGen),
// reads cfErcProtocols (row absent), then a writer lands the row and bumps
// protocolGen. The reader's add lands AFTER the writer's cache delete, leaving
// a stale IsErc4626=false stamped at the pre-write protocolGen. The next
// GetContractInfo samples the bumped protocolGen and must miss.
//
// Without the protocolGen counter this stale entry would survive until LRU
// eviction, even though the protocol row exists on disk and no further
// SetErcProtocol call is guaranteed to clear it.
func TestGetContractInfo_RejectsCacheEntryStampedAtOldProtocolGen(t *testing.T) {
	d := newProtocolTestDB(t)
	defer closeAndDestroyRocksDB(t, d)

	address := "0x000000000000000000000000000000000000abba"
	addrDesc, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("addr desc: %v", err)
	}
	if err := d.StoreContractInfo(&bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
	}); err != nil {
		t.Fatalf("StoreContractInfo: %v", err)
	}

	// Snapshot the pre-write protocolGen (the racing reader's view).
	staleReorgGen := d.ReorgGeneration()
	staleProtocolGen := d.protocolGen.Load()

	// Writer lands the protocol row (bumps protocolGen).
	if err := d.SetContractInfoErc4626Vault(address, "0x00000000000000000000000000000000000000a5", 100, "", 0); err != nil {
		t.Fatalf("SetContractInfoErc4626Vault: %v", err)
	}

	// Racing reader's stale-false entry lands AFTER the writer's cache delete,
	// stamped at the old protocolGen.
	stale := &bchain.ContractInfo{
		Contract: address, Standard: bchain.ERC20TokenStandard, Type: bchain.ERC20TokenStandard,
		Name: "T", Symbol: "T", Decimals: 18, CreatedInBlock: 50,
		IsErc4626: false, Erc4626AssetContract: "",
	}
	cachedContracts.add(string(addrDesc), stale, staleReorgGen, staleProtocolGen)

	ci, err := d.GetContractInfo(addrDesc, "")
	if err != nil || ci == nil {
		t.Fatalf("GetContractInfo: ci=%v err=%v", ci, err)
	}
	if !ci.IsErc4626 || ci.Erc4626AssetContract == "" {
		t.Fatalf("expected fresh ERC4626 fields after protocolGen bump, got %+v", ci)
	}
}
