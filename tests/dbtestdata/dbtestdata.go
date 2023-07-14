package dbtestdata

import (
	"encoding/hex"
	"math/big"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

// Txids, Xpubs and Addresses
const (
	TxidB1T1 = "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840"
	TxidB1T2 = "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"
	TxidB2T1 = "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25"
	TxidB2T2 = "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71"
	TxidB2T3 = "05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"
	TxidB2T4 = "fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"

	Xpub              = "upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q"
	TaprootDescriptor = "tr([5c9e228d/86'/1'/0']tpubDC88gkaZi5HvJGxGDNLADkvtdpni3mLmx6vr2KnXmWMG8zfkBRggsxHVBkUpgcwPe2KKpkyvTJCdXHb1UHEWE64vczyyPQfHr1skBcsRedN/{0,1}/*)#4rqwxvej"

	Addr1 = "mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"  // 76a914010d39800f86122416e28f485029acf77507169288ac
	Addr2 = "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"  // 76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac
	Addr3 = "mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"  // 76a914a08eae93007f22668ab5e4a9c83c8cd1c325e3e088ac
	Addr4 = "2MzmAKayJmja784jyHvRUW1bXPget1csRRG" // a91452724c5178682f70e0ba31c6ec0633755a3b41d987, xpub m/49'/1'/33'/0/0
	Addr5 = "2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1" // a914e921fc4912a315078f370d959f2c4f7b6d2a683c87
	Addr6 = "mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"  // 76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac
	Addr7 = "mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"  // 76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac
	Addr8 = "2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu" // a91495e9fbe306449c991d314afe3c3567d5bf78efd287, xpub m/49'/1'/33'/1/3
	Addr9 = "mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"  // 76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac
	AddrA = "mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj"  // 76a914d03c0d863d189b23b061a95ad32940b65837609f88ac

	TxidB2T1Output3OpReturn = "6a072020f1686f6a20"
)

// Amounts in satoshis
var (
	SatZero         = big.NewInt(0)
	SatB1T1A1       = big.NewInt(100000000)
	SatB1T1A2       = big.NewInt(12345)
	SatB1T1A2Double = big.NewInt(12345 * 2)
	SatB1T2A3       = big.NewInt(1234567890123)
	SatB1T2A4       = big.NewInt(1)
	SatB1T2A5       = big.NewInt(9876)
	SatB2T1A6       = big.NewInt(317283951061)
	SatB2T1A7       = big.NewInt(917283951061)
	SatB2T2A8       = big.NewInt(118641975500)
	SatB2T2A9       = big.NewInt(198641975500)
	SatB2T3A5       = big.NewInt(9000)
	SatB2T4AA       = big.NewInt(1360030331)
)

// AddressToPubKeyHex is a utility conversion function
func AddressToPubKeyHex(addr string, parser bchain.BlockChainParser) string {
	if addr == "" {
		return ""
	}
	b, err := parser.GetAddrDescFromAddress(addr)
	if err != nil {
		glog.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// GetTestBitcoinTypeBlock1 returns block #1
func GetTestBitcoinTypeBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        225493,
			Hash:          "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997",
			Size:          1234567,
			Time:          1521515026,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{
			{
				Txid: TxidB1T1,
				Vin:  []bchain.Vin{},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr1, parser),
						},
						ValueSat: *SatB1T1A1,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr2, parser),
						},
						ValueSat: *SatB1T1A2,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr2, parser),
						},
						ValueSat: *SatB1T1A2,
					},
				},
				Blocktime:     1521515026,
				Time:          1521515026,
				Confirmations: 2,
			},
			{
				Txid: TxidB1T2,
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr3, parser),
						},
						ValueSat: *SatB1T2A3,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr4, parser),
						},
						ValueSat: *SatB1T2A4,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr5, parser),
						},
						ValueSat: *SatB1T2A5,
					},
				},
				Blocktime:     1521515026,
				Time:          1521515026,
				Confirmations: 2,
			},
		},
	}
}

// GetTestBitcoinTypeBlock2 returns block #2
func GetTestBitcoinTypeBlock2(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        225494,
			Hash:          "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
			Size:          2345678,
			Time:          1521595678,
			Confirmations: 1,
		},
		Txs: []bchain.Tx{
			{
				Txid: TxidB2T1,
				Vin: []bchain.Vin{
					// addr3
					{
						Txid: TxidB1T2,
						Vout: 0,
					},
					// addr2
					{
						Txid: TxidB1T1,
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr6, parser),
						},
						ValueSat: *SatB2T1A6,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr7, parser),
						},
						ValueSat: *SatB2T1A7,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidB2T1Output3OpReturn, // OP_RETURN script
						},
						ValueSat: *SatZero,
					},
				},
				Blocktime:     1521595678,
				Time:          1521595678,
				Confirmations: 1,
			},
			{
				Txid: TxidB2T2,
				Vin: []bchain.Vin{
					// spending an output in the same block - addr6
					{
						Txid: TxidB2T1,
						Vout: 0,
					},
					// spending an output in the previous block - addr4
					{
						Txid: TxidB1T2,
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr8, parser),
						},
						ValueSat: *SatB2T2A8,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr9, parser),
						},
						ValueSat: *SatB2T2A9,
					},
				},
				Blocktime:     1521595678,
				Time:          1521595678,
				Confirmations: 1,
			},
			// transaction from the same address in the previous block
			{
				Txid: TxidB2T3,
				Vin: []bchain.Vin{
					// addr5
					{
						Txid: TxidB1T2,
						Vout: 2,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(Addr5, parser),
						},
						ValueSat: *SatB2T3A5,
					},
				},
				Blocktime:     1521595678,
				Time:          1521595678,
				Confirmations: 1,
			},
			// mining transaction
			{
				Txid: TxidB2T4,
				Vin: []bchain.Vin{
					{
						Coinbase: "03bf1e1504aede765b726567696f6e312f50726f6a65637420425443506f6f6c2f01000001bf7e000000000000",
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrA, parser),
						},
						ValueSat: *SatB2T4AA,
					},
					{
						N:            1,
						ScriptPubKey: bchain.ScriptPubKey{},
						ValueSat:     *SatZero,
					},
				},
				Blocktime:     1521595678,
				Time:          1521595678,
				Confirmations: 1,
			},
		},
	}
}
