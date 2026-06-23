//go:build integration

package tests

import (
	"encoding/json"
	"testing"
)

func TestGetMatchableName(t *testing.T) {
	cases := map[string]string{
		"ethereum":                 "ethereum=main",
		"tron":                     "tron=main",
		"bitcoin_regtest":          "bitcoin_regtest=main",
		"bitcoin_testnet":          "bitcoin=test",
		"bitcoin_testnet4":         "bitcoin=test4",
		"ethereum_testnet_sepolia": "ethereum=test_sepolia",
		"ethereum_testnet_hoodi":   "ethereum=test_hoodi",
		"tron_testnet_nile":        "tron=test_nile",
	}
	for coin, want := range cases {
		if got := getMatchableName(coin); got != want {
			t.Errorf("getMatchableName(%q) = %q, want %q", coin, got, want)
		}
	}
}

// TestGetMatchableNameInjective guards against re-introducing the collision where
// every "<coin>_testnet*" key collapsed to "<coin>=test": the deploy connectivity
// regex for one testnet would then also select its siblings.
func TestGetMatchableNameInjective(t *testing.T) {
	tests, err := loadTests("tests.json")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string, len(tests))
	for coin := range tests {
		name := getMatchableName(coin)
		if other, dup := seen[name]; dup {
			t.Errorf("%q and %q both map to %q", coin, other, name)
		}
		seen[name] = coin
	}
}

func TestIsDisabled(t *testing.T) {
	cases := map[string]bool{
		`{"disabled": true}`:                    true,
		`{"disabled": false}`:                   false,
		`{"connectivity": ["http"]}`:            false,
		`{"disabled": true, "api": ["Status"]}`: true,
		`{"disabled": "yes"}`:                   false, // non-bool is ignored, not disabled
	}
	for raw, want := range cases {
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("unmarshal %q: %v", raw, err)
		}
		if got := isDisabled(cfg); got != want {
			t.Errorf("isDisabled(%s) = %v, want %v", raw, got, want)
		}
	}
}

// TestDisabledCoinsSkippedInTestsJSON guards that a coin flagged disabled in the
// real tests.json is not silently treated as a runnable coin.
func TestDisabledCoinsSkippedInTestsJSON(t *testing.T) {
	tests, err := loadTests("tests.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg, ok := tests["tron_testnet_nile"]
	if !ok {
		t.Skip("tron_testnet_nile not present in tests.json")
	}
	if !isDisabled(cfg) {
		t.Errorf("tron_testnet_nile expected to be disabled in tests.json")
	}
}
