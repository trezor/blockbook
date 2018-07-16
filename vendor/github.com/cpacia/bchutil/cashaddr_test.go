package bchutil

import (
	"testing"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
)

var TestVectorsP2PKH = [][]string {
	{"1BpEi6DfDAUFd7GtittLSdBeYJvcoaVggu", "bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a"},
	{"1KXrWXciRDZUpQwQmuM1DbwsKDLYAYsVLR", "bitcoincash:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y0qverfuy"},
	{"16w1D5WRVKJuZUsSRzdLp9w3YGcgoxDXb", "bitcoincash:qqq3728yw0y47sqn6l2na30mcw6zm78dzqre909m2r"},
}

var TestVectorsP2SH = [][]string {
	{"3CWFddi6m4ndiGyKqzYvsFYagqDLPVMTzC", "bitcoincash:ppm2qsznhks23z7629mms6s4cwef74vcwvn0h829pq"},
	{"3LDsS579y7sruadqu11beEJoTjdFiFCdX4", "bitcoincash:pr95sy3j9xwd2ap32xkykttr4cvcu7as4yc93ky28e"},
	{"31nwvkZwyPdgzjBJZXfDmSWsC4ZLKpYyUw", "bitcoincash:pqq3728yw0y47sqn6l2na30mcw6zm78dzq5ucqzc37"},
}

var valid []string = []string {
	"prefix:x64nx6hz",
	"PREFIX:X64NX6HZ",
	"p:gpf8m4h7",
	"bitcoincash:qpzry9x8gf2tvdw0s3jn54khce6mua7lcw20ayyn",
	"bchtest:testnetaddress4d6njnut",
	"bchreg:555555555555555555555555555555555555555555555udxmlmrz",
}

func TestValid(t *testing.T) {
	for _, s := range valid {
		_, _, err := DecodeCashAddress(s)
		if err != nil {
			t.Error(err)
		}
	}
}

var Invalid []string = []string {
	"prefix:x32nx6hz",
	"prEfix:x64nx6hz",
	"prefix:x64nx6Hz",
	"pref1x:6m8cxv73",
	"prefix:",
	":u9wsx07j",
	"bchreg:555555555555555555x55555555555555555555555555udxmlmrz",
	"bchreg:555555555555555555555555555555551555555555555udxmlmrz",
	"pre:fix:x32nx6hz",
	"prefixx64nx6hz",
}

func TestInvalid(t *testing.T) {
	for _, s := range Invalid {
		_, _, err := DecodeCashAddress(s)
		if err == nil {
			t.Error("Failed to error on invalid string")
		}
	}
}

func TestDecodeCashAddress(t *testing.T) {
	// Mainnet
	addr, err := DecodeAddress("bitcoincash:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y0qverfuy", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bitcoincash:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y0qverfuy" {
		t.Error("Address decoding error")
	}
	addr1, err := DecodeAddress("bitcoincash:ppm2qsznhks23z7629mms6s4cwef74vcwvn0h829pq", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr1.String() != "bitcoincash:ppm2qsznhks23z7629mms6s4cwef74vcwvn0h829pq" {
		t.Error("Address decoding error")
	}
	// Testnet
	addr, err = DecodeAddress("bchtest:qr95sy3j9xwd2ap32xkykttr4cvcu7as4ytjg7p7mc", &chaincfg.TestNet3Params)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchtest:qr95sy3j9xwd2ap32xkykttr4cvcu7as4ytjg7p7mc" {
		t.Error("Address decoding error")
	}
	// Regtest
	addr, err = DecodeAddress("bchreg:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y3w7lzdc7", &chaincfg.RegressionNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchreg:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y3w7lzdc7" {
		t.Error("Address decoding error")
	}
}

var dataElement = []byte{203, 72, 18, 50, 41, 156, 213, 116, 49, 81, 172, 75, 45, 99, 174, 25, 142, 123, 176, 169}
// Second address of https://github.com/Bitcoin-UAHF/spec/blob/master/cashaddr.md#examples-of-address-translation
func TestCashAddressPubKeyHash_EncodeAddress(t *testing.T) {
	// Mainnet
	addr, err := NewCashAddressPubKeyHash(dataElement, &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bitcoincash:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y0qverfuy" {
		t.Error("Address decoding error")
	}
	// Testnet
	addr, err = NewCashAddressPubKeyHash(dataElement, &chaincfg.TestNet3Params)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchtest:qr95sy3j9xwd2ap32xkykttr4cvcu7as4ytjg7p7mc" {
		t.Error("Address decoding error")
	}
	// Regtest
	addr, err = NewCashAddressPubKeyHash(dataElement, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchreg:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y3w7lzdc7" {
		t.Error("Address decoding error")
	}
}

var dataElement2 = []byte{118, 160, 64, 83, 189, 160, 168, 139, 218, 81, 119, 184, 106, 21, 195, 178, 159, 85, 152, 115}
// 4th address of https://github.com/Bitcoin-UAHF/spec/blob/master/cashaddr.md#examples-of-address-translation
func TestCashAddressScriptHash_EncodeAddress(t *testing.T) {
	// Mainnet
	addr, err := NewCashAddressScriptHashFromHash(dataElement2, &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bitcoincash:ppm2qsznhks23z7629mms6s4cwef74vcwvn0h829pq" {
		t.Error("Address decoding error")
	}
	// Testnet
	addr, err = NewCashAddressScriptHashFromHash(dataElement2, &chaincfg.TestNet3Params)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchtest:ppm2qsznhks23z7629mms6s4cwef74vcwvhanqgjxu" {
		t.Error("Address decoding error")
	}
	// Regtest
	addr, err = NewCashAddressScriptHashFromHash(dataElement2, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "bchreg:ppm2qsznhks23z7629mms6s4cwef74vcwvdp9ptp96" {
		t.Error("Address decoding error")
	}
}

func TestTestVectors(t *testing.T) {
	for _, v := range TestVectorsP2PKH {
		addr, err := btcutil.DecodeAddress(v[0], &chaincfg.MainNetParams)
		if err != nil {
			t.Error(err)
			return
		}
		addr2, err := NewCashAddressPubKeyHash(addr.ScriptAddress(), &chaincfg.MainNetParams)
		if err != nil {
			t.Error(err)
		}
		if addr2.String() != v[1] {
			t.Error("Failed to derive correct address")
		}
	}
	for _, v := range TestVectorsP2SH {
		addr, err := btcutil.DecodeAddress(v[0], &chaincfg.MainNetParams)
		if err != nil {
			t.Error(err)
			return
		}
		addr2, err := NewCashAddressScriptHashFromHash(addr.ScriptAddress(), &chaincfg.MainNetParams)
		if err != nil {
			t.Error(err)
		}
		if addr2.String() != v[1] {
			t.Error("Failed to derive correct address")
		}
	}
}