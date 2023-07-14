//go:build unittest

package gamecredits

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

func Test_GetAddrDescFromAddress_Testnet(t *testing.T) {
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
			args:    args{address: "GcGBy77CCfZJJhGLALohdahf9eAc7jo7Yk"},
			want:    "76a914ca093a938a0e19e86b36859d9423a475d45eb3a288ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "S84eckDshWupTwErdLKkyDauNwtWfa9rPL"},
			want:    "a9146edfea548a7d6c25aa28e37bf2ea382891882fa687",
			wantErr: false,
		},
	}
	parser := NewGameCreditsParser(GetChainParams("test"), &btc.Configuration{})

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
			args:    args{address: "GcGBy77CCfZJJhGLALohdahf9eAc7jo7Yk"},
			want:    "76a914ca093a938a0e19e86b36859d9423a475d45eb3a288ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "S84eckDshWupTwErdLKkyDauNwtWfa9rPL"},
			want:    "a9146edfea548a7d6c25aa28e37bf2ea382891882fa687",
			wantErr: false,
		},
	}
	parser := NewGameCreditsParser(GetChainParams("main"), &btc.Configuration{})

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
	testTx1 bchain.Tx

	testTxPacked1 = "002151148bbcaa8406010000000123c41ad26dd5782635638effbc9e31c9b4a3b757591a52c83d2770ad82b33e93000000006b483045022100a20302bde6d2fb194bb9c0a8d7beb52ed0b5b72b912da75364efe169d5b74c67022065632d4032673a6093f513b93e380323487ad2708003e161a12e7b7362bf9f4a01210325c1b08d90a016cb73f4e8d37614cac7da00cb78121f21b7b6e0a7d4a03fbae4fdffffff0100f4aa01000000001976a914ca093a938a0e19e86b36859d9423a475d45eb3a288acc54f2100"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "010000000123c41ad26dd5782635638effbc9e31c9b4a3b757591a52c83d2770ad82b33e93000000006b483045022100a20302bde6d2fb194bb9c0a8d7beb52ed0b5b72b912da75364efe169d5b74c67022065632d4032673a6093f513b93e380323487ad2708003e161a12e7b7362bf9f4a01210325c1b08d90a016cb73f4e8d37614cac7da00cb78121f21b7b6e0a7d4a03fbae4fdffffff0100f4aa01000000001976a914ca093a938a0e19e86b36859d9423a475d45eb3a288acc54f2100",
		Blocktime: 1539653891,
		Txid:      "983da8317fff45afb17290d4dd8da6ec1cd8ffbbfa98e53a0754e9b60f8cc0f9",
		LockTime:  2183109,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100a20302bde6d2fb194bb9c0a8d7beb52ed0b5b72b912da75364efe169d5b74c67022065632d4032673a6093f513b93e380323487ad2708003e161a12e7b7362bf9f4a01210325c1b08d90a016cb73f4e8d37614cac7da00cb78121f21b7b6e0a7d4a03fbae4",
				},
				Txid:     "933eb382ad70273dc8521a5957b7a3b4c9319ebcff8e63352678d56dd21ac423",
				Vout:     0,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(27980800),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ca093a938a0e19e86b36859d9423a475d45eb3a288ac",
					Addresses: []string{
						"GcGBy77CCfZJJhGLALohdahf9eAc7jo7Yk",
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
		parser    *GameCreditsParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "gamecredits-1",
			args: args{
				tx:        testTx1,
				height:    2183444,
				blockTime: 1539653891,
				parser:    NewGameCreditsParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *GameCreditsParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "gamecredits-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewGameCreditsParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   2183444,
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
