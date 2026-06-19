//go:build integration

package tests

import "testing"

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
