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
	got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
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
	got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
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
			if got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil); got != nil {
				t.Fatalf("expected nil when totalAssets fails, got %+v", got)
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
		t.Fatal("must not persist when contract is not a vault")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		t.Fatal("must not fetch asset metadata when contract is not a vault")
		return nil, false, nil
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	if got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil); got != nil {
		t.Fatalf("expected nil for non-vault, got %+v", got)
	}
}

func TestBuildErc4626Token_AssetMetadataInvalid_StillReturnsPartial(t *testing.T) {
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
	got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil)
	if got == nil {
		t.Fatal("expected partial result, got nil")
	}
	if got.TotalAssetsSat == nil {
		t.Fatal("totalAssets should still be populated from multicall A")
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

func TestBuildErc4626Token_MulticallError_ReturnsNil(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000a1"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				return nil, errors.New("rpc down")
			},
		},
	}
	persister := func(string, string) error { return nil }
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) { return nil, false, nil }
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	if got := buildErc4626TokenWithDeps(ci, mc, persister, getContractInfo, nil); got != nil {
		t.Fatalf("expected nil on multicall error, got %+v", got)
	}
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
