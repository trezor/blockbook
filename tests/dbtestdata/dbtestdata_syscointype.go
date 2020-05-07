package dbtestdata

import (
	"blockbook/bchain"
	"math/big"
)

// Txids, Addresses
const (
	TxidS1T1INPUT0 = "1cdbe8149fc1bd1bed11317f64f50d2904a9851913904d1c4a8379b8d45a9621"
	TxidS1T0 = "7e2a8cc369b600901e7c18d12e261d612d36f4a9859c3097a686b80791d724a4"
	TxidS1T1 = "e2f8c07fc4fc2d74ef160ea0eff9eb78a3331b023249a0c6fdf2ff9b361b7ecd"
	TxidS2T1INPUT0 = "749def6f74a59c3b2b882f3b2bfc7ee979d25e438c610b01a3ebdcb22ab32b3d"
	TxidS2T0 = "a97b946f5fa4888a092403d89ef24320f0725901865bc8ed0ac0046f55d85a90"
	TxidS2T1 = "d7f7a7fc3862e1822d73d63584df319591747eaaa76aaefe88de004ea64b8aef"


	AddrS1 = "sys1qga74kgt8xsaewscec7v66ptqmyrce608en0wpz"
	AddrS2 = "sys1qcghkwl34flz88geahx0q8ehxatdcdh7ff7jep5"
	AddrS3 = "sys1qfh43lppqe9sfclnf43979dkm63fydg67r5vlcu"
	AddrS4 = "sys1q52avkwf3pr8yuyk68kp3e76mnzwwakha573mz7"
	TxidS1T0OutputReturn = "6a24aa21a9ed99a787405970a1eed7a34b37b2ae2bc31dcf5e6df6984d5d3b7047fba18d6493" // auxpow commitment in coinbase
	TxidS1T1OutputReturn = "6a3401de69a52b01000008001d7b226465736372697074696f6e223a227075626c696376616c7565227d034341541f00001f64648668"
	TxidS2T0OutputReturn = "6a24aa21a9ed0c0edd6a580afc0afa7002418762b74967fdd3f801120074857dcbbdbce31cb3" // auxpow commitment in coinbase
	TxidS2T1OutputReturn = "6a4c5a01de69a52b0100000800217b226465736372697074696f6e223a226e65776465736372697074696f6e32227d034341541f00217b226465736372697074696f6e223a226e65776465736372697074696f6e31227d1f3180028668"
	
)

// Amounts in satoshis
var (
	SatS1T0A1       = big.NewInt(3465000204)
	SatS1T0A2       = big.NewInt(2598753670)
	SatS1T1A1       = big.NewInt(109999796)
	SatS1T1A2       = big.NewInt(449890000000)
	SatS2T0A1       = big.NewInt(3465000212)
	SatS2T1A1       = big.NewInt(109998197)
	SatS1T1INPUT0   = big.NewInt(100000000)
	SatS2T1INPUT0   = big.NewInt(99964500)
	SatS1T1OPRETURN = big.NewInt(50000000000)
)

// GetTestSyscoinTypeBlock1 returns block #1
func GetTestSyscoinTypeBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        158,
			Hash:          "000004138eaa5e65a84b9b7f48fb9f9b1a8aadf27248974cabb3a23f7f20458a",
			Size:          536,
			Time:          1588788257,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS1T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "029e000101",
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
				Blocktime:     1588788257,
				Time:          1588788257,
				Confirmations: 2,
			},
			{
				Version: 130, // asset activate coloured coin tx
				Txid: TxidS1T1,
				Vin: []bchain.Vin{
					{
						Txid: TxidS1T1INPUT0,
						Vout: 1,
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
				Blocktime:     1588788257,
				Time:          1588788257,
				Confirmations: 2,
			},
		},
	}
}

// GetTestSyscoinTypeBlock2 returns block #2
func GetTestSyscoinTypeBlock2(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        165,
			Hash:          "00000de793885472131c2bea4d252281a2c8194fc43453c1ab427a45f968313f",
			Size:          544,
			Time:          1588824028,
			Confirmations: 1,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS2T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "02a5000101",
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS4, parser),
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
				Blocktime:     1588824028,
				Time:          1588824028,
				Confirmations: 2,
			},
			{
				Version: 131, // asset update coloured coin tx
				Txid: TxidS2T1,
				Vin: []bchain.Vin{
					{
						Txid: TxidS2T1INPUT0,
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS3, parser),
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
				Blocktime:     1588824028,
				Time:          1588824028,
				Confirmations: 1,
			},
		},
	}
}
