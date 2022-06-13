//go:build unittest

package polis

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

type testBlock struct {
	size int
	time int64
	txs  []string
}

var testParseBlockTxs = map[int]testBlock{
	// Simple POW block
	50000: {
		size: 1393,
		time: 1520175937,
		txs: []string{
			"b68057244d6ad2df0017bad1cd8a24487e21404b52873c59876a484c93f5a69e",
			"3d27b82972196ecce604c2923bc9105cd683352fd56f8ba052dee57a296a2d71",
			"d22704ddad675d652a6b501694d9e026f19b41842946ea285490315cb944c674",
			"7be2e53414c9480ea1590e40cfa8361ac7594f7435f919a6eeff96367b0dffa9",
			"e2486a9610698888c4baad7001385e95aca053ab9fc7cc9d15280c9c835c975c",
		},
	},
	// Simple POS block
	280000: {
		size: 275,
		time: 1549070495,
		txs: []string{
			"9a820cb226364e852ec5d13bc3ead1ad127bf28ef2808919571200a1262b46b5",
			"fcca99e281fa0c43085dfe82c24b4367ff21d3a148539e781c061fe29a793ab1",
		},
	},
}

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
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
	p := NewPolisParser(GetChainParams("main"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs {
		b := helperLoadBlock(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Errorf("ParseBlock() error %v", err)
		}

		if blk.Size != tb.size {
			t.Errorf("ParseBlock() block size: got %d, want %d", blk.Size, tb.size)
		}

		if blk.Time != tb.time {
			t.Errorf("ParseBlock() block time: got %d, want %d", blk.Time, tb.time)
		}

		if len(blk.Txs) != len(tb.txs) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.txs))
		}

		for ti, tx := range tb.txs {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}

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
			args:    args{address: "P9hRjWq6tMqhroxswc2f5jp2ND2py8YEnu"},
			want:    "76a9140c26ca7967e6fe946f00bf81bcd3b86f43538edf88ac",
			wantErr: false,
		},
	}
	parser := NewPolisParser(GetChainParams("main"), &btc.Configuration{})

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
			name:    "P2PKH1",
			args:    args{script: "76a9140c26ca7967e6fe946f00bf81bcd3b86f43538edf88ac"},
			want:    []string{"P9hRjWq6tMqhroxswc2f5jp2ND2py8YEnu"},
			want2:   true,
			wantErr: false,
		},
	}

	parser := NewPolisParser(GetChainParams("main"), &btc.Configuration{})

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

var (
	testTx1       bchain.Tx
	testTxPacked1 = "0004e3868bca91b06e020000000198160d0ba0168003897358f1a6d2a2499a8e93dc6d341613b960ed2083de3fe0010000006b483045022100c531bd672ed3cb1a9285191ac168f1627e6075a54f5cf4a22ee8ec717b9a047f0220068241d9cc10adf1ddcc30aef4e62863d663af50371ebc639d11532ddf6e636e012102182bc5cd0a82c43c7ed9c4acc4b735e04e7f3275b43ff2514b8a1beb1feb5493feffffff021e470d8d0e0000001976a914344bf2db193190967d3b8da659a3ce2fde5f44a588acb63dd7a4300000001976a91414c343cae45bbcf7a27b8284b8c328587f6cc45588ac85e30400"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "020000000198160d0ba0168003897358f1a6d2a2499a8e93dc6d341613b960ed2083de3fe0010000006b483045022100c531bd672ed3cb1a9285191ac168f1627e6075a54f5cf4a22ee8ec717b9a047f0220068241d9cc10adf1ddcc30aef4e62863d663af50371ebc639d11532ddf6e636e012102182bc5cd0a82c43c7ed9c4acc4b735e04e7f3275b43ff2514b8a1beb1feb5493feffffff021e470d8d0e0000001976a914344bf2db193190967d3b8da659a3ce2fde5f44a588acb63dd7a4300000001976a91414c343cae45bbcf7a27b8284b8c328587f6cc45588ac85e30400",
		Blocktime: 1554132023,
		Txid:      "6882e77c916c5442d09e295b88fbb8a2fac6dbb988975bb00dbded088e0229a9",
		LockTime:  320389,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100c531bd672ed3cb1a9285191ac168f1627e6075a54f5cf4a22ee8ec717b9a047f0220068241d9cc10adf1ddcc30aef4e62863d663af50371ebc639d11532ddf6e636e012102182bc5cd0a82c43c7ed9c4acc4b735e04e7f3275b43ff2514b8a1beb1feb5493",
				},
				Txid:     "e03fde8320ed60b91316346ddc938e9a49a2d2a6f1587389038016a00b0d1698",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(62495999774),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914344bf2db193190967d3b8da659a3ce2fde5f44a588ac",
					Addresses: []string{
						"PDMhGxFYTaomhzSqWKHbUzx7smYUZvZVjd",
					},
				},
			},
			{
				ValueSat: *big.NewInt(208923999670),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91414c343cae45bbcf7a27b8284b8c328587f6cc45588ac",
					Addresses: []string{
						"PAUxb3g3DZNrjgbRidZy3NC9TNhVrPRzAR",
					},
				},
			},
		},
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *PolisParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "polis-1",
			args: args{
				tx:        testTx1,
				height:    320390,
				blockTime: 1554132023,
				parser:    NewPolisParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
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
		parser   *PolisParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "polis-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewPolisParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   320390,
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
