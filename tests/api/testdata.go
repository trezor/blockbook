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

type fiatTruthFixtureSet struct {
	Source    string             `json:"source"`
	Currency  string             `json:"currency"`
	FetchedAt string             `json:"fetchedAt"`
	Cases     []fiatTruthFixture `json:"cases"`
}

type fiatTruthFixture struct {
	Name              string               `json:"name"`
	CoinID            string               `json:"coinId"`
	Symbol            string               `json:"symbol"`
	Contract          string               `json:"contract"`
	Currency          string               `json:"currency"`
	ExpectedRates     []fiatTruthRatePoint `json:"expectedRates"`
	RelativeTolerance float64              `json:"relativeTolerance"`
	Source            string               `json:"source"`
	FetchedAt         string               `json:"fetchedAt"`
}

type fiatTruthRatePoint [2]float64

func (p fiatTruthRatePoint) Timestamp() int64 {
	return int64(p[0])
}

func (p fiatTruthRatePoint) Rate() float64 {
	return p[1]
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

func loadAPIFiatTruthTestData(coin string) (*fiatTruthFixtureSet, error) {
	path := filepath.Join("api/testdata", coin+"_fiat_truth.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v fiatTruthFixtureSet
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
