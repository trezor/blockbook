package dbtestdata

import (
	"blockbook/bchain"
	"math/big"
)

// Txids, Addresses
const (
	TxidS1T1INPUT0 = "b61045108705d2a65774238174bfa9110ccaad43a98a9b289a79da0696cac0b8"
	TxidS1T0 = "8d86636db959a190aed4e65b4ee7e67b6ee0189e03acc27e353e69b88288cacc"
	TxidS1T1 = "a7f5c979d8fc80f05d8434e04cb9e46cdaa56551d23dd790ba5d7f2c15f529fd"
	TxidS2T0 = "5bb051670143eeb1d0cfc3c992ab18e1bd4bb0c78d8914dc54feaee9a894174b"
	TxidS2T1 = "90652f37eeb24374d8cfef5b73ac4c10e31fb54ac864e0d9f8250af76985eb9d"


	AddrS1 = "tsys1q4hg3e2lcyx87muctu26dvmnuz7lpm3lpvcaeyu"
	AddrS2 = "tsys1qq43tjdd753rct3jj39yvr855gytwf3y8p5kuf9"
	AddrS3 = "tsys1qc8wz57zmyjjwtc4q8d8nc3appj5fcwjvd9uj4e"
	AddrS4 = "tsys1qt8aq6hrrlc6ueps4wqc6ynfckrxxrw20ydamc9"
	TxidS1T0OutputReturn = "6a24aa21a9ed38a14bc74124f5735be84026b4462b8bbb0f567291a6861ff7e4c88c6bff03cd" // auxpow commitment in coinbase
	TxidS1T1OutputReturn = "6a3301b8c0ca9601010000080451304655851b7b2264657363223a226348566962476c6a646d46736457553d227d0064008668ff00"
	TxidS2T0OutputReturn = "6a24aa21a9ed68662a3517e59c63e980d2ce5da7b082a34b12edf18ba0860bd8c65564c99923" // auxpow commitment in coinbase
	TxidS2T1OutputReturn = "6a4c6401b8c0ca9601010000080087142b1e58b979e4b2d72d8bca5bb4646ccc032ddbfc001f7b2264657363223a22626d563349484231596d787059335a686248566c227d1b7b2264657363223a226348566962476c6a646d46736457553d227d822400007fff"
	
)

// Amounts in satoshis
var (
	SatS1T0A1       = big.NewInt(3465003450)
	SatS1T1A1       = big.NewInt(84999996550)
	SatS2T0A1       = big.NewInt(3465003950)
	SatS2T1A1       = big.NewInt(84999992600)
	SatS1T1OPRETURN = big.NewInt(15000000000)
)

// GetTestSyscoinTypeBlock1 returns block #1
func GetTestSyscoinTypeBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        112,
			Hash:          "00000797cfd9074de37a557bf0d47bd86c45846f31e163ba688e14dfc498527a",
			Size:          503,
			Time:          1598556954,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS1T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "01700101",
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
				Blocktime:     1598556954,
				Time:          1598556954,
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
							Hex: TxidS1T1OutputReturn, // OP_RETURN script
						},
						ValueSat: *SatS1T1OPRETURN,
					},
					{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS2, parser),
						},
						ValueSat: *SatS1T1A1,
					},
				},
				Blocktime:     1598556954,
				Time:          1598556954,
				Confirmations: 2,
			},
		},
	}
}

// GetTestSyscoinTypeBlock2 returns block #2
func GetTestSyscoinTypeBlock2(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        113,
			Hash:          "00000cade5f8d530b3f0a3b6c9dceaca50627838f2c6fffb807390cba71974e7",
			Size:          554,
			Time:          1598557012,
			Confirmations: 1,
		},
		Txs: []bchain.Tx{
			// mining transaction
			{
				Txid: TxidS2T0,
				Vin: []bchain.Vin{
					{
						Coinbase: "01710101",
					},
				},
				Vout: []bchain.Vout{
					{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: AddressToPubKeyHex(AddrS3, parser),
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
				Blocktime:     1598557012,
				Time:          1598557012,
				Confirmations: 1,
			},
			{
				Version: 131, // asset update coloured coin tx
				Txid: TxidS2T1,
				Vin: []bchain.Vin{
					{
						Txid: TxidS1T1,
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
							Hex: AddressToPubKeyHex(AddrS4, parser),
						},
						ValueSat: *SatS2T1A1,
					},
				},
				Blocktime:     1598557012,
				Time:          1598557012,
				Confirmations: 1,
			},
		},
	}
}
