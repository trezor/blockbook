package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

// fakeMulticaller records calls and replays a sequence of canned responses.
type fakeMulticaller struct {
	calls    [][]bchain.EthereumMulticallCall
	handlers []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error)
	idx      int
}

func (f *fakeMulticaller) EthereumTypeMulticallAggregate3(calls []bchain.EthereumMulticallCall, _ *big.Int) ([]bchain.EthereumMulticallResult, error) {
	copied := append([]bchain.EthereumMulticallCall(nil), calls...)
	f.calls = append(f.calls, copied)
	if f.idx >= len(f.handlers) {
		return nil, fmt.Errorf("unexpected multicall call %d", f.idx)
	}
	h := f.handlers[f.idx]
	f.idx++
	return h(calls)
}

func encodeWordAddress(address string) string {
	a := ethcommon.HexToAddress(address)
	word := make([]byte, 32)
	copy(word[12:], a.Bytes())
	return "0x" + hex.EncodeToString(word)
}

func encodeWordUint(v *big.Int) string {
	word := make([]byte, 32)
	v.FillBytes(word)
	return "0x" + hex.EncodeToString(word)
}

func TestBuildErc4626Token_ColdPath_PersistsAssetAndIssuesTwoMulticalls(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	totalAssets := big.NewInt(123456)
	convertToAssets := big.NewInt(2_000_000_000_000_000_000)
	previewRedeem := big.NewInt(1_999_000_000_000_000_000)
	convertToShares := big.NewInt(500_000)
	previewDeposit := big.NewInt(499_750)

	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			// Multicall A: asset, totalAssets, convertToAssets(1share), previewRedeem(1share)
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 4 {
					t.Fatalf("expected 4 calls in multicall A, got %d", len(calls))
				}
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(asset)},
					{Success: true, Data: encodeWordUint(totalAssets)},
					{Success: true, Data: encodeWordUint(convertToAssets)},
					{Success: true, Data: encodeWordUint(previewRedeem)},
				}, nil
			},
			// Multicall B: convertToShares(1asset), previewDeposit(1asset)
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 2 {
					t.Fatalf("expected 2 calls in multicall B, got %d", len(calls))
				}
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordUint(convertToShares)},
					{Success: true, Data: encodeWordUint(previewDeposit)},
				}, nil
			},
		},
	}

	var persistedAddr, persistedAsset string
	persisted := 0
	persister := func(addr, ast string) error {
		persisted++
		persistedAddr, persistedAsset = addr, ast
		return nil
	}
	getContractInfo := func(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		if !strings.EqualFold(contract, asset) {
			t.Fatalf("unexpected getContractInfo target %s", contract)
		}
		return &bchain.ContractInfo{Contract: asset, Name: "USD Coin", Symbol: "USDC", Decimals: 6}, true, nil
	}

	ci := &bchain.ContractInfo{Contract: vault, Name: "Vault Share", Symbol: "vUSDC", Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if err != nil {
		t.Fatalf("expected nil err on a fully-successful build (cacheable), got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Error != "" {
		t.Fatalf("expected no error string, got %q", got.Error)
	}
	if got.Asset == nil || got.Asset.Decimals != 6 || got.Asset.Symbol != "USDC" {
		t.Fatalf("asset metadata wrong: %+v", got.Asset)
	}
	if got.Share == nil || got.Share.Decimals != 18 || got.Share.Symbol != "vUSDC" {
		t.Fatalf("share metadata wrong: %+v", got.Share)
	}
	if got.TotalAssetsSat == nil || (*big.Int)(got.TotalAssetsSat).Cmp(totalAssets) != 0 {
		t.Fatalf("totalAssets wrong: %v", got.TotalAssetsSat)
	}
	if got.ConvertToAssets1ShareSat == nil || got.PreviewRedeem1ShareSat == nil {
		t.Fatal("share-side conversions missing")
	}
	if got.ConvertToShares1AssetSat == nil || got.PreviewDeposit1AssetSat == nil {
		t.Fatal("asset-side conversions missing")
	}
	if persisted != 1 || persistedAddr != vault || !strings.EqualFold(persistedAsset, asset) {
		t.Fatalf("persister not called correctly: count=%d addr=%s asset=%s", persisted, persistedAddr, persistedAsset)
	}
	if len(mc.calls) != 2 {
		t.Fatalf("expected 2 multicalls, got %d", len(mc.calls))
	}
}

func TestBuildErc4626Token_WarmPath_OneMulticall(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	totalAssets := big.NewInt(50)
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 5 {
					t.Fatalf("expected 5 calls, got %d", len(calls))
				}
				results := make([]bchain.EthereumMulticallResult, 5)
				results[0] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordUint(totalAssets)}
				for i := 1; i < 5; i++ {
					results[i] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordUint(big.NewInt(int64(i)))}
				}
				return results, nil
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("warm path must not persist")
		return nil
	}
	getContractInfo := func(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return &bchain.ContractInfo{Contract: asset, Name: "USDC", Symbol: "USDC", Decimals: 6}, true, nil
	}
	ci := &bchain.ContractInfo{
		Contract:             vault,
		Name:                 "Vault Share",
		Symbol:               "vUSDC",
		Decimals:             18,
		IsErc4626:            true,
		Erc4626AssetContract: asset,
	}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if err != nil {
		t.Fatalf("expected nil err on a fully-successful warm build, got %v", err)
	}
	if got == nil || got.Error != "" {
		t.Fatalf("warm-path failed: %+v", got)
	}
	if len(mc.calls) != 1 {
		t.Fatalf("warm path expected 1 multicall, got %d", len(mc.calls))
	}
	if got.TotalAssetsSat == nil || (*big.Int)(got.TotalAssetsSat).Cmp(totalAssets) != 0 {
		t.Fatalf("totalAssets wrong: %v", got.TotalAssetsSat)
	}
	if got.ConvertToAssets1ShareSat == nil || got.PreviewRedeem1ShareSat == nil ||
		got.ConvertToShares1AssetSat == nil || got.PreviewDeposit1AssetSat == nil {
		t.Fatalf("conversion fields missing: %+v", got)
	}
}

func TestBuildErc4626Token_TotalAssetsFails_NoPersistAndReturnsNil(t *testing.T) {
	// Detection must require BOTH asset() and totalAssets() to succeed. A fungible
	// contract that exposes asset() returning some non-zero value but reverts on
	// totalAssets() must NOT be persisted as an ERC4626 vault, otherwise accountInfo
	// would falsely advertise erc4626 support for it on every subsequent request.
	const vault = "0x00000000000000000000000000000000000000d1"
	const fakeAsset = "0x00000000000000000000000000000000000000ee"

	for _, tc := range []struct {
		name        string
		totalAssets bchain.EthereumMulticallResult
	}{
		{"reverted", bchain.EthereumMulticallResult{Success: false, Data: "0x"}},
		{"undecodable", bchain.EthereumMulticallResult{Success: true, Data: "0x1234"}}, // < 32 bytes
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc := &fakeMulticaller{
				handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
					func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
						return []bchain.EthereumMulticallResult{
							{Success: true, Data: encodeWordAddress(fakeAsset)},
							tc.totalAssets,
							{Success: true, Data: encodeWordUint(big.NewInt(0))},
							{Success: true, Data: encodeWordUint(big.NewInt(0))},
						}, nil
					},
				},
			}
			persisted := 0
			persister := func(string, string) error {
				persisted++
				return nil
			}
			getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
				t.Fatal("must not lazy-fetch asset metadata when detection fails")
				return nil, false, nil
			}
			ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
			got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
			if got != nil {
				t.Fatalf("expected nil when totalAssets fails, got %+v", got)
			}
			// Detection failure is a deterministic on-chain answer ("not a vault")
			// and must be cacheable: err must be nil so the LRU memoises (nil).
			if err != nil {
				t.Fatalf("deterministic 'not a vault' must return nil err so the cache memoises it, got %v", err)
			}
			if persisted != 0 {
				t.Fatalf("must not persist when totalAssets fails (persisted=%d)", persisted)
			}
			if len(mc.calls) != 1 {
				t.Fatalf("expected exactly 1 multicall (no asset-side fetch), got %d", len(mc.calls))
			}
		})
	}
}

func TestBuildErc4626Token_NotAVault_ReturnsNil(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000c1"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)}, // asset() = 0x0
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
				}, nil
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("must not persist as vault when contract is not a vault")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		t.Fatal("must not fetch asset metadata when contract is not a vault")
		return nil, false, nil
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got != nil {
		t.Fatalf("expected nil for non-vault, got %+v", got)
	}
	if err != nil {
		t.Fatalf("'not a vault' must return nil err so the cache can memoise it, got %v", err)
	}
}

func TestBuildErc4626Token_AssetMetadataInvalid_OmitsAssetMetadata(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(asset)},
					{Success: true, Data: encodeWordUint(big.NewInt(7))},
					{Success: true, Data: encodeWordUint(big.NewInt(8))},
					{Success: true, Data: encodeWordUint(big.NewInt(9))},
				}, nil
			},
			// Multicall B should NOT be issued because asset metadata is invalid.
		},
	}
	persister := func(string, string) error { return nil }
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return nil, false, nil // asset contract not a known fungible token
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got == nil {
		t.Fatal("expected partial result, got nil")
	}
	// (nil, false, nil) from the fetcher is a deterministic "not in our store"
	// answer (no transport problem), so the build must report no transient
	// error and stay cacheable for the block.
	if err != nil {
		t.Fatalf("deterministic 'asset metadata unavailable' must remain cacheable, got err %v", err)
	}
	if got.TotalAssetsSat == nil {
		t.Fatal("totalAssets should still be populated from multicall A")
	}
	if got.Asset != nil {
		t.Fatalf("asset metadata must be omitted when decimals are unavailable, got %+v", got.Asset)
	}
	if got.ConvertToShares1AssetSat != nil || got.PreviewDeposit1AssetSat != nil {
		t.Fatalf("asset-side conversions should be skipped when asset metadata invalid")
	}
	if !strings.Contains(got.Error, "asset metadata unavailable") {
		t.Fatalf("expected error to mention asset metadata, got %q", got.Error)
	}
	if len(mc.calls) != 1 {
		t.Fatalf("expected 1 multicall when asset metadata invalid, got %d", len(mc.calls))
	}
}

func TestBuildErc4626Token_ColdMulticallError_ReturnsNilAndTransientErr(t *testing.T) {
	// A multicall A transport error must return (nil, err) — caller sees no
	// enrichment, and the cache layer must skip persisting the negative.
	const vault = "0x00000000000000000000000000000000000000a1"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return nil, errors.New("rpc down")
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("must not persist on transport error")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) { return nil, false, nil }
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got != nil {
		t.Fatalf("expected nil on multicall error, got %+v", got)
	}
	if err == nil {
		t.Fatal("transport error must propagate so the cache skips memoising the negative")
	}
}

func TestBuildErc4626Token_ColdAssetMetadataError_ReturnsResultAndTransientErr(t *testing.T) {
	// Cold detection succeeds, then the asset-metadata fetcher errors transiently
	// (e.g. DB or RPC blip). The vault is real, so the caller must receive the
	// confirmed-vault snapshot — but the build must propagate the error so the
	// cache does not memoise a metadata-less view of the vault for the block.
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(asset)},
					{Success: true, Data: encodeWordUint(big.NewInt(7))},
					{Success: true, Data: encodeWordUint(big.NewInt(1))},
					{Success: true, Data: encodeWordUint(big.NewInt(2))},
				}, nil
			},
		},
	}
	persister := func(string, string) error { return nil }
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return nil, false, errors.New("db blip")
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got == nil {
		t.Fatal("expected confirmed-vault result even when asset metadata fetcher errors")
	}
	if err == nil {
		t.Fatal("metadata-fetcher transient error must propagate so the cache skips this entry")
	}
	if !strings.Contains(got.Error, "asset metadata") {
		t.Fatalf("expected got.Error to mention asset metadata, got %q", got.Error)
	}
	if got.Asset != nil {
		t.Fatalf("asset metadata must be omitted when metadata fetcher errors, got %+v", got.Asset)
	}
	if got.TotalAssetsSat == nil {
		t.Fatal("totalAssets should still be populated from multicall A")
	}
}

func TestBuildErc4626Token_WarmAssetMetadataInvalid_OmitsAssetMetadata(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 3 {
					t.Fatalf("expected totalAssets and share-side calls only, got %d calls", len(calls))
				}
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordUint(big.NewInt(7))},
					{Success: true, Data: encodeWordUint(big.NewInt(1))},
					{Success: true, Data: encodeWordUint(big.NewInt(2))},
				}, nil
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("warm path must not persist")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return nil, false, nil
	}
	ci := &bchain.ContractInfo{
		Contract:             vault,
		Decimals:             18,
		IsErc4626:            true,
		Erc4626AssetContract: asset,
	}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if err != nil {
		t.Fatalf("deterministic asset metadata miss should remain cacheable, got %v", err)
	}
	if got == nil {
		t.Fatal("expected partial warm result")
	}
	if got.Asset != nil {
		t.Fatalf("asset metadata must be omitted when decimals are unavailable, got %+v", got.Asset)
	}
	if got.ConvertToShares1AssetSat != nil || got.PreviewDeposit1AssetSat != nil {
		t.Fatalf("asset-side conversions should be skipped without asset decimals: %+v", got)
	}
	if got.ConvertToAssets1ShareSat == nil || got.PreviewRedeem1ShareSat == nil {
		t.Fatalf("share-side conversions should still be returned: %+v", got)
	}
	if !strings.Contains(got.Error, "asset metadata unavailable") {
		t.Fatalf("expected error to mention asset metadata, got %q", got.Error)
	}
}

func TestBuildErc4626Token_ColdMulticallBError_ReturnsResultAndTransientErr(t *testing.T) {
	// Cold detection succeeds, asset metadata is available, but multicall B
	// (asset-side conversions) errors transiently. The caller gets the partial
	// snapshot; the cache must skip so a fresh attempt happens next request.
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(asset)},
					{Success: true, Data: encodeWordUint(big.NewInt(42))},
					{Success: true, Data: encodeWordUint(big.NewInt(1))},
					{Success: true, Data: encodeWordUint(big.NewInt(2))},
				}, nil
			},
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return nil, errors.New("multicall B down")
			},
		},
	}
	persister := func(string, string) error { return nil }
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return &bchain.ContractInfo{Contract: asset, Name: "USDC", Symbol: "USDC", Decimals: 6}, true, nil
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got == nil {
		t.Fatal("expected partial result on multicall B error")
	}
	if err == nil {
		t.Fatal("multicall B transient error must propagate so the cache skips this entry")
	}
	if got.ConvertToShares1AssetSat != nil || got.PreviewDeposit1AssetSat != nil {
		t.Fatalf("asset-side conversions must be nil when multicall B failed, got %+v", got)
	}
	if !strings.Contains(got.Error, "asset-side multicall") {
		t.Fatalf("expected got.Error to mention asset-side multicall, got %q", got.Error)
	}
}

func TestBuildErc4626Token_WarmMulticallError_ReturnsPartialAndTransientErr(t *testing.T) {
	// Warm path: vault is already known. Multicall transport failure must yield
	// the metadata-only partial result AND a non-nil err so the cache layer
	// skips this entry rather than memoising a totalAssets-less view.
	const vault = "0x00000000000000000000000000000000000000a1"
	const asset = "0x00000000000000000000000000000000000000b2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return nil, errors.New("rpc down")
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("warm path must not persist")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return &bchain.ContractInfo{Contract: asset, Name: "USDC", Symbol: "USDC", Decimals: 6}, true, nil
	}
	ci := &bchain.ContractInfo{
		Contract:             vault,
		Name:                 "Vault Share",
		Symbol:               "vUSDC",
		Decimals:             18,
		IsErc4626:            true,
		Erc4626AssetContract: asset,
	}
	got, err := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got == nil {
		t.Fatal("warm path must return partial result even on multicall error")
	}
	if err == nil {
		t.Fatal("warm-path multicall error must propagate so the cache skips this entry")
	}
	if got.TotalAssetsSat != nil {
		t.Fatalf("totalAssets must be nil when multicall failed, got %v", got.TotalAssetsSat)
	}
	if got.Asset == nil || got.Asset.Decimals != 6 || got.Asset.Symbol != "USDC" {
		t.Fatalf("asset metadata should still be populated: %+v", got.Asset)
	}
	if !strings.Contains(got.Error, "multicall:") {
		t.Fatalf("expected got.Error to mention multicall, got %q", got.Error)
	}
}

// --- enrichErc4626TokensWithDeps (accountInfo lazy-probe path) ---

type fakeContractInfoStore map[string]*bchain.ContractInfo

func (f fakeContractInfoStore) get(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
	ci, ok := f[strings.ToLower(contract)]
	if !ok {
		return nil, false, nil
	}
	return ci, true, nil
}

const erc4626Standard bchain.TokenStandardName = bchain.ERC20TokenStandard

func TestEnrichErc4626Tokens_FlagsKnownVaultAndProbesUnprobed(t *testing.T) {
	const knownVault = "0x00000000000000000000000000000000000000a1"
	const unprobedVault = "0x00000000000000000000000000000000000000a2"
	const knownAsset = "0x00000000000000000000000000000000000000b1"
	const newAsset = "0x00000000000000000000000000000000000000b2"

	store := fakeContractInfoStore{
		strings.ToLower(knownVault): {
			Contract:             knownVault,
			Standard:             erc4626Standard,
			IsErc4626:            true,
			Erc4626AssetContract: knownAsset,
		},
		strings.ToLower(unprobedVault): {
			Contract: unprobedVault,
			Standard: erc4626Standard,
		},
	}

	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 2 {
					t.Fatalf("expected 2 sub-calls (1 unprobed candidate × 2), got %d", len(calls))
				}
				if calls[0].Target != unprobedVault || calls[1].Target != unprobedVault {
					t.Fatalf("unexpected targets: %s, %s", calls[0].Target, calls[1].Target)
				}
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(newAsset)},
					{Success: true, Data: encodeWordUint(big.NewInt(42))},
				}, nil
			},
		},
	}

	persisted := map[string]string{}
	setVault := func(addr, asset string) error {
		persisted[addr] = asset
		return nil
	}
	tokens := Tokens{
		{Contract: knownVault, Standard: erc4626Standard},
		{Contract: unprobedVault, Standard: erc4626Standard},
	}
	enrichErc4626TokensWithDeps(tokens, store.get, mc, setVault, nil, 0, 0, 0)

	if !slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("known vault must be flagged: %v", tokens[0].Protocols)
	}
	if !slicesContains(tokens[1].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("freshly-probed vault must be flagged: %v", tokens[1].Protocols)
	}
	if persisted[unprobedVault] != newAsset {
		t.Fatalf("setVault not called as expected: %v", persisted)
	}
	if len(mc.calls) != 1 {
		t.Fatalf("expected exactly 1 batched multicall, got %d", len(mc.calls))
	}
}

func TestEnrichErc4626Tokens_NegativeProbeDoesNotPersist(t *testing.T) {
	const fakeFungible = "0x00000000000000000000000000000000000000d1"

	store := fakeContractInfoStore{
		strings.ToLower(fakeFungible): {Contract: fakeFungible, Standard: erc4626Standard},
	}
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
				}, nil
			},
		},
	}
	tokens := Tokens{{Contract: fakeFungible, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		nil, 0, 0, 0)
	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("non-vault must not be flagged: %v", tokens[0].Protocols)
	}
	if len(mc.calls) != 1 {
		t.Fatalf("expected one batched probe for non-vault, got %d", len(mc.calls))
	}
}

func TestEnrichErc4626Tokens_RecentNegativeSkipsReprobe(t *testing.T) {
	const fakeFungible = "0x00000000000000000000000000000000000000d2"

	store := fakeContractInfoStore{
		strings.ToLower(fakeFungible): {Contract: fakeFungible, Standard: erc4626Standard},
	}
	negativeCache := newErc4626NegativeCache(4)
	negativeCache.add(fakeFungible, 100, 2, 0)
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				t.Fatal("recent negative cache hit must skip multicall")
				return nil, nil
			},
		},
	}

	tokens := Tokens{{Contract: fakeFungible, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		negativeCache, 101, 2, 0)

	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("non-vault must not be flagged: %v", tokens[0].Protocols)
	}
	if len(mc.calls) != 0 {
		t.Fatalf("expected zero multicalls on recent negative cache hit, got %d", len(mc.calls))
	}
}

func TestEnrichErc4626Tokens_NegativeCacheExpiresAndReprobes(t *testing.T) {
	const fakeFungible = "0x00000000000000000000000000000000000000d3"

	store := fakeContractInfoStore{
		strings.ToLower(fakeFungible): {Contract: fakeFungible, Standard: erc4626Standard},
	}
	negativeCache := newErc4626NegativeCache(4)
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
				}, nil
			},
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
				}, nil
			},
		},
	}

	tokens := Tokens{{Contract: fakeFungible, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		negativeCache, 100, 2, 0)
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		negativeCache, 101, 2, 0)
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		negativeCache, 103, 2, 0)

	if len(mc.calls) != 2 {
		t.Fatalf("expected probe, cached skip, then reprobe after expiry; got %d multicalls", len(mc.calls))
	}
}

func TestEnrichErc4626Tokens_TransportErrorDoesNotPersist(t *testing.T) {
	const unprobed = "0x00000000000000000000000000000000000000e1"
	store := fakeContractInfoStore{
		strings.ToLower(unprobed): {Contract: unprobed, Standard: erc4626Standard},
	}
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return nil, errors.New("rpc down")
			},
		},
	}
	tokens := Tokens{{Contract: unprobed, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("must not setVault on transport error"); return nil },
		nil, 0, 0, 0)
	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("must not flag on transport error: %v", tokens[0].Protocols)
	}
}

func TestEnrichErc4626Tokens_NoMulticallerStillFlagsKnown(t *testing.T) {
	const knownVault = "0x00000000000000000000000000000000000000f1"
	const unprobed = "0x00000000000000000000000000000000000000f2"
	store := fakeContractInfoStore{
		strings.ToLower(knownVault): {Contract: knownVault, Standard: erc4626Standard, IsErc4626: true},
		strings.ToLower(unprobed):   {Contract: unprobed, Standard: erc4626Standard},
	}
	tokens := Tokens{
		{Contract: knownVault, Standard: erc4626Standard},
		{Contract: unprobed, Standard: erc4626Standard},
	}
	// nil multicaller (chain doesn't support multicall): must still flag known vaults.
	enrichErc4626TokensWithDeps(tokens, store.get, nil, nil, nil, 0, 0, 0)

	if !slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("known vault must be flagged even without multicaller: %v", tokens[0].Protocols)
	}
	if slicesContains(tokens[1].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("unprobed must not be flagged when multicaller is unavailable: %v", tokens[1].Protocols)
	}
}

func TestEnrichErc4626Tokens_BatchedMixed(t *testing.T) {
	// One multicall covers multiple unprobed candidates with a mix of outcomes:
	// vault, non-vault, totalAssets-decode-failure.
	const vaultA = "0x0000000000000000000000000000000000000a01"
	const fakeB = "0x0000000000000000000000000000000000000a02"
	const brokenC = "0x0000000000000000000000000000000000000a03"
	const assetA = "0x0000000000000000000000000000000000000ab1"

	store := fakeContractInfoStore{
		strings.ToLower(vaultA):  {Contract: vaultA, Standard: erc4626Standard},
		strings.ToLower(fakeB):   {Contract: fakeB, Standard: erc4626Standard},
		strings.ToLower(brokenC): {Contract: brokenC, Standard: erc4626Standard},
	}
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 6 { // 3 candidates × 2 sub-calls
					t.Fatalf("expected 6 sub-calls, got %d", len(calls))
				}
				return []bchain.EthereumMulticallResult{
					// vaultA: positive
					{Success: true, Data: encodeWordAddress(assetA)},
					{Success: true, Data: encodeWordUint(big.NewInt(100))},
					// fakeB: asset zero
					{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)},
					{Success: true, Data: encodeWordUint(big.NewInt(0))},
					// brokenC: asset OK but totalAssets undecodable
					{Success: true, Data: encodeWordAddress(assetA)},
					{Success: true, Data: "0x1234"}, // <32 bytes
				}, nil
			},
		},
	}

	persistedVaults := map[string]string{}
	setVault := func(addr, asset string) error {
		persistedVaults[addr] = asset
		return nil
	}

	tokens := Tokens{
		{Contract: vaultA, Standard: erc4626Standard},
		{Contract: fakeB, Standard: erc4626Standard},
		{Contract: brokenC, Standard: erc4626Standard},
	}
	enrichErc4626TokensWithDeps(tokens, store.get, mc, setVault, nil, 0, 0, 0)

	if !slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("vaultA should be flagged: %v", tokens[0].Protocols)
	}
	if slicesContains(tokens[1].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("fakeB must not be flagged: %v", tokens[1].Protocols)
	}
	if slicesContains(tokens[2].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("brokenC must not be flagged: %v", tokens[2].Protocols)
	}
	if !strings.EqualFold(persistedVaults[vaultA], assetA) {
		t.Fatalf("vaultA should be persisted with asset %s, got %v", assetA, persistedVaults)
	}
}

func TestEnrichErc4626Tokens_ChunksLargeProbe(t *testing.T) {
	const asset = "0x0000000000000000000000000000000000000bb1"

	store := fakeContractInfoStore{}
	tokens := make(Tokens, 0, erc4626ProbeChunkCandidates+1)
	expectedVaults := map[string]bool{}
	for i := 0; i < erc4626ProbeChunkCandidates+1; i++ {
		contract := fmt.Sprintf("0x%040x", 0x2000+i)
		store[strings.ToLower(contract)] = &bchain.ContractInfo{Contract: contract, Standard: erc4626Standard}
		tokens = append(tokens, Token{Contract: contract, Standard: erc4626Standard})
		if i == 0 || i == erc4626ProbeChunkCandidates {
			expectedVaults[strings.ToLower(contract)] = true
		}
	}

	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 2*erc4626ProbeChunkCandidates {
					t.Fatalf("unexpected first chunk size: %d", len(calls))
				}
				results := make([]bchain.EthereumMulticallResult, len(calls))
				for i := 0; i < len(calls); i += 2 {
					if i == 0 {
						results[i] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordAddress(asset)}
						results[i+1] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordUint(big.NewInt(1))}
						continue
					}
					results[i] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordAddress(erc4626ZeroAddress)}
					results[i+1] = bchain.EthereumMulticallResult{Success: true, Data: encodeWordUint(big.NewInt(0))}
				}
				return results, nil
			},
			func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				if len(calls) != 2 {
					t.Fatalf("unexpected second chunk size: %d", len(calls))
				}
				return []bchain.EthereumMulticallResult{
					{Success: true, Data: encodeWordAddress(asset)},
					{Success: true, Data: encodeWordUint(big.NewInt(1))},
				}, nil
			},
		},
	}

	persistedVaults := map[string]string{}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(addr, assetContract string) error {
			persistedVaults[strings.ToLower(addr)] = assetContract
			return nil
		},
		nil, 0, 0, 0)

	if len(mc.calls) != 2 {
		t.Fatalf("expected two multicall chunks, got %d", len(mc.calls))
	}
	for i := range tokens {
		contractKey := strings.ToLower(tokens[i].Contract)
		if expectedVaults[contractKey] {
			if !slicesContains(tokens[i].Protocols, contractInfoProtocolErc4626) {
				t.Fatalf("expected vault flag for %s", tokens[i].Contract)
			}
			if !strings.EqualFold(persistedVaults[contractKey], asset) {
				t.Fatalf("expected persisted asset for %s, got %q", tokens[i].Contract, persistedVaults[contractKey])
			}
			continue
		}
		if slicesContains(tokens[i].Protocols, contractInfoProtocolErc4626) {
			t.Fatalf("unexpected vault flag for %s", tokens[i].Contract)
		}
	}
}

func TestEnrichErc4626Tokens_NonFungibleSkipped(t *testing.T) {
	const nft = "0x000000000000000000000000000000000000abcd"
	store := fakeContractInfoStore{}
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				t.Fatal("must not probe non-fungible-standard tokens")
				return nil, nil
			},
		},
	}
	tokens := Tokens{{Contract: nft, Standard: bchain.ERC771TokenStandard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { return nil },
		nil, 0, 0, 0)
	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("non-fungible must not be flagged: %v", tokens[0].Protocols)
	}
	if len(mc.calls) != 0 {
		t.Fatalf("expected zero multicalls, got %d", len(mc.calls))
	}
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestErc4626MathAndEncodingBoundaries(t *testing.T) {
	if _, err := erc4626EncodeUintArg(erc4626MethodConvertToShares, nil); err == nil {
		t.Fatal("expected nil arg error")
	}
	if _, err := erc4626EncodeUintArg(erc4626MethodConvertToShares, big.NewInt(-1)); err == nil {
		t.Fatal("expected negative arg error")
	}
	if _, err := erc4626EncodeUintArg(erc4626MethodConvertToShares, new(big.Int).Add(erc4626MaxUint256, big.NewInt(1))); err == nil {
		t.Fatal("expected overflow arg error")
	}
	if _, err := erc4626UnitAmount(78); err == nil {
		t.Fatal("expected unsupported decimals error")
	}
	if _, err := erc4626UnitAmount(-1); err == nil {
		t.Fatal("expected negative decimals error")
	}
	unit, err := erc4626UnitAmount(0)
	if err != nil || unit.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("unexpected 10^0 result: %v, %v", unit, err)
	}
}
