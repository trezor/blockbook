//go:build unittest

package bitzeny

import (
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"

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
			args:    args{address: "Zw74N1RSU2xV3a7SBERBiCP11fMwX5yvMu"},
			want:    "76a914d8658ca5c406149071687d370d1d22d972d2f88488ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "ZiSn1vTSxGu2kFcnkjjm7bYGhT5BVAVfEG"},
			want:    "76a9144d869697281ad18370313122795e56dfdc3a331388ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "3CZ3357bm1K81StpEDQtEH3ho3ULx19nc8"},
			want:    "a9147726fc1144eae1b7bd301d87d0a7f846cadb591887",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "3M1AjZEuBzScbd9pchiGJSVT4yNfwzSmXP"},
			want:    "a914d3d93b5d7f57b94a4fecde93d4489f2b423fd3c287",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "bz1q7rfrdacyyfwx8gppd8ah9hka8npgqsm44prfnd"},
			want:    "0014f0d236f704225c63a02169fb72dedd3cc2804375",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{address: "bz1qd2mspe6m2wpztw4q2mccyvyess6569eu59sfvf0u0vdmdwltr5lse8d7sw"},
			want:    "00206ab700e75b538225baa056f182309984354d173ca1609625fc7b1bb6bbeb1d3f",
			wantErr: false,
		},
	}
	parser := NewBitZenyParser(GetChainParams("main"), &btc.Configuration{})

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
			name:    "P2PKH",
			args:    args{script: "76a914d8658ca5c406149071687d370d1d22d972d2f88488ac"},
			want:    []string{"Zw74N1RSU2xV3a7SBERBiCP11fMwX5yvMu"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9147726fc1144eae1b7bd301d87d0a7f846cadb591887"},
			want:    []string{"3CZ3357bm1K81StpEDQtEH3ho3ULx19nc8"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "0014f0d236f704225c63a02169fb72dedd3cc2804375"},
			want:    []string{"bz1q7rfrdacyyfwx8gppd8ah9hka8npgqsm44prfnd"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "00206ab700e75b538225baa056f182309984354d173ca1609625fc7b1bb6bbeb1d3f"},
			want:    []string{"bz1qd2mspe6m2wpztw4q2mccyvyess6569eu59sfvf0u0vdmdwltr5lse8d7sw"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "OP_RETURN ascii",
			args:    args{script: "6a0461686f6a"},
			want:    []string{"OP_RETURN (ahoj)"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN hex",
			args:    args{script: "6a072020f1686f6a20"},
			want:    []string{"OP_RETURN 2020f1686f6a20"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewBitZenyParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("outputScriptToAddresses() error = %v, wantErr %v", err, tt.wantErr)
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
	testTx1 bchain.Tx

	testTxPacked1 = "001c3f1a8be6859d3e0100000001aef422fb91cd91e556966fed4121ac44017a761d71385596536bb447ae05213e000000006a47304402202341ac4297925257dc72eb418a069c45e76f7070340e27501f6308cc7eff45f802204a347915adceff5f6fc9b8075d95d47887d46b344c7bbc8066d315931b189ad001210228c2520812b7f8c63e7a088c61b6348b22fa0c98812e736a1fd896bc828d3c65feffffff028041f13d000000001976a91478379ea136bb5783b675cd11e412bf0703995aeb88aca9983141000000001976a9144d869697281ad18370313122795e56dfdc3a331388ac193f1c00"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0100000001aef422fb91cd91e556966fed4121ac44017a761d71385596536bb447ae05213e000000006a47304402202341ac4297925257dc72eb418a069c45e76f7070340e27501f6308cc7eff45f802204a347915adceff5f6fc9b8075d95d47887d46b344c7bbc8066d315931b189ad001210228c2520812b7f8c63e7a088c61b6348b22fa0c98812e736a1fd896bc828d3c65feffffff028041f13d000000001976a91478379ea136bb5783b675cd11e412bf0703995aeb88aca9983141000000001976a9144d869697281ad18370313122795e56dfdc3a331388ac193f1c00",
		Blocktime: 1583392607,
		Txid:      "f81c34b300961877328c3aaa7cd5e69068457868309fbf1e92544e3a6a915bcb",
		LockTime:  1851161,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402202341ac4297925257dc72eb418a069c45e76f7070340e27501f6308cc7eff45f802204a347915adceff5f6fc9b8075d95d47887d46b344c7bbc8066d315931b189ad001210228c2520812b7f8c63e7a088c61b6348b22fa0c98812e736a1fd896bc828d3c65",
				},
				Txid:     "3e2105ae47b46b53965538711d767a0144ac2141ed6f9656e591cd91fb22f4ae",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1039221120),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91478379ea136bb5783b675cd11e412bf0703995aeb88ac",
					Addresses: []string{
						"ZnLWULVbAzjy1TSKxGnpkomeeaEDTHk5Nj",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1093769385),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9144d869697281ad18370313122795e56dfdc3a331388ac",
					Addresses: []string{
						"ZiSn1vTSxGu2kFcnkjjm7bYGhT5BVAVfEG",
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
		parser    *BitZenyParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "BitZeny-1",
			args: args{
				tx:        testTx1,
				height:    1851162,
				blockTime: 1583392607,
				parser:    NewBitZenyParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *BitZenyParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "BitZeny-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBitZenyParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   1851162,
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
