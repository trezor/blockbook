// build unittest

package db

import (
	"blockbook/bchain/coins/eth"
	"blockbook/tests/dbtestdata"
	"testing"
)

type testEthereumParser struct {
	*eth.EthereumParser
}

func ethereumTestnetParser() *eth.EthereumParser {
	return eth.NewEthereumParser(1)
}

func verifyAfterEthereumTypeBlock1(t *testing.T, d *RocksDB, afterDisconnect bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{
			"0041eee8",
			"c7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" + uintToHex(1534858022) + varuintToHex(2) + varuintToHex(31839),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for positive values, abs(value)*2 + 1 for negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T1 + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T1 + "00" + dbtestdata.EthTxidB1T2 + "02", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T2 + "01" + dbtestdata.EthTxidB1T2 + "03", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T2 + "00", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser), "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser), "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser), "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "01", nil},
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
			keyPair{
				"0041eee8",
				dbtestdata.EthTxidB1T1 +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + "00" +
					dbtestdata.EthTxidB1T2 +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
					"02" +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
					dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser),
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

func verifyAfterEthereumTypeBlock2(t *testing.T, d *RocksDB) {
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{
			"0041eee8",
			"c7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" + uintToHex(1534858022) + varuintToHex(2) + varuintToHex(31839),
			nil,
		},
		keyPair{
			"0041eee9",
			"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6" + uintToHex(1534859988) + varuintToHex(2) + varuintToHex(2345678),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for positive values, abs(value)*2 + 1 for negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T1 + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T1 + "00" + dbtestdata.EthTxidB1T2 + "02", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T2 + "01" + dbtestdata.EthTxidB1T2 + "03", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "0041eee8", dbtestdata.EthTxidB1T2 + "00", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + "0041eee9", dbtestdata.EthTxidB2T1 + "01" + dbtestdata.EthTxidB2T2 + "05" + dbtestdata.EthTxidB2T2 + "02", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser) + "0041eee9", dbtestdata.EthTxidB2T1 + "00", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + "0041eee9", dbtestdata.EthTxidB2T2 + "01" + dbtestdata.EthTxidB2T2 + "02" + dbtestdata.EthTxidB2T2 + "05" + dbtestdata.EthTxidB2T2 + "04" + dbtestdata.EthTxidB2T2 + "03", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + "0041eee9", dbtestdata.EthTxidB2T2 + "03" + dbtestdata.EthTxidB2T2 + "04", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser) + "0041eee9", dbtestdata.EthTxidB2T2 + "00", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser), "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser), "02" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "02" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser), "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser), "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser), "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "02" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "02", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser), "00" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "01", nil},
		keyPair{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser), "01", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfBlockTxs, []keyPair{
		keyPair{
			"0041eee9",
			dbtestdata.EthTxidB2T1 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser) + "00" +
				dbtestdata.EthTxidB2T2 +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser) +
				"08" +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) +
				dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser) + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
}

// TestRocksDB_Index_EthereumType is an integration test probing the whole indexing functionality for EthereumType chains
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for various addresses / low-high ranges
// 3) GetBestBlock, GetBlockHash
// 4) Test tx caching functionality
// 5) Disconnect block 2 - expect error
// 6) Disconnect the block 2 using BlockTxs column
// 7) Reconnect block 2 and check
// After each step, the content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_EthereumType(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{
		EthereumParser: ethereumTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// connect 1st block - will log warnings about missing UTXO transactions in txAddresses column
	block1 := dbtestdata.GetTestEthereumTypeBlock1(d.chainParser)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock1(t, d, false)

	// connect 2nd block - use some outputs from the 1st block as the inputs and 1 input uses tx from the same block
	block2 := dbtestdata.GetTestEthereumTypeBlock2(d.chainParser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock2(t, d)

}
