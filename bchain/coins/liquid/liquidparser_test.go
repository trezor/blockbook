// build unittest

package liquid

import (
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"github.com/jakm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func Test_GetAddrDescFromAddress(t *testing.T) {
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
			args:    args{address: "QHU1yszeZwVeuJosGJ4JDHuKaLRWmdEYDF"},
			want:    "76a914dd95db91e8f914cbd63bae8e307d54399f060cd688ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "PwogNb9zqhrwPneDqVp18GqcVuygsCvXkU"},
			want:    "76a91405eb4afe4615751cfb813a00846a8d9ef8a9a2e588ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "GhWTZqLPHRK8KfuT6yo1wGisQzn4cXrbPP"},
			want:    "a9140394b3cf9a44782c10105b93962daa8dba304d7f87",
			wantErr: false,
		},
	}
	parser := NewLiquidParser(GetChainParams("main"), &btc.Configuration{})

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
			args:    args{script: "76a914dd95db91e8f914cbd63bae8e307d54399f060cd688ac"},
			want:    []string{"QHU1yszeZwVeuJosGJ4JDHuKaLRWmdEYDF"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{script: "76a91405eb4afe4615751cfb813a00846a8d9ef8a9a2e588ac"},
			want:    []string{"PwogNb9zqhrwPneDqVp18GqcVuygsCvXkU"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9140394b3cf9a44782c10105b93962daa8dba304d7f87"},
			want:    []string{"GhWTZqLPHRK8KfuT6yo1wGisQzn4cXrbPP"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PK compressed",
			args:    args{script: "21020e46e79a2a8d12b9b5d12c7a91adb4e454edfae43c0a0cb805427d2ac7613fd9ac"},
			want:    []string{"QKKEbCNAV7BYdtfSb3cQcJ7QSmFMvtXETz"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed",
			args:    args{script: "41041057356b91bfd3efeff5fc0fa8b865faafafb67bd653c5da2cd16ce15c7b86db0e622c8e1e135f68918a23601eb49208c1ac72c7b64a4ee99c396cf788da16ccac"},
			want:    []string{"QDoUiWY7iZXDrkBzXdk6dru8DbvGqExXuf"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewLiquidParser(GetChainParams("main"), &btc.Configuration{})

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

/*
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
		parser    *LiquidParser
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
				parser:    NewLiquidParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *LiquidParser
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
				parser:   NewLiquidParser(GetChainParams("main"), &btc.Configuration{}),
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
*/
