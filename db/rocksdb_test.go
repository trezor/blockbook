// +build unittest

package db

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/tests/dbtestdata"
	"encoding/binary"
	"encoding/hex"
	"io/ioutil"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/juju/errors"
	"github.com/martinboehm/btcutil/chaincfg"
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
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewRocksDB(tmp, 100000, -1, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("coin-unittest")
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

func bigintToHex(i *big.Int, d *RocksDB) string {
	b := make([]byte, d.chainParser.MaxPackedBigintBytes())
	l := d.chainParser.PackBigint(i, b)
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

func txIndexesHex(tx string, indexes []int32, d *RocksDB) string {
	buf := make([]byte, vlq.MaxLen32)
	// type
	l := d.chainParser.PackVaruint(uint(bchain.BaseCoinMask), buf)
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
		if i >= len(kp) {
			return errors.Errorf("Expected less rows in column %v", cfNames[col])
		}
		key := hex.EncodeToString(it.Key().Data())
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
		{addressKeyHex(dbtestdata.Addr1, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr2, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{1, 2}, d), nil},
		{addressKeyHex(dbtestdata.Addr3, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr4, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.Addr5, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{2}, d), nil},
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
				addressToPubKeyHexWithLength(dbtestdata.Addr1, t, d) + bigintToHex(dbtestdata.SatB1T1A1, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2, d) + 
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2, d),
			nil,
		},
		{
			dbtestdata.TxidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				addressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5, d),
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
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T1A1, d) +
				dbtestdata.TxidB1T1 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr2, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T1A2Double, d) +
			dbtestdata.TxidB1T1 + varuintToHex(1) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2, d) +
			dbtestdata.TxidB1T1 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr3, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T2A3, d) +
				dbtestdata.TxidB1T2 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A3, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr4, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T2A4, d) +
				dbtestdata.TxidB1T2 + varuintToHex(1) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A4, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr5, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T2A5, d) +
				dbtestdata.TxidB1T2 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T2A5, d),
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
		{addressKeyHex(dbtestdata.Addr1, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr2, 225493, d), txIndexesHex(dbtestdata.TxidB1T1, []int32{1, 2}, d), nil},
		{addressKeyHex(dbtestdata.Addr3, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr4, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.Addr5, 225493, d), txIndexesHex(dbtestdata.TxidB1T2, []int32{2}, d), nil},
		{addressKeyHex(dbtestdata.Addr6, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{^0}, d) + txIndexesHex(dbtestdata.TxidB2T1, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr7, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.Addr8, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr9, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{1}, d), nil},
		{addressKeyHex(dbtestdata.Addr3, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{^0}, d), nil},
		{addressKeyHex(dbtestdata.Addr2, 225494, d), txIndexesHex(dbtestdata.TxidB2T1, []int32{^1}, d), nil},
		{addressKeyHex(dbtestdata.Addr5, 225494, d), txIndexesHex(dbtestdata.TxidB2T3, []int32{0, ^0}, d), nil},
		{addressKeyHex(dbtestdata.AddrA, 225494, d), txIndexesHex(dbtestdata.TxidB2T4, []int32{0}, d), nil},
		{addressKeyHex(dbtestdata.Addr4, 225494, d), txIndexesHex(dbtestdata.TxidB2T2, []int32{^1}, d), nil},
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
				addressToPubKeyHexWithLength(dbtestdata.Addr1, t, d) + bigintToHex(dbtestdata.SatB1T1A1, d) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2, d),
			nil,
		},
		{
			dbtestdata.TxidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3, d) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4, d) +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5, d),
			nil,
		},
		{
			dbtestdata.TxidB2T1,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr3, t, d) + bigintToHex(dbtestdata.SatB1T2A3, d) +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr2, t, d) + bigintToHex(dbtestdata.SatB1T1A2, d) +
				"03" +
				spentAddressToPubKeyHexWithLength(dbtestdata.Addr6, t, d) + bigintToHex(dbtestdata.SatB2T1A6, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr7, t, d) + bigintToHex(dbtestdata.SatB2T1A7, d) +
				hex.EncodeToString([]byte{byte(len(dbtestdata.TxidB2T1Output3OpReturn))}) + dbtestdata.TxidB2T1Output3OpReturn + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
		{
			dbtestdata.TxidB2T2,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr6, t, d) + bigintToHex(dbtestdata.SatB2T1A6, d) +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr4, t, d) + bigintToHex(dbtestdata.SatB1T2A4, d) +
				"02" +
				addressToPubKeyHexWithLength(dbtestdata.Addr8, t, d) + bigintToHex(dbtestdata.SatB2T2A8, d) +
				addressToPubKeyHexWithLength(dbtestdata.Addr9, t, d) + bigintToHex(dbtestdata.SatB2T2A9, d),
			nil,
		},
		{
			dbtestdata.TxidB2T3,
			varuintToHex(225494) +
				"01" +
				inputAddressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB1T2A5, d) +
				"01" +
				addressToPubKeyHexWithLength(dbtestdata.Addr5, t, d) + bigintToHex(dbtestdata.SatB2T3A5, d),
			nil,
		},
		{
			dbtestdata.TxidB2T4,
			varuintToHex(225494) +
				"01" + inputAddressToPubKeyHexWithLength("", t, d) + bigintToHex(dbtestdata.SatZero, d) +
				"02" +
				addressToPubKeyHexWithLength(dbtestdata.AddrA, t, d) + bigintToHex(dbtestdata.SatB2T4AA, d) +
				addressToPubKeyHexWithLength("", t, d) + bigintToHex(dbtestdata.SatZero, d),
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
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB1T1A1, d) +
				dbtestdata.TxidB1T1 + varuintToHex(0) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A1, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr2, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T1A2, d) + bigintToHex(dbtestdata.SatB1T1A2, d) +
			dbtestdata.TxidB1T1 + varuintToHex(2) + varuintToHex(225493) + bigintToHex(dbtestdata.SatB1T1A2, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr3, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A3, d) + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr4, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A4, d) + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr5, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB1T2A5, d) + bigintToHex(dbtestdata.SatB2T3A5, d) +
				dbtestdata.TxidB2T3 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T3A5, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr6, d.chainParser),
			"02" + bigintToHex(dbtestdata.SatB2T1A6, d) + bigintToHex(dbtestdata.SatZero, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr7, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB2T1A7, d) +
				dbtestdata.TxidB2T1 + varuintToHex(1) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T1A7, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr8, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB2T2A8, d) +
				dbtestdata.TxidB2T2 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T2A8, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.Addr9, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB2T2A9, d) +
				dbtestdata.TxidB2T2 + varuintToHex(1) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T2A9, d),
			nil,
		},
		{
			dbtestdata.AddressToPubKeyHex(dbtestdata.AddrA, d.chainParser),
			"01" + bigintToHex(dbtestdata.SatZero, d) + bigintToHex(dbtestdata.SatB2T4AA, d) +
				dbtestdata.TxidB2T4 + varuintToHex(0) + varuintToHex(225494) + bigintToHex(dbtestdata.SatB2T4AA, d),
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
		t.Errorf("GetTx: %v, want %v", gtx, tx)
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
	iw := &bchain.DbBlockInfo{
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
	ab, err := d.GetAddressBalance(dbtestdata.Addr5, bchain.AddressBalanceDetailUTXO)
	if err != nil {
		t.Fatal(err)
	}
	abw := &bchain.AddrBalance{
		Txs:        2,
		SentSat:    *dbtestdata.SatB1T2A5,
		BalanceSat: *dbtestdata.SatB2T3A5,
		Utxos: []bchain.Utxo{
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
	taw := &bchain.TxAddresses{
		Height: 225494,
		Inputs: []bchain.TxInput{
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr3, d.chainParser),
				ValueSat: *dbtestdata.SatB1T2A3,
			},
			{
				AddrDesc: addressToAddrDesc(dbtestdata.Addr2, d.chainParser),
				ValueSat: *dbtestdata.SatB1T1A2,
			},
		},
		Outputs: []bchain.TxOutput{
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

func Test_packBigint_unpackBigint(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)
	bigbig1, _ := big.NewInt(0).SetString("123456789123456789012345", 10)
	bigbig2, _ := big.NewInt(0).SetString("12345678912345678901234512389012345123456789123456789012345123456789123456789012345", 10)
	bigbigbig := big.NewInt(0)
	bigbigbig.Mul(bigbig2, bigbig2)
	bigbigbig.Mul(bigbigbig, bigbigbig)
	bigbigbig.Mul(bigbigbig, bigbigbig)
	maxPackedBigintBytes := d.chainParser.MaxPackedBigintBytes()
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
			// PackBigint
			got := d.chainParser.PackBigint(tt.bi, tt.buf)
			if tt.toobiglen == 0 {
				// create buffer that we expect
				bb := tt.bi.Bytes()
				want := append([]byte(nil), byte(len(bb)))
				want = append(want, bb...)
				if got != len(want) {
					t.Errorf("PackBigint() = %v, want %v", got, len(want))
				}
				for i := 0; i < got; i++ {
					if tt.buf[i] != want[i] {
						t.Errorf("PackBigint() buf = %v, want %v", tt.buf[:got], want)
						break
					}
				}
				// UnpackBigint
				got1, got2 := d.chainParser.UnpackBigint(tt.buf)
				if got2 != len(want) {
					t.Errorf("UnpackBigint() = %v, want %v", got2, len(want))
				}
				if tt.bi.Cmp(&got1) != 0 {
					t.Errorf("UnpackBigint() = %v, want %v", got1, tt.bi)
				}
			} else {
				if got != tt.toobiglen {
					t.Errorf("PackBigint() = %v, want toobiglen %v", got, tt.toobiglen)
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
		name string
		hex  string
		data *bchain.TxAddresses
	}{
		{
			name: "1",
			hex:  "7b0216001443aac20a116e09ea4f7914be1c55e4c17aa600b70016001454633aa8bd2e552bd4e89c01e73c1b7905eb58460811207cb68a199872012d001443aac20a116e09ea4f7914be1c55e4c17aa600b70101",
			data: &bchain.TxAddresses{
				Height: 123,
				Inputs: []bchain.TxInput{
					{
						AddrDesc: addressToAddrDesc("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(0),
					},
					{
						AddrDesc: addressToAddrDesc("tb1q233n429a9e2jh48gnsq7w0qm0yz7kkzx0qczw8", parser),
						ValueSat: *big.NewInt(1234123421342341234),
					},
				},
				Outputs: []bchain.TxOutput{
					{
						AddrDesc: addressToAddrDesc("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(1),
						Spent:    true,
					},
				},
			},
		},
		{
			name: "2",
			hex:  "e0390317a9149eb21980dc9d413d8eac27314938b9da920ee53e8705021918f2c017a91409f70b896169c37981d2b54b371df0d81a136a2c870501dd7e28c017a914e371782582a4addb541362c55565d2cdf56f6498870501a1e35ec0052fa9141d9ca71efa36d814424ea6ca1437e67287aebe348705012aadcac02ea91424fbc77cdc62702ade74dcf989c15e5d3f9240bc870501664894c02fa914afbfb74ee994c7d45f6698738bc4226d065266f7870501a1e35ec03276a914d2a37ce20ac9ec4f15dd05a7c6e8e9fbdb99850e88ac043b9943603376a9146b2044146a4438e6e5bfbc65f147afeb64d14fbb88ac05012a05f200",
			data: &bchain.TxAddresses{
				Height: 12345,
				Inputs: []bchain.TxInput{
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
				Outputs: []bchain.TxOutput{
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
		},
		{
			name: "empty address",
			hex:  "baef9a1501000204d2020002162e010162",
			data: &bchain.TxAddresses{
				Height: 123456789,
				Inputs: []bchain.TxInput{
					{
						AddrDesc: []byte(nil),
						ValueSat: *big.NewInt(1234),
					},
				},
				Outputs: []bchain.TxOutput{
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
		},
		{
			name: "empty",
			hex:  "000000",
			data: &bchain.TxAddresses{
				Inputs:  []bchain.TxInput{},
				Outputs: []bchain.TxOutput{},
			},
		},
	}
	varBuf := make([]byte, parser.MaxPackedBigintBytes())
	buf := make([]byte, 1024)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := parser.PackTxAddresses(tt.data, buf, varBuf)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("PackTxAddresses() = %v, want %v", hex, tt.hex)
			}
			got1, err := parser.UnpackTxAddresses(b)
			if err != nil {
				t.Errorf("UnpackTxAddresses() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got1, tt.data) {
				t.Errorf("UnpackTxAddresses() = %+v, want %+v", got1, tt.data)
			}
		})
	}
}

func Test_packAddrBalance_unpackAddrBalance(t *testing.T) {
	parser := bitcoinTestnetParser()
	tests := []struct {
		name string
		hex  string
		data *bchain.AddrBalance
	}{
		{
			name: "no utxos",
			hex:  "7b060b44cc1af8520514faf980ac",
			data: &bchain.AddrBalance{
				BalanceSat: *big.NewInt(90110001324),
				SentSat:    *big.NewInt(12390110001234),
				Txs:        123,
				Utxos:      []bchain.Utxo{},
			},
		},
		{
			name: "utxos",
			hex:  "7b060b44cc1af8520514faf980ac00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa38400c87c440060b2fd12177a6effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac750098faf659010105e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b0782c6df6d84ccd88552087e9cba87a275ffff",
			data: &bchain.AddrBalance{
				BalanceSat: *big.NewInt(90110001324),
				SentSat:    *big.NewInt(12390110001234),
				Txs:        123,
				Utxos: []bchain.Utxo{
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
			data: &bchain.AddrBalance{
				Utxos: []bchain.Utxo{},
			},
		},
	}
	varBuf := make([]byte, parser.MaxPackedBigintBytes())
	buf := make([]byte, 32)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := parser.PackAddrBalance(tt.data, buf, varBuf)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("PackTxAddresses() = %v, want %v", hex, tt.hex)
			}
			got1, err := parser.UnpackAddrBalance(b, parser.PackedTxidLen(), bchain.AddressBalanceDetailUTXO)
			if err != nil {
				t.Errorf("UnpackTxAddresses() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got1, tt.data) {
				t.Errorf("UnpackTxAddresses() = %+v, want %+v", got1, tt.data)
			}
		})
	}
}

func createUtxoMap(ab *bchain.AddrBalance) {
	l := len(ab.Utxos)
	ab.UtxosMap = make(map[string]int, 32)
	for i := 0; i < l; i++ {
		s := string(ab.Utxos[i].BtxID)
		if _, e := ab.UtxosMap[s]; !e {
			ab.UtxosMap[s] = i
		}
	}
}
func TestAddrBalance_utxo_methods(t *testing.T) {
	ab := &bchain.AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
	}

	// AddUtxo
	ab.AddUtxo(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     1,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.AddUtxo(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     4,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.AddUtxo(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T2),
		Vout:     0,
		Height:   5001,
		ValueSat: *big.NewInt(800),
	})
	want := &bchain.AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
		Utxos: []bchain.Utxo{
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

	// AddUtxoInDisconnect
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB2T1),
		Vout:     0,
		Height:   5003,
		ValueSat: *big.NewInt(800),
	})
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB2T1),
		Vout:     1,
		Height:   5003,
		ValueSat: *big.NewInt(800),
	})
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     10,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     2,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     0,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	want = &bchain.AddrBalance{
		Txs:        10,
		SentSat:    *big.NewInt(10000),
		BalanceSat: *big.NewInt(1000),
		Utxos: []bchain.Utxo{
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
		t.Errorf("AddUtxoInDisconnect, got %+v, want %+v", ab, want)
	}

	// MarkUtxoAsSpent
	ab.MarkUtxoAsSpent(hexToBytes(dbtestdata.TxidB2T1), 0)
	want.Utxos[6].Vout = -1
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("MarkUtxoAsSpent, got %+v, want %+v", ab, want)
	}

	// addUtxo with UtxosMap
	for i := 0; i < 20; i += 2 {
		utxo := bchain.Utxo{
			BtxID:    hexToBytes(dbtestdata.TxidB2T2),
			Vout:     int32(i),
			Height:   5009,
			ValueSat: *big.NewInt(800),
		}
		ab.AddUtxo(&utxo)
		want.Utxos = append(want.Utxos, utxo)
	}
	createUtxoMap(want)
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("addUtxo with UtxosMap, got %+v, want %+v", ab, want)
	}

	// MarkUtxoAsSpent with UtxosMap
	ab.MarkUtxoAsSpent(hexToBytes(dbtestdata.TxidB2T1), 1)
	want.Utxos[7].Vout = -1
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("MarkUtxoAsSpent with UtxosMap, got %+v, want %+v", ab, want)
	}

	// AddUtxoInDisconnect with UtxosMap
	ab.AddUtxoInDisconnect(&bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     3,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	})
	want.Utxos = append(want.Utxos, bchain.Utxo{})
	copy(want.Utxos[3+1:], want.Utxos[3:])
	want.Utxos[3] = bchain.Utxo{
		BtxID:    hexToBytes(dbtestdata.TxidB1T1),
		Vout:     3,
		Height:   5000,
		ValueSat: *big.NewInt(100),
	}
	want.UtxosMap = nil
	if !reflect.DeepEqual(ab, want) {
		t.Errorf("AddUtxoInDisconnect with UtxosMap, got %+v, want %+v", ab, want)
	}

}

func Test_reorderUtxo(t *testing.T) {
	utxos := []bchain.Utxo{
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
		utxos []bchain.Utxo
		index int
		want  []bchain.Utxo
	}{
		{
			name:  "middle",
			utxos: utxos,
			index: 4,
			want: []bchain.Utxo{
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
			want: []bchain.Utxo{
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
			want: []bchain.Utxo{
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

func TestRocksTickers(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// Test valid formats
	for _, date := range []string{"20190130", "2019013012", "201901301250", "20190130125030"} {
		_, err := FiatRatesConvertDate(date)
		if err != nil {
			t.Errorf("%v", err)
		}
	}

	// Test invalid formats
	for _, date := range []string{"01102019", "10201901", "", "abc", "20190130xxx"} {
		_, err := FiatRatesConvertDate(date)
		if err == nil {
			t.Errorf("Wrongly-formatted date \"%v\" marked as valid!", date)
		}
	}

	// Test storing & finding tickers
	key, _ := time.Parse(FiatRatesTimeFormat, "20190627000000")
	futureKey, _ := time.Parse(FiatRatesTimeFormat, "20190630000000")

	ts1, _ := time.Parse(FiatRatesTimeFormat, "20190628000000")
	ticker1 := &CurrencyRatesTicker{
		Timestamp: &ts1,
		Rates: map[string]float64{
			"usd": 20000,
		},
	}

	ts2, _ := time.Parse(FiatRatesTimeFormat, "20190629000000")
	ticker2 := &CurrencyRatesTicker{
		Timestamp: &ts2,
		Rates: map[string]float64{
			"usd": 30000,
		},
	}
	err := d.FiatRatesStoreTicker(ticker1)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}
	d.FiatRatesStoreTicker(ticker2)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}

	ticker, err := d.FiatRatesFindTicker(&key) // should find the closest key (ticker1)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker1.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker() // should find the last key (ticker2)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker2.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindTicker(&futureKey) // should not find anything
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker != nil {
		t.Errorf("Ticker found, but the timestamp is older than the last ticker entry.")
	}
}
