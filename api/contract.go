package api

import (
	"strings"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

const contractInfoProtocolErc4626 = "erc4626"

var knownErcProtocols = []string{contractInfoProtocolErc4626}

func contractInfoSupportsRates(standard bchain.TokenStandardName) bool {
	return standard == erc4626EvmFungibleStandard()
}

func contractInfoIncludesProtocol(protocols []string, protocol string) bool {
	for _, value := range protocols {
		if strings.EqualFold(strings.TrimSpace(value), protocol) {
			return true
		}
	}
	return false
}

// ValidateErcProtocols rejects protocol values not recognised by this API.
// Empty and whitespace-only entries are tolerated for convenience.
func ValidateErcProtocols(protocols []string) error {
	for _, p := range protocols {
		normalized := strings.ToLower(strings.TrimSpace(p))
		if normalized == "" {
			continue
		}
		known := false
		for _, k := range knownErcProtocols {
			if normalized == k {
				known = true
				break
			}
		}
		if !known {
			return NewAPIError("Unknown protocol: "+p, true)
		}
	}
	return nil
}

// ValidateProtocolsForChain rejects a non-empty protocols list on coins that
// don't support any protocol enrichments, and otherwise validates the values.
func (w *Worker) ValidateProtocolsForChain(protocols []string) error {
	if len(protocols) == 0 {
		return nil
	}
	if w.chainType != bchain.ChainEthereumType {
		return NewAPIError("protocols parameter is not supported on this coin", true)
	}
	return ValidateErcProtocols(protocols)
}

func (w *Worker) enrichTokenProtocols(tokens Tokens, protocols []string) {
	if !contractInfoIncludesProtocol(protocols, contractInfoProtocolErc4626) {
		return
	}
	// Read best block lazily, only once a relevant protocol was requested, so
	// accountInfo requests without protocol enrichment skip the CF seek.
	// On error proceed with bestHeight==0 (no in-block caching) but log.
	bestHeight, bestHash, err := w.db.GetBestBlock()
	if err != nil {
		glog.Warningf("GetBestBlock for protocol enrichment: %v", err)
	}
	w.enrichErc4626Tokens(tokens, bestHeight, bestHash)
}

// contractInfoResultFromBchain wraps bchain.ContractInfo into the API-level
// ContractInfoResult. Rates and Protocols stay nil; callers that want
// enrichment use GetContractInfoData directly.
func contractInfoResultFromBchain(ci *bchain.ContractInfo, bestHeight uint32) *ContractInfoResult {
	if ci == nil {
		return nil
	}
	return &ContractInfoResult{
		Type:              ci.Type,
		Standard:          ci.Standard,
		Contract:          ci.Contract,
		Name:              ci.Name,
		Symbol:            ci.Symbol,
		Decimals:          ci.Decimals,
		CreatedInBlock:    ci.CreatedInBlock,
		DestructedInBlock: ci.DestructedInBlock,
		BlockHeight:       bestHeight,
	}
}

func (w *Worker) buildContractInfoRates(contract string, standard bchain.TokenStandardName, currency string) *ContractInfoRates {
	if !contractInfoSupportsRates(standard) || w.fiatRates == nil {
		return nil
	}

	currency = strings.ToLower(strings.TrimSpace(currency))
	ticker := getCurrentTicker(w.fiatRates, currency, contract)
	baseRate, baseRateFound := w.GetContractBaseRate(ticker, contract, 0)
	if !baseRateFound && currency == "" {
		return nil
	}

	rates := &ContractInfoRates{}
	if baseRateFound {
		rates.BaseRate = baseRate
	}
	if currency != "" {
		rates.Currency = currency
		if ticker != nil {
			if secondaryRate := ticker.TokenRateInCurrency(contract, currency); secondaryRate > 0 {
				rates.SecondaryRate = float64(secondaryRate)
			}
		}
	}
	return rates
}

func (w *Worker) GetContractInfoData(contract string, currency string, protocols []string) (*ContractInfoResult, error) {
	if w.chainType != bchain.ChainEthereumType {
		return nil, NewAPIError("getContractInfo is not supported on this coin", true)
	}
	if strings.TrimSpace(contract) == "" {
		return nil, NewAPIError("Missing contract", true)
	}
	if err := ValidateErcProtocols(protocols); err != nil {
		return nil, err
	}

	contractInfo, validContract, err := w.GetContractInfo(contract, bchain.UnknownTokenStandard)
	if err != nil {
		return nil, NewAPIError("Invalid contract, "+err.Error(), true)
	}
	if contractInfo == nil || !validContract {
		return nil, NewAPIError("Contract not found", true)
	}

	bestHeight, bestHash, err := w.db.GetBestBlock()
	if err != nil {
		return nil, err
	}

	result := &ContractInfoResult{
		Type:              contractInfo.Type,
		Standard:          contractInfo.Standard,
		Contract:          contractInfo.Contract,
		Name:              contractInfo.Name,
		Symbol:            contractInfo.Symbol,
		Decimals:          contractInfo.Decimals,
		CreatedInBlock:    contractInfo.CreatedInBlock,
		DestructedInBlock: contractInfo.DestructedInBlock,
		Rates:             w.buildContractInfoRates(contractInfo.Contract, contractInfo.Standard, currency),
		BlockHeight:       bestHeight,
	}

	// Probe only for ERC20-shaped contracts (or unknown/unhandled, which covers
	// freshly RPC-fetched contracts with no tagged standard); ERC721/ERC1155
	// would always fail the probe.
	if !contractInfoIncludesProtocol(protocols, contractInfoProtocolErc4626) ||
		(contractInfo.Standard != bchain.UnknownTokenStandard && contractInfo.Standard != bchain.UnhandledTokenStandard && contractInfo.Standard != erc4626EvmFungibleStandard()) {
		return result, nil
	}

	erc4626 := w.buildErc4626Token(contractInfo, bestHeight, bestHash)
	if erc4626 == nil {
		return result, nil
	}
	result.Protocols = &ContractInfoProtocols{Erc4626: erc4626}
	return result, nil
}
