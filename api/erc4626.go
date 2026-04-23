package api

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/trezor/blockbook/bchain"
)

const (
	erc4626MaxDecimals          = 77
	erc4626ZeroAddress          = "0x0000000000000000000000000000000000000000"
	erc4626DetectBatchContracts = 100
)

var (
	erc4626MethodAsset           = erc4626MethodSelector("asset()")
	erc4626MethodTotalAssets     = erc4626MethodSelector("totalAssets()")
	erc4626MethodConvertToAssets = erc4626MethodSelector("convertToAssets(uint256)")
	erc4626MethodConvertToShares = erc4626MethodSelector("convertToShares(uint256)")
	erc4626MethodPreviewDeposit  = erc4626MethodSelector("previewDeposit(uint256)")
	erc4626MethodPreviewRedeem   = erc4626MethodSelector("previewRedeem(uint256)")
	erc4626MethodDecimals        = erc4626MethodSelector("decimals()")
	erc4626MaxUint256            = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

func erc4626MethodSelector(signature string) [4]byte {
	var selector [4]byte
	copy(selector[:], crypto.Keccak256([]byte(signature))[:4])
	return selector
}

func erc4626EvmFungibleStandard() bchain.TokenStandardName {
	if len(bchain.EthereumTokenStandardMap) > int(bchain.FungibleToken) {
		return bchain.EthereumTokenStandardMap[bchain.FungibleToken]
	}
	return bchain.ERC20TokenStandard
}

type erc4626BatchCaller interface {
	EthereumTypeRpcCallBatch(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error)
}

type erc4626ContractInfoFetcher func(contract string, standard bchain.TokenStandardName) (*bchain.ContractInfo, bool, error)
type erc4626DecimalsFetcher func(contract string) (int, error)
type erc4626UintArgCaller func(contract string, selector [4]byte, arg *big.Int) (*big.Int, error)

type erc4626Candidate struct {
	token *Token
	key   string
}

type erc4626VaultProbe struct {
	assetContract string
	totalAssets   *big.Int
}

func erc4626CollectCandidates(tokens Tokens, standard bchain.TokenStandardName) ([]erc4626Candidate, []string) {
	candidates := make([]erc4626Candidate, 0, len(tokens))
	contracts := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for i := range tokens {
		token := &tokens[i]
		if token.Contract == "" || token.Standard != standard {
			continue
		}
		key := strings.ToLower(token.Contract)
		candidates = append(candidates, erc4626Candidate{token: token, key: key})
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		contracts = append(contracts, token.Contract)
	}
	return candidates, contracts
}

func (w *Worker) enrichErc4626Tokens(tokens Tokens) {
	standard := erc4626EvmFungibleStandard()
	candidates, contracts := erc4626CollectCandidates(tokens, standard)
	if len(candidates) == 0 {
		return
	}

	probes := make(map[string]erc4626VaultProbe, len(contracts))
	if batcher, ok := w.chain.(erc4626BatchCaller); ok {
		_ = w.detectErc4626VaultsBatched(contracts, batcher, probes)
	}
	for _, contract := range contracts {
		key := strings.ToLower(contract)
		if _, ok := probes[key]; ok {
			continue
		}
		probe, isVault := w.detectErc4626Vault(contract)
		if !isVault {
			continue
		}
		probes[key] = probe
	}

	for _, candidate := range candidates {
		probe, ok := probes[candidate.key]
		if !ok {
			continue
		}
		candidate.token.Erc4626 = w.fetchErc4626TokenData(candidate.token, probe)
	}
}

func (w *Worker) detectErc4626VaultsBatched(contracts []string, batcher erc4626BatchCaller, probes map[string]erc4626VaultProbe) error {
	for start := 0; start < len(contracts); start += erc4626DetectBatchContracts {
		end := start + erc4626DetectBatchContracts
		if end > len(contracts) {
			end = len(contracts)
		}
		chunk := contracts[start:end]
		calls := make([]bchain.EthereumTypeRPCCall, 0, 2*len(chunk))
		for _, contract := range chunk {
			calls = append(calls, bchain.EthereumTypeRPCCall{
				Data: erc4626EncodeNoArg(erc4626MethodAsset),
				To:   contract,
			})
		}
		for _, contract := range chunk {
			calls = append(calls, bchain.EthereumTypeRPCCall{
				Data: erc4626EncodeNoArg(erc4626MethodTotalAssets),
				To:   contract,
			})
		}
		results, err := batcher.EthereumTypeRpcCallBatch(calls)
		if err != nil {
			return err
		}
		if len(results) != len(calls) {
			return fmt.Errorf("unexpected batch result size: got %d want %d", len(results), len(calls))
		}
		offset := len(chunk)
		for i, contract := range chunk {
			assetResult := results[i]
			totalAssetsResult := results[offset+i]
			if assetResult.Error != nil || totalAssetsResult.Error != nil {
				continue
			}
			assetContract, err := erc4626DecodeAddress(assetResult.Data)
			if err != nil || strings.EqualFold(assetContract, erc4626ZeroAddress) {
				continue
			}
			totalAssets, err := erc4626DecodeUint(totalAssetsResult.Data)
			if err != nil {
				continue
			}
			probes[strings.ToLower(contract)] = erc4626VaultProbe{
				assetContract: assetContract,
				totalAssets:   totalAssets,
			}
		}
	}
	return nil
}

func (w *Worker) detectErc4626Vault(contract string) (erc4626VaultProbe, bool) {
	assetCallResult, err := w.erc4626CallNoArg(contract, erc4626MethodAsset)
	if err != nil {
		return erc4626VaultProbe{}, false
	}
	assetContract, err := erc4626DecodeAddress(assetCallResult)
	if err != nil || strings.EqualFold(assetContract, erc4626ZeroAddress) {
		return erc4626VaultProbe{}, false
	}
	totalAssets, err := w.erc4626CallUintNoArg(contract, erc4626MethodTotalAssets)
	if err != nil {
		return erc4626VaultProbe{}, false
	}
	return erc4626VaultProbe{
		assetContract: assetContract,
		totalAssets:   totalAssets,
	}, true
}

func (w *Worker) fetchErc4626TokenData(token *Token, probe erc4626VaultProbe) *Erc4626Token {
	return erc4626FetchTokenDataWithDeps(token, probe, w.GetContractInfo, w.erc4626CallDecimals, w.erc4626CallUintWithArg)
}

func erc4626FetchTokenDataWithDeps(token *Token, probe erc4626VaultProbe, getContractInfo erc4626ContractInfoFetcher, getDecimals erc4626DecimalsFetcher, callUint erc4626UintArgCaller) *Erc4626Token {
	result := &Erc4626Token{
		Asset: &Erc4626TokenMetadata{
			Contract: probe.assetContract,
		},
		Share: &Erc4626TokenMetadata{
			Contract: token.Contract,
			Name:     token.Name,
			Symbol:   token.Symbol,
			Decimals: token.Decimals,
		},
		TotalAssetsSat: (*Amount)(probe.totalAssets),
	}

	var errs []string

	assetInfo, validAssetContract, err := getContractInfo(probe.assetContract, bchain.UnknownTokenStandard)
	if err != nil {
		errs = append(errs, "asset metadata: "+err.Error())
	} else if assetInfo != nil {
		result.Asset.Name = assetInfo.Name
		result.Asset.Symbol = assetInfo.Symbol
		if validAssetContract {
			result.Asset.Decimals = assetInfo.Decimals
		} else {
			errs = append(errs, "asset metadata unavailable")
		}
	}

	shareDecimals, shareDecimalsResolved := erc4626ResolveDecimals(token.Contract, getContractInfo, getDecimals, nil, false, "share decimals", &errs)
	if shareDecimalsResolved {
		result.Share.Decimals = shareDecimals
		shareUnit, err := erc4626UnitAmount(shareDecimals)
		if err != nil {
			errs = append(errs, "share decimals: "+err.Error())
		} else {
			result.ConvertToAssets1ShareSat = erc4626FetchDerivedAmount(token.Contract, erc4626MethodConvertToAssets, shareUnit, "convertToAssets", callUint, &errs)
			result.PreviewRedeem1ShareSat = erc4626FetchDerivedAmount(token.Contract, erc4626MethodPreviewRedeem, shareUnit, "previewRedeem", callUint, &errs)
		}
	}

	assetDecimals, assetDecimalsResolved := erc4626ResolveDecimals(probe.assetContract, getContractInfo, getDecimals, assetInfo, validAssetContract, "asset decimals", &errs)
	if assetDecimalsResolved {
		result.Asset.Decimals = assetDecimals
		assetUnit, err := erc4626UnitAmount(assetDecimals)
		if err != nil {
			errs = append(errs, "asset decimals: "+err.Error())
		} else {
			result.ConvertToShares1AssetSat = erc4626FetchDerivedAmount(token.Contract, erc4626MethodConvertToShares, assetUnit, "convertToShares", callUint, &errs)
			result.PreviewDeposit1AssetSat = erc4626FetchDerivedAmount(token.Contract, erc4626MethodPreviewDeposit, assetUnit, "previewDeposit", callUint, &errs)
		}
	}

	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}

	return result
}

func erc4626ResolveDecimals(contract string, getContractInfo erc4626ContractInfoFetcher, getDecimals erc4626DecimalsFetcher, fallbackInfo *bchain.ContractInfo, fallbackValid bool, errorLabel string, errs *[]string) (int, bool) {
	decimals, decimalsErr := getDecimals(contract)
	if decimalsErr != nil {
		if !fallbackValid || fallbackInfo == nil {
			var err error
			fallbackInfo, fallbackValid, err = getContractInfo(contract, bchain.UnknownTokenStandard)
			if err != nil {
				*errs = append(*errs, errorLabel+": "+err.Error())
				return 0, false
			}
		}
		if !fallbackValid || fallbackInfo == nil {
			*errs = append(*errs, errorLabel+": "+decimalsErr.Error())
			return 0, false
		}
		return fallbackInfo.Decimals, true
	}
	return decimals, true
}

func erc4626FetchDerivedAmount(contract string, selector [4]byte, arg *big.Int, label string, callUint erc4626UintArgCaller, errs *[]string) *Amount {
	value, err := callUint(contract, selector, arg)
	if err != nil {
		*errs = append(*errs, label+": "+err.Error())
		return nil
	}
	return (*Amount)(value)
}

func (w *Worker) erc4626CallNoArg(contract string, selector [4]byte) (string, error) {
	return w.chain.EthereumTypeRpcCall(erc4626EncodeNoArg(selector), contract, "")
}

func (w *Worker) erc4626CallUintNoArg(contract string, selector [4]byte) (*big.Int, error) {
	data, err := w.erc4626CallNoArg(contract, selector)
	if err != nil {
		return nil, err
	}
	return erc4626DecodeUint(data)
}

func (w *Worker) erc4626CallUintWithArg(contract string, selector [4]byte, arg *big.Int) (*big.Int, error) {
	callData, err := erc4626EncodeUintArg(selector, arg)
	if err != nil {
		return nil, err
	}
	data, err := w.chain.EthereumTypeRpcCall(callData, contract, "")
	if err != nil {
		return nil, err
	}
	return erc4626DecodeUint(data)
}

func (w *Worker) erc4626CallDecimals(contract string) (int, error) {
	decimalsValue, err := w.erc4626CallUintNoArg(contract, erc4626MethodDecimals)
	if err != nil {
		return 0, err
	}
	return erc4626BigIntToDecimals(decimalsValue)
}

func erc4626EncodeNoArg(selector [4]byte) string {
	buf := make([]byte, 4)
	copy(buf, selector[:])
	return "0x" + hex.EncodeToString(buf)
}

func erc4626EncodeUintArg(selector [4]byte, arg *big.Int) (string, error) {
	if arg == nil || arg.Sign() < 0 {
		return "", fmt.Errorf("invalid uint256 argument")
	}
	if arg.Cmp(erc4626MaxUint256) > 0 {
		return "", fmt.Errorf("uint256 argument overflows")
	}
	buf := make([]byte, 4+32)
	copy(buf, selector[:])
	arg.FillBytes(buf[4:])
	return "0x" + hex.EncodeToString(buf), nil
}

func erc4626DecodeHex(data string) ([]byte, error) {
	if strings.HasPrefix(data, "0x") {
		data = data[2:]
	}
	if data == "" {
		return nil, fmt.Errorf("empty result")
	}
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("invalid hex length")
	}
	buf, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func erc4626DecodeUint(data string) (*big.Int, error) {
	buf, err := erc4626DecodeHex(data)
	if err != nil {
		return nil, err
	}
	if len(buf) < 32 {
		return nil, fmt.Errorf("result too short")
	}
	return new(big.Int).SetBytes(buf[:32]), nil
}

func erc4626DecodeAddress(data string) (string, error) {
	buf, err := erc4626DecodeHex(data)
	if err != nil {
		return "", err
	}
	if len(buf) < 32 {
		return "", fmt.Errorf("result too short")
	}
	return ethcommon.BytesToAddress(buf[12:32]).Hex(), nil
}

func erc4626BigIntToDecimals(v *big.Int) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("missing value")
	}
	if !v.IsInt64() {
		return 0, fmt.Errorf("value out of range")
	}
	d := int(v.Int64())
	if d < 0 || d > erc4626MaxDecimals {
		return 0, fmt.Errorf("unsupported decimals %d", d)
	}
	return d, nil
}

func erc4626UnitAmount(decimals int) (*big.Int, error) {
	if decimals < 0 || decimals > erc4626MaxDecimals {
		return nil, fmt.Errorf("unsupported decimals %d", decimals)
	}
	if decimals == 0 {
		return big.NewInt(1), nil
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil), nil
}
