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
	markProbed := func(string) error {
		t.Fatal("markProbed must not be called on positive detection")
		return nil
	}
	getContractInfo := func(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		if !strings.EqualFold(contract, asset) {
			t.Fatalf("unexpected getContractInfo target %s", contract)
		}
		return &bchain.ContractInfo{Contract: asset, Name: "USD Coin", Symbol: "USDC", Decimals: 6}, true, nil
	}

	ci := &bchain.ContractInfo{Contract: vault, Name: "Vault Share", Symbol: "vUSDC", Decimals: 18}
	got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil)
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
	markProbed := func(string) error {
		t.Fatal("warm path must not call markProbed")
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
	got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil)
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
			probedCount := 0
			var probedAddr string
			markProbed := func(addr string) error {
				probedCount++
				probedAddr = addr
				return nil
			}
			getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
				t.Fatal("must not lazy-fetch asset metadata when detection fails")
				return nil, false, nil
			}
			ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
			if got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil); got != nil {
				t.Fatalf("expected nil when totalAssets fails, got %+v", got)
			}
			if persisted != 0 {
				t.Fatalf("must not persist when totalAssets fails (persisted=%d)", persisted)
			}
			if probedCount != 1 || probedAddr != vault {
				t.Fatalf("expected markProbed once with vault addr, got count=%d addr=%s", probedCount, probedAddr)
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
	probedCount := 0
	markProbed := func(addr string) error {
		probedCount++
		if addr != vault {
			t.Fatalf("markProbed called with wrong addr %s", addr)
		}
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		t.Fatal("must not fetch asset metadata when contract is not a vault")
		return nil, false, nil
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	if got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil); got != nil {
		t.Fatalf("expected nil for non-vault, got %+v", got)
	}
	if probedCount != 1 {
		t.Fatalf("expected markProbed exactly once, got %d", probedCount)
	}
}

func TestBuildErc4626Token_AlreadyProbedNotVault_NoRPC(t *testing.T) {
	const vault = "0x00000000000000000000000000000000000000c2"
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				t.Fatal("must not call multicall when Erc4626Probed=true && IsErc4626=false")
				return nil, nil
			},
		},
	}
	persister := func(string, string) error {
		t.Fatal("must not persist again on cached negative")
		return nil
	}
	markProbed := func(string) error {
		t.Fatal("must not re-mark on cached negative")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		t.Fatal("must not fetch metadata on cached negative")
		return nil, false, nil
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18, Erc4626Probed: true}
	if got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil); got != nil {
		t.Fatalf("expected nil for cached negative, got %+v", got)
	}
	if len(mc.calls) != 0 {
		t.Fatalf("expected zero multicalls for cached negative, got %d", len(mc.calls))
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
	markProbed := func(string) error {
		t.Fatal("markProbed must not be called when detection succeeded")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
		return nil, false, nil // asset contract not a known fungible token
	}
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil)
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
	persister := func(string, string) error {
		t.Fatal("must not persist on transport error")
		return nil
	}
	markProbed := func(string) error {
		t.Fatal("must not mark probed on transport error - retry next request")
		return nil
	}
	getContractInfo := func(string, bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) { return nil, false, nil }
	ci := &bchain.ContractInfo{Contract: vault, Decimals: 18}
	if got := buildErc4626TokenWithDeps(ci, mc, persister, markProbed, getContractInfo, nil); got != nil {
		t.Fatalf("expected nil on multicall error, got %+v", got)
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
			Erc4626Probed:        true,
			Erc4626AssetContract: knownAsset,
		},
		strings.ToLower(unprobedVault): {
			Contract: unprobedVault,
			Standard: erc4626Standard,
			// IsErc4626=false, Erc4626Probed=false (default zero values).
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
	markProbed := func(string) error {
		t.Fatal("markProbed must not be called when probe confirms a vault")
		return nil
	}

	tokens := Tokens{
		{Contract: knownVault, Standard: erc4626Standard},
		{Contract: unprobedVault, Standard: erc4626Standard},
	}
	enrichErc4626TokensWithDeps(tokens, store.get, mc, setVault, markProbed, nil)

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

func TestEnrichErc4626Tokens_SkipsAlreadyProbedNegative(t *testing.T) {
	const probedNegative = "0x00000000000000000000000000000000000000c1"

	store := fakeContractInfoStore{
		strings.ToLower(probedNegative): {
			Contract:      probedNegative,
			Standard:      erc4626Standard,
			Erc4626Probed: true,
			// IsErc4626=false: contract was probed and confirmed not a vault.
		},
	}
	mc := &fakeMulticaller{
		handlers: []func(calls []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error){
			func(_ []bchain.EthereumMulticallCall) ([]bchain.EthereumMulticallResult, error) {
				t.Fatal("must not multicall when all candidates are already probed")
				return nil, nil
			},
		},
	}
	tokens := Tokens{{Contract: probedNegative, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called"); return nil },
		func(string) error { t.Fatal("markProbed must not be called"); return nil },
		nil)

	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("probed-negative token must not be flagged: %v", tokens[0].Protocols)
	}
	if len(mc.calls) != 0 {
		t.Fatalf("expected zero multicalls, got %d", len(mc.calls))
	}
}

func TestEnrichErc4626Tokens_NegativeProbePersistsMarkProbedOnly(t *testing.T) {
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
	probedAddrs := map[string]int{}
	tokens := Tokens{{Contract: fakeFungible, Standard: erc4626Standard}}
	enrichErc4626TokensWithDeps(tokens, store.get, mc,
		func(string, string) error { t.Fatal("setVault must not be called for non-vault"); return nil },
		func(addr string) error {
			probedAddrs[addr]++
			return nil
		},
		nil)
	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("non-vault must not be flagged: %v", tokens[0].Protocols)
	}
	if probedAddrs[fakeFungible] != 1 {
		t.Fatalf("markProbed should be called once for non-vault, got %v", probedAddrs)
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
		func(string) error { t.Fatal("must not markProbed on transport error - retry next request"); return nil },
		nil)
	if slicesContains(tokens[0].Protocols, contractInfoProtocolErc4626) {
		t.Fatalf("must not flag on transport error: %v", tokens[0].Protocols)
	}
}

func TestEnrichErc4626Tokens_NoMulticallerStillFlagsKnown(t *testing.T) {
	const knownVault = "0x00000000000000000000000000000000000000f1"
	const unprobed = "0x00000000000000000000000000000000000000f2"
	store := fakeContractInfoStore{
		strings.ToLower(knownVault): {Contract: knownVault, Standard: erc4626Standard, IsErc4626: true, Erc4626Probed: true},
		strings.ToLower(unprobed):   {Contract: unprobed, Standard: erc4626Standard},
	}
	tokens := Tokens{
		{Contract: knownVault, Standard: erc4626Standard},
		{Contract: unprobed, Standard: erc4626Standard},
	}
	// nil multicaller (chain doesn't support multicall): must still flag known vaults.
	enrichErc4626TokensWithDeps(tokens, store.get, nil, nil, nil, nil)

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
	probedNegatives := map[string]bool{}
	setVault := func(addr, asset string) error {
		persistedVaults[addr] = asset
		return nil
	}
	markProbed := func(addr string) error {
		probedNegatives[addr] = true
		return nil
	}

	tokens := Tokens{
		{Contract: vaultA, Standard: erc4626Standard},
		{Contract: fakeB, Standard: erc4626Standard},
		{Contract: brokenC, Standard: erc4626Standard},
	}
	enrichErc4626TokensWithDeps(tokens, store.get, mc, setVault, markProbed, nil)

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
	if !probedNegatives[fakeB] || !probedNegatives[brokenC] {
		t.Fatalf("fakeB and brokenC should be marked probed-negative, got %v", probedNegatives)
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
		func(string) error { return nil },
		nil)
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
