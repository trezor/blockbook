//go:build unittest

package snowgem

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcutil/chaincfg"
)

var (
	testTx1, testTx2 bchain.Tx

	testTxPacked1 = "0a20241803e368d7459f31286a155191ee386896d366d57c19d8e67a8f040d6ff71f12f4010400008085202f890119950c49d69b37d5f4fbb390d852387559e6a6d3fce9f390a409e4acf3f06381020000006a4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93ffeffffff02d3d0a146000000001976a9141a78c04d87f553545ba225b7bc7a271731f659d688ac7c54ae02000000001976a914b86f4b063545ebc2e80522a59d2dd206b707401b88aca68d0e00c58d0e00000000000000000000000018aba4b8ed0520a69b3a28b19b3a32960112208163f0f3ace409a490f3e9fcd3a6e659753852d890b3fbf4d5379bd6490c95191802226a4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93f28feffffff0f3a460a0446a1d0d31a1976a9141a78c04d87f553545ba225b7bc7a271731f659d688ac2223733150636953644665724a78665673397451353571446f3839695676466f7162436a7a3a480a0402ae547c10011a1976a914b86f4b063545ebc2e80522a59d2dd206b707401b88ac22237331653177736d6f7955625673794b726745374b73714c5164374c6975596168526152"
	testTxPacked2 = "0a2071dd4d998b0a711fe5ed21f8661ed27ca8b99afc488f5bbe149ec3c6492ec50312d2010400008085202f89017308714b21338783a435c5e420542a0f6243da5be6dc8bdf19e2d526a318d6a8000000006a47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af12feffffff014dd45023000000001976a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888acaa8d0e00c98d0e00000000000000000000000018f9a6b8ed0520aa9b3a28b59b3a3294011220a8d618a326d5e219df8bdce65bda43620f2a5420e4c535a4838733214b710873226a47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af1228feffffff0f3a460a042350d44d1a1976a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888ac2223733167347a74585446447751326b506253385431666755334c645075666376354d764d"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0400008085202f890119950c49d69b37d5f4fbb390d852387559e6a6d3fce9f390a409e4acf3f06381020000006a4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93ffeffffff02d3d0a146000000001976a9141a78c04d87f553545ba225b7bc7a271731f659d688ac7c54ae02000000001976a914b86f4b063545ebc2e80522a59d2dd206b707401b88aca68d0e00c58d0e000000000000000000000000",
		Blocktime: 1571689003,
		Time:      1571689003,
		Txid:      "241803e368d7459f31286a155191ee386896d366d57c19d8e67a8f040d6ff71f",
		LockTime:  953766,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93f",
				},
				Txid:     "8163f0f3ace409a490f3e9fcd3a6e659753852d890b3fbf4d5379bd6490c9519",
				Vout:     2,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1185009875),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9141a78c04d87f553545ba225b7bc7a271731f659d688ac",
					Addresses: []string{
						"s1PciSdFerJxfVs9tQ55qDo89iVvFoqbCjz",
					},
				},
			},
			{
				ValueSat: *big.NewInt(44979324),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b86f4b063545ebc2e80522a59d2dd206b707401b88ac",
					Addresses: []string{
						"s1e1wsmoyUbVsyKrgE7KsqLQd7LiuYahRaR",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "0400008085202f89017308714b21338783a435c5e420542a0f6243da5be6dc8bdf19e2d526a318d6a8000000006a47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af12feffffff014dd45023000000001976a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888acaa8d0e00c98d0e000000000000000000000000",
		Blocktime: 1571689337,
		Time:      1571689337,
		Txid:      "71dd4d998b0a711fe5ed21f8661ed27ca8b99afc488f5bbe149ec3c6492ec503",
		LockTime:  953770,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af12",
				},
				Txid:     "a8d618a326d5e219df8bdce65bda43620f2a5420e4c535a4838733214b710873",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(592499789),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888ac",
					Addresses: []string{
						"s1g4ztXTFDwQ2kPbS8T1fgU3LdPufcv5MvM",
					},
				},
			},
		},
	}
}

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func TestGetAddrDesc(t *testing.T) {
	type args struct {
		tx     bchain.Tx
		parser *SnowGemParser
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "snowgem-1",
			args: args{
				tx:     testTx1,
				parser: NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
		{
			name: "snowgem-2",
			args: args{
				tx:     testTx2,
				parser: NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for n, vout := range tt.args.tx.Vout {
				got1, err := tt.args.parser.GetAddrDescFromVout(&vout)
				if err != nil {
					t.Errorf("getAddrDescFromVout() error = %v, vout = %d", err, n)
					return
				}
				got2, err := tt.args.parser.GetAddrDescFromAddress(vout.ScriptPubKey.Addresses[0])
				if err != nil {
					t.Errorf("getAddrDescFromAddress() error = %v, vout = %d", err, n)
					return
				}
				if !bytes.Equal(got1, got2) {
					t.Errorf("Address descriptors mismatch: got1 = %v, got2 = %v", got1, got2)
				}
			}
		})
	}
}

func TestPackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *SnowGemParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "snowgem-1",
			args: args{
				tx:        testTx1,
				height:    953777,
				blockTime: 1571689003,
				parser:    NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "snowgem-2",
			args: args{
				tx:        testTx2,
				height:    953781,
				blockTime: 1571689337,
				parser:    NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
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

func TestUnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *SnowGemParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "snowgem-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   953777,
			wantErr: false,
		},
		{
			name: "snowgem-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewSnowGemParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   953781,
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
