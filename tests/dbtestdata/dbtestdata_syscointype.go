package dbtestdata

import (
	"blockbook/bchain"
	"math/big"
)

// Txids, Addresses
const (
	TxidS1T1INPUT0 = "a41fd0ffa372b62e0735fa64e8e57e4ef42f414c2d494fd4b3d0be587533dd10"
	TxidS1T0 = "badaa3550f9d1a5336cc7c6f4c236a9ef4099389247341759e83580a9785dea3"
	TxidS1T1 = "0813f4bb8684b3dc8065a097e8e980de9b22c575bcba710635e997ba2d20eb2d"
	TxidS2T1INPUT0 = "004838c94651832d77166eb9806d062566bdcf9981c3ed339b5e5bb50e36949d"
	TxidS2T0 = "5a76290ed05bb4d178acf6e1809f46c41cf3c079c4c0810f9c6be3b1c1a7a2e6"
	TxidS2T1 = "bae2d8c36c6b8975fe888516ab9523c33c688dcb2210a759008a5cfcbe9b7e2f"


	AddrS1 = "SgzDCepk4G9xyme2i1G1bVtSkb6VxQ5UaJ"
	AddrS2 = "SgBVZhGLjqRz8ufXFwLhZvXpUMKqoduNG8"  
	AddrS3 = "sys1qw8k5920xgy746mcs95c6p0tpyqwas27z8af9qe"
	AddrS4 = "SdzKyvhD2Y3xJvGVSfx96NXszq6x9BZX34"
	AddrS5 = "SaTan8om5wtJJbxxBHQkwzZi3uk3zBsoZg"
	AddrS6 = "burn"
	TxidS1T0OutputReturn = "6a24aa21a9ed84a6a51555ea291dd3ac3603c575bba8da6a26b7efc8b7c279ee457369df369c" // auxpow commitment in coinbase
	TxidS1T1OutputReturn = "6a4c85237b226465736372697074696f6e223a224f6666696369616c205359535820535054227d0000000000000000000000000000000000000000000000000000000000000000e451573e0453595358001471ed42a9e6413d5d6f102d31a0bd61201dd82bc2000000000e28217b3b0100000e28217b3b0100000e28217b3b01000000001f080000"
	TxidS2T0OutputReturn = "6a24aa21a9ed278ee076da765fd1d17ffe4b2c2adfb7a7af71e8bf20228148a71fead16872ec" // auxpow commitment in coinbase
	TxidS2T1OutputReturn = "6a4c55e451573e001471ed42a9e6413d5d6f102d31a0bd61201dd82bc20100046275726e00000e28217b3b0100000000000000000000000000000000000000000000000000000000000000000000000000000000ffffffff"
	
)

// Amounts in satoshis
var (
	SatS1T0A1       = big.NewInt(866253670)
	SatS1T0A2       = big.NewInt(2598753670)
	SatS1T1A1       = big.NewInt(9999266)
	SatS2T0A1       = big.NewInt(866253190)
	SatS2T0A2       = big.NewInt(2598753190)
	SatS2T1A1       = big.NewInt(99958120)
	SatAssetSent	= big.NewInt(88800000000000000)
	SatS1T1INPUT0   = big.NewInt(100000000)
	SatS2T1INPUT0   = big.NewInt(99964500)
)

// GetTestSyscoinTypeBlock1 returns block #1
func GetTestSyscoinTypeBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        249727,
			Hash:          "78ae6476a514897c8a6984032e5d0e4a44424055f0c2d7b5cf664ae8c8c20487",
			Size:          1551,
			Time:          1574279564,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS1T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "037fcf030101",
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
							Hex: AddressToPubKeyHex(AddrS2, parser),
						},
						ValueSat: *SatS1T0A2,
					},
					{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: TxidS1T0OutputReturn, // OP_RETURN script
						},
						ValueSat:     *SatZero,
					},
				},
				Blocktime:     1574279564,
				Time:          1574279564,
				Confirmations: 2,
			},
			{
				Version: 29698, // asset activate coloured coin tx
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
							Hex: TxidS1T1OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatZero,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS3, parser),
						},
						ValueSat: *SatS1T1A1,
					},
				},
				Blocktime:     1574279564,
				Time:          1574279564,
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
