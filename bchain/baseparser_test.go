//go:build unittest

package bchain

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/common"
)

func NewBaseParser(adp int) *BaseParser {
	return &BaseParser{
		AmountDecimalPoint: adp,
	}
}

var amounts = []struct {
	a           *big.Int
	s           string
	adp         int
	alternative string
}{
	{big.NewInt(123456789), "1.23456789", 8, "!"},
	{big.NewInt(2), "0.00000002", 8, "!"},
	{big.NewInt(300000000), "3", 8, "!"},
	{big.NewInt(498700000), "4.987", 8, "!"},
	{big.NewInt(567890), "0.00000000000056789", 18, "!"},
	{big.NewInt(-100000000), "-1", 8, "!"},
	{big.NewInt(-8), "-0.00000008", 8, "!"},
	{big.NewInt(-89012345678), "-890.12345678", 8, "!"},
	{big.NewInt(-12345), "-0.00012345", 8, "!"},
	{big.NewInt(12345678), "0.123456789012", 8, "0.12345678"},                       // test of truncation of too many decimal places
	{big.NewInt(12345678), "0.0000000000000000000000000000000012345678", 1234, "!"}, // test of too big number decimal places
}

var scientificNotationAmounts = []struct {
	a   *big.Int
	s   string
	adp int
}{
	{big.NewInt(97), "9.7e-7", 8},
	{big.NewInt(97), "9.7E-7", 8},
	{big.NewInt(970000000), "9.7e+0", 8},
	{big.NewInt(-8), "-8e-8", 8},
	{big.NewInt(12345678), "1.23456789e-1", 8},
	{big.NewInt(0), "9.7e-20", 8},
}

func TestBaseParser_AmountToDecimalString(t *testing.T) {
	for _, tt := range amounts {
		t.Run(tt.s, func(t *testing.T) {
			if got := NewBaseParser(tt.adp).AmountToDecimalString(tt.a); got != tt.s && got != tt.alternative {
				t.Errorf("BaseParser.AmountToDecimalString() = %v, want %v", got, tt.s)
			}
		})
	}
}

func TestBaseParser_AmountToBigIntScientificNotation(t *testing.T) {
	for _, tt := range scientificNotationAmounts {
		t.Run(tt.s, func(t *testing.T) {
			got, err := NewBaseParser(tt.adp).AmountToBigInt(common.JSONNumber(tt.s))
			if err != nil {
				t.Errorf("BaseParser.AmountToBigInt() error = %v", err)
				return
			}
			if got.Cmp(tt.a) != 0 {
				t.Errorf("BaseParser.AmountToBigInt() = %v, want %v", got, tt.a)
			}
		})
	}
}

func TestBaseParser_AmountToBigIntScientificNotationInvalid(t *testing.T) {
	cases := []string{
		"9.7e",
		"9.7ee-7",
		"e-7",
		"--1",
		"1.2.3e1",
		"1e2000",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := NewBaseParser(8).AmountToBigInt(common.JSONNumber(tc))
			if err == nil {
				t.Errorf("BaseParser.AmountToBigInt() expected error for %q", tc)
			}
		})
	}
}

func TestBaseParser_AmountToBigIntScientificNotationExpansionLimit(t *testing.T) {
	p := NewBaseParser(0)

	if _, err := p.AmountToBigInt(common.JSONNumber("1e1023")); err != nil {
		t.Fatalf("BaseParser.AmountToBigInt() unexpected error at limit: %v", err)
	}
	if _, err := p.AmountToBigInt(common.JSONNumber("1e1024")); err == nil {
		t.Fatalf("BaseParser.AmountToBigInt() expected error above limit")
	}
}

func TestBaseParser_AmountToBigInt(t *testing.T) {
	for _, tt := range amounts {
		t.Run(tt.s, func(t *testing.T) {
			got, err := NewBaseParser(tt.adp).AmountToBigInt(common.JSONNumber(tt.s))
			if err != nil {
				t.Errorf("BaseParser.AmountToBigInt() error = %v", err)
				return
			}
			if got.Cmp(tt.a) != 0 {
				t.Errorf("BaseParser.AmountToBigInt() = %v, want %v", got, tt.a)
			}
		})
	}
}
