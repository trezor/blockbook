// +build unittest

package db

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

type testEthereumParser struct {
	*eth.EthereumParser
}

func ethereumTestnetParser() *eth.EthereumParser {
	return eth.NewEthereumParser(1)
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
		{addressKeyHex(dbtestdata.EthAddr3e, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{1}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr20, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^0, ^1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract4a, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{0}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser), "0101", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser), "0201" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser), "0101" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "0101", nil},
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
		{
			"0041eee8",
			"c7b98df95acfd11c51ba25611a39e004fe56c8fdfc1582af99354fcd09c17b11" + uintToHex(1534858022) + varuintToHex(2) + varuintToHex(31839),
			nil,
		},
		{
			"0041eee9",
			"2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee" + uintToHex(1534859988) + varuintToHex(2) + varuintToHex(2345678),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.EthAddr3e, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{1}) + txIndexesHex(dbtestdata.EthTxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr20, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{^0, ^1}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract4a, 4321000, d), txIndexesHex(dbtestdata.EthTxidB1T2, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr55, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^2, 1}) + txIndexesHex(dbtestdata.EthTxidB2T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.EthAddr9f, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.EthAddr4b, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^0, 1, ^2, 2, ^1}), nil},
		{addressKeyHex(dbtestdata.EthAddr7b, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{^1, 2}), nil},
		{addressKeyHex(dbtestdata.EthAddrContract47, 4321001, d), txIndexesHex(dbtestdata.EthTxidB2T2, []int32{0}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := checkColumn(d, cfAddressContracts, []keyPair{
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr3e, d.chainParser), "0101", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr55, d.chainParser), "0402" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "02" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "01", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr20, d.chainParser), "0101" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser), "0101", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr9f, d.chainParser), "0101", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr4b, d.chainParser), "0101" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "02" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "02", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddr7b, d.chainParser), "0100" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract4a, d.chainParser) + "01" + dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract0d, d.chainParser) + "01", nil},
		{dbtestdata.AddressToPubKeyHex(dbtestdata.EthAddrContract47, d.chainParser), "0101", nil},
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

	// connect 2nd block
	block2 := dbtestdata.GetTestEthereumTypeBlock2(d.chainParser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterEthereumTypeBlock2(t, d)

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

	// get transactions for various addresses / low-high ranges
	verifyGetTransactions(t, d, "0x"+dbtestdata.EthAddr55, 0, 10000000, []txidIndex{
		{"0x" + dbtestdata.EthTxidB2T2, ^2},
		{"0x" + dbtestdata.EthTxidB2T2, 1},
		{"0x" + dbtestdata.EthTxidB2T1, ^0},
		{"0x" + dbtestdata.EthTxidB1T2, 1},
		{"0x" + dbtestdata.EthTxidB1T1, 0},
	}, nil)
	verifyGetTransactions(t, d, "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eBad", 500000, 1000000, []txidIndex{}, errors.New("Address missing"))

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
		Txs:    2,
		Size:   2345678,
		Time:   1534859988,
		Height: 4321001,
	}
	if !reflect.DeepEqual(info, iw) {
		t.Errorf("GetBlockInfo() = %+v, want %+v", info, iw)
	}

	// Test tx caching functionality, leave one tx in db to test cleanup in DisconnectBlock
	testTxCache(t, d, block1, &block1.Txs[0])
	testTxCache(t, d, block2, &block2.Txs[0])
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
	verifyAfterEthereumTypeBlock2(t, d)

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
	verifyAfterEthereumTypeBlock2(t, d)

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

}
