//go:build unittest

package db

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

type testEthereumParser struct {
	*eth.EthereumParser
}

func ethereumTestnetParser() *eth.EthereumParser {
	return eth.NewEthereumParser(1, true)
}

func bigintFromStringToHex(s string) string {
	var b big.Int
	b.SetString(s, 0)
	return bigintToHex(&b)
}

func verifyAfterEthereumTypeBlock1(t *testing.T, d *RocksDB, afterDisconnect bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"0041eee8",
			"c7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" + uintToHex(1534858022) + varuintToHex(2) + varuintToHex(31839),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.EthAddr3e, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^1, 1}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{2}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr20, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^0, ^2}), nil},
		{addressKeyHex(dbtestdata.EthAddr9f, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^1, 1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract4a, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{0, 1}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser), "020102", nil},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser),
			"020100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("10000000000000000000000"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser),
			"010100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintToHex(big.NewInt(0)), nil,
		},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser), "010002", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "010101", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	var destructedInBlock uint
	if afterDisconnect {
		destructedInBlock = 44445
	}
	if err := checkColumn(d, cfContracts, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser),
			"0b436f6e7472616374203734" + // Contract 74
				"03533734" + // S74
				"054552433230" + // ERC20
				varuintToHex(12) + varuintToHex(44444) + varuintToHex(destructedInBlock),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfInternalData, []keyPair{
		{
			dbtestdata.EthTxidB1T2,
			"06" +
				"01" + dbtestdata.EthAddr9f + dbtestdata.EthAddrContract4a + "030f4240" +
				"00" + dbtestdata.EthAddr3e + dbtestdata.EthAddr9f + "030f4241" +
				"00" + dbtestdata.EthAddr3e + dbtestdata.EthAddr3e + "030f4242",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	var blockTxsKp []keyPair
	if afterDisconnect {
		blockTxsKp = []keyPair{}
	} else {
		blockTxsKp = []keyPair{
			{
				"0041eee8",
				dbtestdata.EthTxidB1T1 +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + "00" +
					dbtestdata.EthTxidB1T2 +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
					"01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("10000000000000000000000"),
				nil,
			},
		}
	}
	if err := checkColumn(d, cfBlockTxs, blockTxsKp); err != nil {
		{
			t.Fatal(err)
		}
	}
}

func verifyAfterEthereumTypeBlock2(t *testing.T, d *RocksDB, wantBlockInternalDataError bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"0041eee8",
			"c7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" + uintToHex(1534858022) + varuintToHex(2) + varuintToHex(31839),
			nil,
		},
		{
			"0041eee9",
			"2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee" + uintToHex(1534859988) + varuintToHex(6) + varuintToHex(2345678),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.EthAddr3e, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^1, 1}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{2}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr20, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^0, ^2}), nil},
		{addressKeyHex(dbtestdata.EthAddr9f, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^1, 1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract4a, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{0, 1}), nil},

		{addressKeyHex(dbtestdata.EthAddrZero, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T5, []int32{transferFrom}), nil},
		{addressKeyHex(dbtestdata.EthAddr3e, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T4, []int32{^0, 2}), nil},
		{addressKeyHex(dbtestdata.EthAddr4b, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^0, ^1, 2, ^3, 3, ^2}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T6, []int32{0, ^0, 4, ^4}) + txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^3, 2}) + txIndexesHex(dbtestdata.EthTxidB2T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr5d, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T5, []int32{^0, 2}), nil},
		{addressKeyHex(dbtestdata.EthAddr7b, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T3, []int32{4}) + txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^2, 3}), nil},
		{addressKeyHex(dbtestdata.EthAddr83, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T3, []int32{^0, ^2}), nil},
		{addressKeyHex(dbtestdata.EthAddr92, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T4, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr9f, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{1}) + txIndexesHex(dbtestdata.EthTxidB2T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddrA3, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T4, []int32{^2}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract0d, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract47, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract4a, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract6f, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T5, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddrContractCd, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T3, []int32{0}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser),
			"010100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintToHex(big.NewInt(0)), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser),
			"030202" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) + varuintToHex(1<<2+uint(bchain.MultiToken)) + varuintToHex(1) + bigintFromStringToHex("150") + bigintFromStringToHex("1"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser),
			"010101" +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + varuintToHex(2<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("8086") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(2<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("871180000950184"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser),
			"050300" +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(2<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("10000000854307892726464") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("0") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("0"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr5d, d.chainParser),
			"010100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) + varuintToHex(1<<2+uint(bchain.MultiToken)) + varuintToHex(2) + bigintFromStringToHex("1776") + bigintFromStringToHex("1") + bigintFromStringToHex("1898") + bigintFromStringToHex("10"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser),
			"020000" +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("0") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + varuintToHex(1<<2+uint(bchain.FungibleToken)) + bigintFromStringToHex("7674999999999991915") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContractCd, d.chainParser) + varuintToHex(1<<2+uint(bchain.NonFungibleToken)) + varuintToHex(1) + bigintFromStringToHex("1"), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr83, d.chainParser),
			"010100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContractCd, d.chainParser) + varuintToHex(1<<2+uint(bchain.NonFungibleToken)) + varuintToHex(0), nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrA3, d.chainParser),
			"010000" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) + varuintToHex(1<<2+uint(bchain.MultiToken)) + varuintToHex(0), nil,
		},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr92, d.chainParser), "010100", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser), "030104", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser), "010001", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser), "010100", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "020102", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser), "010100", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContractCd, d.chainParser), "010100", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfContracts, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser),
			"0b436f6e7472616374203734" + // Contract 74
				"03533734" + // S74
				"054552433230" + // ERC20
				varuintToHex(12) + varuintToHex(44444) + varuintToHex(44445),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfInternalData, []keyPair{
		{
			dbtestdata.EthTxidB1T2,
			"06" +
				"01" + dbtestdata.EthAddr9f + dbtestdata.EthAddrContract4a + "030f4240" +
				"00" + dbtestdata.EthAddr3e + dbtestdata.EthAddr9f + "030f4241" +
				"00" + dbtestdata.EthAddr3e + dbtestdata.EthAddr3e + "030f4242",
			nil,
		},
		{
			dbtestdata.EthTxidB2T1,
			"00" + hex.EncodeToString([]byte(dbtestdata.EthTx3InternalData.Error)),
			nil,
		},
		{
			dbtestdata.EthTxidB2T2,
			"05" + dbtestdata.EthAddrContract0d +
				"00" + dbtestdata.EthAddr4b + dbtestdata.EthAddr9f + "030f424a" +
				"02" + dbtestdata.EthAddrContract4a + dbtestdata.EthAddr9f + "030f424b",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"0041eee9",
			dbtestdata.EthTxidB2T1 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser) + "00" +
				dbtestdata.EthTxidB2T2 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser) +
				"04" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("7675000000000000001") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("854307892726464") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("871180000950184") +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("7674999999999991915") +
				dbtestdata.EthTxidB2T3 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr83, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContractCd, d.chainParser) +
				"01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr83, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContractCd, d.chainParser) + varuintToHex(uint(bchain.NonFungibleToken)) + bigintFromStringToHex("1") +
				dbtestdata.EthTxidB2T4 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr92, d.chainParser) +
				"01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrA3, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) + varuintToHex(uint(bchain.MultiToken)) + "01" + bigintFromStringToHex("150") + bigintFromStringToHex("1") +
				dbtestdata.EthTxidB2T5 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr5d, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) +
				"01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrZero, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr5d, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract6f, d.chainParser) + varuintToHex(uint(bchain.MultiToken)) + "02" + bigintFromStringToHex("1776") + bigintFromStringToHex("1") + bigintFromStringToHex("1898") + bigintFromStringToHex("10") +
				dbtestdata.EthTxidB2T6 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) +
				"01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + varuintToHex(uint(bchain.FungibleToken)) + bigintFromStringToHex("10000000000000000000000"),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	var addressAliases []keyPair
	addressAliases = []keyPair{
		{
			hex.EncodeToString([]byte(dbtestdata.EthAddr7bEIP55)),
			hex.EncodeToString([]byte("address7b")),
			nil,
		},
		{
			hex.EncodeToString([]byte(dbtestdata.EthAddr20EIP55)),
			hex.EncodeToString([]byte("address20")),
			nil,
		},
	}
	if err := checkColumn(d, cfAddressAliases, addressAliases); err != nil {
		{
			t.Fatal(err)
		}
	}

	var internalDataError []keyPair
	if wantBlockInternalDataError {
		internalDataError = []keyPair{
			{
				"0041eee9",
				"2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee" + "00" + hex.EncodeToString([]byte("test error")),
				nil,
			},
		}
	}
	if err := checkColumn(d, cfBlockInternalDataErrors, internalDataError); err != nil {
		{
			t.Fatal(err)
		}
	}

}

func formatInternalData(in *bchain.EthereumInternalData) *bchain.EthereumInternalData {
	out := *in
	if out.Type == bchain.CREATE {
		out.Contract = eth.EIP55AddressFromAddress(out.Contract)
	}
	for i := range out.Transfers {
		t := &out.Transfers[i]
		t.From = eth.EIP55AddressFromAddress(t.From)
		t.To = eth.EIP55AddressFromAddress(t.To)
	}
	out.Error = eth.UnpackInternalTransactionError([]byte(in.Error))
	return &out
}

func testFourByteSignature(t *testing.T, d *RocksDB) {
	fourBytes := uint32(1234123)
	id := uint32(42313)
	signature := bchain.FourByteSignature{
		Name:       "xyz",
		Parameters: []string{"address", "(bytes,uint256[],uint256)", "uint16"},
	}
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.StoreFourByteSignature(wb, fourBytes, id, &signature); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetFourByteSignature(fourBytes, id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*got, signature) {
		t.Errorf("testFourByteSignature: got %+v, want %+v", got, signature)
	}
	gotSlice, err := d.GetFourByteSignatures(fourBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*gotSlice, []bchain.FourByteSignature{signature}) {
		t.Errorf("testFourByteSignature: got %+v, want %+v", *gotSlice, []bchain.FourByteSignature{signature})
	}
}

// TestRocksDB_Index_EthereumType is an integration test probing the whole indexing functionality for EthereumType chains
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for various addresses / low-high ranges
// 3) GetBestBlock, GetBlockHash
// 4) Test tx caching functionality
// 5) Disconnect the block 2 using BlockTxs column
// 6) Reconnect block 2 and check
// After each step, the content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_EthereumType(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	if len(d.is.BlockTimes) != 0 {
		t.Fatal("Expecting is.BlockTimes 0, got ", len(d.is.BlockTimes))
	}

	// connect 1st block
	block1 := dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock1(t, d, false)

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// connect 2nd block, simulate InternalDataError and AddressAlias
	block2 := dbtestdata.GetTestEthereumTypeBlock2(d.chainParser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock2(t, d, true)
	block2.CoinSpecificData = nil

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

	// get transactions for various addresses / low-high ranges
	verifyGetTransactions(t, d, "0x"+dbtestdata.EthAddr55, 0, 10000000, []txidIndex{
		{"0x" + dbtestdata.EthTxidB2T6, 0},
		{"0x" + dbtestdata.EthTxidB2T6, ^0},
		{"0x" + dbtestdata.EthTxidB2T6, 4},
		{"0x" + dbtestdata.EthTxidB2T6, ^4},
		{"0x" + dbtestdata.EthTxidB2T2, ^3},
		{"0x" + dbtestdata.EthTxidB2T2, 2},
		{"0x" + dbtestdata.EthTxidB2T1, ^0},
		{"0x" + dbtestdata.EthTxidB1T2, 2},
		{"0x" + dbtestdata.EthTxidB1T1, 0},
	}, nil)
	verifyGetTransactions(t, d, "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eBad", 500000, 1000000, []txidIndex{}, errors.New("Address missing"))

	id, err := d.GetEthereumInternalData(dbtestdata.EthTxidB1T1)
	if err != nil || id != nil {
		t.Errorf("GetEthereumInternalData(%s) = %+v, want %+v, err %v", dbtestdata.EthTxidB1T1, id, nil, err)
	}
	id, err = d.GetEthereumInternalData(dbtestdata.EthTxidB1T2)
	if err != nil || !reflect.DeepEqual(id, formatInternalData(dbtestdata.EthTx2InternalData)) {
		t.Errorf("GetEthereumInternalData(%s) = %+v, want %+v, err %v", dbtestdata.EthTxidB1T2, id, formatInternalData(dbtestdata.EthTx2InternalData), err)
	}
	id, err = d.GetEthereumInternalData(dbtestdata.EthTxidB2T1)
	if err != nil || !reflect.DeepEqual(id, formatInternalData(dbtestdata.EthTx3InternalData)) {
		t.Errorf("GetEthereumInternalData(%s) = %+v, want %+v, err %v", dbtestdata.EthTxidB2T1, id, formatInternalData(dbtestdata.EthTx3InternalData), err)
	}
	id, err = d.GetEthereumInternalData(dbtestdata.EthTxidB2T2)
	if err != nil || !reflect.DeepEqual(id, formatInternalData(dbtestdata.EthTx4InternalData)) {
		t.Errorf("GetEthereumInternalData(%s) = %+v, want %+v, err %v", dbtestdata.EthTxidB2T2, id, formatInternalData(dbtestdata.EthTx4InternalData), err)
	}

	// GetBestBlock
	height, hash, err := d.GetBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	if height != 4321001 {
		t.Fatalf("GetBestBlock: got height %v, expected %v", height, 4321001)
	}
	if hash != "0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee" {
		t.Fatalf("GetBestBlock: got hash %v, expected %v", hash, "0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee")
	}

	// GetBlockHash
	hash, err = d.GetBlockHash(4321000)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "0xc7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" {
		t.Fatalf("GetBlockHash: got hash %v, expected %v", hash, "0xc7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11")
	}

	// Not connected block
	hash, err = d.GetBlockHash(4321002)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("GetBlockHash: got hash '%v', expected ''", hash)
	}

	// GetBlockHash
	info, err := d.GetBlockInfo(4321001)
	if err != nil {
		t.Fatal(err)
	}
	iw := &BlockInfo{
		Hash:   "0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee",
		Txs:    6,
		Size:   2345678,
		Time:   1534859988,
		Height: 4321001,
	}
	if !reflect.DeepEqual(info, iw) {
		t.Errorf("GetBlockInfo() = %+v, want %+v", info, iw)
	}

	// Test to store and get FourByteSignature
	testFourByteSignature(t, d)

	// Test tx caching functionality, leave one tx in db to test cleanup in DisconnectBlock
	testTxCache(t, d, block1, &block1.Txs[0])
	// InternalData are not packed and stored in DB, remove them so that the test does not fail
	esd, _ := block2.Txs[0].CoinSpecificData.(bchain.EthereumSpecificData)
	eid := esd.InternalData
	esd.InternalData = nil
	block2.Txs[0].CoinSpecificData = esd
	testTxCache(t, d, block2, &block2.Txs[0])
	// restore InternalData
	esd.InternalData = eid
	block2.Txs[0].CoinSpecificData = esd
	if err = d.PutTx(&block2.Txs[1], block2.Height, block2.Txs[1].Blocktime); err != nil {
		t.Fatal(err)
	}
	if err = d.PutTx(&block2.Txs[1], block2.Height, block2.Txs[1].Blocktime); err != nil {
		t.Fatal(err)
	}
	// check that there is only the last tx in the cache
	packedTx, err := d.chainParser.PackTx(&block2.Txs[1], block2.Height, block2.Txs[1].Blocktime)
	if err := checkColumn(d, cfTransactions, []keyPair{
		{dbtestdata.EthTxidB2T2, hex.EncodeToString(packedTx), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// try to disconnect both blocks, however only the last one is kept, it is not possible
	err = d.DisconnectBlockRangeEthereumType(4321000, 4321001)
	if err == nil || err.Error() != "Cannot disconnect blocks with height 4321000 and lower. It is necessary to rebuild index." {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock2(t, d, true)

	// disconnect the 2nd block, verify that the db contains only data from the 1st block with restored unspentTxs
	// and that the cached tx is removed
	err = d.DisconnectBlockRangeEthereumType(4321001, 4321001)
	if err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock1(t, d, true)
	if err := checkColumn(d, cfTransactions, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// connect block again and verify the state of db
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock2(t, d, false)

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

}

func Test_BulkConnect_EthereumType(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	bc, err := d.InitBulkConnect()
	if err != nil {
		t.Fatal(err)
	}

	if d.is.DbState != common.DbStateInconsistent {
		t.Fatal("DB not in DbStateInconsistent")
	}

	if len(d.is.BlockTimes) != 0 {
		t.Fatal("Expecting is.BlockTimes 0, got ", len(d.is.BlockTimes))
	}

	if err := bc.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(d.chainParser), false); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	// connect 2nd block, simulate InternalDataError
	block2 := dbtestdata.GetTestEthereumTypeBlock2(d.chainParser)
	if err := bc.ConnectBlock(block2, true); err != nil {
		t.Fatal(err)
	}
	block2.CoinSpecificData = nil

	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}

	if d.is.DbState != common.DbStateOpen {
		t.Fatal("DB not in DbStateOpen")
	}

	verifyAfterEthereumTypeBlock2(t, d, true)

	if len(d.is.BlockTimes) != 4321002 {
		t.Fatal("Expecting is.BlockTimes 4321002, got ", len(d.is.BlockTimes))
	}
}

func Test_packUnpackEthInternalData(t *testing.T) {
	parser := ethereumTestnetParser()
	db := &RocksDB{chainParser: parser}
	tests := []struct {
		name string
		data ethInternalData
		want *bchain.EthereumInternalData
	}{
		{
			name: "CALL 1",
			data: ethInternalData{
				internalType: bchain.CALL,
				transfers: []ethInternalTransfer{
					{
						internalType: bchain.CALL,
						from:         addressToAddrDesc(dbtestdata.EthAddr3e, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr20, parser),
						value:        *big.NewInt(412342134),
					},
				},
			},
			want: &bchain.EthereumInternalData{
				Type: bchain.CALL,
				Transfers: []bchain.EthereumInternalTransfer{
					{
						Type:  bchain.CALL,
						From:  eth.EIP55AddressFromAddress(dbtestdata.EthAddr3e),
						To:    eth.EIP55AddressFromAddress(dbtestdata.EthAddr20),
						Value: *big.NewInt(412342134),
					},
				},
			},
		},
		{
			name: "CALL 2",
			data: ethInternalData{
				internalType: bchain.CALL,
				errorMsg:     "error error error",
				transfers: []ethInternalTransfer{
					{
						internalType: bchain.CALL,
						from:         addressToAddrDesc(dbtestdata.EthAddr3e, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr20, parser),
						value:        *big.NewInt(4123421341),
					},
					{
						internalType: bchain.CREATE,
						from:         addressToAddrDesc(dbtestdata.EthAddr4b, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr55, parser),
						value:        *big.NewInt(123),
					},
					{
						internalType: bchain.SELFDESTRUCT,
						from:         addressToAddrDesc(dbtestdata.EthAddr7b, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr83, parser),
						value:        *big.NewInt(67890),
					},
				},
			},
			want: &bchain.EthereumInternalData{
				Type:  bchain.CALL,
				Error: "error error error",
				Transfers: []bchain.EthereumInternalTransfer{
					{
						Type:  bchain.CALL,
						From:  eth.EIP55AddressFromAddress(dbtestdata.EthAddr3e),
						To:    eth.EIP55AddressFromAddress(dbtestdata.EthAddr20),
						Value: *big.NewInt(4123421341),
					},
					{
						Type:  bchain.CREATE,
						From:  eth.EIP55AddressFromAddress(dbtestdata.EthAddr4b),
						To:    eth.EIP55AddressFromAddress(dbtestdata.EthAddr55),
						Value: *big.NewInt(123),
					},
					{
						Type:  bchain.SELFDESTRUCT,
						From:  eth.EIP55AddressFromAddress(dbtestdata.EthAddr7b),
						To:    eth.EIP55AddressFromAddress(dbtestdata.EthAddr83),
						Value: *big.NewInt(67890),
					},
				},
			},
		},
		{
			name: "CREATE",
			data: ethInternalData{
				internalType: bchain.CREATE,
				contract:     addressToAddrDesc(dbtestdata.EthAddrContract0d, parser),
			},
			want: &bchain.EthereumInternalData{
				Type:      bchain.CREATE,
				Contract:  eth.EIP55AddressFromAddress(dbtestdata.EthAddrContract0d),
				Transfers: []bchain.EthereumInternalTransfer{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := packEthInternalData(&tt.data)
			got, err := db.unpackEthInternalData(packed)
			if err != nil {
				t.Errorf("unpackEthInternalData() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("packEthInternalData/unpackEthInternalData = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_packUnpackAddrContracts(t *testing.T) {
	parser := ethereumTestnetParser()
	type args struct {
		buf      []byte
		addrDesc bchain.AddressDescriptor
	}
	tests := []struct {
		name string
		data AddrContracts
	}{
		{
			name: "1",
			data: AddrContracts{
				TotalTxs:       30,
				NonContractTxs: 20,
				InternalTxs:    10,
				Contracts:      []AddrContract{},
			},
		},
		{
			name: "2",
			data: AddrContracts{
				TotalTxs:       12345,
				NonContractTxs: 444,
				InternalTxs:    8873,
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract0d, parser),
						Txs:      8,
						Value:    *big.NewInt(793201132),
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Txs:      41235,
						Ids: Ids{
							*big.NewInt(1),
							*big.NewInt(2),
							*big.NewInt(3),
							*big.NewInt(3144223412344123),
							*big.NewInt(5),
						},
					},
					{
						Type:     bchain.MultiToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract4a, parser),
						Txs:      64,
						MultiTokenValues: MultiTokenValues{
							{
								Id:    *big.NewInt(1),
								Value: *big.NewInt(1412341234),
							},
							{
								Id:    *big.NewInt(123412341234),
								Value: *big.NewInt(3),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := packAddrContracts(&tt.data)
			got, err := unpackAddrContracts(packed, nil)
			if err != nil {
				t.Errorf("unpackAddrContracts() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, &tt.data) {
				t.Errorf("unpackAddrContracts() = %v, want %v", got, tt.data)
			}
		})
	}
}

func Test_addToContracts(t *testing.T) {
	// the test builds addToContracts that keeps contracts of an address
	// the test adds and removes values from addToContracts, therefore the order of tests is important
	addrContracts := &AddrContracts{}
	parser := ethereumTestnetParser()
	type args struct {
		index      int32
		contract   bchain.AddressDescriptor
		transfer   *bchain.TokenTransfer
		addTxCount bool
	}
	tests := []struct {
		name              string
		args              args
		wantIndex         int32
		wantAddrContracts *AddrContracts
	}{
		{
			name: "ERC20 to",
			args: args{
				index:    1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
				transfer: &bchain.TokenTransfer{
					Type:  bchain.FungibleToken,
					Value: *big.NewInt(123456),
				},
				addTxCount: true,
			},
			wantIndex: 0 + ContractIndexOffset, // the first contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Txs:      1,
						Value:    *big.NewInt(123456),
					},
				},
			},
		},
		{
			name: "ERC20 from",
			args: args{
				index:    ^1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
				transfer: &bchain.TokenTransfer{
					Type:  bchain.FungibleToken,
					Value: *big.NewInt(23456),
				},
				addTxCount: true,
			},
			wantIndex: ^(0 + ContractIndexOffset), // the first contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
				},
			},
		},
		{
			name: "ERC721 to id 1",
			args: args{
				index:    1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
				transfer: &bchain.TokenTransfer{
					Type:  bchain.NonFungibleToken,
					Value: *big.NewInt(1),
				},
				addTxCount: true,
			},
			wantIndex: 1 + ContractIndexOffset, // the 2nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      1,
						Ids:      Ids{*big.NewInt(1)},
					},
				},
			},
		},
		{
			name: "ERC721 to id 2",
			args: args{
				index:    1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
				transfer: &bchain.TokenTransfer{
					Type:  bchain.NonFungibleToken,
					Value: *big.NewInt(2),
				},
				addTxCount: true,
			},
			wantIndex: 1 + ContractIndexOffset, // the 2nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      2,
						Ids:      Ids{*big.NewInt(1), *big.NewInt(2)},
					},
				},
			},
		},
		{
			name: "ERC721 from id 1, addTxCount=false",
			args: args{
				index:    ^1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
				transfer: &bchain.TokenTransfer{
					Type:  bchain.NonFungibleToken,
					Value: *big.NewInt(1),
				},
				addTxCount: false,
			},
			wantIndex: ^(1 + ContractIndexOffset), // the 2nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      2,
						Ids:      Ids{*big.NewInt(2)},
					},
				},
			},
		},
		{
			name: "ERC1155 to id 11, value 56789",
			args: args{
				index:    1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
				transfer: &bchain.TokenTransfer{
					Type: bchain.MultiToken,
					MultiTokenValues: []bchain.MultiTokenValue{
						{
							Id:    *big.NewInt(11),
							Value: *big.NewInt(56789),
						},
					},
				},
				addTxCount: true,
			},
			wantIndex: 2 + ContractIndexOffset, // the 3nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      2,
						Ids:      Ids{*big.NewInt(2)},
					},
					{
						Type:     bchain.MultiToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
						Txs:      1,
						MultiTokenValues: MultiTokenValues{
							{
								Id:    *big.NewInt(11),
								Value: *big.NewInt(56789),
							},
						},
					},
				},
			},
		},
		{
			name: "ERC1155 to id 11, value 111 and id 22, value 222",
			args: args{
				index:    1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
				transfer: &bchain.TokenTransfer{
					Type: bchain.MultiToken,
					MultiTokenValues: []bchain.MultiTokenValue{
						{
							Id:    *big.NewInt(11),
							Value: *big.NewInt(111),
						},
						{
							Id:    *big.NewInt(22),
							Value: *big.NewInt(222),
						},
					},
				},
				addTxCount: true,
			},
			wantIndex: 2 + ContractIndexOffset, // the 3nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      2,
						Ids:      Ids{*big.NewInt(2)},
					},
					{
						Type:     bchain.MultiToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
						Txs:      2,
						MultiTokenValues: MultiTokenValues{
							{
								Id:    *big.NewInt(11),
								Value: *big.NewInt(56900),
							},
							{
								Id:    *big.NewInt(22),
								Value: *big.NewInt(222),
							},
						},
					},
				},
			},
		},
		{
			name: "ERC1155 from id 11, value 112 and id 22, value 222",
			args: args{
				index:    ^1,
				contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
				transfer: &bchain.TokenTransfer{
					Type: bchain.MultiToken,
					MultiTokenValues: []bchain.MultiTokenValue{
						{
							Id:    *big.NewInt(11),
							Value: *big.NewInt(112),
						},
						{
							Id:    *big.NewInt(22),
							Value: *big.NewInt(222),
						},
					},
				},
				addTxCount: true,
			},
			wantIndex: ^(2 + ContractIndexOffset), // the 3nd contract of the address
			wantAddrContracts: &AddrContracts{
				Contracts: []AddrContract{
					{
						Type:     bchain.FungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract47, parser),
						Value:    *big.NewInt(100000),
						Txs:      2,
					},
					{
						Type:     bchain.NonFungibleToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						Txs:      2,
						Ids:      Ids{*big.NewInt(2)},
					},
					{
						Type:     bchain.MultiToken,
						Contract: addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
						Txs:      3,
						MultiTokenValues: MultiTokenValues{
							{
								Id:    *big.NewInt(11),
								Value: *big.NewInt(56788),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contractIndex, found := findContractInAddressContracts(tt.args.contract, addrContracts.Contracts)
			if !found {
				contractIndex = len(addrContracts.Contracts)
				addrContracts.Contracts = append(addrContracts.Contracts, AddrContract{
					Contract: tt.args.contract,
					Type:     tt.args.transfer.Type,
				})
			}
			if got := addToContract(&addrContracts.Contracts[contractIndex], contractIndex, tt.args.index, tt.args.contract, tt.args.transfer, tt.args.addTxCount); got != tt.wantIndex {
				t.Errorf("addToContracts() = %v, want %v", got, tt.wantIndex)
			}
			if !reflect.DeepEqual(addrContracts, tt.wantAddrContracts) {
				t.Errorf("addToContracts() = %+v, want %+v", addrContracts, tt.wantAddrContracts)
			}
		})
	}
}

func Test_packUnpackBlockTx(t *testing.T) {
	parser := ethereumTestnetParser()
	tests := []struct {
		name    string
		blockTx ethBlockTx
		pos     int
	}{
		{
			name: "no contract",
			blockTx: ethBlockTx{
				btxID:     hexToBytes(dbtestdata.EthTxidB1T1),
				from:      addressToAddrDesc(dbtestdata.EthAddr3e, parser),
				to:        addressToAddrDesc(dbtestdata.EthAddr55, parser),
				contracts: []ethBlockTxContract{},
			},
			pos: 73,
		},
		{
			name: "ERC20",
			blockTx: ethBlockTx{
				btxID: hexToBytes(dbtestdata.EthTxidB1T1),
				from:  addressToAddrDesc(dbtestdata.EthAddr3e, parser),
				to:    addressToAddrDesc(dbtestdata.EthAddr55, parser),
				contracts: []ethBlockTxContract{
					{
						from:         addressToAddrDesc(dbtestdata.EthAddr20, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr5d, parser),
						contract:     addressToAddrDesc(dbtestdata.EthAddrContract4a, parser),
						transferType: bchain.FungibleToken,
						value:        *big.NewInt(10000),
					},
				},
			},
			pos: 137,
		},
		{
			name: "multiple contracts",
			blockTx: ethBlockTx{
				btxID: hexToBytes(dbtestdata.EthTxidB1T1),
				from:  addressToAddrDesc(dbtestdata.EthAddr3e, parser),
				to:    addressToAddrDesc(dbtestdata.EthAddr55, parser),
				contracts: []ethBlockTxContract{
					{
						from:         addressToAddrDesc(dbtestdata.EthAddr20, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr3e, parser),
						contract:     addressToAddrDesc(dbtestdata.EthAddrContract4a, parser),
						transferType: bchain.FungibleToken,
						value:        *big.NewInt(987654321),
					},
					{
						from:         addressToAddrDesc(dbtestdata.EthAddr4b, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr55, parser),
						contract:     addressToAddrDesc(dbtestdata.EthAddrContract6f, parser),
						transferType: bchain.NonFungibleToken,
						value:        *big.NewInt(13),
					},
					{
						from:         addressToAddrDesc(dbtestdata.EthAddr5d, parser),
						to:           addressToAddrDesc(dbtestdata.EthAddr7b, parser),
						contract:     addressToAddrDesc(dbtestdata.EthAddrContractCd, parser),
						transferType: bchain.MultiToken,
						idValues: []bchain.MultiTokenValue{
							{
								Id:    *big.NewInt(1234),
								Value: *big.NewInt(98765),
							},
							{
								Id:    *big.NewInt(5566),
								Value: *big.NewInt(12341234421),
							},
						},
					},
				},
			},
			pos: 280,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 0)
			packed := packBlockTx(buf, &tt.blockTx)
			got, pos, err := unpackBlockTx(packed, 0)
			if err != nil {
				t.Errorf("unpackBlockTx() error = %v", err)
				return
			}
			if !reflect.DeepEqual(*got, tt.blockTx) {
				t.Errorf("unpackBlockTx() got = %v, want %v", *got, tt.blockTx)
			}
			if pos != tt.pos {
				t.Errorf("unpackBlockTx() pos = %v, want %v", pos, tt.pos)
			}
		})
	}
}

func Test_packUnpackFourByteSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature bchain.FourByteSignature
	}{
		{
			name: "no params",
			signature: bchain.FourByteSignature{
				Name: "abcdef",
			},
		},
		{
			name: "one param",
			signature: bchain.FourByteSignature{
				Name:       "opqr",
				Parameters: []string{"uint16"},
			},
		},
		{
			name: "multiple params",
			signature: bchain.FourByteSignature{
				Name:       "xyz",
				Parameters: []string{"address", "(bytes,uint256[],uint256)", "uint16"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := packFourByteSignature(&tt.signature)
			if got, err := unpackFourByteSignature(buf); !reflect.DeepEqual(*got, tt.signature) || err != nil {
				t.Errorf("packUnpackFourByteSignature() = %v, want %v, error %v", *got, tt.signature, err)
			}
		})
	}
}

func Test_packUnpackContractInfo(t *testing.T) {
	tests := []struct {
		name         string
		contractInfo bchain.ContractInfo
	}{
		{
			name:         "empty",
			contractInfo: bchain.ContractInfo{},
		},
		{
			name: "unknown",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.UnknownTokenType,
				Name:              "Test contract",
				Symbol:            "TCT",
				Decimals:          18,
				CreatedInBlock:    1234567,
				DestructedInBlock: 234567890,
			},
		},
		{
			name: "ERC20",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.ERC20TokenType,
				Name:              "GreenContract🟢",
				Symbol:            "🟢",
				Decimals:          0,
				CreatedInBlock:    1,
				DestructedInBlock: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := packContractInfo(&tt.contractInfo)
			if got, err := unpackContractInfo(buf); !reflect.DeepEqual(*got, tt.contractInfo) || err != nil {
				t.Errorf("packUnpackContractInfo() = %v, want %v, error %v", *got, tt.contractInfo, err)
			}
		})
	}
}
