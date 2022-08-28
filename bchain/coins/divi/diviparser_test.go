//go:build unittest

package divi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

// Test getting the address details from the address hash

func Test_GetAddrDescFromAddress_Mainnet(t *testing.T) {
	type args struct {
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "P2PKH1",
			args:    args{address: "DDSsBchWiVfvPVn6Ldp1nL7k4L77cSDqM7"},
			want:    "76a9145b1d583a4c270f2f14be77b298f0a9c6df97471388ac",
			wantErr: false,
		},
	}
	parser := NewDiviParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

func Test_GetAddressesFromAddrDesc(t *testing.T) {
	type args struct {
		script string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		want2   bool
		wantErr bool
	}{
		{
			name:    "Normal",
			args:    args{script: "76a914cb1196fb1b98d04b0cb8d2ffde3c2de3eb83d9fe88ac"},
			want:    []string{"DPepnMkaNHKCa6cQi7oBThrdiFEwSSYFzv"},
			want2:   true,
			wantErr: false,
		},
	}

	parser := NewDiviParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got2, tt.want2)
			}
		})
	}
}

// Test the packing and unpacking of raw transaction data

var (
	// Mint transaction
	testTx1       bchain.Tx
	testTxPacked1 = "0a20f7a5324866ba18058ab032196f34458d19f7ec5a4ac284670c3ef07bfa724644124201000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0603de3d060101ffffffff010000000000000000000000000018aefd9ce90528defb1832140a0c30336465336430363031303128ffffffff0f3a00"

	// Normal transaction
	testTx2       bchain.Tx
	testTxPacked2 = "0a20eace41778a2940ff423b72a42033990eb5d6092810734a5806da6f3e5b34086412ea010100000001084b029489e1cddf726080c447c8a2b1d4bbe43024db31b8b19bc07585db9555010000006a473044022017422b9e3414d6233fa75f9eb7778469bebbb40686b0f7eb77d90a04c80149610220411f1063086fe205ea821ceb0de89e8158e202aba00f5ebb92b51f97381311fd012102ccb10a2f0603a0624b8708abefb5f4700631fc131c5de38b51e0359e2ffa7d1cffffffff03000000000000000000f260de1a580100001976a9145b1d583a4c270f2f14be77b298f0a9c6df97471388ac009ca6920c0000001976a914cb1196fb1b98d04b0cb8d2ffde3c2de3eb83d9fe88ac0000000018aefd9ce90528defb1832960112205595db8575c09bb1b831db2430e4bbd4b1a2c847c4806072dfcde18994024b081801226a473044022017422b9e3414d6233fa75f9eb7778469bebbb40686b0f7eb77d90a04c80149610220411f1063086fe205ea821ceb0de89e8158e202aba00f5ebb92b51f97381311fd012102ccb10a2f0603a0624b8708abefb5f4700631fc131c5de38b51e0359e2ffa7d1c28ffffffff0f3a003a490a0601581ade60f210011a1976a9145b1d583a4c270f2f14be77b298f0a9c6df97471388ac222244445373426368576956667650566e364c6470316e4c376b344c3737635344714d373a480a050c92a69c0010021a1976a914cb1196fb1b98d04b0cb8d2ffde3c2de3eb83d9fe88ac2222445065706e4d6b614e484b436136635169376f425468726469464577535359467a76"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0603de3d060101ffffffff0100000000000000000000000000",
		Txid:     "f7a5324866ba18058ab032196f34458d19f7ec5a4ac284670c3ef07bfa724644",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				Coinbase: "03de3d060101",
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "",
				},
			},
		},
		Blocktime: 1562853038,
		Time:      1562853038,
	}

	testTx2 = bchain.Tx{
		Hex:      "0100000001084b029489e1cddf726080c447c8a2b1d4bbe43024db31b8b19bc07585db9555010000006a473044022017422b9e3414d6233fa75f9eb7778469bebbb40686b0f7eb77d90a04c80149610220411f1063086fe205ea821ceb0de89e8158e202aba00f5ebb92b51f97381311fd012102ccb10a2f0603a0624b8708abefb5f4700631fc131c5de38b51e0359e2ffa7d1cffffffff03000000000000000000f260de1a580100001976a9145b1d583a4c270f2f14be77b298f0a9c6df97471388ac009ca6920c0000001976a914cb1196fb1b98d04b0cb8d2ffde3c2de3eb83d9fe88ac00000000",
		Txid:     "eace41778a2940ff423b72a42033990eb5d6092810734a5806da6f3e5b340864",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022017422b9e3414d6233fa75f9eb7778469bebbb40686b0f7eb77d90a04c80149610220411f1063086fe205ea821ceb0de89e8158e202aba00f5ebb92b51f97381311fd012102ccb10a2f0603a0624b8708abefb5f4700631fc131c5de38b51e0359e2ffa7d1c",
				},
				Txid:     "5595db8575c09bb1b831db2430e4bbd4b1a2c847c4806072dfcde18994024b08",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "",
				},
			},
			{
				ValueSat: *big.NewInt(1477919531250),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9145b1d583a4c270f2f14be77b298f0a9c6df97471388ac",
					Addresses: []string{
						"DDSsBchWiVfvPVn6Ldp1nL7k4L77cSDqM7",
					},
				},
			},
			{
				ValueSat: *big.NewInt(54000000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914cb1196fb1b98d04b0cb8d2ffde3c2de3eb83d9fe88ac",
					Addresses: []string{
						"DPepnMkaNHKCa6cQi7oBThrdiFEwSSYFzv",
					},
				},
			},
		},
		Blocktime: 1562853038,
		Time:      1562853038,
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *DivicoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "divi-1",
			args: args{
				tx:        testTx1,
				height:    409054,
				blockTime: 1562853038,
				parser:    NewDiviParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "divi-2",
			args: args{
				tx:        testTx2,
				height:    409054,
				blockTime: 1562853038,
				parser:    NewDiviParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.parser.PackTx(&tt.args.tx, tt.args.height, tt.args.blockTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("packTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("packTx() = %v, want %v", h, tt.want)
			}
		})
	}
}

func Test_UnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *DivicoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "divi-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewDiviParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   409054,
			wantErr: false,
		},
		{
			name: "divi-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewDiviParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   409054,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.packedTx)
			got, got1, err := tt.args.parser.UnpackTx(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("unpackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unpackTx() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("unpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

// Block test - looks for size, time, and transaction hashes

type testBlock struct {
	size int
	time int64
	tx   []string
}

var testParseBlockTxs = map[int]testBlock{
	407407: {
		size: 479,
		time: 1562753629,
		tx: []string{
			"3f8f01aec6717ede0e167f267fe486f18ddd25a13afd910dc1d41537aa1c6658",
			"b25224449d0f5266073876e924c4d6a4f127175aae151a66db6619e4ca41fe1d",
		},
	},
	409054: {
		size: 479,
		time: 1562853038,
		tx: []string{
			"f7a5324866ba18058ab032196f34458d19f7ec5a4ac284670c3ef07bfa724644",
			"eace41778a2940ff423b72a42033990eb5d6092810734a5806da6f3e5b340864",
		},
	},
	408074: {
		size: 1303,
		time: 1562794078,
		tx: []string{
			"bf0004680570d49eefab2ab806bd41f99587b6f3e65d1e0fb1d8e8f766f211f3",
			"8a334d86443d5e54d3d112b7ab4eff79ed0b879cbc62c580beee080b3c9e1142",
			"1ba350ba68b8db6af589136a85246c961694434ec2ffd1ad9c86831965b96932",
			"e05dcfece505455e8b4bcaeeb9ae1060fcf9c95ad1402c4fbd3b2c2bf1778683",
			"d3980118dedde2666d5bcd03ebf2c2d91ad6056404503afe0c37ed6cdd549f62",
		},
	},
}

func helperLoadBlock(t *testing.T, height int) []byte {
	name := fmt.Sprintf("block_dump.%d", height)
	path := filepath.Join("testdata", name)

	d, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d = bytes.TrimSpace(d)

	b := make([]byte, hex.DecodedLen(len(d)))
	_, err = hex.Decode(b, d)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func TestParseBlock(t *testing.T) {
	p := NewDiviParser(GetChainParams("main"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs {
		b := helperLoadBlock(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Fatal(err)
		}

		if blk.Size != tb.size {
			t.Errorf("ParseBlock() block size: got %d, want %d", blk.Size, tb.size)
		}

		if blk.Time != tb.time {
			t.Errorf("ParseBlock() block time: got %d, want %d", blk.Time, tb.time)
		}

		if len(blk.Txs) != len(tb.tx) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.tx))
		}

		for ti, tx := range tb.tx {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}
