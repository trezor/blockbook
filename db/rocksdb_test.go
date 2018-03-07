package db

import (
	"blockbook/bchain"
	"encoding/hex"
	"reflect"
	"testing"
)

var testTx1 = bchain.Tx{
	Hex:       "01000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700",
	Blocktime: 1519053802,
	Txid:      "056e3d82e5ffd0e915fb9b62797d76263508c34fe3e5dbed30dd3e943930f204",
	LockTime:  512115,
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
var testTxPacked1 = "0001e2408ba8d7af5401000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700"

var testTx2 = bchain.Tx{
	Hex:       "010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000",
	Blocktime: 1235678901,
	Txid:      "474e6795760ebe81cb4023dc227e5a0efe340e1771c89a0035276361ed733de7",
	LockTime:  0,
	Vin: []bchain.Vin{
		{
			ScriptSig: bchain.ScriptSig{
				Hex: "160014550da1f5d25a9dae2eafd6902b4194c4c6500af6",
			},
			Txid:     "c13e32a4428e31f85d7aee4ec7344504b12e72aaffcbde0160200d2ac7f0649d",
			Vout:     0,
			Sequence: 4294967295,
		},
	},
	Vout: []bchain.Vout{
		{
			Value: .1,
			N:     0,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex: "a914cd668d781ece600efa4b2404dc91fd26b8b8aed887",
				Addresses: []string{
					"2NByHN6A8QYkBATzxf4pRGbCSHD5CEN2TRu",
				},
			},
		},
		{
			Value: 9.20081157,
			N:     1,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex: "a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a87",
				Addresses: []string{
					"2MvZguYaGjM7JihBgNqgLF2Ca2Enb76Hj9D",
				},
			},
		},
	},
}
var testTxPacked2 = "0007c91a899ab7da6a010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000"

func Test_packTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "btc-1",
			args:    args{testTx1, 123456, 1519053802},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name:    "testnet-1",
			args:    args{testTx2, 510234, 1235678901},
			want:    testTxPacked2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := packTx(&tt.args.tx, tt.args.height, tt.args.blockTime)
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
		parser   *bchain.BitcoinBlockParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "btc-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   &bchain.BitcoinBlockParser{Params: bchain.GetChainParams("main")},
			},
			want:    &testTx1,
			want1:   123456,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				packedTx: testTxPacked2,
				parser:   &bchain.BitcoinBlockParser{Params: bchain.GetChainParams("test")},
			},
			want:    &testTx2,
			want1:   510234,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.packedTx)
			got, got1, err := unpackTx(b, tt.args.parser)
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
