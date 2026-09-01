package api

import (
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/db"
)

// contractProbeCacheCapacity bounds the negative cache; eviction is not a correctness event,
// it just costs the one probe every request pays for that contract today.
const contractProbeCacheCapacity = 8192

// contractInfoProbes are the verdicts a batched pre-pass already resolved, keyed by address
// descriptor, so the per-contract path does not read the same contract again.
type contractInfoProbes map[string]bchain.EthereumContractInfoResult

// contractInfoBatchResolver is the chain-side seam for batched metadata reads, satisfied by
// EVM chains via Multicall3 aggregate3.
type contractInfoBatchResolver interface {
	EthereumTypeGetContractInfos(contractDescs []bchain.AddressDescriptor) []bchain.EthereumContractInfoResult
}

// contractProbeCacheState samples the height and reorg generation that scope negative cache
// entries. A zero height (nothing indexed yet) disables the cache for this request.
func (w *Worker) contractProbeCacheState() (bestHeight uint32, reorgGen uint64) {
	_, bestHeight, _, _ = w.is.GetSyncState()
	return bestHeight, w.db.ReorgGeneration()
}

// resolveContractInfo returns the pre-pass verdict for cd, else reads this one contract from the
// chain, collapsing concurrent probes. (nil, nil) is a conclusive "no token here" - cacheable.
func (w *Worker) resolveContractInfo(cd bchain.AddressDescriptor, probes contractInfoProbes) (*bchain.ContractInfo, error) {
	if probe, ok := probes[string(cd)]; ok {
		return copyContractInfo(probe.Info), probe.Err
	}
	v, err, _ := w.contractProbeGroup.Do(string(cd), func() (interface{}, error) {
		return w.chain.GetContractInfo(cd)
	})
	info, _ := v.(*bchain.ContractInfo)
	return copyContractInfo(info), err
}

// copyContractInfo hands every caller its own struct: the batch pre-pass shares one result
// across callers that go on to mutate and store it.
func copyContractInfo(ci *bchain.ContractInfo) *bchain.ContractInfo {
	if ci == nil {
		return nil
	}
	c := *ci
	return &c
}

// unknownContractInfo is how an unresolved contract is surfaced: no name or symbol and the coin's
// decimals. Callers pair it with validContract=false, which keeps the contract out of balance reads.
func (w *Worker) unknownContractInfo(cd bchain.AddressDescriptor) *bchain.ContractInfo {
	contractInfo := &bchain.ContractInfo{Standard: bchain.UnknownTokenStandard, Decimals: w.chainParser.AmountDecimals()}
	if addresses, _, _ := w.chainParser.GetAddressesFromAddrDesc(cd); len(addresses) > 0 {
		contractInfo.Contract = addresses[0]
	}
	return contractInfo
}

// contractNeedsChainRefresh reports whether a stored record must still be read from the chain: an
// unhandled standard (sync skips the fetch for internal-tx contracts), or a zero-parsed name/symbol.
func contractNeedsChainRefresh(contractInfo *bchain.ContractInfo) bool {
	return contractInfo.Standard == bchain.UnhandledTokenStandard ||
		(len(contractInfo.Name) > 0 && contractInfo.Name[0] == 0) ||
		(len(contractInfo.Symbol) > 0 && contractInfo.Symbol[0] == 0)
}

// prefetchContractInfos resolves in one batched call the metadata of the contracts the token loop
// would otherwise probe one at a time, skipping those the index describes or a recent probe settled.
func (w *Worker) prefetchContractInfos(contracts []db.AddrContract) contractInfoProbes {
	resolver, ok := w.chain.(contractInfoBatchResolver)
	if !ok {
		return nil
	}
	bestHeight, reorgGen := w.contractProbeCacheState()
	unresolved := make([]bchain.AddressDescriptor, 0, len(contracts))
	for i := range contracts {
		cd := contracts[i].Contract
		// read with an unknown standard: promoting it is the per-contract path's job
		contractInfo, err := w.db.GetContractInfo(cd, bchain.UnknownTokenStandard)
		if err != nil {
			continue
		}
		if contractInfo == nil {
			if !w.contractProbeCache.contains(string(cd), bestHeight, reorgGen) {
				unresolved = append(unresolved, cd)
			}
			continue
		}
		if contractNeedsChainRefresh(contractInfo) {
			unresolved = append(unresolved, cd)
		}
	}
	// one contract is not worth a batch - the per-contract path reads it just as cheaply
	if len(unresolved) < 2 {
		return nil
	}
	results := resolver.EthereumTypeGetContractInfos(unresolved)
	if len(results) != len(unresolved) {
		return nil
	}
	probes := make(contractInfoProbes, len(unresolved))
	for i, cd := range unresolved {
		probes[string(cd)] = results[i]
	}
	return probes
}
