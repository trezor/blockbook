// build unittest

package db

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
)

// simplified explanation of signed varint packing, used in many index data structures
// for number n, the packing is: 2*n if n>=0 else 2*(-n)-1
// takes only 1 byte if abs(n)<127

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
	d, err := NewRocksDB(tmp, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("btc-testnet")
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

func addressToPubKeyHex(addr string, t *testing.T, d *RocksDB) string {
	if addr == "" {
		return ""
	}
	b, err := d.chainParser.AddressToOutputScript(addr)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func inputAddressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := addressToPubKeyHex(addr, t, d)
	return hex.EncodeToString([]byte{byte(len(h) / 2)}) + h
}

func addressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := addressToPubKeyHex(addr, t, d)
	return hex.EncodeToString([]byte{byte(len(h))}) + h
}

func spentAddressToPubKeyHexWithLength(addr string, t *testing.T, d *RocksDB) string {
	h := addressToPubKeyHex(addr, t, d)
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

const (
	txidB1T1 = "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840"
	txidB1T2 = "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"
	txidB2T1 = "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25"
	txidB2T2 = "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71"
	txidB2T3 = "05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"
	txidB2T4 = "fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"

	addr1 = "mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"  // 76a914010d39800f86122416e28f485029acf77507169288ac
	addr2 = "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"  // 76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac
	addr3 = "mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"  // 76a914a08eae93007f22668ab5e4a9c83c8cd1c325e3e088ac
	addr4 = "2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS" // a9144a21db08fb6882cb152e1ff06780a430740f770487
	addr5 = "2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1" // a914e921fc4912a315078f370d959f2c4f7b6d2a683c87
	addr6 = "mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"  // 76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac
	addr7 = "mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"  // 76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac
	addr8 = "mwwoKQE5Lb1G4picHSHDQKg8jw424PF9SC"  // 76a914b434eb0c1a3b7a02e8a29cc616e791ef1e0bf51f88ac
	addr9 = "mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"  // 76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac
	addrA = "mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj"  // 76a914d03c0d863d189b23b061a95ad32940b65837609f88ac
)

var (
	satZero   = big.NewInt(0)
	satB1T1A1 = big.NewInt(100000000)
	satB1T1A2 = big.NewInt(12345)
	satB1T2A3 = big.NewInt(1234567890123)
	satB1T2A4 = big.NewInt(1)
	satB1T2A5 = big.NewInt(9876)
	satB2T1A6 = big.NewInt(317283951061)
	satB2T1A7 = big.NewInt(917283951061)
	satB2T2A8 = big.NewInt(118641975500)
	satB2T2A9 = big.NewInt(198641975500)
	satB2T3A5 = big.NewInt(9000)
	satB2T4AA = big.NewInt(1360030331)
)

func getTestUTXOBlock1(t *testing.T, d *RocksDB) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height: 225493,
			Hash:   "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997",
			Size:   1234567,
			Time:   1534858021,
		},
		Txs: []bchain.Tx{
			bchain.Tx{
				Txid: txidB1T1,
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr1, t, d),
						},
						ValueSat: *satB1T1A1,
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr2, t, d),
						},
						ValueSat: *satB1T1A2,
					},
				},
				Blocktime: 22549300000,
				Time:      22549300000,
			},
			bchain.Tx{
				Txid: txidB1T2,
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr3, t, d),
						},
						ValueSat: *satB1T2A3,
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr4, t, d),
						},
						ValueSat: *satB1T2A4,
					},
					bchain.Vout{
						N: 2,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr5, t, d),
						},
						ValueSat: *satB1T2A5,
					},
				},
				Blocktime: 22549300001,
				Time:      22549300001,
			},
		},
	}
}

func getTestUTXOBlock2(t *testing.T, d *RocksDB) *bchain.Block {
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height: 225494,
			Hash:   "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
			Size:   2345678,
			Time:   1534859123,
		},
		Txs: []bchain.Tx{
			bchain.Tx{
				Txid: txidB2T1,
				Vin: []bchain.Vin{
					// addr3
					bchain.Vin{
						Txid: txidB1T2,
						Vout: 0,
					},
					// addr2
					bchain.Vin{
						Txid: txidB1T1,
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr6, t, d),
						},
						ValueSat: *satB2T1A6,
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr7, t, d),
						},
						ValueSat: *satB2T1A7,
					},
				},
				Blocktime: 22549400000,
				Time:      22549400000,
			},
			bchain.Tx{
				Txid: txidB2T2,
				Vin: []bchain.Vin{
					// spending an output in the same block - addr6
					bchain.Vin{
						Txid: txidB2T1,
						Vout: 0,
					},
					// spending an output in the previous block - addr4
					bchain.Vin{
						Txid: txidB1T2,
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr8, t, d),
						},
						ValueSat: *satB2T2A8,
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr9, t, d),
						},
						ValueSat: *satB2T2A9,
					},
				},
				Blocktime: 22549400001,
				Time:      22549400001,
			},
			// transaction from the same address in the previous block
			bchain.Tx{
				Txid: txidB2T3,
				Vin: []bchain.Vin{
					// addr5
					bchain.Vin{
						Txid: txidB1T2,
						Vout: 2,
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addr5, t, d),
						},
						ValueSat: *satB2T3A5,
					},
				},
				Blocktime: 22549400002,
				Time:      22549400002,
			},
			// mining transaction
			bchain.Tx{
				Txid: txidB2T4,
				Vin: []bchain.Vin{
					bchain.Vin{
						Coinbase: "03bf1e1504aede765b726567696f6e312f50726f6a65637420425443506f6f6c2f01000001bf7e000000000000",
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex(addrA, t, d),
						},
						ValueSat: *satB2T4AA,
					},
					bchain.Vout{
						N:            1,
						ScriptPubKey: bchain.ScriptPubKey{},
						ValueSat:     *satZero,
					},
				},
				Blocktime: 22549400003,
				Time:      22549400003,
			},
		},
	}
}

func verifyAfterUTXOBlock1(t *testing.T, d *RocksDB, afterDisconnect bool) {
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{
			"000370d5",
			"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997" + uintToHex(1534858021) + varuintToHex(2) + varuintToHex(1234567),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for non negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{addressToPubKeyHex(addr1, t, d) + "000370d5", txidB1T1 + "00", nil},
		keyPair{addressToPubKeyHex(addr2, t, d) + "000370d5", txidB1T1 + "02", nil},
		keyPair{addressToPubKeyHex(addr3, t, d) + "000370d5", txidB1T2 + "00", nil},
		keyPair{addressToPubKeyHex(addr4, t, d) + "000370d5", txidB1T2 + "02", nil},
		keyPair{addressToPubKeyHex(addr5, t, d) + "000370d5", txidB1T2 + "04", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfTxAddresses, []keyPair{
		keyPair{
			txidB1T1,
			varuintToHex(225493) +
				"00" +
				"02" +
				addressToPubKeyHexWithLength(addr1, t, d) + bigintToHex(satB1T1A1) +
				addressToPubKeyHexWithLength(addr2, t, d) + bigintToHex(satB1T1A2),
			nil,
		},
		keyPair{
			txidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				addressToPubKeyHexWithLength(addr3, t, d) + bigintToHex(satB1T2A3) +
				addressToPubKeyHexWithLength(addr4, t, d) + bigintToHex(satB1T2A4) +
				addressToPubKeyHexWithLength(addr5, t, d) + bigintToHex(satB1T2A5),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		keyPair{addressToPubKeyHex(addr1, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T1A1), nil},
		keyPair{addressToPubKeyHex(addr2, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T1A2), nil},
		keyPair{addressToPubKeyHex(addr3, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T2A3), nil},
		keyPair{addressToPubKeyHex(addr4, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T2A4), nil},
		keyPair{addressToPubKeyHex(addr5, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T2A5), nil},
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
				"000370d5",
				txidB1T1 + "00" + txidB1T2 + "00",
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

func verifyAfterUTXOBlock2(t *testing.T, d *RocksDB) {
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{
			"000370d5",
			"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997" + uintToHex(1534858021) + varuintToHex(2) + varuintToHex(1234567),
			nil,
		},
		keyPair{
			"000370d6",
			"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6" + uintToHex(1534859123) + varuintToHex(4) + varuintToHex(2345678),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{addressToPubKeyHex(addr1, t, d) + "000370d5", txidB1T1 + "00", nil},
		keyPair{addressToPubKeyHex(addr2, t, d) + "000370d5", txidB1T1 + "02", nil},
		keyPair{addressToPubKeyHex(addr3, t, d) + "000370d5", txidB1T2 + "00", nil},
		keyPair{addressToPubKeyHex(addr4, t, d) + "000370d5", txidB1T2 + "02", nil},
		keyPair{addressToPubKeyHex(addr5, t, d) + "000370d5", txidB1T2 + "04", nil},
		keyPair{addressToPubKeyHex(addr6, t, d) + "000370d6", txidB2T1 + "00" + txidB2T2 + "01", nil},
		keyPair{addressToPubKeyHex(addr7, t, d) + "000370d6", txidB2T1 + "02", nil},
		keyPair{addressToPubKeyHex(addr8, t, d) + "000370d6", txidB2T2 + "00", nil},
		keyPair{addressToPubKeyHex(addr9, t, d) + "000370d6", txidB2T2 + "02", nil},
		keyPair{addressToPubKeyHex(addr3, t, d) + "000370d6", txidB2T1 + "01", nil},
		keyPair{addressToPubKeyHex(addr2, t, d) + "000370d6", txidB2T1 + "03", nil},
		keyPair{addressToPubKeyHex(addr5, t, d) + "000370d6", txidB2T3 + "00" + txidB2T3 + "01", nil},
		keyPair{addressToPubKeyHex(addrA, t, d) + "000370d6", txidB2T4 + "00", nil},
		keyPair{addressToPubKeyHex(addr4, t, d) + "000370d6", txidB2T2 + "03", nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfTxAddresses, []keyPair{
		keyPair{
			txidB1T1,
			varuintToHex(225493) +
				"00" +
				"02" +
				addressToPubKeyHexWithLength(addr1, t, d) + bigintToHex(satB1T1A1) +
				spentAddressToPubKeyHexWithLength(addr2, t, d) + bigintToHex(satB1T1A2),
			nil,
		},
		keyPair{
			txidB1T2,
			varuintToHex(225493) +
				"00" +
				"03" +
				spentAddressToPubKeyHexWithLength(addr3, t, d) + bigintToHex(satB1T2A3) +
				spentAddressToPubKeyHexWithLength(addr4, t, d) + bigintToHex(satB1T2A4) +
				spentAddressToPubKeyHexWithLength(addr5, t, d) + bigintToHex(satB1T2A5),
			nil,
		},
		keyPair{
			txidB2T1,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(addr3, t, d) + bigintToHex(satB1T2A3) +
				inputAddressToPubKeyHexWithLength(addr2, t, d) + bigintToHex(satB1T1A2) +
				"02" +
				spentAddressToPubKeyHexWithLength(addr6, t, d) + bigintToHex(satB2T1A6) +
				addressToPubKeyHexWithLength(addr7, t, d) + bigintToHex(satB2T1A7),
			nil,
		},
		keyPair{
			txidB2T2,
			varuintToHex(225494) +
				"02" +
				inputAddressToPubKeyHexWithLength(addr6, t, d) + bigintToHex(satB2T1A6) +
				inputAddressToPubKeyHexWithLength(addr4, t, d) + bigintToHex(satB1T2A4) +
				"02" +
				addressToPubKeyHexWithLength(addr8, t, d) + bigintToHex(satB2T2A8) +
				addressToPubKeyHexWithLength(addr9, t, d) + bigintToHex(satB2T2A9),
			nil,
		},
		keyPair{
			txidB2T3,
			varuintToHex(225494) +
				"01" +
				inputAddressToPubKeyHexWithLength(addr5, t, d) + bigintToHex(satB1T2A5) +
				"01" +
				addressToPubKeyHexWithLength(addr5, t, d) + bigintToHex(satB2T3A5),
			nil,
		},
		keyPair{
			txidB2T4,
			varuintToHex(225494) +
				"01" + inputAddressToPubKeyHexWithLength("", t, d) + bigintToHex(satZero) +
				"02" +
				addressToPubKeyHexWithLength(addrA, t, d) + bigintToHex(satB2T4AA) +
				addressToPubKeyHexWithLength("", t, d) + bigintToHex(satZero),
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddressBalance, []keyPair{
		keyPair{addressToPubKeyHex(addr1, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB1T1A1), nil},
		keyPair{addressToPubKeyHex(addr2, t, d), "02" + bigintToHex(satB1T1A2) + bigintToHex(satZero), nil},
		keyPair{addressToPubKeyHex(addr3, t, d), "02" + bigintToHex(satB1T2A3) + bigintToHex(satZero), nil},
		keyPair{addressToPubKeyHex(addr4, t, d), "02" + bigintToHex(satB1T2A4) + bigintToHex(satZero), nil},
		keyPair{addressToPubKeyHex(addr5, t, d), "02" + bigintToHex(satB1T2A5) + bigintToHex(satB2T3A5), nil},
		keyPair{addressToPubKeyHex(addr6, t, d), "02" + bigintToHex(satB2T1A6) + bigintToHex(satZero), nil},
		keyPair{addressToPubKeyHex(addr7, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB2T1A7), nil},
		keyPair{addressToPubKeyHex(addr8, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB2T2A8), nil},
		keyPair{addressToPubKeyHex(addr9, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB2T2A9), nil},
		keyPair{addressToPubKeyHex(addrA, t, d), "01" + bigintToHex(satZero) + bigintToHex(satB2T4AA), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{
		keyPair{
			"000370d6",
			txidB2T1 + "02" + txidB1T2 + "00" + txidB1T1 + "02" +
				txidB2T2 + "02" + txidB2T1 + "00" + txidB1T2 + "02" +
				txidB2T3 + "01" + txidB1T2 + "04" +
				txidB2T4 + "01" + "0000000000000000000000000000000000000000000000000000000000000000" + "00",
			nil,
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
}

type txidVoutOutput struct {
	txid     string
	vout     uint32
	isOutput bool
}

func verifyGetTransactions(t *testing.T, d *RocksDB, addr string, low, high uint32, wantTxids []txidVoutOutput, wantErr error) {
	gotTxids := make([]txidVoutOutput, 0)
	addToTxids := func(txid string, vout uint32, isOutput bool) error {
		gotTxids = append(gotTxids, txidVoutOutput{txid, vout, isOutput})
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

type testBitcoinParser struct {
	*btc.BitcoinParser
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
	if fmt.Sprint(gtx) != fmt.Sprint(tx) {
		t.Errorf("GetTx: %v, want %v", gtx, tx)
	}
	if err := d.DeleteTx(tx.Txid); err != nil {
		t.Fatal(err)
	}
}

// TestRocksDB_Index_UTXO is an integration test probing the whole indexing functionality for UTXO chains
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for various addresses / low-high ranges
// 3) GetBestBlock, GetBlockHash
// 4) Test tx caching functionality
// 5) Disconnect block 2 - expect error
// 6) Disconnect the block 2 using BlockTxs column
// 7) Reconnect block 2 and check
// After each step, the content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_UTXO(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// connect 1st block - will log warnings about missing UTXO transactions in txAddresses column
	block1 := getTestUTXOBlock1(t, d)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock1(t, d, false)

	// connect 2nd block - use some outputs from the 1st block as the inputs and 1 input uses tx from the same block
	block2 := getTestUTXOBlock2(t, d)
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock2(t, d)

	// get transactions for various addresses / low-high ranges
	verifyGetTransactions(t, d, addr2, 0, 1000000, []txidVoutOutput{
		txidVoutOutput{txidB1T1, 1, true},
		txidVoutOutput{txidB2T1, 1, false},
	}, nil)
	verifyGetTransactions(t, d, addr2, 225493, 225493, []txidVoutOutput{
		txidVoutOutput{txidB1T1, 1, true},
	}, nil)
	verifyGetTransactions(t, d, addr2, 225494, 1000000, []txidVoutOutput{
		txidVoutOutput{txidB2T1, 1, false},
	}, nil)
	verifyGetTransactions(t, d, addr2, 500000, 1000000, []txidVoutOutput{}, nil)
	verifyGetTransactions(t, d, addr8, 0, 1000000, []txidVoutOutput{
		txidVoutOutput{txidB2T2, 0, true},
	}, nil)
	verifyGetTransactions(t, d, "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eBad", 500000, 1000000, []txidVoutOutput{}, errors.New("checksum mismatch"))

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
		Txid: "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
		Txs:  4,
		Size: 2345678,
		Time: time.Unix(1534859123, 0),
	}
	if !reflect.DeepEqual(info, iw) {
		t.Errorf("GetAddressBalance() = %+v, want %+v", info, iw)
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
		keyPair{block2.Txs[1].Txid, hex.EncodeToString(packedTx), nil},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	// DisconnectBlock for UTXO chains is not possible
	err = d.DisconnectBlock(block2)
	if err == nil || err.Error() != "DisconnectBlock is not supported for UTXO chains" {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock2(t, d)

	// try to disconnect both blocks, however only the last one is kept, it is not possible
	err = d.DisconnectBlockRangeUTXO(225493, 225494)
	if err == nil || err.Error() != "Cannot disconnect blocks with height 225493 and lower. It is necessary to rebuild index." {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock2(t, d)

	// disconnect the 2nd block, verify that the db contains only data from the 1st block with restored unspentTxs
	// and that the cached tx is removed
	err = d.DisconnectBlockRangeUTXO(225494, 225494)
	if err != nil {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock1(t, d, true)
	if err := checkColumn(d, cfTransactions, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	// connect block again and verify the state of db
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	verifyAfterUTXOBlock2(t, d)

	// test public methods for address balance and tx addresses

	ab, err := d.GetAddressBalance(addr5)
	if err != nil {
		t.Fatal(err)
	}
	abw := &AddrBalance{
		Txs:        2,
		SentSat:    *satB1T2A5,
		BalanceSat: *satB2T3A5,
	}
	if !reflect.DeepEqual(ab, abw) {
		t.Errorf("GetAddressBalance() = %+v, want %+v", ab, abw)
	}
	rs := ab.ReceivedSat()
	rsw := satB1T2A5.Add(satB1T2A5, satB2T3A5)
	if rs.Cmp(rsw) != 0 {
		t.Errorf("GetAddressBalance().ReceivedSat() = %v, want %v", rs, rsw)
	}

	ta, err := d.GetTxAddresses(txidB2T1)
	if err != nil {
		t.Fatal(err)
	}
	taw := &TxAddresses{
		Height: 225494,
		Inputs: []TxInput{
			{
				addrID:   addressToOutput(addr3, d.chainParser),
				ValueSat: *satB1T2A3,
			},
			{
				addrID:   addressToOutput(addr2, d.chainParser),
				ValueSat: *satB1T1A2,
			},
		},
		Outputs: []TxOutput{
			{
				addrID:   addressToOutput(addr6, d.chainParser),
				Spent:    true,
				ValueSat: *satB2T1A6,
			},
			{
				addrID:   addressToOutput(addr7, d.chainParser),
				Spent:    false,
				ValueSat: *satB2T1A7,
			},
		},
	}
	if !reflect.DeepEqual(ta, taw) {
		t.Errorf("GetTxAddresses() = %+v, want %+v", ta, taw)
	}
	ia, err := ta.Inputs[0].Addresses(d.chainParser)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ia, []string{addr3}) {
		t.Errorf("GetTxAddresses().Inputs[0].Addresses() = %v, want %v", ia, []string{addr3})
	}

}

func Test_BulkConnect_UTXO(t *testing.T) {
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

	if err := bc.ConnectBlock(getTestUTXOBlock1(t, d), false); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfBlockTxs, []keyPair{}); err != nil {
		{
			t.Fatal(err)
		}
	}

	if err := bc.ConnectBlock(getTestUTXOBlock2(t, d), true); err != nil {
		t.Fatal(err)
	}

	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}

	if d.is.DbState != common.DbStateOpen {
		t.Fatal("DB not in DbStateOpen")
	}

	verifyAfterUTXOBlock2(t, d)
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

func addressToOutput(addr string, parser bchain.BlockChainParser) []byte {
	b, err := parser.AddressToOutputScript(addr)
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
		data *TxAddresses
	}{
		{
			name: "1",
			hex:  "7b0216001443aac20a116e09ea4f7914be1c55e4c17aa600b70016001454633aa8bd2e552bd4e89c01e73c1b7905eb58460811207cb68a199872012d001443aac20a116e09ea4f7914be1c55e4c17aa600b70101",
			data: &TxAddresses{
				Height: 123,
				Inputs: []TxInput{
					{
						addrID:   addressToOutput("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(0),
					},
					{
						addrID:   addressToOutput("tb1q233n429a9e2jh48gnsq7w0qm0yz7kkzx0qczw8", parser),
						ValueSat: *big.NewInt(1234123421342341234),
					},
				},
				Outputs: []TxOutput{
					{
						addrID:   addressToOutput("tb1qgw4vyzs3dcy75nmezjlpc40yc9a2vq9hghdyt2", parser),
						ValueSat: *big.NewInt(1),
						Spent:    true,
					},
				},
			},
		},
		{
			name: "2",
			hex:  "e0390317a9149eb21980dc9d413d8eac27314938b9da920ee53e8705021918f2c017a91409f70b896169c37981d2b54b371df0d81a136a2c870501dd7e28c017a914e371782582a4addb541362c55565d2cdf56f6498870501a1e35ec0052fa9141d9ca71efa36d814424ea6ca1437e67287aebe348705012aadcac02ea91424fbc77cdc62702ade74dcf989c15e5d3f9240bc870501664894c02fa914afbfb74ee994c7d45f6698738bc4226d065266f7870501a1e35ec03276a914d2a37ce20ac9ec4f15dd05a7c6e8e9fbdb99850e88ac043b9943603376a9146b2044146a4438e6e5bfbc65f147afeb64d14fbb88ac05012a05f200",
			data: &TxAddresses{
				Height: 12345,
				Inputs: []TxInput{
					{
						addrID:   addressToOutput("2N7iL7AvS4LViugwsdjTB13uN4T7XhV1bCP", parser),
						ValueSat: *big.NewInt(9011000000),
					},
					{
						addrID:   addressToOutput("2Mt9v216YiNBAzobeNEzd4FQweHrGyuRHze", parser),
						ValueSat: *big.NewInt(8011000000),
					},
					{
						addrID:   addressToOutput("2NDyqJpHvHnqNtL1F9xAeCWMAW8WLJmEMyD", parser),
						ValueSat: *big.NewInt(7011000000),
					},
				},
				Outputs: []TxOutput{
					{
						addrID:   addressToOutput("2MuwoFGwABMakU7DCpdGDAKzyj2nTyRagDP", parser),
						ValueSat: *big.NewInt(5011000000),
						Spent:    true,
					},
					{
						addrID:   addressToOutput("2Mvcmw7qkGXNWzkfH1EjvxDcNRGL1Kf2tEM", parser),
						ValueSat: *big.NewInt(6011000000),
					},
					{
						addrID:   addressToOutput("2N9GVuX3XJGHS5MCdgn97gVezc6EgvzikTB", parser),
						ValueSat: *big.NewInt(7011000000),
						Spent:    true,
					},
					{
						addrID:   addressToOutput("mzii3fuRSpExMLJEHdHveW8NmiX8MPgavk", parser),
						ValueSat: *big.NewInt(999900000),
					},
					{
						addrID:   addressToOutput("mqHPFTRk23JZm9W1ANuEFtwTYwxjESSgKs", parser),
						ValueSat: *big.NewInt(5000000000),
						Spent:    true,
					},
				},
			},
		},
		{
			name: "empty address",
			hex:  "baef9a1501000204d2020002162e010162",
			data: &TxAddresses{
				Height: 123456789,
				Inputs: []TxInput{
					{
						addrID:   []byte{},
						ValueSat: *big.NewInt(1234),
					},
				},
				Outputs: []TxOutput{
					{
						addrID:   []byte{},
						ValueSat: *big.NewInt(5678),
					},
					{
						addrID:   []byte{},
						ValueSat: *big.NewInt(98),
						Spent:    true,
					},
				},
			},
		},
		{
			name: "empty",
			hex:  "000000",
			data: &TxAddresses{
				Inputs:  []TxInput{},
				Outputs: []TxOutput{},
			},
		},
	}
	varBuf := make([]byte, maxPackedBigintBytes)
	buf := make([]byte, 1024)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := packTxAddresses(tt.data, buf, varBuf)
			hex := hex.EncodeToString(b)
			if !reflect.DeepEqual(hex, tt.hex) {
				t.Errorf("packTxAddresses() = %v, want %v", hex, tt.hex)
			}
			got1, err := unpackTxAddresses(b)
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
