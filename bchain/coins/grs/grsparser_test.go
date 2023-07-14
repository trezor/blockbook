//go:build unittest

package grs

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

var (
	testTx1, testTx2 bchain.Tx

	testTxPacked1 = "0a20f56521b17b828897f72b30dd21b0192fd942342e89acbb06abf1d446282c30f512bf0101000000014a9d1fdba915e0907ab02f04f88898863112a2b4fdcf872c7414588c47c874cb000000006a47304402201fb96d20d0778f54520ab59afe70d5fb20e500ecc9f02281cf57934e8029e8e10220383d5a3e80f2e1eb92765b6da0f23d454aecbd8236f083d483e9a7430236876101210331693756f749180aeed0a65a0fab0625a2250bd9abca502282a4cf0723152e67ffffffff01a0330300000000001976a914fe40329c95c5598ac60752a5310b320cb52d18e688ac0000000018ffff87da0528a6f383013294011220cb74c8478c5814742c87cffdb4a21231869888f8042fb07a90e015a9db1f9d4a226a47304402201fb96d20d0778f54520ab59afe70d5fb20e500ecc9f02281cf57934e8029e8e10220383d5a3e80f2e1eb92765b6da0f23d454aecbd8236f083d483e9a7430236876101210331693756f749180aeed0a65a0fab0625a2250bd9abca502282a4cf0723152e6728ffffffff0f3a440a030333a01a1976a914fe40329c95c5598ac60752a5310b320cb52d18e688ac222246744d347a416e39615659674867786d616d5742675750795a7362365268766b4139"
	testTxPacked2 = "0a209b5c4859a8a31e69788cb4402812bb28f14ad71cbd8c60b09903478bc56f79a312e00101000000000101d1613f483f2086d076c82fe34674385a86beb08f052d5405fe1aed397f852f4f0000000000feffffff02404b4c000000000017a9147a55d61848e77ca266e79a39bfc85c580a6426c987a8386f0000000000160014cc8067093f6f843d6d3e22004a4290cd0c0f336b02483045022100ea8780bc1e60e14e945a80654a41748bbf1aa7d6f2e40a88d91dfc2de1f34bd10220181a474a3420444bd188501d8d270736e1e9fe379da9970de992ff445b0972e3012103adc58245cf28406af0ef5cc24b8afba7f1be6c72f279b642d85c48798685f862d9ed090018caa384da0520d9db2728dadb27322812204f2f857f39ed1afe05542d058fb0be865a387446e32fc876d086203f483f61d128feffffff0f3a430a034c4b401a17a9147a55d61848e77ca266e79a39bfc85c580a6426c9872223324e345135466855323439374272794666556762716b414a453837614b4476335633653a4d0a036f38a810011a160014cc8067093f6f843d6d3e22004a4290cd0c0f336b222c746772733171656a7178777a666c64377a72366d663779677179357335736535787137766d74396c6b643537"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000014a9d1fdba915e0907ab02f04f88898863112a2b4fdcf872c7414588c47c874cb000000006a47304402201fb96d20d0778f54520ab59afe70d5fb20e500ecc9f02281cf57934e8029e8e10220383d5a3e80f2e1eb92765b6da0f23d454aecbd8236f083d483e9a7430236876101210331693756f749180aeed0a65a0fab0625a2250bd9abca502282a4cf0723152e67ffffffff01a0330300000000001976a914fe40329c95c5598ac60752a5310b320cb52d18e688ac00000000",
		Blocktime: 1531052031,
		Time:      1531052031,
		Txid:      "f56521b17b828897f72b30dd21b0192fd942342e89acbb06abf1d446282c30f5",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402201fb96d20d0778f54520ab59afe70d5fb20e500ecc9f02281cf57934e8029e8e10220383d5a3e80f2e1eb92765b6da0f23d454aecbd8236f083d483e9a7430236876101210331693756f749180aeed0a65a0fab0625a2250bd9abca502282a4cf0723152e67",
				},
				Txid:     "cb74c8478c5814742c87cffdb4a21231869888f8042fb07a90e015a9db1f9d4a",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(209824),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914fe40329c95c5598ac60752a5310b320cb52d18e688ac",
					Addresses: []string{
						"FtM4zAn9aVYgHgxmamWBgWPyZsb6RhvkA9",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "01000000000101d1613f483f2086d076c82fe34674385a86beb08f052d5405fe1aed397f852f4f0000000000feffffff02404b4c000000000017a9147a55d61848e77ca266e79a39bfc85c580a6426c987a8386f0000000000160014cc8067093f6f843d6d3e22004a4290cd0c0f336b02483045022100ea8780bc1e60e14e945a80654a41748bbf1aa7d6f2e40a88d91dfc2de1f34bd10220181a474a3420444bd188501d8d270736e1e9fe379da9970de992ff445b0972e3012103adc58245cf28406af0ef5cc24b8afba7f1be6c72f279b642d85c48798685f862d9ed0900",
		Blocktime: 1530991050,
		Time:      1530991050,
		Txid:      "9b5c4859a8a31e69788cb4402812bb28f14ad71cbd8c60b09903478bc56f79a3",
		LockTime:  650713,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "4f2f857f39ed1afe05542d058fb0be865a387446e32fc876d086203f483f61d1",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(5000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9147a55d61848e77ca266e79a39bfc85c580a6426c987",
					Addresses: []string{
						"2N4Q5FhU2497BryFfUgbqkAJE87aKDv3V3e",
					},
				},
			},
			{
				ValueSat: *big.NewInt(7289000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "0014cc8067093f6f843d6d3e22004a4290cd0c0f336b",
					Addresses: []string{
						"tgrs1qejqxwzfld7zr6mf7ygqy5s5se5xq7vmt9lkd57",
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
		parser *GroestlcoinParser
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "grs-1",
			args: args{
				tx:     testTx1,
				parser: NewGroestlcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
		{
			name: "grs-2",
			args: args{
				tx:     testTx2,
				parser: NewGroestlcoinParser(GetChainParams("test"), &btc.Configuration{}),
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
		parser    *GroestlcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "grs-1",
			args: args{
				tx:        testTx1,
				height:    2161062,
				blockTime: 1531052031,
				parser:    NewGroestlcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "grs-2",
			args: args{
				tx:        testTx2,
				height:    650714,
				blockTime: 1530991050,
				parser:    NewGroestlcoinParser(GetChainParams("test"), &btc.Configuration{}),
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
		parser   *GroestlcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "grs-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewGroestlcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   2161062,
			wantErr: false,
		},
		{
			name: "grs-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewGroestlcoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   650714,
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
