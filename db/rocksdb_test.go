package db

import (
	"blockbook/bchain"
	"encoding/hex"
	"reflect"
	"testing"
)

var testTx = bchain.Tx{
	// Blocktime: 1520253021,
	Hex:      "01000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700",
	Txid:     "056e3d82e5ffd0e915fb9b62797d76263508c34fe3e5dbed30dd3e943930f204",
	LockTime: 512115,
	// Time:      1520253022,
	Vin: []bchain.Vin{
		{
			ScriptSig: bchain.ScriptSig{
				Hex: "4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80",
			},
			Txid:     "425fed43ba74e9205875eb934d5bcf7bf338f146f70d4002d94bf5cbc9229a7f",
			Vout:     4,
			Sequence: 4294967294,
		},
	},
	Vout: []bchain.Vout{
		{
			Value: 0.00038812,
			N:     0,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex: "a9146144d57c8aff48492c9dfb914e120b20bad72d6f87",
				Addresses: []string{
					"3AZKvpKhSh1o8t1QrX3UeXG9d2BhCRnbcK",
				},
			},
		},
	},
}

var testTxPacked = "0001e24001000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700"

func Test_packTx(t *testing.T) {
	type args struct {
		tx     bchain.Tx
		height uint32
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "1",
			args:    args{testTx, 123456},
			want:    testTxPacked,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := packTx(&tt.args.tx, tt.args.height)
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

func Test_unpackTx(t *testing.T) {
	type args struct {
		packedTx string
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name:    "1",
			args:    args{packedTx: testTxPacked},
			want:    &testTx,
			want1:   123456,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.packedTx)
			got, got1, err := unpackTx(b)
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
