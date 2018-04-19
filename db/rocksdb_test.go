package db

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/juju/errors"
)

func setupRocksDB(t *testing.T, p bchain.BlockChainParser) *RocksDB {
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewRocksDB(tmp, p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func closeAnddestroyRocksDB(t *testing.T, d *RocksDB) {
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(d.path)
}

func addressToPubKeyHex(addr string, t *testing.T, d *RocksDB) string {
	b, err := d.chainParser.AddressToOutputScript(addr)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func addressToPubKeyHexWithLenght(addr string, t *testing.T, d *RocksDB) string {
	h := addressToPubKeyHex(addr, t, d)
	// length is signed varint, therefore 2 times big, we can take len(h) as the correct value
	return strconv.FormatInt(int64(len(h)), 16) + h
}

type keyPair struct {
	Key, Value string
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
			return errors.Errorf("Expected less rows in column %v", col)
		}
		key := hex.EncodeToString(it.Key().Data())
		if key != kp[i].Key {
			return errors.Errorf("Incorrect key %v found in column %v row %v, expecting %v", key, col, i, kp[i].Key)
		}
		val := hex.EncodeToString(it.Value().Data())
		if val != kp[i].Value {
			return errors.Errorf("Incorrect value %v found in column %v row %v, expecting %v", val, col, i, kp[i].Value)
		}
		i++
	}
	if i != len(kp) {
		return errors.Errorf("Expected more rows in column %v: found %v, expected %v", col, i, len(kp))
	}
	return nil
}

// TestRocksDB_Index_UTXO is a composite test testing the whole indexing functionality for UTXO chains
// It does the following:
// 1) Connect two blocks (inputs from 2nd block are spending some outputs from the 1st block)
// 2) GetTransactions for known addresses
// 3) Disconnect block 2
// 4) GetTransactions for known addresses
// 5) Connect the block 2 back
// After each step, the whole content of DB is examined and any difference against expected state is regarded as failure
func TestRocksDB_Index_UTXO(t *testing.T) {
	d := setupRocksDB(t, &btc.BitcoinParser{Params: btc.GetChainParams("test")})
	defer closeAnddestroyRocksDB(t, d)

	// connect 1st block - will log warnings about missing UTXO transactions in cfUnspentTxs column
	block1 := bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height: 225493,
			Hash:   "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997",
		},
		Txs: []bchain.Tx{
			bchain.Tx{
				Txid: "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti", t, d),
						},
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", t, d),
						},
					},
				},
			},
			bchain.Tx{
				Txid: "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75",
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw", t, d),
						},
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS", t, d),
						},
					},
				},
			},
		},
	}
	if err := d.ConnectBlock(&block1); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{"000370d5", "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997"},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	// the vout is encoded as signed varint, i.e. value * 2 for non negative values
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{addressToPubKeyHex("mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti", t, d) + "000370d5", "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" + "00"},
		keyPair{addressToPubKeyHex("mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", t, d) + "000370d5", "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" + "02"},
		keyPair{addressToPubKeyHex("mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw", t, d) + "000370d5", "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" + "00"},
		keyPair{addressToPubKeyHex("2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS", t, d) + "000370d5", "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" + "02"},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfUnspentTxs, []keyPair{
		keyPair{
			"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
			addressToPubKeyHexWithLenght("mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti", t, d) + "00" + addressToPubKeyHexWithLenght("mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", t, d) + "02",
		},
		keyPair{
			"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75",
			addressToPubKeyHexWithLenght("mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw", t, d) + "00" + addressToPubKeyHexWithLenght("2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS", t, d) + "02",
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}

	// connect 2nd block - use some outputs from the 1st block as the inputs and 1 input uses tx from the same block
	block2 := bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Height: 225494,
			Hash:   "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
		},
		Txs: []bchain.Tx{
			bchain.Tx{
				Txid: "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25",
				Vin: []bchain.Vin{
					bchain.Vin{
						Txid: "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75",
						Vout: 0,
					},
					bchain.Vin{
						Txid: "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX", t, d),
						},
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL", t, d),
						},
					},
				},
			},
			bchain.Tx{
				Txid: "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71",
				Vin: []bchain.Vin{
					bchain.Vin{
						Txid: "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25",
						Vout: 0,
					},
					bchain.Vin{
						Txid: "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75",
						Vout: 1,
					},
				},
				Vout: []bchain.Vout{
					bchain.Vout{
						N: 0,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mwwoKQE5Lb1G4picHSHDQKg8jw424PF9SC", t, d),
						},
					},
					bchain.Vout{
						N: 1,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: addressToPubKeyHex("mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP", t, d),
						},
					},
				},
			},
		},
	}
	if err := d.ConnectBlock(&block2); err != nil {
		t.Fatal(err)
	}
	if err := checkColumn(d, cfHeight, []keyPair{
		keyPair{"000370d5", "0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997"},
		keyPair{"000370d6", "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfAddresses, []keyPair{
		keyPair{addressToPubKeyHex("mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti", t, d) + "000370d5", "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" + "00"},
		keyPair{addressToPubKeyHex("mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", t, d) + "000370d5", "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" + "02"},
		keyPair{addressToPubKeyHex("mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw", t, d) + "000370d5", "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" + "00"},
		keyPair{addressToPubKeyHex("2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS", t, d) + "000370d5", "effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" + "02"},
		keyPair{addressToPubKeyHex("mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX", t, d) + "000370d6", "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" + "00" + "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" + "01"},
		keyPair{addressToPubKeyHex("mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL", t, d) + "000370d6", "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" + "02"},
		keyPair{addressToPubKeyHex("mwwoKQE5Lb1G4picHSHDQKg8jw424PF9SC", t, d) + "000370d6", "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" + "00"},
		keyPair{addressToPubKeyHex("mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP", t, d) + "000370d6", "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" + "02"},
		keyPair{addressToPubKeyHex("mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw", t, d) + "000370d6", "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" + "01"},
		keyPair{addressToPubKeyHex("mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", t, d) + "000370d6", "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" + "03"},
		keyPair{addressToPubKeyHex("2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS", t, d) + "000370d6", "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" + "03"},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
	if err := checkColumn(d, cfUnspentTxs, []keyPair{
		keyPair{
			"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
			addressToPubKeyHexWithLenght("mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti", t, d) + "00",
		},
		keyPair{
			"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25",
			addressToPubKeyHexWithLenght("mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL", t, d) + "02",
		},
		keyPair{
			"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71",
			addressToPubKeyHexWithLenght("mwwoKQE5Lb1G4picHSHDQKg8jw424PF9SC", t, d) + "00" + addressToPubKeyHexWithLenght("mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP", t, d) + "02",
		},
	}); err != nil {
		{
			t.Fatal(err)
		}
	}
}
