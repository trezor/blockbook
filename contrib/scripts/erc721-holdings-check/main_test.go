package main

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	vlq "github.com/bsm/go-vlq"
	"github.com/linxGnu/grocksdb"
)

func TestInspectRowFindsNonAdjacentERC721Duplicates(t *testing.T) {
	address := testAddress(0xa1)
	row := packTestRow(
		testContract{standard: 0, value: 9},
		testContract{standard: 1, ids: []uint64{3, 1, 3, 1}},
		testContract{standard: 2, multiTokenValues: [][2]uint64{{7, 2}, {8, 4}}},
	)

	summary, findings, err := inspectRow(address, row)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ERC721Contracts != 1 || summary.ERC721IDs != 4 {
		t.Fatalf("ERC-721 counts = (%d, %d), want (1, 4)", summary.ERC721Contracts, summary.ERC721IDs)
	}
	if summary.DuplicateGroups != 2 || summary.DuplicateEntries != 2 {
		t.Fatalf("duplicate counts = (%d, %d), want (2, 2)", summary.DuplicateGroups, summary.DuplicateEntries)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(findings))
	}
	if findings[0].TokenID.Cmp(big.NewInt(3)) != 0 || findings[0].Occurrences != 2 {
		t.Fatalf("first finding = %+v, want token ID 3 twice", findings[0])
	}
	if findings[1].TokenID.Cmp(big.NewInt(1)) != 0 || findings[1].Occurrences != 2 {
		t.Fatalf("second finding = %+v, want token ID 1 twice", findings[1])
	}
}

func TestInspectRowCleanAndMalformed(t *testing.T) {
	address := testAddress(0xa2)
	clean := packTestRow(testContract{standard: 1, ids: []uint64{0, 1, 2}})
	summary, findings, err := inspectRow(address, clean)
	if err != nil {
		t.Fatal(err)
	}
	if summary.DuplicateGroups != 0 || len(findings) != 0 {
		t.Fatalf("clean row reported duplicates: %+v, %+v", summary, findings)
	}

	if _, _, err := inspectRow(address, clean[:len(clean)-1]); err == nil {
		t.Fatal("expected truncated row to fail")
	}
}

func TestExecuteScansSecondaryAndReturnsFindingExitCode(t *testing.T) {
	primaryPath := t.TempDir()
	secondaryPath := t.TempDir()
	primary := openPrimaryTestDB(t, primaryPath)
	defer primary.close()
	writeTestData(t, primary, supportedAddressContractsVersion, map[byte][]byte{
		0x01: packTestRow(testContract{standard: 1, ids: []uint64{1, 2}}),
		0x02: packTestRow(testContract{standard: 1, ids: []uint64{4, 4, 5, 5}}),
	})

	var out, errOut bytes.Buffer
	code := execute([]string{
		"--db", primaryPath,
		"--secondary-path", secondaryPath,
		"--max-findings", "1",
	}, &out, &errOut)
	if code != exitFindings {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitFindings, errOut.String())
	}
	if got := strings.Count(out.String(), "duplicate address="); got != 1 {
		t.Fatalf("printed findings = %d, want 1; output=%s", got, out.String())
	}
	for _, want := range []string{
		"duplicate_groups=2",
		"duplicate_entries=2",
		"omitted 1 duplicate findings",
		"REINDEX REQUIRED",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output does not contain %q: %s", want, out.String())
		}
	}
}

func TestExecuteCleanDatabase(t *testing.T) {
	primaryPath := t.TempDir()
	primary := openPrimaryTestDB(t, primaryPath)
	defer primary.close()
	writeTestData(t, primary, supportedAddressContractsVersion, map[byte][]byte{
		0x01: packTestRow(testContract{standard: 1, ids: []uint64{1, 2, 3}}),
	})

	var out, errOut bytes.Buffer
	code := execute([]string{"--db", primaryPath, "--secondary-path", t.TempDir()}, &out, &errOut)
	if code != exitClean {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitClean, errOut.String())
	}
	if !strings.Contains(out.String(), "does not prove that every stored token owner is correct") {
		t.Fatalf("clean caveat missing from output: %s", out.String())
	}
}

func TestExecuteRejectsUnsupportedSchema(t *testing.T) {
	primaryPath := t.TempDir()
	primary := openPrimaryTestDB(t, primaryPath)
	defer primary.close()
	writeTestData(t, primary, supportedAddressContractsVersion+1, nil)

	var out, errOut bytes.Buffer
	code := execute([]string{"--db", primaryPath, "--secondary-path", t.TempDir()}, &out, &errOut)
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errOut.String(), "unsupported addressContracts schema version") {
		t.Fatalf("unexpected error output: %s", errOut.String())
	}
}

func TestExecuteRejectsMissingColumnFamilyAndInvalidFlags(t *testing.T) {
	primaryPath := t.TempDir()
	primary := openPrimaryWithoutAddressContracts(t, primaryPath)
	defer primary.close()

	var out, errOut bytes.Buffer
	if code := execute([]string{"--db", primaryPath, "--secondary-path", t.TempDir()}, &out, &errOut); code != exitError {
		t.Fatalf("missing column family exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errOut.String(), "open RocksDB secondary") {
		t.Fatalf("unexpected missing-column error: %s", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := execute([]string{"--db", primaryPath, "--max-findings", "-1"}, &out, &errOut); code != exitError {
		t.Fatalf("invalid flags exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(errOut.String(), "must not be negative") {
		t.Fatalf("unexpected usage error: %s", errOut.String())
	}
}

type testContract struct {
	standard         uint64
	value            uint64
	ids              []uint64
	multiTokenValues [][2]uint64
}

func packTestRow(contracts ...testContract) []byte {
	var buf []byte
	buf = appendTestVaruint(buf, 0)
	buf = appendTestVaruint(buf, 0)
	buf = appendTestVaruint(buf, 0)
	buf = appendTestVaruint(buf, uint64(len(contracts)))
	for i, contract := range contracts {
		buf = append(buf, testAddress(byte(i+1))...)
		buf = appendTestVaruint(buf, 1<<2|contract.standard)
		switch contract.standard {
		case 0:
			buf = appendTestBigint(buf, contract.value)
		case 1:
			buf = appendTestVaruint(buf, uint64(len(contract.ids)))
			for _, id := range contract.ids {
				buf = appendTestBigint(buf, id)
			}
		case 2:
			buf = appendTestVaruint(buf, uint64(len(contract.multiTokenValues)))
			for _, value := range contract.multiTokenValues {
				buf = appendTestBigint(buf, value[0])
				buf = appendTestBigint(buf, value[1])
			}
		}
	}
	return buf
}

func appendTestVaruint(buf []byte, value uint64) []byte {
	var encoded [vlq.MaxLen64]byte
	length := vlq.PutUint(encoded[:], value)
	return append(buf, encoded[:length]...)
}

func appendTestBigint(buf []byte, value uint64) []byte {
	encoded := new(big.Int).SetUint64(value).Bytes()
	buf = append(buf, byte(len(encoded)))
	return append(buf, encoded...)
}

func testAddress(seed byte) []byte {
	address := make([]byte, ethereumAddressLength)
	for i := range address {
		address[i] = seed
	}
	return address
}

func openPrimaryTestDB(t *testing.T, path string) *secondaryReader {
	t.Helper()
	cache := grocksdb.NewLRUCache(64 << 20)
	reader := &secondaryReader{cache: cache}
	dbOpts, dbBlockOpts := newReadOptions(cache, -1)
	defaultOpts, defaultBlockOpts := newReadOptions(cache, -1)
	contractsOpts, contractsBlockOpts := newReadOptions(cache, -1)
	dbOpts.SetCreateIfMissing(true)
	dbOpts.SetCreateIfMissingColumnFamilies(true)
	reader.opts = []*grocksdb.Options{dbOpts, defaultOpts, contractsOpts}
	reader.blockOpts = []*grocksdb.BlockBasedTableOptions{dbBlockOpts, defaultBlockOpts, contractsBlockOpts}

	db, handles, err := grocksdb.OpenDbColumnFamilies(
		dbOpts,
		path,
		[]string{defaultCF, addressContractsCF},
		[]*grocksdb.Options{defaultOpts, contractsOpts},
	)
	if err != nil {
		reader.close()
		t.Fatal(err)
	}
	reader.db = db
	reader.cfHandles = handles
	return reader
}

func openPrimaryWithoutAddressContracts(t *testing.T, path string) *secondaryReader {
	t.Helper()
	cache := grocksdb.NewLRUCache(64 << 20)
	reader := &secondaryReader{cache: cache}
	dbOpts, dbBlockOpts := newReadOptions(cache, -1)
	defaultOpts, defaultBlockOpts := newReadOptions(cache, -1)
	dbOpts.SetCreateIfMissing(true)
	reader.opts = []*grocksdb.Options{dbOpts, defaultOpts}
	reader.blockOpts = []*grocksdb.BlockBasedTableOptions{dbBlockOpts, defaultBlockOpts}

	db, handles, err := grocksdb.OpenDbColumnFamilies(dbOpts, path, []string{defaultCF}, []*grocksdb.Options{defaultOpts})
	if err != nil {
		reader.close()
		t.Fatal(err)
	}
	reader.db = db
	reader.cfHandles = handles
	return reader
}

func writeTestData(t *testing.T, primary *secondaryReader, version uint32, rows map[byte][]byte) {
	t.Helper()
	state := struct {
		DBColumns []struct {
			Name    string `json:"name"`
			Version uint32 `json:"version"`
		} `json:"dbColumns"`
	}{}
	state.DBColumns = append(state.DBColumns, struct {
		Name    string `json:"name"`
		Version uint32 `json:"version"`
	}{Name: addressContractsCF, Version: version})
	encodedState, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	wo := grocksdb.NewDefaultWriteOptions()
	defer wo.Destroy()
	if err := primary.db.PutCF(wo, primary.cfHandles[0], []byte(internalStateKey), encodedState); err != nil {
		t.Fatal(err)
	}
	for seed, row := range rows {
		if err := primary.db.PutCF(wo, primary.cfHandles[1], testAddress(seed), row); err != nil {
			t.Fatal(err)
		}
	}
	flush := grocksdb.NewDefaultFlushOptions()
	defer flush.Destroy()
	for _, handle := range primary.cfHandles {
		if err := primary.db.FlushCF(handle, flush); err != nil {
			t.Fatal(err)
		}
	}
}
