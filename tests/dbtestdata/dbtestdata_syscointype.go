package dbtestdata

import (
	"blockbook/bchain"
	"math/big"
)

// Txids, Addresses
const (
	TxidS1T1INPUT0 = "8e745614bb3bfc3ae9fbd9ec36a69c0667a128c4e017aba6951d04a3d515255d"
	TxidS1T0 = "8c6442513c9d34ef1fb15bed9c318dc60e05a5802075347b9425fc8257662e5b"
	TxidS1T1 = "a1d8703b18f53501c5165aa3cc2a9ff22edfadd35b4cba25c5212d6bf67ff4d8"
	TxidS2T0 = "abc6a23c418cf96217f6c89308fff72e3e075216105a866142313ed1a6010da7"
	TxidS2T1 = "4e1f1d8da238c0684e0f6649cc1e8ca1b18dde2f15a18660078106e26c82d085"


	AddrS1 = "sys1qyz3lpck0d408ukzfp8p95q7s79traduvk5raga"
	AddrS2 = "sys1quecv5gwlkakghzkf0a95m83zy68dpt6z90yg9w"
	AddrS3 = "sys1qxxsw663zfufvvelygzrykt0d0gsaku8kz8pn3a"
	AddrS5 = "sys1q66dnt6mle02m5v8lckym8e53utmx6has5qrl8q"
	TxidS1T0OutputReturn = "6a24aa21a9edba34b1ff320a4a633d3ec439863183244049aca52ca18305cb7d87e2b47bcabb" // auxpow commitment in coinbase
	TxidS1T1OutputReturn = "6a3301b8c0ca9601010000080451304655851b7b2264657363223a226348566962476c6a646d46736457553d227d0064008668ff00"
	TxidS2T0OutputReturn = "6a24aa21a9ed65bb8625e4244bf10a8e0aad66ed16c842e61cc19ab2649fe79280a352ac08ae" // auxpow commitment in coinbase
	TxidS2T1OutputReturn = "6a4c6401b8c0ca9601010000080087142b1e58b979e4b2d72d8bca5bb4646ccc032ddbfc001f7b2264657363223a22626d563349484231596d787059335a686248566c227d1b7b2264657363223a226348566962476c6a646d46736457553d227d822400007fff"
	
)

// Amounts in satoshis
var (
	SatS1T0A1       = big.NewInt(3465000204)
	SatS1T0A2       = big.NewInt(2598753670)
	SatS1T1A1       = big.NewInt(99999796)
	SatS1T1A2       = big.NewInt(49900000000)
	SatS2T0A1       = big.NewInt(3465000207)
	SatS2T1A1       = big.NewInt(99999589)
	SatS1T1INPUT0   = big.NewInt(100000000)
	SatS2T1INPUT0   = big.NewInt(99964500)
	SatS1T1OPRETURN = big.NewInt(50000000000)
)

// GetTestSyscoinTypeBlock1 returns block #1
func GetTestSyscoinTypeBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        171,
			Hash:          "00000da4905f27bad527f9ec2fb78090ee4079bd4d7219ee2f450e5439d0ed38",
			Size:          536,
			Time:          1588899698,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS1T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "02ab000101",
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS1, parser),
						},
						ValueSat: *SatS1T0A1,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS1T0OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatZero,
					},
				},
				Blocktime:     1588899698,
				Time:          1588899698,
				Confirmations: 2,
			},
			{
				Version: 130, // asset activate coloured coin tx
				Txid: TxidS1T1,
				Vin: []bchain.Vin{
					{
						Txid: TxidS1T1INPUT0,
						Vout: 0,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS2, parser),
						},
						ValueSat: *SatS1T1A1,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS1T1OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatS1T1OPRETURN,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS3, parser),
						},
						ValueSat: *SatS1T1A2,
					},
				},
				Blocktime:     1588899698,
				Time:          1588899698,
				Confirmations: 2,
			},
		},
	}
}

// GetTestSyscoinTypeBlock2 returns block #2
func GetTestSyscoinTypeBlock2(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        182,
			Hash:          "00000e4afb4178a83b1b6e05872c5754b007f94b7645d93443a4ee51c45a2d74",
			Size:          539,
			Time:          1588899730,
			Confirmations: 1,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS2T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "02b6000101",
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS1, parser),
						},
						ValueSat: *SatS2T0A1,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS2T0OutputReturn, // OP_RETURN script
						},
						ValueSat:     *SatZero,
					},
				},
				Blocktime:     1588899730,
				Time:          1588899730,
				Confirmations: 1,
			},
			{
				Version: 131, // asset update coloured coin tx
				Txid: TxidS2T1,
				Vin: []bchain.Vin{
					{
						Txid: TxidS1T1,
						Vout: 0,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS5, parser),
						},
						ValueSat: *SatS2T1A1,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS2T1OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatZero,
					},
				},
				Blocktime:     1588899730,
				Time:          1588899730,
				Confirmations: 1,
			},
		},
	}
}
