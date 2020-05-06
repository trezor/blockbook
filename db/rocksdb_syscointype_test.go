// +build unittest

package db

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/sys"
	"blockbook/tests/dbtestdata"
	"math/big"
	"reflect"
	"testing"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/juju/errors"
	"encoding/hex"
	vlq "github.com/bsm/go-vlq"
)

type testSyscoinParser struct {
	*syscoin.SyscoinParser
}

func syscoinTestParser() *syscoin.SyscoinParser {
	return syscoin.NewSyscoinParser(syscoin.GetChainParams("main"),
	&btc.Configuration{BlockAddressesToKeep: 1})
}

func txIndexesHexSyscoin(tx string, assetsMask bchain.AssetsMask, indexes []int32, d *RocksDB) string {
	buf := make([]byte, vlq.MaxLen32)
	l := d.chainParser.PackVaruint(uint(assetsMask), buf)
	tx = hex.EncodeToString(buf[:l]) + tx
	for i, index := range indexes {
		index <<= 1
		if i == len(indexes)-1 {
			index |= 1
		}
		l = d.chainParser.PackVarint32(index, buf)
		tx += hex.EncodeToString(buf[:l])
	}
	return tx
} 
func verifyAfterSyscoinTypeBlock1(t *testing.T, d *RocksDB, afterDisconnect bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"0000009e",
			"000004138eaa5e65a84b9b7f48fb9f9b1a8aadf27248974cabb3a23f7f20458a" + uintToHex(1588788257) + varuintToHex(2) + varuintToHex(536),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for non negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.AddrS1, 158, d), txIndexesHexSyscoin(dbtestdata.TxidS1T0, bchain.SyscoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS2, 158, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.SyscoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS3, 158, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.SyscoinMask, []int32{2}, d), nil},
	
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS1, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T0A1, d) +
			"00" +	dbtestdata.TxidS1T0 + varuintToHex(0) + varuintToHex(158) + bigintToHex(dbtestdata.SatS1T0A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS2, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T1A0, d) +
			"00" + dbtestdata.TxidS1T1 + varuintToHex(1) + varuintToHex(158) + bigintToHex(dbtestdata.SatS1T1A0, d),
			nil,
		},
		// asset activate
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS3, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T1A1, d) +
			"01" + varuintToHex(732260830) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatZero, d) + varuintToHex(1) +
				dbtestdata.TxidS1T1 + varuintToHex(1) + varuintToHex(158) + bigintToHex(dbtestdata.SatS1T1A1, d) + varuintToHex(732260830) + bigintToHex(dbtestdata.SatZero, d),
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
				"0000009e",
				dbtestdata.TxidS1T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
				dbtestdata.TxidS1T1 + "01" + dbtestdata.TxidS1T1INPUT0 + "02",
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

func verifyAfterSyscoinTypeBlock2(t *testing.T, d *RocksDB) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"0003cf7f",
			"78ae6476a514897c8a6984032e5d0e4a44424055f0c2d7b5cf664ae8c8c20487" + uintToHex(1574279564) + varuintToHex(2) + varuintToHex(1551),
			nil,
		},
		{
			"00054cb2",
			"6609d44688868613991b0cd5ed981a76526caed6b0f7b1be242f5a93311636c6" + uintToHex(1580142055) + varuintToHex(2) + varuintToHex(1611),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.AddrS1, 249727, d), txIndexesHexSyscoin(dbtestdata.TxidS1T0, bchain.SyscoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS2, 249727, d), txIndexesHexSyscoin(dbtestdata.TxidS1T0, bchain.SyscoinMask, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.AddrS3, 249727, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.SyscoinMask, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.AddrS4, 347314, d), txIndexesHexSyscoin(dbtestdata.TxidS2T0, bchain.SyscoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS5, 347314, d), txIndexesHexSyscoin(dbtestdata.TxidS2T0, bchain.SyscoinMask, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.AddrS3, 347314, d), txIndexesHexSyscoin(dbtestdata.TxidS2T1, bchain.SyscoinMask, []int32{1}, d), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	addedAmount := new(big.Int).Set(dbtestdata.SatS1T1A1)
	addedAmount.Add(addedAmount, dbtestdata.SatS2T1A1)
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS1, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T0A1, d) +
			"00" + dbtestdata.TxidS1T0 + varuintToHex(0) + varuintToHex(249727) + bigintToHex(dbtestdata.SatS1T0A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS2, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T0A2, d) +
			"00" + dbtestdata.TxidS1T0 + varuintToHex(1) + varuintToHex(249727) + bigintToHex(dbtestdata.SatS1T0A2, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS3, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(addedAmount, d) +
			"01" + varuintToHex(1045909988) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatAssetSent, d) + varuintToHex(2) +
				dbtestdata.TxidS1T1 + varuintToHex(1) + varuintToHex(249727) + bigintToHex(dbtestdata.SatS1T1A1, d) +
				dbtestdata.TxidS2T1 + varuintToHex(1) + varuintToHex(347314) + bigintToHex(dbtestdata.SatS2T1A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS4, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS2T0A1, d) +
			"00" + dbtestdata.TxidS2T0 + varuintToHex(0) + varuintToHex(347314) + bigintToHex(dbtestdata.SatS2T0A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS5, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS2T0A2, d) +
			"00" + dbtestdata.TxidS2T0 + varuintToHex(1) + varuintToHex(347314) + bigintToHex(dbtestdata.SatS2T0A2, d),
			nil,
		},
		// burn should have a address balance as asset output from S2T1
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS6, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatZero, d) +
			"01" + varuintToHex(1045909988) + bigintToHex(dbtestdata.SatAssetSent, d) + bigintToHex(dbtestdata.SatZero, d) + varuintToHex(1),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
}

// TestRocksDB_Index_SyscoinType is an integration test probing the whole indexing functionality for Syscoin which is a BitcoinType chain
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for various addresses / low-high ranges
// 3) GetBestBlock, GetBlockHash
// 4) Test tx caching functionality
// 5) Disconnect the block 2 using BlockTxs column
// 6) Reconnect block 2 and check
// After each step, the content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_SyscoinType(t *testing.T) {
	d := setupRocksDB(t, &testSyscoinParser{
		SyscoinParser: syscoinTestParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	if len(d.is.BlockTimes) != 0 {
		t.Fatal("Expecting is.BlockTimes 0, got ", len(d.is.BlockTimes))
	}

	// connect 1st block - will log warnings about missing UTXO transactions in txAddresses column
	block1 := dbtestdata.GetTestSyscoinTypeBlock1(d.chainParser)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock1(t, d, false)

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}
/*
	// connect 2nd block - use some outputs from the 1st block as the inputs and 1 input uses tx from the same block
	block2 := dbtestdata.GetTestSyscoinTypeBlock2(d.chainParser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock2(t, d)

	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"00054cb2",
			dbtestdata.TxidS2T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS2T1INPUT0 + "02",
			nil,
		},
		{
			"0003cf7f",
			dbtestdata.TxidS1T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS1T1 + "01" + dbtestdata.TxidS1T1INPUT0 + "02",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

	// get transactions for various addresses / low-high ranges
	verifyGetTransactions(t, d, dbtestdata.AddrS3, 0, 1000000, []txidIndex{
		{dbtestdata.TxidS2T1, 1},
		{dbtestdata.TxidS1T1, 1},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS3, 249727, 249727, []txidIndex{
		{dbtestdata.TxidS1T1, 1},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS3, 347314, 1000000, []txidIndex{
		{dbtestdata.TxidS2T1, 1},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS3, 500000, 1000000, []txidIndex{}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS4, 0, 1000000, []txidIndex{
		{dbtestdata.TxidS2T0, 0},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS6, 0, 1000000, []txidIndex{}, nil)
	verifyGetTransactions(t, d, "SgBVZhGLjqRz8ufXFwLhZvXpUMKqoduBad", 500000, 1000000, []txidIndex{}, errors.New("checksum mismatch"))

	// GetBestBlock
	height, hash, err := d.GetBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	if height != 347314 {
		t.Fatalf("GetBestBlock: got height %v, expected %v", height, 347314)
	}
	if hash != "6609d44688868613991b0cd5ed981a76526caed6b0f7b1be242f5a93311636c6" {
		t.Fatalf("GetBestBlock: got hash %v, expected %v", hash, "6609d44688868613991b0cd5ed981a76526caed6b0f7b1be242f5a93311636c6")
	}

	// GetBlockHash
	hash, err = d.GetBlockHash(249727)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "78ae6476a514897c8a6984032e5d0e4a44424055f0c2d7b5cf664ae8c8c20487" {
		t.Fatalf("GetBlockHash: got hash %v, expected %v", hash, "78ae6476a514897c8a6984032e5d0e4a44424055f0c2d7b5cf664ae8c8c20487")
	}

	// Not connected block
	hash, err = d.GetBlockHash(347315)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("GetBlockHash: got hash '%v', expected ''", hash)
	}

	// GetBlockHash
	info, err := d.GetBlockInfo(347314)
	if err != nil {
		t.Fatal(err)
	}
	iw := &bchain.DbBlockInfo{
		Hash:   "6609d44688868613991b0cd5ed981a76526caed6b0f7b1be242f5a93311636c6",
		Txs:    2,
		Size:   1611,
		Time:   1580142055,
		Height: 347314,
	}
	if !reflect.DeepEqual(info, iw) {
		t.Errorf("GetBlockInfo() = %+v, want %+v", info, iw)
	}

	// try to disconnect both blocks, however only the last one is kept, it is not possible
	err = d.DisconnectBlockRangeBitcoinType(249727, 347314)
	if err == nil || err.Error() != "Cannot disconnect blocks with height 249728 and lower. It is necessary to rebuild index." {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock2(t, d)

	// disconnect the 2nd block, verify that the db contains only data from the 1st block with restored unspentTxs
	// and that the cached tx is removed
	err = d.DisconnectBlockRangeBitcoinType(347314, 347314)
	if err != nil {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock1(t, d, false)
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
	verifyAfterSyscoinTypeBlock2(t, d)

	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"00054cb2",
			dbtestdata.TxidS2T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS2T1INPUT0 + "02",
			nil,
		},
		{
			"0003cf7f",
			dbtestdata.TxidS1T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS1T1 + "01" + dbtestdata.TxidS1T1INPUT0 + "02",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	
	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 2, got ", len(d.is.BlockTimes))
	}

	// test public methods for address balance and tx addresses
	ab, err := d.GetAddressBalance(dbtestdata.AddrS3, bchain.AddressBalanceDetailUTXO)
	if err != nil {
		t.Fatal(err)
	}
	addedAmount := new(big.Int).Set(dbtestdata.SatS1T1A1)
	addedAmount.Add(addedAmount, dbtestdata.SatS2T1A1)
	abw := &bchain.AddrBalance{
		Txs:        2,
		SentSat:    *dbtestdata.SatZero,
		BalanceSat: *addedAmount,
		Utxos: []bchain.Utxo{
			{
				BtxID:    hexToBytes(dbtestdata.TxidS1T1),
				Vout:     1,
				Height:   249727,
				ValueSat: *dbtestdata.SatS1T1A1,
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidS2T1),
				Vout:     1,
				Height:   347314,
				ValueSat: *dbtestdata.SatS2T1A1,
			},
		},
		AssetBalances: map[uint32]*bchain.AssetBalance {
			1045909988: &bchain.AssetBalance{
				SentSat: 	dbtestdata.SatAssetSent,
				BalanceSat: dbtestdata.SatZero,
				Transfers:	2,
			},
		},
	}
	if !reflect.DeepEqual(ab, abw) {
		t.Errorf("GetAddressBalance() = %+v, want %+v", ab, abw)
	}
	rs := ab.ReceivedSat()
	rsw := addedAmount
	if rs.Cmp(rsw) != 0 {
		t.Errorf("GetAddressBalance().ReceivedSat() = %v, want %v", rs, rsw)
	}

	rsa := bchain.ReceivedSatFromBalances(dbtestdata.SatZero, dbtestdata.SatAssetSent)
	rswa := dbtestdata.SatAssetSent
	if rsa.Cmp(rswa) != 0 {
		t.Errorf("GetAddressBalance().ReceivedSatFromBalances() = %v, want %v", rsa, rswa)
	}

	ta, err := d.GetTxAddresses(dbtestdata.TxidS2T1)
	if err != nil {
		t.Fatal(err)
	}
	taw := &bchain.TxAddresses{
		Version: 29701,
		Height: 347314,
		Inputs: []bchain.TxInput{
			{
				// input won't be found because there is many transactions within the range of blocks we chose to isolate asset data for this test
				ValueSat: *dbtestdata.SatZero,
			},
		},
		Outputs: []bchain.TxOutput{
			{
				AddrDesc: hexToBytes(dbtestdata.TxidS2T1OutputReturn),
				Spent:    false,
				ValueSat: *dbtestdata.SatZero,
			},
			{
				AddrDesc: addressToAddrDesc(dbtestdata.AddrS3, d.chainParser),
				Spent:    false,
				ValueSat: *dbtestdata.SatS2T1A1,
			},
		},
	}
	if !reflect.DeepEqual(ta, taw) {
		t.Errorf("GetTxAddresses() = %+v, want %+v", ta, taw)
	}*/

}

func Test_BulkConnect_SyscoinType(t *testing.T) {
	d := setupRocksDB(t, &testSyscoinParser{
		SyscoinParser: syscoinTestParser(),
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

	if err := bc.ConnectBlock(dbtestdata.GetTestSyscoinTypeBlock1(d.chainParser), false); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := bc.ConnectBlock(dbtestdata.GetTestSyscoinTypeBlock2(d.chainParser), true); err != nil {
		t.Fatal(err)
	}

	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}

	if d.is.DbState != common.DbStateOpen {
		t.Fatal("DB not in DbStateOpen")
	}

	verifyAfterSyscoinTypeBlock2(t, d)
	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"00054cb2",
			dbtestdata.TxidS2T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS2T1INPUT0 + "02",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if len(d.is.BlockTimes) != 347315 {
		t.Fatal("Expecting is.BlockTimes 347315, got ", len(d.is.BlockTimes))
	}
	chaincfg.ResetParams()
}
