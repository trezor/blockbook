//go:build unittest

package db

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	vlq "github.com/bsm/go-vlq"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// simplified explanation of signed varint packing, used in many index data structures
// for number n, the packing is: 2*n if n>=0 else 2*(-n)-1
// takes only 1 byte if abs(n)<127

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

type testBitcoinParser struct {
	*btc.BitcoinParser
}

func bitcoinTestnetParser() *btc.BitcoinParser {
	return btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})
}

func setupRocksDB(t *testing.T, p bchain.BlockChainParser) *RocksDB {
	tmp, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewRocksDB(tmp, 100000, -1, p, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	return d
}

func closeAndDestroyRocksDB(t *testing.T, d *RocksDB) {
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(d.path)
}

func inputAddressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := dbtestdata.AddressToPubKeyHex(addr, d.chainParser)
	return hex.EncodeToString([]byte{byte(len(h) / 2)}) + h
}

func addressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := dbtestdata.AddressToPubKeyHex(addr, d.chainParser)
	return hex.EncodeToString([]byte{byte(len(h))}) + h
}

func spentAddressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := dbtestdata.AddressToPubKeyHex(addr, d.chainParser)
	return hex.EncodeToString([]byte{byte(len(h) + 1)}) + h
}

func bigintToHex(i *big.Int) string {
	b := make([]byte, maxPackedBigintBytes)
	l := packBigint(i, b)
	return hex.EncodeToString(b[:l])
}

func varuintToHex(i uint) string {
	b := make([]byte, vlq.MaxLen64)
	l := vlq.PutUint(b, uint64(i))
	return hex.EncodeToString(b[:l])
}

func uintToHex(i uint32) string {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i)
	return hex.EncodeToString(buf)
}

func hexToBytes(h string) []byte {
	b, _ := hex.DecodeString(h)
	return b
}

func addressKeyHex(a string, height uint32, d *RocksDB) string {
	return dbtestdata.AddressToPubKeyHex(a, d.chainParser) + uintToHex(^height)
}

func txIndexesHex(tx string, indexes []int32) string {
	buf := make([]byte, vlq.MaxLen32)
	for i, index := range indexes {
		index <<= 1
		if i == len(indexes)-1 {
			index |= 1
		}
		l := packVarint32(index, buf)
		tx += hex.EncodeToString(buf[:l])
	}
	return tx
}

// keyPair is used to compare given key value in DB with expected
// for more complicated compares it is possible to specify CompareFunc
type keyPair struct {
	Key, Value  string
	CompareFunc func(string) bool
}

func compareFuncBlockAddresses(t *testing.T, v string, expected []string) bool {
	for _, e := range expected {
		lb := len(v)
		v = strings.Replace(v, e, "", 1)
		if lb == len(v) {
			t.Error(e, " not found in ", v)
			return false
		}
	}
	if len(v) != 0 {
		t.Error("not expected content ", v)
	}
	return len(v) == 0
}

func checkColumn(d *RocksDB, col int, kp []keyPair) error {
	sort.Slice(kp, func(i, j int) bool {
		return kp[i].Key < kp[j].Key
	})
	it := d.db.NewIteratorCF(d.ro, d.cfh[col])
	defer it.Close()
	i := 0
	for it.SeekToFirst(); it.Valid(); it.Next() {
		key := hex.EncodeToString(it.Key().Data())
		if i >= len(kp) {
			return errors.Errorf("Expected less rows in column %v, superfluous key %v", cfNames[col], key)
		}
		if key != kp[i].Key {
			return errors.Errorf("Incorrect key %v found in column %v row %v, expecting %v", key, cfNames[col], i, kp[i].Key)
		}
		val := hex.EncodeToString(it.Value().Data())
		var valOK bool
		if kp[i].CompareFunc == nil {
			valOK = val == kp[i].Value
		} else {
			valOK = kp[i].CompareFunc(val)
		}
		if !valOK {
			return errors.Errorf("Incorrect value %v found in column %v row %v key %v, expecting %v", val, cfNames[col], i, key, kp[i].Value)
		}
		i++
	}
	if i != len(kp) {
		return errors.Errorf("Expected more rows in column %v: got %v, expected %v", cfNames[col], i, len(kp))
	}
	return nil
}

func verifyAfterBitcoinTypeBlock1(t *testing.T, d *RocksDB, afterDisconnect bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"000370d5",
			"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997" + uintToHex(1521515026) + varuintToHex(2) + varuintToHex(1234567),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for non negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.Addr1, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr2, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{1, 2}), nil},
		{addressKeyHex(dbtestdata.Addr3, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr4, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{1}), nil},
		{addressKeyHex(dbtestdata.Addr5, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{2}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfTxAddresses, []keyPair{
		{
			dbtestdata.TxidB1T1,
			varuintToHex(225493) +
				"00" +
				"03" +
				addressToPubKeyHexWithLength(dbtestdata.Addr1, t, d) + bigintToHex(dbtestdata.SatB1T1A1) +
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2) +
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2),
			nil,
		},
		{
			dbtestdata.TxidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				addressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3) +
				addressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4) +
				addressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr1, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T1A1) +
				dbtestdata.TxidB1T1 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A1),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr2, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T1A2Double) +
				dbtestdata.TxidB1T1 + varuintToHex(1) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2) +
				dbtestdata.TxidB1T1 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr3, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T2A3) +
				dbtestdata.TxidB1T2 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A3),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr4, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T2A4) +
				dbtestdata.TxidB1T2 + varuintToHex(1) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A4),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr5, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T2A5) +
				dbtestdata.TxidB1T2 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A5),
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
				"000370d5",
				dbtestdata.TxidB1T1 + "00" + dbtestdata.TxidB1T2 + "00",
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

func verifyAfterBitcoinTypeBlock2(t *testing.T, d *RocksDB) {
	if err := checkColumn(d, cfHeight, []keyPair{
		{
			"000370d5",
			"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997" + uintToHex(1521515026) + varuintToHex(2) + varuintToHex(1234567),
			nil,
		},
		{
			"000370d6",
			"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6" + uintToHex(1521595678) + varuintToHex(4) + varuintToHex(2345678),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		{addressKeyHex(dbtestdata.Addr1, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr2, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{1, 2}), nil},
		{addressKeyHex(dbtestdata.Addr3, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr4, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{1}), nil},
		{addressKeyHex(dbtestdata.Addr5, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{2}), nil},
		{addressKeyHex(dbtestdata.Addr6, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{^0}) + txIndexesHex(dbtestdata.TxidB2T1, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr7, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{1}), nil},
		{addressKeyHex(dbtestdata.Addr8, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr9, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{1}), nil},
		{addressKeyHex(dbtestdata.Addr3, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{^0}), nil},
		{addressKeyHex(dbtestdata.Addr2, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{^1}), nil},
		{addressKeyHex(dbtestdata.Addr5, 225494, d), txIndexesHex(dbtestdata.TxidB2T3, []int32{0, ^0}), nil},
		{addressKeyHex(dbtestdata.AddrA, 225494, d), txIndexesHex(dbtestdata.TxidB2T4, []int32{0}), nil},
		{addressKeyHex(dbtestdata.Addr4, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{^1}), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfTxAddresses, []keyPair{
		{
			dbtestdata.TxidB1T1,
			varuintToHex(225493) +
				"00" +
				"03" +
				addressToPubKeyHexWithLength(dbtestdata.Addr1, t, d) + bigintToHex(dbtestdata.SatB1T1A1) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2) +
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2),
			nil,
		},
		{
			dbtestdata.TxidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5),
			nil,
		},
		{
			dbtestdata.TxidB2T1,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3) +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2) +
				"03" +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr6, t, d) + bigintToHex(dbtestdata.SatB2T1A6) +
				addressToPubKeyHexWithLength(dbtestdata.Addr7, t, d) + bigintToHex(dbtestdata.SatB2T1A7) +
				hex.EncodeToString([]byte{byte(len(dbtestdata.TxidB2T1Output3OpReturn))}) + dbtestdata.TxidB2T1Output3OpReturn + bigintToHex(dbtestdata.SatZero),
			nil,
		},
		{
			dbtestdata.TxidB2T2,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr6, t, d) + bigintToHex(dbtestdata.SatB2T1A6) +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4) +
				"02" +
				addressToPubKeyHexWithLength(dbtestdata.Addr8, t, d) + bigintToHex(dbtestdata.SatB2T2A8) +
				addressToPubKeyHexWithLength(dbtestdata.Addr9, t, d) + bigintToHex(dbtestdata.SatB2T2A9),
			nil,
		},
		{
			dbtestdata.TxidB2T3,
			varuintToHex(225494) +
				"01" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5) +
				"01" +
				addressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB2T3A5),
			nil,
		},
		{
			dbtestdata.TxidB2T4,
			varuintToHex(225494) +
				"01" + inputAddressToPubKeyHexWithLength("", t, d) + bigintToHex(dbtestdata.SatZero) +
				"02" +
				addressToPubKeyHexWithLength(dbtestdata.AddrA, t, d) + bigintToHex(dbtestdata.SatB2T4AA) +
				addressToPubKeyHexWithLength("", t, d) + bigintToHex(dbtestdata.SatZero),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr1, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB1T1A1) +
				dbtestdata.TxidB1T1 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A1),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr2, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T1A2) + bigintToHex(dbtestdata.SatB1T1A2) +
				dbtestdata.TxidB1T1 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr3, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A3) + bigintToHex(dbtestdata.SatZero),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr4, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A4) + bigintToHex(dbtestdata.SatZero),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr5, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A5) + bigintToHex(dbtestdata.SatB2T3A5) +
				dbtestdata.TxidB2T3 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T3A5),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr6, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB2T1A6) + bigintToHex(dbtestdata.SatZero),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr7, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB2T1A7) +
				dbtestdata.TxidB2T1 + varuintToHex(1) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T1A7),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr8, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB2T2A8) +
				dbtestdata.TxidB2T2 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T2A8),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr9, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB2T2A9) +
				dbtestdata.TxidB2T2 + varuintToHex(1) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T2A9),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrA, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero) + bigintToHex(dbtestdata.SatB2T4AA) +
				dbtestdata.TxidB2T4 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T4AA),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{
		{
			"000370d6",
			dbtestdata.TxidB2T1 + "02" + dbtestdata.TxidB1T2 + "00" + dbtestdata.TxidB1T1 + "02" +
				dbtestdata.TxidB2T2 + "02" + dbtestdata.TxidB2T1 + "00" + dbtestdata.TxidB1T2 + "02" +
				dbtestdata.TxidB2T3 + "01" + dbtestdata.TxidB1T2 + "04" +
				dbtestdata.TxidB2T4 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
}

type txidIndex struct {
	txid  string
	index int32
}

func verifyGetTransactions(t *testing.T, d *RocksDB, addr string, low, high uint32, wantTxids []txidIndex, wantErr error) {
	gotTxids := make([]txidIndex, 0)
	addToTxids := func(txid string, height uint32, indexes []int32) error {
		for _, index := range indexes {
			gotTxids = append(gotTxids, txidIndex{txid, index})
		}
		return nil
	}
	if err := d.GetTransactions(addr, low, high, addToTxids); err != nil {
		if wantErr == nil || wantErr.Error() != err.Error() {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(gotTxids, wantTxids) {
		t.Errorf("GetTransactions() = %v, want %v", gotTxids, wantTxids)
	}
}

// override PackTx and UnpackTx to default BaseParser functionality
// BitcoinParser uses tx hex which is not available for the test transactions
func (p *testBitcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.BaseParser.PackTx(tx, height, blockTime)
}

func (p *testBitcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.BaseParser.UnpackTx(buf)
}

func testTxCache(t *testing.T, d *RocksDB, b *bchain.Block, tx *bchain.Tx) {
	if err := d.PutTx(tx, b.Height, tx.Blocktime); err != nil {
		t.Fatal(err)
	}
	gtx, height, err := d.GetTx(tx.Txid)
	if err != nil {
		t.Fatal(err)
	}
	if b.Height != height {
		t.Fatalf("GetTx: got height %v, expected %v", height, b.Height)
	}
	// Confirmations are not stored in the DB, set them from input tx
	gtx.Confirmations = tx.Confirmations
	if !reflect.DeepEqual(gtx, tx) {
		t.Errorf("GetTx: %+v, want %+v", gtx, tx)
	}
	if err := d.DeleteTx(tx.Txid); err != nil {
		t.Fatal(err)
	}
}

// TestRocksDB_Index_BitcoinType is an integration test probing the whole indexing functionality for BitcoinType chains
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for various addresses / low-high ranges
// 3) GetBestBlock, GetBlockHash
// 4) Test tx caching functionality
// 5) Disconnect the block 2 using BlockTxs column
// 6) Reconnect block 2 and check
// After each step, the content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_BitcoinType(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	if len(d.is.BlockTimes) != 0 {
		t.Fatal("Expecting is.BlockTimes 0, got ", len(d.is.BlockTimes))
	}

	// connect 1st block - will log warnings about missing UTXO transactions in txAddresses column
	block1 := dbtestdata.GetTestBitcoinTypeBlock1(d.chainParser)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterBitcoinTypeBlock1(t, d, false)

	if len(d.is.BlockTimes) != 1 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// connect 2nd block - use some outputs from the 1st block as the inputs and 1 input uses tx from the same block
	block2 := dbtestdata.GetTestBitcoinTypeBlock2(d.chainParser)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterBitcoinTypeBlock2(t, d)

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// get transactions for various addresses / low-high ranges
	verifyGetTransactions(t, d, dbtestdata.Addr2, 0, 1000000, []txidIndex{
		{dbtestdata.TxidB2T1, ^1},
		{dbtestdata.TxidB1T1, 1},
		{dbtestdata.TxidB1T1, 2},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.Addr2, 225493, 225493, []txidIndex{
		{dbtestdata.TxidB1T1, 1},
		{dbtestdata.TxidB1T1, 2},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.Addr2, 225494, 1000000, []txidIndex{
		{dbtestdata.TxidB2T1, ^1},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.Addr2, 500000, 1000000, []txidIndex{}, nil)
	verifyGetTransactions(t, d, dbtestdata.Addr8, 0, 1000000, []txidIndex{
		{dbtestdata.TxidB2T2, 0},
	}, nil)
	verifyGetTransactions(t, d, dbtestdata.Addr6, 0, 1000000, []txidIndex{
		{dbtestdata.TxidB2T2, ^0},
		{dbtestdata.TxidB2T1, 0},
	}, nil)
	verifyGetTransactions(t, d, "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eBad", 500000, 1000000, []txidIndex{}, errors.New("checksum mismatch"))

	// GetBestBlock
	height, hash, err := d.GetBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	if height != 225494 {
		t.Fatalf("GetBestBlock: got height %v, expected %v", height, 225494)
	}
	if hash != "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6" {
		t.Fatalf("GetBestBlock: got hash %v, expected %v", hash, "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6")
	}

	// GetBlockHash
	hash, err = d.GetBlockHash(225493)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997" {
		t.Fatalf("GetBlockHash: got hash %v, expected %v", hash, "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997")
	}

	// Not connected block
	hash, err = d.GetBlockHash(225495)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("GetBlockHash: got hash '%v', expected ''", hash)
	}

	// GetBlockHash
	info, err := d.GetBlockInfo(225494)
	if err != nil {
		t.Fatal(err)
	}
	iw := &BlockInfo{
		Hash:   "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
		Txs:    4,
		Size:   2345678,
		Time:   1521595678,
		Height: 225494,
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
	// check that there is only the last tx in the cache
	packedTx, err := d.chainParser.PackTx(&block2.Txs[1], block2.Height, block2.Txs[1].Blocktime)
	if err := checkColumn(d, cfTransactions, []keyPair{
		{block2.Txs[1].Txid, hex.EncodeToString(packedTx), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	// try to disconnect both blocks, however only the last one is kept, it is not possible
	err = d.DisconnectBlockRangeBitcoinType(225493, 225494)
	if err == nil || err.Error() != "Cannot disconnect blocks with height 225493 and lower. It is necessary to rebuild index." {
		t.Fatal(err)
	}
	verifyAfterBitcoinTypeBlock2(t, d)

	// disconnect the 2nd block, verify that the db contains only data from the 1st block with restored unspentTxs
	// and that the cached tx is removed
	err = d.DisconnectBlockRangeBitcoinType(225494, 225494)
	if err != nil {
		t.Fatal(err)
	}
	verifyAfterBitcoinTypeBlock1(t, d, true)
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
	verifyAfterBitcoinTypeBlock2(t, d)

	if len(d.is.BlockTimes) != 2 {
		t.Fatal("Expecting is.BlockTimes 1, got ", len(d.is.BlockTimes))
	}

	// test public methods for address balance and tx addresses
	ab, err := d.GetAddressBalance(dbtestdata.Addr5, AddressBalanceDetailUTXO)
	if err != nil {
		t.Fatal(err)
	}
	abw := &AddrBalance{
		Txs:        2,
		SentSat:    *dbtestdata.SatB1T2A5,
		BalanceSat: *dbtestdata.SatB2T3A5,
		Utxos: []Utxo{
			{
				BtxID:    hexToBytes(dbtestdata.TxidB2T3),
				Vout:     0,
				Height:   225494,
				ValueSat: *dbtestdata.SatB2T3A5,
			},
		},
	}
	if !reflect.DeepEqual(ab, abw) {
		t.Errorf("GetAddressBalance() = %+v, want %+v", ab, abw)
	}
	rs := ab.ReceivedSat()
	rsw := dbtestdata.SatB1T2A5.Add(dbtestdata.SatB1T2A5, dbtestdata.SatB2T3A5)
	if rs.Cmp(rsw) != 0 {
		t.Errorf("GetAddressBalance().ReceivedSat() = %v, want %v", rs, rsw)
	}

	ta, err := d.GetTxAddresses(dbtestdata.TxidB2T1)
	if err != nil {
		t.Fatal(err)
	}
	taw := &TxAddresses{
		Height: 225494,
		Inputs: []TxInput{
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr3, d.chainParser),
				ValueSat: *dbtestdata.SatB1T2A3,
			},
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr2, d.chainParser),
				ValueSat: *dbtestdata.SatB1T1A2,
			},
		},
		Outputs: []TxOutput{
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr6, d.chainParser),
				Spent:    true,
				ValueSat: *dbtestdata.SatB2T1A6,
			},
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr7, d.chainParser),
				Spent:    false,
				ValueSat: *dbtestdata.SatB2T1A7,
			},
			{
				AddrDesc: hexToBytes(dbtestdata.TxidB2T1Output3OpReturn),
				Spent:    false,
				ValueSat: *dbtestdata.SatZero,
			},
		},
	}
	if !reflect.DeepEqual(ta, taw) {
		t.Errorf("GetTxAddresses() = %+v, want %+v", ta, taw)
	}
	ia, _, err := ta.Inputs[0].Addresses(d.chainParser)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ia, []string{dbtestdata.Addr3}) {
		t.Errorf("GetTxAddresses().Inputs[0].Addresses() = %v, want %v", ia, []string{dbtestdata.Addr3})
	}

}

func Test_BulkConnect_BitcoinType(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
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

	if err := bc.ConnectBlock(dbtestdata.GetTestBitcoinTypeBlock1(d.chainParser), false); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := bc.ConnectBlock(dbtestdata.GetTestBitcoinTypeBlock2(d.chainParser), true); err != nil {
		t.Fatal(err)
	}

	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}

	if d.is.DbState != common.DbStateOpen {
		t.Fatal("DB not in DbStateOpen")
	}

	verifyAfterBitcoinTypeBlock2(t, d)

	if len(d.is.BlockTimes) != 225495 {
		t.Fatal("Expecting is.BlockTimes 225495, got ", len(d.is.BlockTimes))
	}
}

func Test_BlockFilter_GetAndStore(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	blockHash := "0000000000000003d0c9722718f8ee86c2cf394f9cd458edb1c854de2a7b1a91"
	blockFilter := "042c6340895e413d8a811fa0"
	blockFilterBytes, _ := hex.DecodeString(blockFilter)

	// Empty at the beginning
	got, err := d.GetBlockFilter(blockHash)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	if got != want {
		t.Fatalf("GetBlockFilter(%s) = %s, want %s", blockHash, got, want)
	}

	// Store the filter
	wb := grocksdb.NewWriteBatch()
	if err := d.storeBlockFilter(wb, blockHash, blockFilterBytes); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}

	// Get the filter
	got, err = d.GetBlockFilter(blockHash)
	if err != nil {
		t.Fatal(err)
	}
	want = blockFilter
	if got != want {
		t.Fatalf("GetBlockFilter(%s) = %s, want %s", blockHash, got, want)
	}
}

func Test_packBigint_unpackBigint(t *testing.T) {
	bigbig1, _ := big.NewInt(0).SetString("123456789123456789012345", 10)
	bigbig2, _ := big.NewInt(0).SetString("12345678912345678901234512389012345123456789123456789012345123456789123456789012345", 10)
	bigbigbig := big.NewInt(0)
	bigbigbig.Mul(bigbig2, bigbig2)
	bigbigbig.Mul(bigbigbig, bigbigbig)
	bigbigbig.Mul(bigbigbig, bigbigbig)
	tests := []struct {
		name      string
		bi        *big.Int
		buf       []byte
		toobiglen int
	}{
		{
			name: "0",
			bi:   big.NewInt(0),
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name: "1",
			bi:   big.NewInt(1),
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name: "54321",
			bi:   big.NewInt(54321),
			buf:  make([]byte, 249),
		},
		{
			name: "12345678",
			bi:   big.NewInt(12345678),
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name: "123456789123456789",
			bi:   big.NewInt(123456789123456789),
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name: "bigbig1",
			bi:   bigbig1,
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name: "bigbig2",
			bi:   bigbig2,
			buf:  make([]byte, maxPackedBigintBytes),
		},
		{
			name:      "bigbigbig",
			bi:        bigbigbig,
			buf:       make([]byte, maxPackedBigintBytes),
			toobiglen: 242,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// packBigint
			got := packBigint(tt.bi, tt.buf)
			if tt.toobiglen == 0 {
				// create buffer that we expect
				bb := tt.bi.Bytes()
				want := append([]byte(nil), byte(len(bb)))
				want = append(want, bb...)
				if got != len(want) {
					t.Errorf("packBigint() = %v, want %v", got, len(want))
				}
				for i := 0; i < got; i++ {
					if tt.buf[i] != want[i] {
						t.Errorf("packBigint() buf = %v, want %v", tt.buf[:got], want)
						break
					}
				}
				// unpackBigint
				got1, got2 := unpackBigint(tt.buf)
				if got2 != len(want) {
					t.Errorf("unpackBigint() = %v, want %v", got2, len(want))
				}
				if tt.bi.Cmp(&got1) != 0 {
					t.Errorf("unpackBigint() = %v, want %v", got1, tt.bi)
				}
			} else {
				if got != tt.toobiglen {
					t.Errorf("packBigint() = %v, want toobiglen %v", got, tt.toobiglen)
				}
			}
		})
	}
}

func addressToAddrDesc(addr string, parser bchain.BlockChainParser) []byte {
	b, err := parser.GetAddrDescFromAddress(addr)
	if err != nil {
		panic(err)
	}
	return b
}

func Test_packTxAddresses_unpackTxAddresses(t *testing.T) {
	parser := bitcoinTestnetParser()
	tests := []struct {
		name    string
		hex     string
		data    *TxAddresses
		rocksDB *RocksDB
	}{
		{
			name: "1",
			hex:  "7b0216001443aac20a116e09ea4f7914be1c55e4c17aa600b70016001454633aa8bd2e552bd4e89c01e73c1b7905eb58460811207cb68a199872012d001443aac20a116e09ea4f7914be1c55e4c17aa600b70101",
			data: &TxAddresses{
				Height: 123,
				Inputs: []TxInput{
					{
						AddrDesc: addressToAddrDesc("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(0),
					},
					{
						AddrDesc: addressToAddrDesc("tb1q233n429a9e2jh48gnsq7w0qm0yz7kkzx0qczw8", parser),
						ValueSat: *big.NewInt(1234123421342341234),
					},
				},
				Outputs: []TxOutput{
					{
						AddrDesc: addressToAddrDesc("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(1),
						Spent:    true,
					},
				},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: false},
		},
		{
			name: "2",
			hex:  "e0390317a9149eb21980dc9d413d8eac27314938b9da920ee53e8705021918f2c017a91409f70b896169c37981d2b54b371df0d81a136a2c870501dd7e28c017a914e371782582a4addb541362c55565d2cdf56f6498870501a1e35ec0052fa9141d9ca71efa36d814424ea6ca1437e67287aebe348705012aadcac02ea91424fbc77cdc62702ade74dcf989c15e5d3f9240bc870501664894c02fa914afbfb74ee994c7d45f6698738bc4226d065266f7870501a1e35ec03276a914d2a37ce20ac9ec4f15dd05a7c6e8e9fbdb99850e88ac043b9943603376a9146b2044146a4438e6e5bfbc65f147afeb64d14fbb88ac05012a05f200",
			data: &TxAddresses{
				Height: 12345,
				Inputs: []TxInput{
					{
						AddrDesc: addressToAddrDesc("2N7iL7AvS4LViugwsdjTB13uN4T7XhV1bCP", parser),
						ValueSat: *big.NewInt(9011000000),
					},
					{
						AddrDesc: addressToAddrDesc("2Mt9v216YiNBAzobeNEzd4FQweHrGyuRHze", parser),
						ValueSat: *big.NewInt(8011000000),
					},
					{
						AddrDesc: addressToAddrDesc("2NDyqJpHvHnqNtL1F9xAeCWMAW8WLJmEMyD", parser),
						ValueSat: *big.NewInt(7011000000),
					},
				},
				Outputs: []TxOutput{
					{
						AddrDesc: addressToAddrDesc("2MuwoFGwABMakU7DCpdGDAKzyj2nTyRagDP", parser),
						ValueSat: *big.NewInt(5011000000),
						Spent:    true,
					},
					{
						AddrDesc: addressToAddrDesc("2Mvcmw7qkGXNWzkfH1EjvxDcNRGL1Kf2tEM", parser),
						ValueSat: *big.NewInt(6011000000),
					},
					{
						AddrDesc: addressToAddrDesc("2N9GVuX3XJGHS5MCdgn97gVezc6EgvzikTB", parser),
						ValueSat: *big.NewInt(7011000000),
						Spent:    true,
					},
					{
						AddrDesc: addressToAddrDesc("mzii3fuRSpExMLJEHdHveW8NmiX8MPgavk", parser),
						ValueSat: *big.NewInt(999900000),
					},
					{
						AddrDesc: addressToAddrDesc("mqHPFTRk23JZm9W1ANuEFtwTYwxjESSgKs", parser),
						ValueSat: *big.NewInt(5000000000),
						Spent:    true,
					},
				},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: false},
		},
		{
			name: "empty address",
			hex:  "baef9a1501000204d2020002162e010162",
			data: &TxAddresses{
				Height: 123456789,
				Inputs: []TxInput{
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(1234),
					},
				},
				Outputs: []TxOutput{
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(5678),
					},
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(98),
						Spent:    true,
					},
				},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: false},
		},
		{
			name: "empty",
			hex:  "000000",
			data: &TxAddresses{
				Inputs:  []TxInput{},
				Outputs: []TxOutput{},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: false},
		},
		{
			name: "extendedIndex 1",
			hex:  "e0398241032ea9149eb21980dc9d413d8eac27314938b9da920ee53e8705021918f2c0c50c7ce2f5670fd52de738288299bd854a85ef1bb304f62f35ced1bd49a8a810002ea91409f70b896169c37981d2b54b371df0d81a136a2c870501dd7e28c0e96672c7fcc8da131427fcea7e841028614813496a56c11e8a6185c16861c495012ea914e371782582a4addb541362c55565d2cdf56f6498870501a1e35ec0ed308c72f9804dfeefdbb483ef8fd1e638180ad81d6b33f4b58d36d19162fa6d8106052fa9141d9ca71efa36d814424ea6ca1437e67287aebe348705012aadcac000b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa38400081ce8685592ea91424fbc77cdc62702ade74dcf989c15e5d3f9240bc870501664894c02fa914afbfb74ee994c7d45f6698738bc4226d065266f7870501a1e35ec0effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75ef17a1f4233276a914d2a37ce20ac9ec4f15dd05a7c6e8e9fbdb99850e88ac043b9943603376a9146b2044146a4438e6e5bfbc65f147afeb64d14fbb88ac05012a05f2007c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25a9956d8396f32a",
			data: &TxAddresses{
				Height: 12345,
				VSize:  321,
				Inputs: []TxInput{
					{
						AddrDesc: addressToAddrDesc("2N7iL7AvS4LViugwsdjTB13uN4T7XhV1bCP", parser),
						ValueSat: *big.NewInt(9011000000),
						Txid:     "c50c7ce2f5670fd52de738288299bd854a85ef1bb304f62f35ced1bd49a8a810",
						Vout:     0,
					},
					{
						AddrDesc: addressToAddrDesc("2Mt9v216YiNBAzobeNEzd4FQweHrGyuRHze", parser),
						ValueSat: *big.NewInt(8011000000),
						Txid:     "e96672c7fcc8da131427fcea7e841028614813496a56c11e8a6185c16861c495",
						Vout:     1,
					},
					{
						AddrDesc: addressToAddrDesc("2NDyqJpHvHnqNtL1F9xAeCWMAW8WLJmEMyD", parser),
						ValueSat: *big.NewInt(7011000000),
						Txid:     "ed308c72f9804dfeefdbb483ef8fd1e638180ad81d6b33f4b58d36d19162fa6d",
						Vout:     134,
					},
				},
				Outputs: []TxOutput{
					{
						AddrDesc:    addressToAddrDesc("2MuwoFGwABMakU7DCpdGDAKzyj2nTyRagDP", parser),
						ValueSat:    *big.NewInt(5011000000),
						Spent:       true,
						SpentTxid:   dbtestdata.TxidB1T1,
						SpentIndex:  0,
						SpentHeight: 432112345,
					},
					{
						AddrDesc: addressToAddrDesc("2Mvcmw7qkGXNWzkfH1EjvxDcNRGL1Kf2tEM", parser),
						ValueSat: *big.NewInt(6011000000),
					},
					{
						AddrDesc:    addressToAddrDesc("2N9GVuX3XJGHS5MCdgn97gVezc6EgvzikTB", parser),
						ValueSat:    *big.NewInt(7011000000),
						Spent:       true,
						SpentTxid:   dbtestdata.TxidB1T2,
						SpentIndex:  14231,
						SpentHeight: 555555,
					},
					{
						AddrDesc: addressToAddrDesc("mzii3fuRSpExMLJEHdHveW8NmiX8MPgavk", parser),
						ValueSat: *big.NewInt(999900000),
					},
					{
						AddrDesc:    addressToAddrDesc("mqHPFTRk23JZm9W1ANuEFtwTYwxjESSgKs", parser),
						ValueSat:    *big.NewInt(5000000000),
						Spent:       true,
						SpentTxid:   dbtestdata.TxidB2T1,
						SpentIndex:  674541,
						SpentHeight: 6666666,
					},
				},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: true},
		},
		{
			name: "extendedIndex empty address",
			hex:  "baef9a152d01010204d2020002162e010162fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db03e039",
			data: &TxAddresses{
				Height: 123456789,
				VSize:  45,
				Inputs: []TxInput{
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(1234),
					},
				},
				Outputs: []TxOutput{
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(5678),
					},
					{
						AddrDesc:    []byte(nil),
						ValueSat:    *big.NewInt(98),
						Spent:       true,
						SpentTxid:   dbtestdata.TxidB2T4,
						SpentIndex:  3,
						SpentHeight: 12345,
					},
				},
			},
			rocksDB: &RocksDB{chainParser: parser, extendedIndex: true},
		},
	}
	varBuf := make([]byte, maxPackedBigintBytes)
	buf := make([]byte, 1024)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.rocksDB.packTxAddresses(tt.data, buf, varBuf)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("packTxAddresses() = %v, want %v", hex, tt.hex)
			}
			got1, err := tt.rocksDB.unpackTxAddresses(b)
			if err != nil {
				t.Errorf("unpackTxAddresses() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got1, tt.data) {
				t.Errorf("unpackTxAddresses() = %+v, want %+v", got1, tt.data)
			}
		})
	}
}

func Test_packAddrBalance_unpackAddrBalance(t *testing.T) {
	parser := bitcoinTestnetParser()
	tests := []struct {
		name string
		hex  string
		data *AddrBalance
	}{
		{
			name: "no utxos",
			hex:  "7b060b44cc1af8520514faf980ac",
			data: &AddrBalance{
				BalanceSat: *big.NewInt(90110001324),
				SentSat:    *big.NewInt(12390110001234),
				Txs:        123,
				Utxos:      []Utxo{},
			},
		},
		{
			name: "utxos",
			hex:  "7b060b44cc1af8520514faf980ac00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa38400c87c440060b2fd12177a6effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac750098faf659010105e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b0782c6df6d84ccd88552087e9cba87a275ffff",
			data: &AddrBalance{
				BalanceSat: *big.NewInt(90110001324),
				SentSat:    *big.NewInt(12390110001234),
				Txs:        123,
				Utxos: []Utxo{
					{
						BtxID:    hexToBytes(dbtestdata.TxidB1T1),
						Vout:     12,
						Height:   123456,
						ValueSat: *big.NewInt(12390110001234 - 90110001324),
					},
					{
						BtxID:    hexToBytes(dbtestdata.TxidB1T2),
						Vout:     0,
						Height:   52345689,
						ValueSat: *big.NewInt(1),
					},
					{
						BtxID:    hexToBytes(dbtestdata.TxidB2T3),
						Vout:     5353453,
						Height:   1234567890,
						ValueSat: *big.NewInt(9123372036854775807),
					},
				},
			},
		},
		{
			name: "empty",
			hex:  "000000",
			data: &AddrBalance{
				Utxos: []Utxo{},
			},
		},
	}
	varBuf := make([]byte, maxPackedBigintBytes)
	buf := make([]byte, 32)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := packAddrBalance(tt.data, buf, varBuf)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("packTxAddresses() = %v, want %v", hex, tt.hex)
			}
			got1, err := unpackAddrBalance(b, parser.PackedTxidLen(), AddressBalanceDetailUTXO)
			if err != nil {
				t.Errorf("unpackTxAddresses() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got1, tt.data) {
				t.Errorf("unpackTxAddresses() = %+v, want %+v", got1, tt.data)
			}
		})
	}
}

func createUtxoMap(ab *AddrBalance) {
	l := len(ab.Utxos)
	ab.utxosMap = make(map[string]int, 32)
	for i := 0; i < l; i++ {
		s := string(ab.Utxos[i].BtxID)
		if _, e := ab.utxosMap[s]; !e {
			ab.utxosMap[s] = i
		}
	}
}
func TestAddrBalance_utxo_methods(t *testing.T) {
	ab := &AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
	}

	// addUtxo
	ab.addUtxo(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     1,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.addUtxo(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     4,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.addUtxo(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T2),
		Vout:     0,
		Height:   5001,
		ValueSat: *big.NewInt(800),
	})
	want := &AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
		Utxos: []Utxo{
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     1,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     4,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T2),
				Vout:     0,
				Height:   5001,
				ValueSat: *big.NewInt(800),
			},
		},
	}
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("addUtxo, got %+v, want %+v", ab, want)
	}

	// addUtxoInDisconnect
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB2T1),
		Vout:     0,
		Height:   5003,
		ValueSat: *big.NewInt(800),
	})
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB2T1),
		Vout:     1,
		Height:   5003,
		ValueSat: *big.NewInt(800),
	})
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     10,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     2,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     0,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	want = &AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
		Utxos: []Utxo{
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     0,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     1,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     2,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     4,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T1),
				Vout:     10,
				Height:   5000,
				ValueSat: *big.NewInt(100),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB1T2),
				Vout:     0,
				Height:   5001,
				ValueSat: *big.NewInt(800),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB2T1),
				Vout:     0,
				Height:   5003,
				ValueSat: *big.NewInt(800),
			},
			{
				BtxID:    hexToBytes(dbtestdata.TxidB2T1),
				Vout:     1,
				Height:   5003,
				ValueSat: *big.NewInt(800),
			},
		},
	}
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("addUtxoInDisconnect, got %+v, want %+v", ab, want)
	}

	// markUtxoAsSpent
	ab.markUtxoAsSpent(hexToBytes(dbtestdata.TxidB2T1), 0)
	want.Utxos[6].Vout = -1
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("markUtxoAsSpent, got %+v, want %+v", ab, want)
	}

	// addUtxo with utxosMap
	for i := 0; i < 20; i += 2 {
		utxo := Utxo{
			BtxID:    hexToBytes(dbtestdata.TxidB2T2),
			Vout:     int32(i),
			Height:   5009,
			ValueSat: *big.NewInt(800),
		}
		ab.addUtxo(&utxo)
		want.Utxos = append(want.Utxos, utxo)
	}
	createUtxoMap(want)
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("addUtxo with utxosMap, got %+v, want %+v", ab, want)
	}

	// markUtxoAsSpent with utxosMap
	ab.markUtxoAsSpent(hexToBytes(dbtestdata.TxidB2T1), 1)
	want.Utxos[7].Vout = -1
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("markUtxoAsSpent with utxosMap, got %+v, want %+v", ab, want)
	}

	// addUtxoInDisconnect with utxosMap
	ab.addUtxoInDisconnect(&Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     3,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	want.Utxos = append(want.Utxos, Utxo{})
	copy(want.Utxos[3+1:], want.Utxos[3:])
	want.Utxos[3] = Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     3,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	}
	want.utxosMap = nil
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("addUtxoInDisconnect with utxosMap, got %+v, want %+v", ab, want)
	}

}

func Test_reorderUtxo(t *testing.T) {
	utxos := []Utxo{
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T1),
			Vout:  3,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T1),
			Vout:  1,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T1),
			Vout:  0,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T2),
			Vout:  0,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T2),
			Vout:  2,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB1T2),
			Vout:  1,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB2T1),
			Vout:  2,
		},
		{
			BtxID: hexToBytes(dbtestdata.TxidB2T1),
			Vout:  0,
		},
	}
	tests := []struct {
		name  string
		utxos []Utxo
		index int
		want  []Utxo
	}{
		{
			name:  "middle",
			utxos: utxos,
			index: 4,
			want: []Utxo{
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  3,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  2,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  2,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  0,
				},
			},
		},
		{
			name:  "start",
			utxos: utxos,
			index: 1,
			want: []Utxo{
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  3,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  2,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  2,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  0,
				},
			},
		},
		{
			name:  "end",
			utxos: utxos,
			index: 6,
			want: []Utxo{
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T1),
					Vout:  3,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  1,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB1T2),
					Vout:  2,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  0,
				},
				{
					BtxID: hexToBytes(dbtestdata.TxidB2T1),
					Vout:  2,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reorderUtxo(tt.utxos, tt.index)
			if !reflect.DeepEqual(tt.utxos, tt.want) {
				t.Errorf("reorderUtxo %s, got %+v, want %+v", tt.name, tt.utxos, tt.want)
			}
		})
	}
}

func Test_packUnpackString(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "ahoj"},
		{name: ""},
		{name: "very long long very long long very long long very long long very long long very long long very long long very long long very long long very long long very long long very long long very long long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := packString(tt.name)
			if got, l := unpackString(buf); !reflect.DeepEqual(got, tt.name) || l != len(buf) {
				t.Errorf("Test_packUnpackString() = %v, want %v, len %d, want len %d", got, tt.name, l, len(buf))
			}
		})
	}
}

func TestRocksDB_packTxIndexes_unpackTxIndexes(t *testing.T) {
	type args struct {
		txi []txIndexes
	}
	tests := []struct {
		name string
		data []txIndexes
		hex  string
	}{
		{
			name: "1",
			data: []txIndexes{
				{
					btxID:   hexToBytes(dbtestdata.TxidB1T1),
					indexes: []int32{1},
				},
			},
			hex: "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa384006",
		},
		{
			name: "2",
			data: []txIndexes{
				{
					btxID:   hexToBytes(dbtestdata.TxidB1T1),
					indexes: []int32{-2, 1, 3, 1234, -53241},
				},
				{
					btxID:   hexToBytes(dbtestdata.TxidB1T2),
					indexes: []int32{-2, -1, 0, 1, 2, 3},
				},
			},
			hex: "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac7507030004080e00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa384007040ca6488cff61",
		},
		{
			name: "3",
			data: []txIndexes{
				{
					btxID:   hexToBytes(dbtestdata.TxidB2T1),
					indexes: []int32{-2, 1, 3},
				},
				{
					btxID:   hexToBytes(dbtestdata.TxidB1T1),
					indexes: []int32{-2, -1, 0, 1, 2, 3},
				},
				{
					btxID:   hexToBytes(dbtestdata.TxidB1T2),
					indexes: []int32{-2},
				},
			},
			hex: "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac750500b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa384007030004080e7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d2507040e",
		},
	}
	d := &RocksDB{
		chainParser: &testBitcoinParser{
			BitcoinParser: bitcoinTestnetParser(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := d.packTxIndexes(tt.data)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("packTxIndexes() = %v, want %v", hex, tt.hex)
			}
			got, err := d.unpackTxIndexes(b)
			if err != nil {
				t.Errorf("unpackTxIndexes() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, tt.data) {
				t.Errorf("unpackTxIndexes() = %+v, want %+v", got, tt.data)
			}
		})
	}
}
