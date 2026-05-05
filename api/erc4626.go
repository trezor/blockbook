package api

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

const (
	erc4626MaxDecimals = 77
	erc4626ZeroAddress = "0x0000000000000000000000000000000000000000"
)

var (
	erc4626MethodAsset           = erc4626MethodSelector("asset()")
	erc4626MethodTotalAssets     = erc4626MethodSelector("totalAssets()")
	erc4626MethodConvertToAssets = erc4626MethodSelector("convertToAssets(uint256)")
	erc4626MethodConvertToShares = erc4626MethodSelector("convertToShares(uint256)")
	erc4626MethodPreviewDeposit  = erc4626MethodSelector("previewDeposit(uint256)")
	erc4626MethodPreviewRedeem   = erc4626MethodSelector("previewRedeem(uint256)")
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

// erc4626MulticallCaller is the duck-typed access to Multicall3 aggregate3.
// The chain type implements this when its RPC client supports multicall.
type erc4626MulticallCaller interface {
	EthereumTypeMulticallAggregate3(calls []bchain.EthereumMulticallCall, blockNumber *big.Int) ([]bchain.EthereumMulticallResult, error)
}

// erc4626ContractInfoFetcher / erc4626VaultPersister
// isolate the deps that buildErc4626TokenWithDeps reaches for, so unit tests
// can inject fakes.
type erc4626ContractInfoFetcher func(contract string, standard bchain.TokenStandardName) (*bchain.ContractInfo, bool, error)
type erc4626VaultPersister func(address string, assetContract string) error

// enrichErc4626Tokens marks tokens whose contract is a known ERC4626 vault.
// In steady state this is a pure cache lookup for contracts already confirmed
// as vaults. For every other fungible-token contract, it issues one batched
// multicall (asset() + totalAssets() per candidate), persists only positive
// detections, and marks confirmed vaults in the response. Negative results are
// intentionally not persisted so dormant or upgradeable contracts remain
// probeable on future requests.
func (w *Worker) enrichErc4626Tokens(tokens Tokens) {
	mc, _ := w.chain.(erc4626MulticallCaller)
	setVault := func(addr, asset string) error { return w.db.SetContractInfoErc4626Vault(addr, asset) }
	var blockNumber *big.Int
	if h, _, err := w.db.GetBestBlock(); err == nil && h > 0 {
		blockNumber = new(big.Int).SetUint64(uint64(h))
	}
	enrichErc4626TokensWithDeps(tokens, w.GetContractInfo, mc, setVault, blockNumber)
}

func enrichErc4626TokensWithDeps(
	tokens Tokens,
	getContractInfo erc4626ContractInfoFetcher,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	blockNumber *big.Int,
) {
	standard := erc4626EvmFungibleStandard()

	type candidate struct {
		token    *Token
		contract string
	}
	var candidates []candidate

	// First pass: flag known vaults from indexed metadata; collect every other
	// fungible token as a candidate for the batched probe.
	for i := range tokens {
		token := &tokens[i]
		if token.Contract == "" || token.Standard != standard {
			continue
		}
		ci, _, err := getContractInfo(token.Contract, standard)
		if err != nil || ci == nil {
			continue
		}
		if ci.IsErc4626 {
			token.Protocols = append(token.Protocols, contractInfoProtocolErc4626)
			continue
		}
		candidates = append(candidates, candidate{token: token, contract: token.Contract})
	}

	if len(candidates) == 0 || mc == nil {
		return
	}

	// One multicall covers every unprobed candidate in the wallet, two sub-calls each.
	calls := make([]bchain.EthereumMulticallCall, 0, 2*len(candidates))
	for _, c := range candidates {
		calls = append(calls,
			bchain.EthereumMulticallCall{Target: c.contract, CallData: erc4626EncodeNoArg(erc4626MethodAsset), AllowFailure: true},
			bchain.EthereumMulticallCall{Target: c.contract, CallData: erc4626EncodeNoArg(erc4626MethodTotalAssets), AllowFailure: true},
		)
	}
	results, err := mc.EthereumTypeMulticallAggregate3(calls, blockNumber)
	if err != nil || len(results) != len(calls) {
		// Transport failure: don't persist anything; next accountInfo request retries.
		return
	}

	for i, c := range candidates {
		assetResult := results[i*2]
		totalAssetsResult := results[i*2+1]

		// Strict gate: both asset() (non-zero address) and totalAssets() (decodes)
		// must pass. Same criterion as the contractInfo cold path.
		var assetContract string
		if assetResult.Success {
			if addr, derr := erc4626DecodeAddress(assetResult.Data); derr == nil && !strings.EqualFold(addr, erc4626ZeroAddress) {
				assetContract = addr
			}
		}
		if assetContract == "" || !totalAssetsResult.Success {
			continue
		}
		if _, derr := erc4626DecodeUint(totalAssetsResult.Data); derr != nil {
			continue
		}
		if err := setVault(c.contract, assetContract); err != nil {
			glog.Warningf("SetContractInfoErc4626Vault contract %v asset %v: %v", c.contract, assetContract, err)
		}
		c.token.Protocols = append(c.token.Protocols, contractInfoProtocolErc4626)
	}
}

// buildErc4626Token returns the rich vault snapshot for a single contract on the
// getContractInfo path. It uses Multicall3 aggregate3 to collapse the per-vault
// eth_call flurry into a number of round-trips that does not depend on the
// number of conversion fields requested:
//
//	Cold path (no cached asset address):  2 multicalls + lazy asset metadata fetch.
//	  A: asset() + totalAssets() + convertToAssets(1share) + previewRedeem(1share)
//	  B: convertToShares(1asset) + previewDeposit(1asset)
//	Warm path (asset address cached):     1 multicall observing one block.
//	  totalAssets() + the four conversion calls together.
//
// Results are cached per (contract, blockHeight) and shared across concurrent
// callers via singleflight; identical requests within the same block see zero
// upstream traffic. The caller provides the response block height, and the
// multicall is pinned to that exact height so protocols.erc4626 matches the
// blockHeight returned to the client.
//
// Returns nil if the contract is not (or no longer) a vault. The caller is
// expected to have already filtered by standard.
func (w *Worker) buildErc4626Token(contractInfo *bchain.ContractInfo, bestHeight uint32) *Erc4626Token {
	if contractInfo == nil || contractInfo.Contract == "" {
		return nil
	}
	mc, ok := w.chain.(erc4626MulticallCaller)
	if !ok {
		return nil
	}
	setVault := func(addr, asset string) error {
		return w.db.SetContractInfoErc4626Vault(addr, asset)
	}

	// The caller owns bestHeight selection. If it has no usable height (0), fall
	// through to "latest" for the live read; we just lose the in-block caching
	// for this request.
	if bestHeight == 0 {
		return buildErc4626TokenWithDeps(contractInfo, mc, setVault, w.GetContractInfo, nil)
	}
	blockNumber := new(big.Int).SetUint64(uint64(bestHeight))
	return erc4626CacheLookupOrBuild(erc4626LiveCache, erc4626CacheKey(contractInfo.Contract, bestHeight), func() *Erc4626Token {
		return buildErc4626TokenWithDeps(contractInfo, mc, setVault, w.GetContractInfo, blockNumber)
	})
}

func buildErc4626TokenWithDeps(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) *Erc4626Token {
	if ci.Erc4626AssetContract == "" {
		return buildErc4626TokenCold(ci, mc, setVault, getContractInfo, blockNumber)
	}
	return buildErc4626TokenWarm(ci, mc, getContractInfo, blockNumber)
}

// buildErc4626TokenCold runs the first-time enrichment for a vault we don't yet
// have a cached asset address for. It also doubles as the detection path: if
// asset() returns the zero address (or fails), the contract is not a vault and
// we return nil without persisting anything.
func buildErc4626TokenCold(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) *Erc4626Token {
	contract := ci.Contract
	shareDec := ci.Decimals

	// Build multicall A. Share-side conversion calls only fit if we can compute
	// a share unit; if shareDec is out of range we still issue asset()+totalAssets().
	shareUnit, shareUnitErr := erc4626UnitAmount(shareDec)
	callsA := []bchain.EthereumMulticallCall{
		{Target: contract, CallData: erc4626EncodeNoArg(erc4626MethodAsset), AllowFailure: true},
		{Target: contract, CallData: erc4626EncodeNoArg(erc4626MethodTotalAssets), AllowFailure: true},
	}
	if shareUnitErr == nil {
		convertToAssetsData, _ := erc4626EncodeUintArg(erc4626MethodConvertToAssets, shareUnit)
		previewRedeemData, _ := erc4626EncodeUintArg(erc4626MethodPreviewRedeem, shareUnit)
		callsA = append(callsA,
			bchain.EthereumMulticallCall{Target: contract, CallData: convertToAssetsData, AllowFailure: true},
			bchain.EthereumMulticallCall{Target: contract, CallData: previewRedeemData, AllowFailure: true},
		)
	}
	resA, err := mc.EthereumTypeMulticallAggregate3(callsA, blockNumber)
	// Transport errors are not strict-detection failures (the chain may simply
	// be unreachable), so we don't persist a negative result; the next request
	// retries.
	if err != nil || len(resA) < 2 {
		return nil
	}

	// Strict vault detection: BOTH asset() and totalAssets() must succeed and decode.
	// Persisting on asset() alone would let any fungible contract that happens to
	// expose an asset() method get permanently marked as ERC4626 in the DB. Both
	// methods are mandated by EIP-4626, so demanding both is the correct gate.
	// Detection failures remain ephemeral; only positive matches are persisted.
	if !resA[0].Success {
		return nil
	}
	assetContract, err := erc4626DecodeAddress(resA[0].Data)
	if err != nil || strings.EqualFold(assetContract, erc4626ZeroAddress) {
		return nil
	}
	if !resA[1].Success {
		return nil
	}
	totalAssets, err := erc4626DecodeUint(resA[1].Data)
	if err != nil {
		return nil
	}

	if err := setVault(contract, assetContract); err != nil {
		glog.Warningf("SetContractInfoErc4626Vault contract %v asset %v: %v", contract, assetContract, err)
	}

	result := &Erc4626Token{
		Asset: &Erc4626TokenMetadata{Contract: assetContract},
		Share: &Erc4626TokenMetadata{
			Contract: contract,
			Name:     ci.Name,
			Symbol:   ci.Symbol,
			Decimals: shareDec,
		},
		TotalAssetsSat: (*Amount)(totalAssets),
	}
	var errs []string

	if shareUnitErr != nil {
		errs = append(errs, "share decimals: "+shareUnitErr.Error())
	}

	// resA[2/3] are share-side conversions when shareUnit was valid; failures
	// here are non-fatal because the vault is already confirmed real.
	if len(resA) > 2 {
		result.ConvertToAssets1ShareSat = decodeMulticallAmount(resA[2], "convertToAssets", &errs)
	}
	if len(resA) > 3 {
		result.PreviewRedeem1ShareSat = decodeMulticallAmount(resA[3], "previewRedeem", &errs)
	}

	// Asset metadata: lazy fetch (3 RPCs first time per asset, cache hit afterwards).
	assetInfo, validAsset, err := getContractInfo(assetContract, bchain.UnknownTokenStandard)
	if err != nil {
		errs = append(errs, "asset metadata: "+err.Error())
	} else if assetInfo == nil || !validAsset {
		errs = append(errs, "asset metadata unavailable")
	} else {
		result.Asset.Name = assetInfo.Name
		result.Asset.Symbol = assetInfo.Symbol
		result.Asset.Decimals = assetInfo.Decimals
	}

	// Multicall B: asset-side conversions, only if we have a valid asset decimals.
	if validAsset && assetInfo != nil {
		assetUnit, err := erc4626UnitAmount(assetInfo.Decimals)
		if err != nil {
			errs = append(errs, "asset decimals: "+err.Error())
		} else {
			convertToSharesData, _ := erc4626EncodeUintArg(erc4626MethodConvertToShares, assetUnit)
			previewDepositData, _ := erc4626EncodeUintArg(erc4626MethodPreviewDeposit, assetUnit)
			callsB := []bchain.EthereumMulticallCall{
				{Target: contract, CallData: convertToSharesData, AllowFailure: true},
				{Target: contract, CallData: previewDepositData, AllowFailure: true},
			}
			resB, err := mc.EthereumTypeMulticallAggregate3(callsB, blockNumber)
			if err != nil {
				errs = append(errs, "asset-side multicall: "+err.Error())
			} else if len(resB) >= 2 {
				result.ConvertToShares1AssetSat = decodeMulticallAmount(resB[0], "convertToShares", &errs)
				result.PreviewDeposit1AssetSat = decodeMulticallAmount(resB[1], "previewDeposit", &errs)
			}
		}
	}

	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}
	return result
}

// buildErc4626TokenWarm runs the steady-state enrichment. The asset address is
// known; the asset metadata is almost always cached. All time-varying fields
// come from a single multicall, observed at the same block.
func buildErc4626TokenWarm(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) *Erc4626Token {
	contract := ci.Contract
	assetContract := ci.Erc4626AssetContract
	shareDec := ci.Decimals

	result := &Erc4626Token{
		Asset: &Erc4626TokenMetadata{Contract: assetContract},
		Share: &Erc4626TokenMetadata{
			Contract: contract,
			Name:     ci.Name,
			Symbol:   ci.Symbol,
			Decimals: shareDec,
		},
	}
	var errs []string

	assetInfo, validAsset, err := getContractInfo(assetContract, bchain.UnknownTokenStandard)
	if err != nil {
		errs = append(errs, "asset metadata: "+err.Error())
	} else if assetInfo == nil || !validAsset {
		errs = append(errs, "asset metadata unavailable")
	} else {
		result.Asset.Name = assetInfo.Name
		result.Asset.Symbol = assetInfo.Symbol
		result.Asset.Decimals = assetInfo.Decimals
	}

	shareUnit, shareUnitErr := erc4626UnitAmount(shareDec)
	if shareUnitErr != nil {
		errs = append(errs, "share decimals: "+shareUnitErr.Error())
	}
	var assetUnit *big.Int
	if validAsset && assetInfo != nil {
		var assetUnitErr error
		assetUnit, assetUnitErr = erc4626UnitAmount(assetInfo.Decimals)
		if assetUnitErr != nil {
			errs = append(errs, "asset decimals: "+assetUnitErr.Error())
		}
	}

	// Compose one multicall with totalAssets first, plus any of the four
	// conversion calls whose unit amounts are known. Skipping a conversion is
	// preferable to issuing it with bogus inputs.
	calls := []bchain.EthereumMulticallCall{
		{Target: contract, CallData: erc4626EncodeNoArg(erc4626MethodTotalAssets), AllowFailure: true},
	}
	type sink struct {
		idx    int
		label  string
		target **Amount
	}
	sinks := []sink{
		{idx: 0, label: "totalAssets", target: &result.TotalAssetsSat},
	}
	if shareUnit != nil {
		convertToAssetsData, _ := erc4626EncodeUintArg(erc4626MethodConvertToAssets, shareUnit)
		previewRedeemData, _ := erc4626EncodeUintArg(erc4626MethodPreviewRedeem, shareUnit)
		idx := len(calls)
		calls = append(calls,
			bchain.EthereumMulticallCall{Target: contract, CallData: convertToAssetsData, AllowFailure: true},
			bchain.EthereumMulticallCall{Target: contract, CallData: previewRedeemData, AllowFailure: true},
		)
		sinks = append(sinks,
			sink{idx: idx, label: "convertToAssets", target: &result.ConvertToAssets1ShareSat},
			sink{idx: idx + 1, label: "previewRedeem", target: &result.PreviewRedeem1ShareSat},
		)
	}
	if assetUnit != nil {
		convertToSharesData, _ := erc4626EncodeUintArg(erc4626MethodConvertToShares, assetUnit)
		previewDepositData, _ := erc4626EncodeUintArg(erc4626MethodPreviewDeposit, assetUnit)
		idx := len(calls)
		calls = append(calls,
			bchain.EthereumMulticallCall{Target: contract, CallData: convertToSharesData, AllowFailure: true},
			bchain.EthereumMulticallCall{Target: contract, CallData: previewDepositData, AllowFailure: true},
		)
		sinks = append(sinks,
			sink{idx: idx, label: "convertToShares", target: &result.ConvertToShares1AssetSat},
			sink{idx: idx + 1, label: "previewDeposit", target: &result.PreviewDeposit1AssetSat},
		)
	}

	res, err := mc.EthereumTypeMulticallAggregate3(calls, blockNumber)
	if err != nil {
		errs = append(errs, "multicall: "+err.Error())
	} else {
		for _, s := range sinks {
			if s.idx >= len(res) {
				continue
			}
			*s.target = decodeMulticallAmount(res[s.idx], s.label, &errs)
		}
	}

	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}
	return result
}

func decodeMulticallAmount(r bchain.EthereumMulticallResult, label string, errs *[]string) *Amount {
	if !r.Success {
		*errs = append(*errs, label+": call reverted")
		return nil
	}
	v, err := erc4626DecodeUint(r.Data)
	if err != nil {
		*errs = append(*errs, label+": "+err.Error())
		return nil
	}
	return (*Amount)(v)
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

func erc4626UnitAmount(decimals int) (*big.Int, error) {
	if decimals < 0 || decimals > erc4626MaxDecimals {
		return nil, fmt.Errorf("unsupported decimals %d", decimals)
	}
	if decimals == 0 {
		return big.NewInt(1), nil
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil), nil
}
