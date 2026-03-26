package eth

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestContractFixes_LookupNILDecimalsCaseInsensitive(t *testing.T) {
	contractLower := "0x7cf9a80db3b29ee8efe3710aadb7b95270572d47"
	contractMixed := "0x7Cf9A80DB3b29Ee8eFe3710aAdB7B95270572d47"

	f1 := getContractFix(contractLower)
	if f1 == nil {
		t.Fatalf("expected NIL contract fix to be found for %s", contractLower)
	}
	if f1.Decimals != 6 {
		t.Fatalf("unexpected NIL decimals: got %d, want %d", f1.Decimals, 6)
	}

	f2 := getContractFix(contractMixed)
	if f2 == nil {
		t.Fatalf("expected NIL contract fix to be found for %s", contractMixed)
	}
	if f2.Decimals != 6 {
		t.Fatalf("unexpected NIL decimals for mixed-case contract: got %d, want %d", f2.Decimals, 6)
	}
}

func TestApplyContractFixToContractInfo_UpdatesDecimalsAndMetadata(t *testing.T) {
	contract := "0x7cf9a80db3b29ee8efe3710aadb7b95270572d47"
	ci := &bchain.ContractInfo{
		Contract: contract,
		Decimals: 18, // wrong; should be overridden
		// Name/Symbol are intentionally empty to ensure they get filled when the DB value is incomplete.
		Name:   "",
		Symbol: "",
	}

	changed := ApplyContractFixToContractInfo(ci, contract)
	if !changed {
		t.Fatalf("expected ApplyContractFixToContractInfo to report a change")
	}
	if ci.Decimals != 6 {
		t.Fatalf("unexpected decimals after apply: got %d, want %d", ci.Decimals, 6)
	}
	if ci.Name != "Nillion" {
		t.Fatalf("unexpected name after apply: got %q, want %q", ci.Name, "Nillion")
	}
	if ci.Symbol != "NIL" {
		t.Fatalf("unexpected symbol after apply: got %q, want %q", ci.Symbol, "NIL")
	}
}

