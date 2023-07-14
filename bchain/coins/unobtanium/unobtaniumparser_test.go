//go:build unittest

package unobtanium

import (
	"encoding/hex"
	"math/big"
	"os"
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
			args:    args{address: "uNv31DrZT8DMGK4eV3FmvvxteCW2h26xXK"},
			want:    "76a9142b9db958ed02331e9fe78eff8c33cc9b276e40f188ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "ued4doshafG2qqzGZ5T7RBEm34sdMVm46e"},
			want:    "76a914d7ea06ca9357862a9d5855cc54ceb093e69a4bc088ac",
			wantErr: false,
		},
	}
	parser := NewUnobtaniumParser(GetChainParams("main"), &btc.Configuration{})

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

var (
	testTx1       bchain.Tx
	testTxPacked1 = "0004bd8d8ab4b387540100000002128690c8080ff5b648ad4e38e391f28843cb92c55fe1611a952ea3c9b8eafb2f000000006a47304402200df61ba8dcc1b7228f50eb40346ad237a1d5fae9445a4251f3acfce5728148c402202c43452804855fc08f42fe7db612d9f622a5ee46044279a7a9f7f7c18154a1a1012102248a489fb192cdf37124e15a898f08794a42c5ae2cab3de09a6f92073ae6c904ffffffffe0e35a761650d32866b56d5b4417acd5c6be9d27517be55682da1c4fe83fc6e2010000006c493046022100c40143c5fb6051986921b42565dd20f3201ff1a5365bf0d4110ab0f33c38b22d022100e851372081f381a3edee1b74b54dc6a7806fee6b152a2be855d6bc39974b461a0121028f6f386e276c56ea7ff8894c417a20e82c488ac51400fe51f41c2a243e2de736ffffffff0245420f00000000001976a914d7ea06ca9357862a9d5855cc54ceb093e69a4bc088acb86ef808000000001976a914f943a1cc03a501143ad57fd82394c2952b99e47888ac00000000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0100000002128690c8080ff5b648ad4e38e391f28843cb92c55fe1611a952ea3c9b8eafb2f000000006a47304402200df61ba8dcc1b7228f50eb40346ad237a1d5fae9445a4251f3acfce5728148c402202c43452804855fc08f42fe7db612d9f622a5ee46044279a7a9f7f7c18154a1a1012102248a489fb192cdf37124e15a898f08794a42c5ae2cab3de09a6f92073ae6c904ffffffffe0e35a761650d32866b56d5b4417acd5c6be9d27517be55682da1c4fe83fc6e2010000006c493046022100c40143c5fb6051986921b42565dd20f3201ff1a5365bf0d4110ab0f33c38b22d022100e851372081f381a3edee1b74b54dc6a7806fee6b152a2be855d6bc39974b461a0121028f6f386e276c56ea7ff8894c417a20e82c488ac51400fe51f41c2a243e2de736ffffffff0245420f00000000001976a914d7ea06ca9357862a9d5855cc54ceb093e69a4bc088acb86ef808000000001976a914f943a1cc03a501143ad57fd82394c2952b99e47888ac00000000",
		Blocktime: 1397121514,
		Txid:      "9888815899d3b2e0f26b1eab51229082cf1faf4cd03a12fea2c8afa66701541f",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402200df61ba8dcc1b7228f50eb40346ad237a1d5fae9445a4251f3acfce5728148c402202c43452804855fc08f42fe7db612d9f622a5ee46044279a7a9f7f7c18154a1a1012102248a489fb192cdf37124e15a898f08794a42c5ae2cab3de09a6f92073ae6c904",
				},
				Txid:     "2ffbeab8c9a32e951a61e15fc592cb4388f291e3384ead48b6f50f08c8908612",
				Vout:     0,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "493046022100c40143c5fb6051986921b42565dd20f3201ff1a5365bf0d4110ab0f33c38b22d022100e851372081f381a3edee1b74b54dc6a7806fee6b152a2be855d6bc39974b461a0121028f6f386e276c56ea7ff8894c417a20e82c488ac51400fe51f41c2a243e2de736",
				},
				Txid:     "e2c63fe84f1cda8256e57b51279dbec6d5ac17445b6db56628d35016765ae3e0",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1000005),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914d7ea06ca9357862a9d5855cc54ceb093e69a4bc088ac",
					Addresses: []string{
						"ued4doshafG2qqzGZ5T7RBEm34sdMVm46e",
					},
				},
			},
			{
				ValueSat: *big.NewInt(150499000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914f943a1cc03a501143ad57fd82394c2952b99e47888ac",
					Addresses: []string{
						"uhfQH3AD3huadZuzHTB7TWHoWXbJpJhS6B",
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
		parser    *UnobtaniumParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Unobtanium-1",
			args: args{
				tx:        testTx1,
				height:    310669,
				blockTime: 1397121514,
				parser:    NewUnobtaniumParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *UnobtaniumParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "Unobtanium-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewUnobtaniumParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   310669,
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
