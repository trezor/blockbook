package eth

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

// Contract Fixes:
// We maintain a small allowlist of ERC-20 contracts with known correct token metadata
// (currently mainly `decimals`). This prevents users from seeing incorrect scaling
// when a contract's ABI calls fail or return unexpected results.

// Contract fixes JSON is kept as an in-code string to avoid build/tooling issues
// around embedding files from nested/non-package paths.
var contractFixesJSON = []byte(`[
  {
    "standard": "ERC20",
    "contract": "0xC19B6A4Ac7C7Cc24459F08984Bbd09664af17bD1",
    "name": "Sensorium",
    "symbol": "SENSO",
    "decimals": 0,
    "createdInBlock": 11098997
  },
  {
    "standard": "ERC20",
    "contract": "0xd5F7838F5C461fefF7FE49ea5ebaF7728bB0ADfa",
    "name": "mETH",
    "symbol": "mETH",
    "decimals": 18,
    "createdInBlock": 18290587
  },
  {
    "standard": "ERC20",
    "contract": "0xE6829d9a7eE3040e1276Fa75293Bde931859e8fA",
    "name": "cmETH",
    "symbol": "cmETH",
    "decimals": 18,
    "createdInBlock": 20439180
  },
  {
    "type": "ERC20",
    "standard": "ERC20",
    "contract": "0x6f40d4A6237C257fff2dB00FA0510DeEECd303eb",
    "name": "Fluid",
    "symbol": "FLUID",
    "decimals": 18,
    "createdInBlock": 12183236
  },
  {
    "standard": "ERC20",
    "contract": "0x7cf9a80db3b29ee8efe3710aadb7b95270572d47",
    "name": "Nillion",
    "symbol": "NIL",
    "decimals": 6
  }
]`)

type contractFix struct {
	Standard string `json:"standard"`
	Contract string `json:"contract"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

var (
	contractFixesOnce       sync.Once
	contractFixesByAddress  map[string]*contractFix
	contractFixesList       []contractFix
	contractFixesLoadErrLog sync.Once
)

func normalizeContractAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.ToLower(addr)
	return strings.TrimPrefix(addr, "0x")
}

func loadContractFixes() {
	var fixes []contractFix
	if err := json.Unmarshal(contractFixesJSON, &fixes); err != nil {
		contractFixesByAddress = nil
		// Ensure we don't spam logs if this function is called repeatedly.
		contractFixesLoadErrLog.Do(func() {
			glog.Errorf("eth: cannot unmarshal embedded contract fixes: %v", err)
		})
		return
	}

	contractFixesByAddress = make(map[string]*contractFix, len(fixes))
	contractFixesList = fixes
	for i := range fixes {
		key := normalizeContractAddress(fixes[i].Contract)
		if key == "" {
			continue
		}
		contractFixesByAddress[key] = &fixes[i]
	}
}

func getContractFix(contractAddress string) *contractFix {
	contractFixesOnce.Do(loadContractFixes)
	if contractFixesByAddress == nil {
		return nil
	}
	return contractFixesByAddress[normalizeContractAddress(contractAddress)]
}

type ContractFixInfo struct {
	Contract string
	Name     string
	Symbol   string
	Decimals int
}

// ContractFixes returns the full override list.
// The slice is safe to read after initialization; callers should treat it as read-only.
func ContractFixes() []ContractFixInfo {
	contractFixesOnce.Do(loadContractFixes)
	if contractFixesByAddress == nil {
		return nil
	}
	out := make([]ContractFixInfo, 0, len(contractFixesList))
	for i := range contractFixesList {
		out = append(out, ContractFixInfo{
			Contract: contractFixesList[i].Contract,
			Name:     contractFixesList[i].Name,
			Symbol:   contractFixesList[i].Symbol,
			Decimals: contractFixesList[i].Decimals,
		})
	}
	return out
}

// ApplyContractFixToContractInfo updates contract metadata in-place if we have an override.
// It is intentionally conservative to stay backwards compatible:
// - decimals are overwritten only when they differ
// - name/symbol are overwritten only when the existing values are empty
func ApplyContractFixToContractInfo(contractInfo *bchain.ContractInfo, contractAddress string) bool {
	if contractInfo == nil {
		return false
	}
	fix := getContractFix(contractAddress)
	if fix == nil {
		return false
	}
	changed := false

	if contractInfo.Decimals != fix.Decimals {
		contractInfo.Decimals = fix.Decimals
		changed = true
	}
	if fix.Name != "" && contractInfo.Name == "" {
		contractInfo.Name = fix.Name
		changed = true
	}
	if fix.Symbol != "" && contractInfo.Symbol == "" {
		contractInfo.Symbol = fix.Symbol
		changed = true
	}
	return changed
}

