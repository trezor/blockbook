//go:build integration

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type erc4626Fixture struct {
	Name     string `json:"name"`
	Holder   string `json:"holder"`
	Contract string `json:"contract"`
}

type testData struct {
	ERC4626Fixtures []erc4626Fixture `json:"erc4626Fixtures,omitempty"`
	// NonVaultContracts is a list of EIP-55 addresses known not to be ERC-4626
	// vaults. The strict-gate negative test asserts that even with
	// ?protocols=erc4626, none of these come back with a protocols.erc4626
	// payload — protecting against a regression where the detection gate
	// falsely accepts contracts that merely expose asset() or totalAssets().
	NonVaultContracts []string `json:"nonVaultContracts,omitempty"`
}

func loadAPITestData(coin string) (*testData, error) {
	path := filepath.Join("api/testdata", coin+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v testData
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
