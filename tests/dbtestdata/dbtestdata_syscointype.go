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
	TxidS2T1INPUT0 = "004838c94651832d77166eb9806d062566bdcf9981c3ed339b5e5bb50e36949d"
	TxidS2T0 = "5a76290ed05bb4d178acf6e1809f46c41cf3c079c4c0810f9c6be3b1c1a7a2e6"
	TxidS2T1 = "bae2d8c36c6b8975fe888516ab9523c33c688dcb2210a759008a5cfcbe9b7e2f"


	AddrS1 = "sys1qga74kgt8xsaewscec7v66ptqmyrce608en0wpz"
	AddrS2 = "sys1qcghkwl34flz88geahx0q8ehxatdcdh7ff7jep5"
	AddrS3 = "sys1qfh43lppqe9sfclnf43979dkm63fydg67r5vlcu"
	AddrS4 = "SdzKyvhD2Y3xJvGVSfx96NXszq6x9BZX34"
	AddrS5 = "SaTan8om5wtJJbxxBHQkwzZi3uk3zBsoZg"
	AddrS6 = "burn"
	TxidS1T0OutputReturn = "6a24aa21a9ed99a787405970a1eed7a34b37b2ae2bc31dcf5e6df6984d5d3b7047fba18d6493" // auxpow commitment in coinbase
	TxidS1T1OutputReturn = "6a3401e923942601000008001d7b226465736372697074696f6e223a227075626c696376616c7565227d034341541f00001f64648668"
	TxidS2T0OutputReturn = "6a24aa21a9ed278ee076da765fd1d17ffe4b2c2adfb7a7af71e8bf20228148a71fead16872ec" // auxpow commitment in coinbase
	TxidS2T1OutputReturn = "6a4c55e451573e001471ed42a9e6413d5d6f102d31a0bd61201dd82bc20100046275726e00000e28217b3b0100000000000000000000000000000000000000000000000000000000000000000000000000000000ffffffff"
	
)

// Amounts in satoshis
var (
	SatS1T0A1       = big.NewInt(3465000204)
	SatS1T0A2       = big.NewInt(2598753670)
	SatS1T1A1       = big.NewInt(109999796)
	SatS1T1A2       = big.NewInt(449890000000)
	SatS2T0A1       = big.NewInt(866253190)
	SatS2T0A2       = big.NewInt(2598753190)
	SatS2T1A1       = big.NewInt(99958120)
	SatAssetSent	= big.NewInt(88800000000000000)
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
			Height:        347314,
			Hash:          "6609d44688868613991b0cd5ed981a76526caed6b0f7b1be242f5a93311636c6",
			Size:          1611,
			Time:          1580142055,
			Confirmations: 1,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS2T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "03b24c0501020fe4b883e5bda9e7a59ee4bb99e9b1bc205b323032302d30312d32375431363a32303a35352e3035343134373631385a5d",
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
							Hex: AddressToPubKeyHex(AddrS5, parser),
						},
						ValueSat: *SatS2T0A2,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS2T0OutputReturn, // OP_RETURN script
						},
						ValueSat:     *SatZero,
					},
				},
				Blocktime:     1574279564,
				Time:          1574279564,
				Confirmations: 2,
			},
			{
				Version: 29701, // asset send coloured coin tx
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
							Hex: TxidS2T1OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatZero,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS3, parser),
						},
						ValueSat: *SatS2T1A1,
					},
				},
				Blocktime:     1580142055,
				Time:          1580142055,
				Confirmations: 1,
			},
		},
	}
}
