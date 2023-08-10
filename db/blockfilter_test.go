//go:build unittest

package db

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestComputeBlockFilter(t *testing.T) {
	// TODO: add more (vectorized) tests, with taproot txs
	// - both taprootOnly=true and taprootOnly=false
	// - check that decoding with different P does not work
	allAddrDesc := getallAddrDesc()
	blockHash := "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"
	taprootOnly := false
	got := computeBlockFilter(allAddrDesc, blockHash, taprootOnly)
	want := "0847a3118f0a689307a375c45c1b02379119579910ee80"
	if got != want {
		t.Errorf("computeBlockFilter() failed, expected: %s, got: %s", want, got)
	}
}

func getallAddrDesc() [][]byte {
	allAddrDesc := make([][]byte, 0)
	parser := bitcoinTestnetParser()

	// TODO: this data is copied exactly, make it common and reuse it
	ta := &TxAddresses{
		Height: 12345,
		VSize:  321,
		Inputs: []TxInput{
			{
				AddrDesc: addressToAddrDesc("2N7iL7AvS4LViugwsdjTB13uN4T7XhV1bCP", parser),
				ValueSat: *big.NewInt(9011000000),
				Txid:     "c50c7ce2f5670fd52de738288299bd854a85ef1bb304f62f35ced1bd49a8a810",
				Vout:     0,
			},
			{
				AddrDesc: addressToAddrDesc("2Mt9v216YiNBAzobeNEzd4FQweHrGyuRHze", parser),
				ValueSat: *big.NewInt(8011000000),
				Txid:     "e96672c7fcc8da131427fcea7e841028614813496a56c11e8a6185c16861c495",
				Vout:     1,
			},
			{
				AddrDesc: addressToAddrDesc("2NDyqJpHvHnqNtL1F9xAeCWMAW8WLJmEMyD", parser),
				ValueSat: *big.NewInt(7011000000),
				Txid:     "ed308c72f9804dfeefdbb483ef8fd1e638180ad81d6b33f4b58d36d19162fa6d",
				Vout:     134,
			},
		},
		Outputs: []TxOutput{
			{
				AddrDesc:    addressToAddrDesc("2MuwoFGwABMakU7DCpdGDAKzyj2nTyRagDP", parser),
				ValueSat:    *big.NewInt(5011000000),
				Spent:       true,
				SpentTxid:   dbtestdata.TxidB1T1,
				SpentIndex:  0,
				SpentHeight: 432112345,
			},
			{
				AddrDesc: addressToAddrDesc("2Mvcmw7qkGXNWzkfH1EjvxDcNRGL1Kf2tEM", parser),
				ValueSat: *big.NewInt(6011000000),
			},
			{
				AddrDesc:    addressToAddrDesc("2N9GVuX3XJGHS5MCdgn97gVezc6EgvzikTB", parser),
				ValueSat:    *big.NewInt(7011000000),
				Spent:       true,
				SpentTxid:   dbtestdata.TxidB1T2,
				SpentIndex:  14231,
				SpentHeight: 555555,
			},
			{
				AddrDesc: addressToAddrDesc("mzii3fuRSpExMLJEHdHveW8NmiX8MPgavk", parser),
				ValueSat: *big.NewInt(999900000),
			},
			{
				AddrDesc:    addressToAddrDesc("mqHPFTRk23JZm9W1ANuEFtwTYwxjESSgKs", parser),
				ValueSat:    *big.NewInt(5000000000),
				Spent:       true,
				SpentTxid:   dbtestdata.TxidB2T1,
				SpentIndex:  674541,
				SpentHeight: 6666666,
			},
		},
	}

	for _, input := range ta.Inputs {
		allAddrDesc = append(allAddrDesc, input.AddrDesc)
	}
	for _, output := range ta.Outputs {
		allAddrDesc = append(allAddrDesc, output.AddrDesc)
	}

	return allAddrDesc
}
