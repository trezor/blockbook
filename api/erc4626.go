package api

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

const (
	erc4626MaxDecimals = 77
	erc4626ZeroAddress = "0x0000000000000000000000000000000000000000"
	// Two sub-calls per candidate (asset + totalAssets); chunk to bound aggregate3 payload size.
	erc4626ProbeChunkCandidates = 64

	// erc4626NegativeProbeTTLDuration is how long a "definitively not a vault"
	// result stays in the in-memory negative cache before re-probing. Keeping
	// it expressed as wall-clock time (rather than a fixed block count) means
	// the user-visible TTL is ~the same regardless of the chain's block
	// cadence; the per-coin block count is derived from the chain's
	// configured averageBlockTimeMs at request time.
	erc4626NegativeProbeTTLDuration = 15 * time.Minute
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

// erc4626MulticallCaller is the chain-side seam used by enrichment; satisfied
// by chains whose RPC client supports Multicall3 aggregate3.
type erc4626MulticallCaller interface {
	EthereumTypeMulticallAggregate3(calls []bchain.EthereumMulticallCall, blockNumber *big.Int) ([]bchain.EthereumMulticallResult, error)
}

// erc4626BlockTimeProvider exposes the chain's configured average block time
// so the API can convert chain-time settings (negative-cache TTL) into a
// per-coin block count at request time. Implemented by EVM coins via
// EthereumRPC.AverageBlockTimeDuration.
type erc4626BlockTimeProvider interface {
	AverageBlockTimeDuration() (time.Duration, error)
}

// erc4626BlocksForDuration converts a wall-clock duration to the equivalent
// per-chain block count, rounding up so a duration of "at least N" is honored.
// Returns 0 when either input is non-positive — callers treat 0 as
// "configuration unavailable, skip the time-derived behavior."
func erc4626BlocksForDuration(d, blockTime time.Duration) uint32 {
	if d <= 0 || blockTime <= 0 {
		return 0
	}
	n := (d + blockTime - 1) / blockTime
	if n < 1 {
		return 1
	}
	return uint32(n)
}

// erc4626NegativeProbeTTLBlocks resolves the negative-cache TTL to a per-coin
// block count using the chain's configured averageBlockTimeMs. Returns 0 if
// the chain doesn't expose a block time (e.g. non-EVM); the caller treats 0
// as "do not negative-cache for this request" — safe fallback that just
// forfeits the optimization.
func (w *Worker) erc4626NegativeProbeTTLBlocks() uint32 {
	provider, ok := w.chain.(erc4626BlockTimeProvider)
	if !ok {
		return 0
	}
	bt, err := provider.AverageBlockTimeDuration()
	if err != nil {
		glog.Warningf("erc4626: averageBlockTime unavailable, negative cache disabled: %v", err)
		return 0
	}
	return erc4626BlocksForDuration(erc4626NegativeProbeTTLDuration, bt)
}

type erc4626ContractInfoFetcher func(contract string, standard bchain.TokenStandardName) (*bchain.ContractInfo, bool, error)

// erc4626VaultPersister anchors the row to the observation height so a
// future disconnect of that range removes it.
type erc4626VaultPersister func(address, assetContract string) error

// enrichErc4626Tokens marks tokens whose contract is a known ERC4626 vault.
// Known vaults are flagged from indexed metadata; remaining fungibles are
// probed in one batched multicall, with positives persisted and negatives kept
// in-memory only (so dormant/upgradeable contracts stay probeable).
func (w *Worker) enrichErc4626Tokens(tokens Tokens, bestHeight uint32, bestHash string) {
	mc, _ := w.chain.(erc4626MulticallCaller)
	// Sample reorgGen+bestHash before the multicall; writer rejects if the
	// observed block is no longer canonical (see SetErcProtocol).
	reorgGen := w.db.ReorgGeneration()
	// Resolve the wall-clock negative-cache TTL into a per-coin block count
	// once per request. 0 falls back to "do not negative-cache" (no-op).
	negativeTTLBlocks := w.erc4626NegativeProbeTTLBlocks()
	setVault := func(addr, asset string) error {
		return w.db.SetContractInfoErc4626Vault(addr, asset, bestHeight, bestHash, reorgGen)
	}
	enrichErc4626TokensWithDeps(tokens, w.GetContractInfo, mc, setVault, erc4626NegativeProbeCache, bestHeight, negativeTTLBlocks, reorgGen)
}

func enrichErc4626TokensWithDeps(
	tokens Tokens,
	getContractInfo erc4626ContractInfoFetcher,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	negativeCache *erc4626NegativeCache,
	bestHeight uint32,
	negativeTTLBlocks uint32,
	reorgGen uint64,
) {
	var blockNumber *big.Int
	if bestHeight > 0 {
		blockNumber = new(big.Int).SetUint64(uint64(bestHeight))
	}
	standard := erc4626EvmFungibleStandard()

	type candidate struct {
		token    *Token
		contract string
	}
	var candidates []candidate

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
			negativeCache.remove(token.Contract)
			token.Protocols = append(token.Protocols, contractInfoProtocolErc4626)
			continue
		}
		if negativeCache.contains(token.Contract, bestHeight, reorgGen) {
			continue
		}
		candidates = append(candidates, candidate{token: token, contract: token.Contract})
	}

	if len(candidates) == 0 || mc == nil {
		return
	}

	for start := 0; start < len(candidates); start += erc4626ProbeChunkCandidates {
		end := start + erc4626ProbeChunkCandidates
		if end > len(candidates) {
			end = len(candidates)
		}
		chunk := candidates[start:end]
		calls := make([]bchain.EthereumMulticallCall, 0, 2*len(chunk))
		for _, c := range chunk {
			calls = append(calls,
				bchain.EthereumMulticallCall{Target: c.contract, CallData: erc4626EncodeNoArg(erc4626MethodAsset), AllowFailure: true},
				bchain.EthereumMulticallCall{Target: c.contract, CallData: erc4626EncodeNoArg(erc4626MethodTotalAssets), AllowFailure: true},
			)
		}
		results, err := mc.EthereumTypeMulticallAggregate3(calls, blockNumber)
		if err != nil || len(results) != len(calls) {
			// Skip chunk on transport failure; the next request retries.
			continue
		}

		for i, c := range chunk {
			assetResult := results[i*2]
			totalAssetsResult := results[i*2+1]

			// EIP-4626 mandates both asset() and totalAssets(); detection requires both.
			var assetContract string
			if assetResult.Success {
				if addr, derr := erc4626DecodeAddress(assetResult.Data); derr == nil && !strings.EqualFold(addr, erc4626ZeroAddress) {
					assetContract = addr
				}
			}
			if assetContract == "" || !totalAssetsResult.Success {
				negativeCache.add(c.contract, bestHeight, negativeTTLBlocks, reorgGen)
				continue
			}
			if _, derr := erc4626DecodeUint(totalAssetsResult.Data); derr != nil {
				negativeCache.add(c.contract, bestHeight, negativeTTLBlocks, reorgGen)
				continue
			}
			// Persistence is best-effort; on error or silent refusal (reorg
			// gen/hash mismatch), the response is still flagged from the live
			// probe and the negative cache is cleared so the next request retries.
			if err := setVault(c.contract, assetContract); err != nil {
				glog.Warningf("SetContractInfoErc4626Vault contract %v asset %v: %v", c.contract, assetContract, err)
			}
			negativeCache.remove(c.contract)
			c.token.Protocols = append(c.token.Protocols, contractInfoProtocolErc4626)
		}
	}
}

// buildErc4626Token returns the vault snapshot for one contract pinned to
// bestHeight. Cold path: 2 multicalls + lazy asset metadata. Warm (asset
// address cached): 1 multicall. Results memoized per (contract, height,
// reorgGen) and deduped by singleflight. Returns nil for non-vaults; caller
// is expected to have filtered by standard.
func (w *Worker) buildErc4626Token(contractInfo *bchain.ContractInfo, bestHeight uint32, bestHash string) *Erc4626Token {
	if contractInfo == nil || contractInfo.Contract == "" {
		return nil
	}
	mc, ok := w.chain.(erc4626MulticallCaller)
	if !ok {
		return nil
	}
	// Sample reorgGen+bestHash before the multicall; see SetErcProtocol.
	reorgGen := w.db.ReorgGeneration()
	setVault := func(addr, asset string) error {
		return w.db.SetContractInfoErc4626Vault(addr, asset, bestHeight, bestHash, reorgGen)
	}

	// bestHeight==0: no usable height, skip cache and read "latest" once.
	if bestHeight == 0 {
		token, _ := buildErc4626TokenWithDeps(contractInfo, mc, setVault, w.GetContractInfo, nil)
		return token
	}
	blockNumber := new(big.Int).SetUint64(uint64(bestHeight))
	return erc4626CacheLookupOrBuild(erc4626LiveCache, erc4626CacheKey(contractInfo.Contract, bestHeight, reorgGen), func() (*Erc4626Token, error) {
		return buildErc4626TokenWithDeps(contractInfo, mc, setVault, w.GetContractInfo, blockNumber)
	})
}

// buildErc4626TokenWithDeps returns the enrichment plus a cache-policy signal:
// err==nil ⇒ stable answer (cacheable); err!=nil ⇒ transient external failure,
// don't cache.
func buildErc4626TokenWithDeps(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) (*Erc4626Token, error) {
	if ci.Erc4626AssetContract == "" {
		return buildErc4626TokenCold(ci, mc, setVault, getContractInfo, blockNumber)
	}
	return buildErc4626TokenWarm(ci, mc, getContractInfo, blockNumber)
}

// buildErc4626TokenCold is detection + first-time enrichment. (nil,nil) means
// deterministically not-a-vault at this block (cacheable). (_,err) means
// transient upstream failure (don't cache).
func buildErc4626TokenCold(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	setVault erc4626VaultPersister,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) (*Erc4626Token, error) {
	contract := ci.Contract
	shareDec := ci.Decimals

	// Multicall A: detection + share-side conversions (skipped if shareUnit invalid).
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
	if err != nil {
		return nil, err
	}
	if len(resA) < 2 {
		// Short response is transport-shaped, not a deterministic "no".
		return nil, fmt.Errorf("multicall aggregate3: short response %d", len(resA))
	}

	// EIP-4626 mandates both asset() and totalAssets(); detection requires both.
	// Deterministic answers — (nil,nil) is cacheable.
	if !resA[0].Success {
		return nil, nil
	}
	assetContract, err := erc4626DecodeAddress(resA[0].Data)
	if err != nil || strings.EqualFold(assetContract, erc4626ZeroAddress) {
		return nil, nil
	}
	if !resA[1].Success {
		return nil, nil
	}
	totalAssets, err := erc4626DecodeUint(resA[1].Data)
	if err != nil {
		return nil, nil
	}

	if err := setVault(contract, assetContract); err != nil {
		glog.Warningf("SetContractInfoErc4626Vault contract %v asset %v: %v", contract, assetContract, err)
	}

	result := &Erc4626Token{
		Share: &Erc4626TokenMetadata{
			Contract: contract,
			Name:     ci.Name,
			Symbol:   ci.Symbol,
			Decimals: shareDec,
		},
		TotalAssetsSat: (*Amount)(totalAssets),
	}
	var errs []string
	// transientErr captures upstream transport failures only; on-chain decode
	// failures stay in errs (stable, vault is already confirmed).
	var transientErr error

	if shareUnitErr != nil {
		errs = append(errs, "share decimals: "+shareUnitErr.Error())
	}

	if len(resA) > 2 {
		result.ConvertToAssets1ShareSat = decodeMulticallAmount(resA[2], "convertToAssets", &errs)
	}
	if len(resA) > 3 {
		result.PreviewRedeem1ShareSat = decodeMulticallAmount(resA[3], "previewRedeem", &errs)
	}

	// Asset metadata: fetcher error is transient; (nil, false, nil) is a stable absence.
	// Do not emit asset until decimals are known: callers use asset presence as
	// the signal that conversion amounts can be scaled into whole asset units.
	assetInfo, validAsset, err := getContractInfo(assetContract, bchain.UnknownTokenStandard)
	if err != nil {
		errs = append(errs, "asset metadata: "+err.Error())
		transientErr = err
	} else if assetInfo == nil || !validAsset {
		errs = append(errs, "asset metadata unavailable")
	} else {
		result.Asset = &Erc4626TokenMetadata{
			Contract: assetContract,
			Name:     assetInfo.Name,
			Symbol:   assetInfo.Symbol,
			Decimals: assetInfo.Decimals,
		}
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
				if transientErr == nil {
					transientErr = err
				}
			} else if len(resB) >= 2 {
				result.ConvertToShares1AssetSat = decodeMulticallAmount(resB[0], "convertToShares", &errs)
				result.PreviewDeposit1AssetSat = decodeMulticallAmount(resB[1], "previewDeposit", &errs)
			}
		}
	}

	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}
	return result, transientErr
}

// buildErc4626TokenWarm is the steady-state path: one multicall for all
// time-varying fields. Always returns the metadata-only result on multicall
// error (vault is already confirmed); transient errors signal cache to skip.
func buildErc4626TokenWarm(
	ci *bchain.ContractInfo,
	mc erc4626MulticallCaller,
	getContractInfo erc4626ContractInfoFetcher,
	blockNumber *big.Int,
) (*Erc4626Token, error) {
	contract := ci.Contract
	assetContract := ci.Erc4626AssetContract
	shareDec := ci.Decimals

	result := &Erc4626Token{
		Share: &Erc4626TokenMetadata{
			Contract: contract,
			Name:     ci.Name,
			Symbol:   ci.Symbol,
			Decimals: shareDec,
		},
	}
	var errs []string
	var transientErr error // first upstream failure; non-nil tells cache to skip

	// Do not emit asset until decimals are known: callers use asset presence as
	// the signal that conversion amounts can be scaled into whole asset units.
	assetInfo, validAsset, err := getContractInfo(assetContract, bchain.UnknownTokenStandard)
	if err != nil {
		errs = append(errs, "asset metadata: "+err.Error())
		transientErr = err
	} else if assetInfo == nil || !validAsset {
		errs = append(errs, "asset metadata unavailable")
	} else {
		result.Asset = &Erc4626TokenMetadata{
			Contract: assetContract,
			Name:     assetInfo.Name,
			Symbol:   assetInfo.Symbol,
			Decimals: assetInfo.Decimals,
		}
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

	// totalAssets first, then any conversion calls whose unit amount is known.
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
		if transientErr == nil {
			transientErr = err
		}
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
	return result, transientErr
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
