// +build unittest

package snowgem

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
)

var (
	testTx1, testTx2 bchain.Tx

	testTxPacked1 = "0a20241803e368d7459f31286a155191ee386896d366d57c19d8e67a8f040d6ff71f12ab100400008085202f890119950c49d69b37d5f4fbb390d852387559e6a6d3fce9f390a409e4acf3f06381020000006a4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93ffeffffff02d3d0a146000000001976a9141a78c04d87f553545ba225b7bc7a271731f659d688ac7c54ae02000000001976a914b86f4b063545ebc2e80522a59d2dd206b707401b88aca68d0e00c58d0e00000000000000000000000018a0f1c9d505200028b0eb113299010a0012208163f0f3ace409a490f3e9fcd3a6e659753852d890b3fbf4d5379bd6490c95191800226b4730440220452aedf599e575598eb36d27ed98a6d388efda6e9be2bab96f16d0644e7df3060220669f4f3a4976ed73fa3ca9ecaad84dcf6ec35099c3bad631499985ea6a378d19012102ed9fb7fb61ec514be890ab45a925d554ff12050f099514251d5ebe904accc93f28ffffffff0f3a490a05043c1aec8e10001a1976a9141a78c04d87f553545ba225b7bc7a271731f659d688ac22237431566d4854547770457477766f6a786f644e32435351714c596931687a59336341713a470a0398968010011a1976a914b86f4b063545ebc2e80522a59d2dd206b707401b88ac222374315934794c31344143486141626a656d6b647057376e594e48576e763179516244414000"
	testTxPacked2 = "0a2071dd4d998b0a711fe5ed21f8661ed27ca8b99afc488f5bbe149ec3c6492ec50312e2010400008085202f89017308714b21338783a435c5e420542a0f6243da5be6dc8bdf19e2d526a318d6a8000000006a47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af12feffffff014dd45023000000001976a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888acaa8d0e00c98d0e00000000000000000000000018e4b1c9d50520eeea1128f9ea113299010a001220a8d618a326d5e219df8bdce65bda43620f2a5420e4c535a4838733214b7108731800226b47304402207ce5ebcb2dc5e8027b5d672babd2e6aaa186a917caf2b44eec63f7db16277b8b02207a89214d825fae08ebc86bca1f46579e770e830bd31b8101498207a2d901fd74012103c3fe8969a7b08f1d586a68da70d6aeff61aa3b4cbe7ca2cb5aae11529ca2af1228feffffff0f3a490a050184a7a72310001a1976a914cef34ec02e80351cf4f9d63843fc79a77c9ab71888ac222374316563784d587070685554525158474c586e56684a367563714433445a69706464674000"
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
