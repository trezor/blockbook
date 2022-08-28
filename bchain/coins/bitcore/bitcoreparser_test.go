//go:build unittest

package bitcore

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
			name:    "pubkeyhash1",
			args:    args{address: "2HbJfcGD6NTm318VeBjfd2hLf44hHkzHVV"},
			want:    "76a9143236327ebad3be5e336777bb3562439720f38dc488ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "2XFmLkpyBzJncnnkALQ3qnMqSqUdqcBdc4"},
			want:    "76a914c815e7f760bbd5f109d58e848cf78ba808d972e088ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "scUVfntFvnyTHRCvBwUNyKGVFLiBb2iHVK"},
			want:    "a914c7ec567ef583a96a74c02980cc42f728cc987c3287",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "sVeAXe1CMWVAuq5174hG49QRfkBp4GFvAu"},
			want:    "a9147cf7a3a6b1305871ff5f0f064aaa634880ff67ab87",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "btx1qnfkmarp8pe8q05690zd48qma3gmp0pp66gqsv3"},
			want:    "00149a6dbe8c270e4e07d345789b53837d8a3617843a",
			wantErr: false,
		},
	}
	parser := NewBitcoreParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "0a20fcd4f2e45787a33571bc9b2ce939d6e8e51fa053296de9240f05455702bd954012e2010200000001f69bd1fd76e52a426f21332e3b7cfbc3350eacbd21c6e0c11a7ae11919803ef0010000006b483045022100d1fa62b9d7860a03e1dcd4734fe42457cb508ebb49e896d7a77748d997d09fba022005f1657b39451afe97076d8667fe5f6f18ca76391521ab84d09d5b82137d933b0121035aaf032f13761f27465467dc73f1998a80dd4d85a6353d2832a7244d7b591d3effffffff02a87322b3010000001976a914d0c320db3fbd0abe2b6fe31a3bca4fed8ce8669588ac94b94f37000000001976a9145584ee07090af59938e991c9d8e9e945c99a449f88ac0000000018858a8ce20528f9f3133297011220f03e801919e17a1ac1e0c621bdac0e35c3fb7c3b2e33216f422ae576fdd19bf61801226b483045022100d1fa62b9d7860a03e1dcd4734fe42457cb508ebb49e896d7a77748d997d09fba022005f1657b39451afe97076d8667fe5f6f18ca76391521ab84d09d5b82137d933b0121035aaf032f13761f27465467dc73f1998a80dd4d85a6353d2832a7244d7b591d3e28ffffffff0f3a460a0501b32273a81a1976a914d0c320db3fbd0abe2b6fe31a3bca4fed8ce8669588ac22223259336546797741414673617039757139726942474143684e326858356a6e7268753a470a04374fb99410011a1976a9145584ee07090af59938e991c9d8e9e945c99a449f88ac2222324c6f7a646b704450723562356b6a66445042315a76454c597735734475684139594002"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0200000001f69bd1fd76e52a426f21332e3b7cfbc3350eacbd21c6e0c11a7ae11919803ef0010000006b483045022100d1fa62b9d7860a03e1dcd4734fe42457cb508ebb49e896d7a77748d997d09fba022005f1657b39451afe97076d8667fe5f6f18ca76391521ab84d09d5b82137d933b0121035aaf032f13761f27465467dc73f1998a80dd4d85a6353d2832a7244d7b591d3effffffff02a87322b3010000001976a914d0c320db3fbd0abe2b6fe31a3bca4fed8ce8669588ac94b94f37000000001976a9145584ee07090af59938e991c9d8e9e945c99a449f88ac00000000",
		Blocktime: 1547896069,
		Time:      1547896069,
		Txid:      "fcd4f2e45787a33571bc9b2ce939d6e8e51fa053296de9240f05455702bd9540",
		LockTime:  0,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100d1fa62b9d7860a03e1dcd4734fe42457cb508ebb49e896d7a77748d997d09fba022005f1657b39451afe97076d8667fe5f6f18ca76391521ab84d09d5b82137d933b0121035aaf032f13761f27465467dc73f1998a80dd4d85a6353d2832a7244d7b591d3e",
				},
				Txid:     "f03e801919e17a1ac1e0c621bdac0e35c3fb7c3b2e33216f422ae576fdd19bf6",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(7300346792),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914d0c320db3fbd0abe2b6fe31a3bca4fed8ce8669588ac",
					Addresses: []string{
						"2Y3eFywAAFsap9uq9riBGAChN2hX5jnrhu",
					},
				},
			},
			{
				ValueSat: *big.NewInt(927971732),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9145584ee07090af59938e991c9d8e9e945c99a449f88ac",
					Addresses: []string{
						"2LozdkpDPr5b5kjfDPB1ZvELYw5sDuhA9Y",
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
		parser    *BitcoreParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "bitcore-1",
			args: args{
				tx:        testTx1,
				height:    326137,
				blockTime: 1547896069,
				parser:    NewBitcoreParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *BitcoreParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "bitcore-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBitcoreParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   326137,
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
