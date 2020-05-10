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
	"fmt"
	"bytes"
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
			"000000ab",
			"00000da4905f27bad527f9ec2fb78090ee4079bd4d7219ee2f450e5439d0ed38" + uintToHex(1588899698) + varuintToHex(2) + varuintToHex(536),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for non negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.AddrS1, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T0, bchain.BaseCoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS2, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.AssetActivateMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS3, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.BaseCoinMask, []int32{2}, d), nil},
	
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS1, d.chainParser),
			varuintToHex(1) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T0A1, d) +
			/*assetbalances*/varuintToHex(0) +	dbtestdata.TxidS1T0 + varuintToHex(0) + varuintToHex(171) + bigintToHex(dbtestdata.SatS1T0A1, d) + /*asset info*/varuintToHex(0),
			nil,
		},
		// asset activate
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS2, d.chainParser),
			varuintToHex(1) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T1A1, d) +
			varuintToHex(1) + varuintToHex(720034467) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatZero, d) + varuintToHex(1) +
			dbtestdata.TxidS1T1 + varuintToHex(0) + varuintToHex(171) + bigintToHex(dbtestdata.SatS1T1A1, d) + varuintToHex(1) + varuintToHex(720034467) + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS3, d.chainParser),
			varuintToHex(1) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T1A2, d) +
			varuintToHex(0) + dbtestdata.TxidS1T1 + varuintToHex(2) + varuintToHex(171) + bigintToHex(dbtestdata.SatS1T1A2, d) + varuintToHex(0),
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
				"000000ab",
				dbtestdata.TxidS1T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
				dbtestdata.TxidS1T1 + "01" + dbtestdata.TxidS1T1INPUT0 + "00",
				nil,
			},
		}
	}

	if err := checkColumn(d, cfBlockTxs, blockTxsKp); err != nil {
		{
			t.Fatal(err)
		}
	}
	dBAsset, err := d.GetAsset(720034467, nil)
	if dBAsset == nil || err != nil {
		if dBAsset == nil {
			t.Fatal("asset not found after block 1")
		}
		t.Fatal(err)
	}
	if dBAsset.Transactions != 1 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dbAsset.Transaction: ", dBAsset.Transactions, ". Expected: 1"))
	}
	if dBAsset.AssetObj.Symbol != "CAT" {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Symbol: ", dBAsset.AssetObj.Symbol , ". Expected: CAT"))
	}
	if !bytes.Equal(dBAsset.AssetObj.PubData, []byte("{\"description\":\"publicvalue\"}")) {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PubData: ", dBAsset.AssetObj.PubData  , ". Expected: {\"description\":\"publicvalue\"}"))
	}
	if dBAsset.AssetObj.UpdateFlags != 31 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.UpdateFlags: ", dBAsset.AssetObj.UpdateFlags  , ". Expected: 31"))
	}
	if dBAsset.AssetObj.Balance != 10000000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Balance: ", dBAsset.AssetObj.Balance  , ". Expected: 10000000000"))
	}
	if dBAsset.AssetObj.TotalSupply != 10000000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.TotalSupply: ", dBAsset.AssetObj.TotalSupply  , ". Expected: 10000000000"))
	}
	if dBAsset.AssetObj.MaxSupply != 100000000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.MaxSupply: ", dBAsset.AssetObj.MaxSupply  , ". Expected: 100000000000"))
	}
	if dBAsset.AssetObj.Precision != 8 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Precision: ", dBAsset.AssetObj.Precision  , ". Expected: 8"))
	}
	if dBAsset.AssetObj.PrevPubData != "" {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PrevPubData: ", dBAsset.AssetObj.PrevPubData  , ". Expected: ''"))
	}
	if dBAsset.AssetObj.PrevUpdateFlags != 0 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PrevUpdateFlags: ", dBAsset.AssetObj.PrevUpdateFlags  , ". Expected: 0"))
	}
}
func verifyAfterSyscoinTypeBlock2(t *testing.T, d *RocksDB) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"000000b6",
			"00000e4afb4178a83b1b6e05872c5754b007f94b7645d93443a4ee51c45a2d74" + uintToHex(1588899730) + varuintToHex(2) + varuintToHex(539),
			nil,
		},
		{
			"000000ab",
			"00000da4905f27bad527f9ec2fb78090ee4079bd4d7219ee2f450e5439d0ed38" + uintToHex(1588899698) + varuintToHex(2) + varuintToHex(536),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.AddrS1, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T0, bchain.BaseCoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS2, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.AssetActivateMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS3, 171, d), txIndexesHexSyscoin(dbtestdata.TxidS1T1, bchain.BaseCoinMask, []int32{2}, d), nil},
		{addressKeyHex(dbtestdata.AddrS2, 182, d), txIndexesHexSyscoin(dbtestdata.TxidS2T1, bchain.AssetActivateMask, []int32{^0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS1, 182, d), txIndexesHexSyscoin(dbtestdata.TxidS2T0, bchain.BaseCoinMask, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.AddrS5, 182, d), txIndexesHexSyscoin(dbtestdata.TxidS2T1, bchain.AssetUpdateMask, []int32{0}, d), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	S1addedAmount := new(big.Int).Set(dbtestdata.SatS1T0A1)
	S1addedAmount.Add(S1addedAmount, dbtestdata.SatS2T0A1)
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS1, d.chainParser),
			varuintToHex(2) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(S1addedAmount, d) +
			/*assetbalances*/varuintToHex(0) +	dbtestdata.TxidS1T0 + varuintToHex(0) + varuintToHex(171) + bigintToHex(dbtestdata.SatS1T0A1, d) + /*asset info*/varuintToHex(0) + 
			dbtestdata.TxidS2T0 + varuintToHex(0) + varuintToHex(182) + bigintToHex(dbtestdata.SatS2T0A1, d) + /*asset info*/varuintToHex(0),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS2, d.chainParser),
			varuintToHex(2) + bigintToHex(dbtestdata.SatS1T1A1, d) + bigintToHex(dbtestdata.SatZero, d) +
			varuintToHex(1) + varuintToHex(720034467) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatZero, d) + /* 2 transfers, one activate one spend of active*/varuintToHex(2),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS3, d.chainParser),
			varuintToHex(1) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS1T1A2, d) +
			varuintToHex(0) + dbtestdata.TxidS1T1 + varuintToHex(2) + varuintToHex(171) + bigintToHex(dbtestdata.SatS1T1A2, d) + varuintToHex(0),
			nil,
		},
		// asset update. asset activate (AddrS2), should be spent
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrS5, d.chainParser),
			varuintToHex(1) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatS2T1A1, d) +
			varuintToHex(1) + varuintToHex(720034467) + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatZero, d) + varuintToHex(1) +
			dbtestdata.TxidS2T1 + varuintToHex(0) + varuintToHex(182) + bigintToHex(dbtestdata.SatS2T1A1, d) +  varuintToHex(1) + varuintToHex(720034467) + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	dBAsset, err := d.GetAsset(720034467, nil)
	if dBAsset == nil || err != nil {
		if dBAsset == nil {
			t.Fatal("asset not found after block 1")
		}
		t.Fatal(err)
	}
	if dBAsset.Transaction != 2 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dbAsset.Transaction: ", dBAsset.Transaction, ". Expected: 2"))
	}
	if dBAsset.AssetObj.Symbol != "CAT" {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Symbol: ", dBAsset.AssetObj.Symbol , ". Expected: CAT"))
	}
	if !bytes.Equal(dBAsset.AssetObj.PubData, []byte("{\"description\":\"newdescription1\"}")) {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PubData: ", dBAsset.AssetObj.PubData  , ". Expected: {\"description\":\"newdescription1\"}"))
	}
	if dBAsset.AssetObj.UpdateFlags != 31 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.UpdateFlags: ", dBAsset.AssetObj.UpdateFlags  , ". Expected: 31"))
	}
	if dBAsset.AssetObj.Balance != 10500000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Balance: ", dBAsset.AssetObj.Balance  , ". Expected: 10500000000"))
	}
	if dBAsset.AssetObj.TotalSupply != 10500000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.TotalSupply: ", dBAsset.AssetObj.TotalSupply  , ". Expected: 10000000000"))
	}
	if dBAsset.AssetObj.MaxSupply != 100000000000 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.MaxSupply: ", dBAsset.AssetObj.MaxSupply  , ". Expected: 100000000000"))
	}
	if dBAsset.AssetObj.Precision != 8 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.Precision: ", dBAsset.AssetObj.Precision  , ". Expected: 8"))
	}
	if !bytes.Equal(dBAsset.AssetObj.PrevPubData, []byte("{\"description\":\"publicvalue\"}")) {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PrevPubData: ", dBAsset.AssetObj.PrevPubData  , ". Expected: {\"description\":\"publicvalue\"}"))
	}
	if dBAsset.AssetObj.PrevUpdateFlags != 31 {
		t.Fatal(fmt.Sprint("Block1: Property mismatch dBAsset.AssetObj.PrevUpdateFlags: ", dBAsset.AssetObj.PrevUpdateFlags  , ". Expected: 31"))
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

	// connect 1st block - create asset
	block1 := dbtestdata.GetTestSyscoinTypeBlock1(d.chainParser)
	for i, _ := range block1.Txs {
		tx := &block1.Txs[i]
		err := d.chainParser.LoadAssets(tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock1(t, d, false)

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// connect 2nd block - update asset
	block2 := dbtestdata.GetTestSyscoinTypeBlock2(d.chainParser)
	for i, _ := range block2.Txs {
		tx := &block2.Txs[i]
		err := d.chainParser.LoadAssets(tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock2(t, d)

	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"000000b6",
			dbtestdata.TxidS2T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS1T1 + "00",
			nil,
		},
		{
			"000000ab",
			dbtestdata.TxidS1T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS1T1 + "01" + dbtestdata.TxidS1T1INPUT0 + "00",
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
	verifyGetTransactions(t, d, dbtestdata.AddrS2, 0, 1000000, []txidIndex{
		{dbtestdata.TxidS2T1, ^0},
		{dbtestdata.TxidS1T1, 0},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS2, 171, 171, []txidIndex{
		{dbtestdata.TxidS1T1, 0},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS2, 182, 1000000, []txidIndex{
		{dbtestdata.TxidS2T1, ^0},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS2, 500000, 1000000, []txidIndex{}, nil)
	verifyGetTransactions(t, d, dbtestdata.AddrS1, 0, 1000000, []txidIndex{
		{dbtestdata.TxidS2T0, 0},
		{dbtestdata.TxidS1T0, 0},
	}, nil)
	verifyGetTransactions(t, d, "SgBVZhGLjqRz8ufXFwLhZvXpUMKqoduBad", 500000, 1000000, []txidIndex{}, errors.New("checksum mismatch"))

	// GetBestBlock
	height, hash, err := d.GetBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	if height != 182 {
		t.Fatalf("GetBestBlock: got height %v, expected %v", height, 182)
	}
	if hash != "00000e4afb4178a83b1b6e05872c5754b007f94b7645d93443a4ee51c45a2d74" {
		t.Fatalf("GetBestBlock: got hash %v, expected %v", hash, "00000e4afb4178a83b1b6e05872c5754b007f94b7645d93443a4ee51c45a2d74")
	}

	// GetBlockHash
	hash, err = d.GetBlockHash(171)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "00000da4905f27bad527f9ec2fb78090ee4079bd4d7219ee2f450e5439d0ed38" {
		t.Fatalf("GetBlockHash: got hash %v, expected %v", hash, "00000da4905f27bad527f9ec2fb78090ee4079bd4d7219ee2f450e5439d0ed38")
	}

	// Not connected block
	hash, err = d.GetBlockHash(183)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("GetBlockHash: got hash '%v', expected ''", hash)
	}

	// GetBlockHash
	info, err := d.GetBlockInfo(182)
	if err != nil {
		t.Fatal(err)
	}
	iw := &bchain.DbBlockInfo{
		Hash:   "00000e4afb4178a83b1b6e05872c5754b007f94b7645d93443a4ee51c45a2d74",
		Txs:    2,
		Size:   539,
		Time:   1588899730,
		Height: 182,
	}
	if !reflect.DeepEqual(info, iw) {
		t.Errorf("GetBlockInfo() = %+v, want %+v", info, iw)
	}

	// try to disconnect both blocks, however only the last one is kept, it is not possible
	err = d.DisconnectBlockRangeBitcoinType(171, 182)
	if err == nil || err.Error() != "Cannot disconnect blocks with height 172 and lower. It is necessary to rebuild index." {
		t.Fatal(err)
	}
	verifyAfterSyscoinTypeBlock2(t, d)

	// disconnect the 2nd block, verify that the db contains only data from the 1st block with restored unspentTxs
	// and that the cached tx is removed
	err = d.DisconnectBlockRangeBitcoinType(182, 182)
	if err != nil {
		t.Fatal(err)
	}
	t.Errorf("DisconnectBlockRangeBitcoinType 182")
	verifyAfterSyscoinTypeBlock1(t, d, false)
	if err := checkColumn(d, cfTransactions, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}
	t.Errorf("verified block 1")

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// connect block again and verify the state of db
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	t.Errorf("verify block 2")
	verifyAfterSyscoinTypeBlock2(t, d)
	t.Errorf("verified block 2")
	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"000000b6",
			dbtestdata.TxidS2T0 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00" +
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS1T1 + "02",
			nil,
		},
		{
			"000000ab",
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
	ab, err := d.GetAddressBalance(dbtestdata.AddrS5, bchain.AddressBalanceDetailUTXO)
	if err != nil {
		t.Fatal(err)
	}
	abw := &bchain.AddrBalance{
		Txs:        2,
		SentSat:    *dbtestdata.SatZero,
		BalanceSat: *dbtestdata.SatS2T1A1,
		Utxos: []bchain.Utxo{
			{
				BtxID:    hexToBytes(dbtestdata.TxidS2T1),
				Vout:     0,
				Height:   182,
				ValueSat: *dbtestdata.SatS2T1A1,
				AssetInfo: &bchain.AssetInfo{AssetGuid: 720034467, ValueSat: dbtestdata.SatZero},
			},
		},
		AssetBalances: map[uint32]*bchain.AssetBalance {
			720034467: &bchain.AssetBalance{
				SentSat: 	dbtestdata.SatZero,
				BalanceSat: dbtestdata.SatZero,
				Transfers:	2,
			},
		},
	}
	if !reflect.DeepEqual(ab, abw) {
		t.Errorf("GetAddressBalance() = %+v, want %+v", ab, abw)
	}

	ta, err := d.GetTxAddresses(dbtestdata.TxidS2T1)
	if err != nil {
		t.Fatal(err)
	}
	// spends an asset (activate) output to another output
	taw := &bchain.TxAddresses{
		Version: 131,
		Height: 182,
		Inputs: []bchain.TxInput{
			{
				ValueSat: *dbtestdata.SatZero,
				AssetInfo: &bchain.AssetInfo{AssetGuid: 720034467, ValueSat: dbtestdata.SatZero},
			},
		},
		Outputs: []bchain.TxOutput{
			{
				AddrDesc: addressToAddrDesc(dbtestdata.AddrS5, d.chainParser),
				Spent:    false,
				ValueSat: *dbtestdata.SatS2T1A1,
				AssetInfo: &bchain.AssetInfo{AssetGuid: 720034467, ValueSat: dbtestdata.SatZero},
			},
			{
				AddrDesc: hexToBytes(dbtestdata.TxidS2T1OutputReturn),
				Spent:    false,
				ValueSat: *dbtestdata.SatZero,
			},
		},
	}
	if !reflect.DeepEqual(ta, taw) {
		t.Errorf("GetTxAddresses() = %+v, want %+v", ta, taw)
	}
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

	block1 := dbtestdata.GetTestSyscoinTypeBlock1(d.chainParser)
	for i, _ := range block1.Txs {
		tx := &block1.Txs[i]
		err := d.chainParser.LoadAssets(tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := bc.ConnectBlock(block1, false); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}
/*
	block2 := dbtestdata.GetTestSyscoinTypeBlock2(d.chainParser)
	for i, _ := range block2.Txs {
		tx := &block2.Txs[i]
		err := d.chainParser.LoadAssets(tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := bc.ConnectBlock(block2, true); err != nil {
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
			dbtestdata.TxidS2T1 + "01" + dbtestdata.TxidS1T1 + "02",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if len(d.is.BlockTimes) != 347315 {
		t.Fatal("Expecting is.BlockTimes 347315, got ", len(d.is.BlockTimes))
	}*/
	chaincfg.ResetParams()
}
