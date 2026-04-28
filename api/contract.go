package api

import (
	"strings"

	"github.com/trezor/blockbook/bchain"
)

const contractInfoProtocolErc4626 = "erc4626"

var knownContractProtocols = []string{contractInfoProtocolErc4626}

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

// ValidateContractProtocols rejects protocol values not recognised by this API.
// Empty and whitespace-only entries are tolerated for convenience.
func ValidateContractProtocols(protocols []string) error {
	for _, p := range protocols {
		normalized := strings.ToLower(strings.TrimSpace(p))
		if normalized == "" {
			continue
		}
		known := false
		for _, k := range knownContractProtocols {
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
	return ValidateContractProtocols(protocols)
}

func (w *Worker) enrichTokenProtocols(tokens Tokens, protocols []string) {
	if !contractInfoIncludesProtocol(protocols, contractInfoProtocolErc4626) {
		return
	}
	w.enrichErc4626Tokens(tokens)
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
	if err := ValidateContractProtocols(protocols); err != nil {
		return nil, err
	}

	contractInfo, validContract, err := w.GetContractInfo(contract, bchain.UnknownTokenStandard)
	if err != nil {
		return nil, NewAPIError("Invalid contract, "+err.Error(), true)
	}
	if contractInfo == nil || !validContract {
		return nil, NewAPIError("Contract not found", true)
	}

	bestHeight, _, err := w.db.GetBestBlock()
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

	if !contractInfoIncludesProtocol(protocols, contractInfoProtocolErc4626) || w.chainType != bchain.ChainEthereumType || contractInfo.Standard != erc4626EvmFungibleStandard() {
		return result, nil
	}

	probe, isVault := w.detectErc4626Vault(contractInfo.Contract)
	if !isVault {
		return result, nil
	}

	result.Protocols = &ContractInfoProtocols{
		Erc4626: w.fetchErc4626TokenData(&Token{
			Contract: contractInfo.Contract,
			Name:     contractInfo.Name,
			Symbol:   contractInfo.Symbol,
			Decimals: contractInfo.Decimals,
			Standard: contractInfo.Standard,
		}, probe),
	}
	return result, nil
}
