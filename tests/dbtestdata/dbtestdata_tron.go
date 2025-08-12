package dbtestdata

import (
	"github.com/trezor/blockbook/bchain"
)

// Addresses
const (
	TronAddrZero        = "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb"
	TronAddrTZ          = "TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt" // 0xff324071970b2b08822caa310c1bb458e63a5033
	TronAddrTD          = "TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD" // 0x242aa579f130bf6fea5eac12aa6b846fb8b293ab
	TronAddrContractTX1 = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf" // 0xeca9bc828a3005b9a3b909f2cc5c2a54794de05f
	TronAddrContractTR  = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t" // TRC20 (USDT)
	TronAddrContractTV  = "TVj7RNVHy6thbM7BWdSe9G6gXwKhjhdNZS" // TRC20 (KLV)
	TronAddrContractTU  = "TU2MJ5Veik1LRAgjeSzEdvmDYx7mefJZvd" // non TRC20
	TronAddrContractTA  = "TQEepeTijBFcWjnwF7N6THWEYpxJjpwqdd" // TRC721
	TronAddrContractTX2 = "TXWLT4N9vDcmNHDnSuKv2odhBtizYuEMKJ" // TRC1155
)

// Blocks
const (
	Block0 = 99999
	Block1 = 100000
)

const (
	// TRC 20
	// TronAddrTZ -> TronAddrContractTX1
	// TronAddrTZ -> TronAddrTD, value 11231310
	TronTx1Id     = "0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"
	TronTx1Packed = "08a7a5a31a1a9a011201d218ba722a44a9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e3220a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b3023a14eca9bc828a3005b9a3b909f2cc5c2a54794de05f4214ff324071970b2b08822caa310c1bb458e63a503322a8010a02393a1201011a9e010a14eca9bc828a3005b9a3b909f2cc5c2a54794de05f12200000000000000000000000000000000000000000000000000000000000ab604e1a20ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef1a20000000000000000000000000ff324071970b2b08822caa310c1bb458e63a50331a20000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab"
)

var TronBlock1SpecificData = &bchain.EthereumBlockSpecificData{
	Contracts: []bchain.ContractInfo{
		{
			Contract:       TronAddrContractTR,
			Type:           TRC20TokenType,
			Name:           "USD Token",
			Symbol:         "USDT",
			Decimals:       12,
			CreatedInBlock: Block0,
		},
	},
}

func GetTestTronBlock0(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        Block0,
			Hash:          "0x0000000000000000000000000000000000000000000000000000000000000000",
			Time:          1694226700,
			Confirmations: 2,
		},
		Txs: []bchain.Tx{},
	}
}

func GetTestTronBlock1(parser bchain.BlockChainParser) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height:        Block1,
			Hash:          "0x11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
			Size:          12345,
			Time:          1677700000,
			Confirmations: 99,
		},
		Txs: unpackTxs([]packedAndInternal{{
			packed: TronTx1Packed,
		}}, parser),
		CoinSpecificData: TronBlock1SpecificData,
	}
}
