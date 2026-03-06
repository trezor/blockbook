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
