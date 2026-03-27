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

type fakeErc4626Batcher struct {
	calls [][]bchain.EthereumTypeRPCCall
	fn    func(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error)
}

func (f *fakeErc4626Batcher) EthereumTypeRpcCallBatch(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
	copied := append([]bchain.EthereumTypeRPCCall(nil), calls...)
	f.calls = append(f.calls, copied)
	if f.fn != nil {
		return f.fn(calls)
	}
	return make([]bchain.EthereumTypeRPCCallResult, len(calls)), nil
}

func testEncodeWordAddress(address string) string {
	a := ethcommon.HexToAddress(address)
	word := make([]byte, 32)
	copy(word[12:], a.Bytes())
	return "0x" + hex.EncodeToString(word)
}

func testEncodeWordUint(v *big.Int) string {
	word := make([]byte, 32)
	v.FillBytes(word)
	return "0x" + hex.EncodeToString(word)
}

func TestErc4626CollectCandidates(t *testing.T) {
	standard := erc4626EvmFungibleStandard()
	tokens := Tokens{
		{Contract: "0xAa", Standard: standard},
		{Contract: "0xBb", Standard: bchain.ERC1155TokenStandard},
		{Contract: "0xAa", Standard: standard},
		{Contract: "", Standard: standard},
		{Contract: "0xCc", Standard: standard},
	}

	candidates, contracts := erc4626CollectCandidates(tokens, standard)

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
	if len(contracts) != 2 {
		t.Fatalf("expected 2 unique contracts, got %d", len(contracts))
	}
	if candidates[0].token != &tokens[0] || candidates[1].token != &tokens[2] || candidates[2].token != &tokens[4] {
		t.Fatalf("candidate token pointers are not in expected order")
	}
	if contracts[0] != "0xAa" || contracts[1] != "0xCc" {
		t.Fatalf("unexpected unique contracts order: %v", contracts)
	}
	if candidates[0].key != "0xaa" || candidates[1].key != "0xaa" || candidates[2].key != "0xcc" {
		t.Fatalf("unexpected normalized keys: %+v", candidates)
	}
}

func TestErc4626DetectVaultsBatched(t *testing.T) {
	contracts := []string{
		"0x00000000000000000000000000000000000000a1",
		"0x00000000000000000000000000000000000000b2",
		"0x00000000000000000000000000000000000000c3",
	}
	assetToken := "0x0000000000000000000000000000000000000dA1"
	expectedTotalAssets := big.NewInt(123456)

	batcher := &fakeErc4626Batcher{
		fn: func(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
			if len(calls) != 6 {
				return nil, fmt.Errorf("expected 6 calls, got %d", len(calls))
			}
			results := make([]bchain.EthereumTypeRPCCallResult, len(calls))
			// First contract: valid vault.
			results[0].Data = testEncodeWordAddress(assetToken)
			results[3].Data = testEncodeWordUint(expectedTotalAssets)
			// Second contract: zero asset address -> not a vault.
			results[1].Data = testEncodeWordAddress(erc4626ZeroAddress)
			results[4].Data = testEncodeWordUint(big.NewInt(1))
			// Third contract: invalid output -> ignored.
			results[2].Data = "0x1234"
			results[5].Data = "0x"
			return results, nil
		},
	}

	probes := map[string]erc4626VaultProbe{}
	if err := (&Worker{}).detectErc4626VaultsBatched(contracts, batcher, probes); err != nil {
		t.Fatalf("detectErc4626VaultsBatched failed: %v", err)
	}
	if len(batcher.calls) != 1 {
		t.Fatalf("expected one batch call, got %d", len(batcher.calls))
	}
	gotCalls := batcher.calls[0]
	expectedAssetCallData := erc4626EncodeNoArg(erc4626MethodAsset)
	expectedTotalAssetsCallData := erc4626EncodeNoArg(erc4626MethodTotalAssets)
	for i, contract := range contracts {
		if gotCalls[i].Data != expectedAssetCallData || gotCalls[i].To != contract {
			t.Fatalf("asset call[%d] mismatch: got %+v", i, gotCalls[i])
		}
		if gotCalls[len(contracts)+i].Data != expectedTotalAssetsCallData || gotCalls[len(contracts)+i].To != contract {
			t.Fatalf("totalAssets call[%d] mismatch: got %+v", i, gotCalls[len(contracts)+i])
		}
	}

	if len(probes) != 1 {
		t.Fatalf("expected 1 detected vault, got %d", len(probes))
	}
	probe, ok := probes[strings.ToLower(contracts[0])]
	if !ok {
		t.Fatalf("expected probe for %s", contracts[0])
	}
	if !strings.EqualFold(probe.assetContract, assetToken) {
		t.Fatalf("asset contract mismatch: got %s want %s", probe.assetContract, assetToken)
	}
	if probe.totalAssets.Cmp(expectedTotalAssets) != 0 {
		t.Fatalf("totalAssets mismatch: got %s want %s", probe.totalAssets, expectedTotalAssets)
	}
}

func TestErc4626DetectVaultsBatchedChunking(t *testing.T) {
	contracts := make([]string, 205)
	for i := range contracts {
		contracts[i] = fmt.Sprintf("0x%040x", i+1)
	}

	batcher := &fakeErc4626Batcher{
		fn: func(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
			return make([]bchain.EthereumTypeRPCCallResult, len(calls)), nil
		},
	}
	probes := map[string]erc4626VaultProbe{}
	if err := (&Worker{}).detectErc4626VaultsBatched(contracts, batcher, probes); err != nil {
		t.Fatalf("detectErc4626VaultsBatched failed: %v", err)
	}
	if len(batcher.calls) != 3 {
		t.Fatalf("expected 3 chunked batch calls, got %d", len(batcher.calls))
	}
	if len(batcher.calls[0]) != 200 || len(batcher.calls[1]) != 200 || len(batcher.calls[2]) != 10 {
		t.Fatalf("unexpected chunk sizes: %d, %d, %d", len(batcher.calls[0]), len(batcher.calls[1]), len(batcher.calls[2]))
	}
}

func TestErc4626DetectVaultsBatchedErrors(t *testing.T) {
	t.Run("batch rpc error", func(t *testing.T) {
		batcher := &fakeErc4626Batcher{
			fn: func(_ []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
				return nil, errors.New("boom")
			},
		}
		probes := map[string]erc4626VaultProbe{}
		err := (&Worker{}).detectErc4626VaultsBatched([]string{"0x0000000000000000000000000000000000000001"}, batcher, probes)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("result size mismatch", func(t *testing.T) {
		batcher := &fakeErc4626Batcher{
			fn: func(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
				return make([]bchain.EthereumTypeRPCCallResult, len(calls)-1), nil
			},
		}
		probes := map[string]erc4626VaultProbe{}
		err := (&Worker{}).detectErc4626VaultsBatched([]string{"0x0000000000000000000000000000000000000001"}, batcher, probes)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestErc4626FetchTokenDataOmitsDerivedFieldsOnUnresolvedDecimals(t *testing.T) {
	token := &Token{
		Contract: "0x00000000000000000000000000000000000000a1",
		Name:     "Vault Share",
		Symbol:   "vSHARE",
		Decimals: 0,
	}
	probe := erc4626VaultProbe{
		assetContract: "0x00000000000000000000000000000000000000b2",
		totalAssets:   big.NewInt(999),
	}

	uintCalls := 0
	result := erc4626FetchTokenDataWithDeps(
		token,
		probe,
		func(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
			return nil, false, nil
		},
		func(contract string) (int, error) {
			if strings.EqualFold(contract, token.Contract) {
				return 0, errors.New("share decimals unavailable")
			}
			if strings.EqualFold(contract, probe.assetContract) {
				return 0, errors.New("asset decimals unavailable")
			}
			return 0, fmt.Errorf("unexpected decimals contract %s", contract)
		},
		func(_ string, _ [4]byte, _ *big.Int) (*big.Int, error) {
			uintCalls++
			return nil, errors.New("unexpected call")
		},
	)

	if uintCalls != 0 {
		t.Fatalf("expected no derived uint calls, got %d", uintCalls)
	}
	if result.ConvertToAssets1ShareSat != nil || result.PreviewRedeem1ShareSat != nil || result.ConvertToShares1AssetSat != nil || result.PreviewDeposit1AssetSat != nil {
		t.Fatalf("expected derived fields to be omitted on unresolved decimals, got %+v", result)
	}
	if result.TotalAssetsSat == nil || (*big.Int)(result.TotalAssetsSat).Cmp(probe.totalAssets) != 0 {
		t.Fatalf("unexpected total assets: got %v want %v", result.TotalAssetsSat, probe.totalAssets)
	}
	if !strings.Contains(result.Error, "share decimals: share decimals unavailable") {
		t.Fatalf("missing share decimals error: %q", result.Error)
	}
	if !strings.Contains(result.Error, "asset decimals: asset decimals unavailable") {
		t.Fatalf("missing asset decimals error: %q", result.Error)
	}
}

func TestErc4626FetchTokenDataUsesTrustedMetadataFallbackForDecimals(t *testing.T) {
	token := &Token{
		Contract: "0x00000000000000000000000000000000000000a1",
		Name:     "Vault Share",
		Symbol:   "vSHARE",
		Decimals: 0,
	}
	probe := erc4626VaultProbe{
		assetContract: "0x00000000000000000000000000000000000000b2",
		totalAssets:   big.NewInt(1234),
	}

	type uintCall struct {
		selector [4]byte
		arg      *big.Int
	}
	var calls []uintCall

	result := erc4626FetchTokenDataWithDeps(
		token,
		probe,
		func(contract string, _ bchain.TokenStandardName) (*bchain.ContractInfo, bool, error) {
			switch {
			case strings.EqualFold(contract, token.Contract):
				return &bchain.ContractInfo{Contract: token.Contract, Name: token.Name, Symbol: token.Symbol, Decimals: 18}, true, nil
			case strings.EqualFold(contract, probe.assetContract):
				return &bchain.ContractInfo{Contract: probe.assetContract, Name: "USD Coin", Symbol: "USDC", Decimals: 6}, true, nil
			default:
				return nil, false, fmt.Errorf("unexpected metadata contract %s", contract)
			}
		},
		func(_ string) (int, error) {
			return 0, errors.New("decimals unavailable")
		},
		func(_ string, selector [4]byte, arg *big.Int) (*big.Int, error) {
			calls = append(calls, uintCall{selector: selector, arg: new(big.Int).Set(arg)})
			return big.NewInt(int64(len(calls))), nil
		},
	)

	if result.Error != "" {
		t.Fatalf("expected trusted fallback to avoid error, got %q", result.Error)
	}
	if result.Share == nil || result.Share.Decimals != 18 {
		t.Fatalf("unexpected share metadata: %+v", result.Share)
	}
	if result.Asset == nil || result.Asset.Decimals != 6 || result.Asset.Symbol != "USDC" {
		t.Fatalf("unexpected asset metadata: %+v", result.Asset)
	}
	if len(calls) != 4 {
		t.Fatalf("expected 4 derived uint calls, got %d", len(calls))
	}
	shareUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	assetUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	if calls[0].selector != erc4626MethodConvertToAssets || calls[0].arg.Cmp(shareUnit) != 0 {
		t.Fatalf("unexpected first call: %+v", calls[0])
	}
	if calls[1].selector != erc4626MethodPreviewRedeem || calls[1].arg.Cmp(shareUnit) != 0 {
		t.Fatalf("unexpected second call: %+v", calls[1])
	}
	if calls[2].selector != erc4626MethodConvertToShares || calls[2].arg.Cmp(assetUnit) != 0 {
		t.Fatalf("unexpected third call: %+v", calls[2])
	}
	if calls[3].selector != erc4626MethodPreviewDeposit || calls[3].arg.Cmp(assetUnit) != 0 {
		t.Fatalf("unexpected fourth call: %+v", calls[3])
	}
	if result.ConvertToAssets1ShareSat == nil || result.PreviewRedeem1ShareSat == nil || result.ConvertToShares1AssetSat == nil || result.PreviewDeposit1AssetSat == nil {
		t.Fatalf("expected all derived fields to be populated, got %+v", result)
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
	if _, err := erc4626BigIntToDecimals(big.NewInt(78)); err == nil {
		t.Fatal("expected unsupported decimals error")
	}
	if _, err := erc4626BigIntToDecimals(big.NewInt(-1)); err == nil {
		t.Fatal("expected negative decimals error")
	}
	if _, err := erc4626UnitAmount(78); err == nil {
		t.Fatal("expected unsupported decimals error")
	}
	unit, err := erc4626UnitAmount(0)
	if err != nil || unit.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("unexpected 10^0 result: %v, %v", unit, err)
	}
}
