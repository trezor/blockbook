// Command erc721-holdings-check scans Blockbook's addressContracts column
// family for duplicate ERC-721 token IDs without modifying the primary DB.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/linxGnu/grocksdb"
)

const (
	defaultCF                        = "default"
	addressContractsCF               = "addressContracts"
	internalStateKey                 = "internalState"
	supportedAddressContractsVersion = 7
	ethereumAddressLength            = 20
	progressRows                     = 1_000_000

	exitClean    = 0
	exitFindings = 1
	exitError    = 2
)

type config struct {
	dbPath        string
	secondaryPath string
	maxFindings   int64
}

type scanSummary struct {
	Rows             int64
	ERC721Contracts  int64
	ERC721IDs        int64
	DuplicateGroups  int64
	DuplicateEntries int64
	PrintedFindings  int64
	Duration         time.Duration
}

type rowSummary struct {
	ERC721Contracts  int64
	ERC721IDs        int64
	DuplicateGroups  int64
	DuplicateEntries int64
}

type finding struct {
	Address     []byte
	Contract    []byte
	TokenID     *big.Int
	Occurrences int
}

func main() {
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}

func execute(args []string, out, errOut io.Writer) int {
	cfg, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return exitError
	}

	summary, err := run(cfg, out, errOut)
	if err != nil {
		fmt.Fprintln(errOut, "ERC-721 holdings check failed:", err)
		return exitError
	}

	fmt.Fprintf(out, "scanned rows=%d erc721_contracts=%d erc721_ids=%d duplicate_groups=%d duplicate_entries=%d duration=%s\n",
		summary.Rows,
		summary.ERC721Contracts,
		summary.ERC721IDs,
		summary.DuplicateGroups,
		summary.DuplicateEntries,
		summary.Duration.Round(time.Millisecond),
	)
	if summary.DuplicateGroups > summary.PrintedFindings {
		fmt.Fprintf(out, "omitted %d duplicate findings because of --max-findings\n", summary.DuplicateGroups-summary.PrintedFindings)
	}
	if summary.DuplicateGroups > 0 {
		fmt.Fprintln(out, "REINDEX REQUIRED: duplicate ERC-721 holdings were found; rebuild this Blockbook database with a fixed binary.")
		return exitFindings
	}

	fmt.Fprintln(out, "No duplicate ERC-721 holding IDs found. This does not prove that every stored token owner is correct.")
	return exitClean
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("erc721-holdings-check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.dbPath, "db", "", "path to the primary Blockbook RocksDB directory")
	fs.StringVar(&cfg.secondaryPath, "secondary-path", "", "directory for RocksDB secondary metadata; defaults to a temporary directory")
	fs.Int64Var(&cfg.maxFindings, "max-findings", 100, "maximum detailed findings to print; 0 means unlimited")
	if err := fs.Parse(args); err != nil {
		return cfg, usageError(err)
	}
	if cfg.dbPath == "" {
		return cfg, usageError(errors.New("missing required --db"))
	}
	if cfg.maxFindings < 0 {
		return cfg, usageError(errors.New("--max-findings must not be negative"))
	}
	return cfg, nil
}

func usageError(err error) error {
	return fmt.Errorf(`%w

Usage:
  go run ./contrib/scripts/erc721-holdings-check --db /path/to/blockbook/db [flags]

Flags:
  --db              path to the primary Blockbook RocksDB directory
  --secondary-path  directory for RocksDB secondary metadata; defaults to a temporary directory
  --max-findings    maximum detailed findings to print; 0 means unlimited

Exit codes:
  0  no duplicate ERC-721 holding IDs found
  1  duplicate ERC-721 holding IDs found; reindex required
  2  usage, schema, RocksDB, or decoding error`, err)
}

func run(cfg config, out, errOut io.Writer) (scanSummary, error) {
	secondaryPath := cfg.secondaryPath
	removeSecondary := false
	if secondaryPath == "" {
		tmp, err := os.MkdirTemp("", "blockbook-erc721-check-secondary-*")
		if err != nil {
			return scanSummary{}, err
		}
		secondaryPath = tmp
		removeSecondary = true
	} else if err := os.MkdirAll(secondaryPath, 0o755); err != nil {
		return scanSummary{}, err
	}
	if removeSecondary {
		defer os.RemoveAll(secondaryPath)
	}

	reader, err := openSecondary(cfg.dbPath, secondaryPath)
	if err != nil {
		return scanSummary{}, err
	}
	defer reader.close()
	if err := reader.db.TryCatchUpWithPrimary(); err != nil {
		return scanSummary{}, fmt.Errorf("catch up with primary: %w", err)
	}
	if err := reader.validateSchema(); err != nil {
		return scanSummary{}, err
	}
	return reader.scan(out, errOut, cfg.maxFindings)
}

type secondaryReader struct {
	db        *grocksdb.DB
	cfHandles []*grocksdb.ColumnFamilyHandle
	opts      []*grocksdb.Options
	blockOpts []*grocksdb.BlockBasedTableOptions
	cache     *grocksdb.Cache
}

func openSecondary(dbPath, secondaryPath string) (*secondaryReader, error) {
	cache := grocksdb.NewLRUCache(64 << 20)
	reader := &secondaryReader{cache: cache}

	dbOpts, dbBlockOpts := newReadOptions(cache, -1)
	defaultOpts, defaultBlockOpts := newReadOptions(cache, -1)
	contractsOpts, contractsBlockOpts := newReadOptions(cache, -1)
	reader.opts = []*grocksdb.Options{dbOpts, defaultOpts, contractsOpts}
	reader.blockOpts = []*grocksdb.BlockBasedTableOptions{dbBlockOpts, defaultBlockOpts, contractsBlockOpts}

	db, handles, err := grocksdb.OpenDbAsSecondaryColumnFamilies(
		dbOpts,
		filepath.Clean(dbPath),
		filepath.Clean(secondaryPath),
		[]string{defaultCF, addressContractsCF},
		[]*grocksdb.Options{defaultOpts, contractsOpts},
	)
	if err != nil {
		reader.close()
		return nil, fmt.Errorf("open RocksDB secondary: %w", err)
	}
	reader.db = db
	reader.cfHandles = handles
	return reader, nil
}

func newReadOptions(cache *grocksdb.Cache, maxOpenFiles int) (*grocksdb.Options, *grocksdb.BlockBasedTableOptions) {
	blockOpts := grocksdb.NewDefaultBlockBasedTableOptions()
	blockOpts.SetBlockSize(32 << 10)
	blockOpts.SetBlockCache(cache)
	blockOpts.SetFormatVersion(4)

	opts := grocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(blockOpts)
	opts.SetMaxOpenFiles(maxOpenFiles)
	opts.SetCompression(grocksdb.LZ4HCCompression)
	return opts, blockOpts
}

func (r *secondaryReader) validateSchema() error {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()
	value, err := r.db.GetCF(ro, r.cfHandles[0], []byte(internalStateKey))
	if err != nil {
		return fmt.Errorf("read internal state: %w", err)
	}
	defer value.Free()
	if len(value.Data()) == 0 {
		return errors.New("internal state is missing")
	}

	var state struct {
		DBColumns []struct {
			Name    string `json:"name"`
			Version uint32 `json:"version"`
		} `json:"dbColumns"`
	}
	if err := json.Unmarshal(value.Data(), &state); err != nil {
		return fmt.Errorf("decode internal state: %w", err)
	}
	for _, column := range state.DBColumns {
		if column.Name == addressContractsCF {
			if column.Version != supportedAddressContractsVersion {
				return fmt.Errorf("unsupported addressContracts schema version %d, expected %d", column.Version, supportedAddressContractsVersion)
			}
			return nil
		}
	}
	return errors.New("internal state does not describe the addressContracts column")
}

func (r *secondaryReader) scan(out, errOut io.Writer, maxFindings int64) (scanSummary, error) {
	start := time.Now()
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()
	ro.SetFillCache(false)

	it := r.db.NewIteratorCF(ro, r.cfHandles[1])
	defer it.Close()
	var summary scanSummary
	for it.SeekToFirst(); it.Valid(); it.Next() {
		address := it.Key().Data()
		if len(address) != ethereumAddressLength {
			return summary, fmt.Errorf("invalid addressContracts key length %d for key %s", len(address), hex.EncodeToString(address))
		}
		row, findings, err := inspectRow(address, it.Value().Data())
		if err != nil {
			return summary, fmt.Errorf("decode addressContracts row %s: %w", hex.EncodeToString(address), err)
		}
		summary.Rows++
		summary.ERC721Contracts += row.ERC721Contracts
		summary.ERC721IDs += row.ERC721IDs
		summary.DuplicateGroups += row.DuplicateGroups
		summary.DuplicateEntries += row.DuplicateEntries
		for _, f := range findings {
			if maxFindings == 0 || summary.PrintedFindings < maxFindings {
				fmt.Fprintf(out, "duplicate address=0x%s contract=0x%s token_id=%s occurrences=%d\n",
					hex.EncodeToString(f.Address), hex.EncodeToString(f.Contract), f.TokenID.String(), f.Occurrences)
				summary.PrintedFindings++
			}
		}
		if summary.Rows%progressRows == 0 {
			fmt.Fprintf(errOut, "scanned %d addressContracts rows, found %d duplicate groups\n", summary.Rows, summary.DuplicateGroups)
		}
	}
	if err := it.Err(); err != nil {
		return summary, fmt.Errorf("iterate addressContracts: %w", err)
	}
	summary.Duration = time.Since(start)
	return summary, nil
}

func inspectRow(address, buf []byte) (rowSummary, []finding, error) {
	d := rowDecoder{buf: buf}
	for _, name := range []string{"total transactions", "non-contract transactions", "internal transactions"} {
		if _, err := d.varuint(name); err != nil {
			return rowSummary{}, nil, err
		}
	}
	contractCount, err := d.varuint("contract count")
	if err != nil {
		return rowSummary{}, nil, err
	}

	var summary rowSummary
	var findings []finding
	for contractIndex := uint64(0); contractIndex < contractCount; contractIndex++ {
		contract, err := d.bytes(ethereumAddressLength, "contract address")
		if err != nil {
			return summary, nil, fmt.Errorf("contract %d: %w", contractIndex, err)
		}
		txsAndStandard, err := d.varuint("transaction count and token standard")
		if err != nil {
			return summary, nil, fmt.Errorf("contract %d: %w", contractIndex, err)
		}
		standard := txsAndStandard & 3
		switch standard {
		case 0: // ERC-20
			if _, err := d.bigint("ERC-20 value"); err != nil {
				return summary, nil, fmt.Errorf("contract %d: %w", contractIndex, err)
			}
		case 1: // ERC-721
			idCount, err := d.varuint("ERC-721 ID count")
			if err != nil {
				return summary, nil, fmt.Errorf("contract %d: %w", contractIndex, err)
			}
			summary.ERC721Contracts++
			summary.ERC721IDs += int64(idCount)
			counts := make(map[string]int)
			order := make([]string, 0)
			for idIndex := uint64(0); idIndex < idCount; idIndex++ {
				id, err := d.bigint("ERC-721 token ID")
				if err != nil {
					return summary, nil, fmt.Errorf("contract %d ID %d: %w", contractIndex, idIndex, err)
				}
				key := string(id)
				if counts[key] == 0 {
					order = append(order, key)
				}
				counts[key]++
			}
			for _, key := range order {
				occurrences := counts[key]
				if occurrences > 1 {
					summary.DuplicateGroups++
					summary.DuplicateEntries += int64(occurrences - 1)
					findings = append(findings, finding{
						Address:     append([]byte(nil), address...),
						Contract:    append([]byte(nil), contract...),
						TokenID:     new(big.Int).SetBytes([]byte(key)),
						Occurrences: occurrences,
					})
				}
			}
		case 2: // ERC-1155
			valueCount, err := d.varuint("ERC-1155 value count")
			if err != nil {
				return summary, nil, fmt.Errorf("contract %d: %w", contractIndex, err)
			}
			for valueIndex := uint64(0); valueIndex < valueCount; valueIndex++ {
				if _, err := d.bigint("ERC-1155 token ID"); err != nil {
					return summary, nil, fmt.Errorf("contract %d value %d: %w", contractIndex, valueIndex, err)
				}
				if _, err := d.bigint("ERC-1155 value"); err != nil {
					return summary, nil, fmt.Errorf("contract %d value %d: %w", contractIndex, valueIndex, err)
				}
			}
		default:
			return summary, nil, fmt.Errorf("contract %d: unsupported token standard %d", contractIndex, standard)
		}
	}
	if d.offset != len(buf) {
		return summary, nil, fmt.Errorf("%d trailing bytes after %d contracts", len(buf)-d.offset, contractCount)
	}
	return summary, findings, nil
}

type rowDecoder struct {
	buf    []byte
	offset int
}

func (d *rowDecoder) varuint(name string) (uint64, error) {
	value, length := vlq.Uint(d.buf[d.offset:])
	if length == 0 {
		return 0, fmt.Errorf("truncated %s at offset %d", name, d.offset)
	}
	if length < 0 {
		return 0, fmt.Errorf("overflowed %s at offset %d", name, d.offset)
	}
	d.offset += length
	return value, nil
}

func (d *rowDecoder) bytes(length int, name string) ([]byte, error) {
	if length < 0 || len(d.buf)-d.offset < length {
		return nil, fmt.Errorf("truncated %s at offset %d", name, d.offset)
	}
	value := d.buf[d.offset : d.offset+length]
	d.offset += length
	return value, nil
}

func (d *rowDecoder) bigint(name string) ([]byte, error) {
	length, err := d.bytes(1, name+" length")
	if err != nil {
		return nil, err
	}
	return d.bytes(int(length[0]), name)
}

func (r *secondaryReader) close() {
	for _, handle := range r.cfHandles {
		if handle != nil {
			handle.Destroy()
		}
	}
	if r.db != nil {
		r.db.Close()
	}
	for _, opt := range r.opts {
		if opt != nil {
			opt.Destroy()
		}
	}
	for _, blockOpt := range r.blockOpts {
		if blockOpt != nil {
			blockOpt.Destroy()
		}
	}
	if r.cache != nil {
		r.cache.Destroy()
	}
}
