// +build unittest

package polis

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"
	"github.com/martinboehm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
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
	testTxPacked1 = ""
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
