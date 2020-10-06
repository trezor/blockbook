// +build unittest

package bcd

import (
	"bytes"
	"encoding/hex"
	"fmt"
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

	testTxPacked1 = "0a2092e15e581f1df1f66fc4cda0b9ffe21af614417e9cdd98accbaa87f2c8cd42a412dd010c00000000000000000000000000000000000000000000000000000000000000000000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff1503ec6d0904e66bb55e023043092906000000000000ffffffff0213a0814a000000001976a9143aaebaf2cac6975116ea6f48a4b82a331ab581b488ac0000000000000000266a24aa21a9ede8d2d526595d1dc32b5c07b921dd8b7366a6051603e20282b88abb40a82f4c7b012000000000000000000000000000000000000000000000000000000000000000000000000018e6d7d5f505200028ecdb2532340a2a303365633664303930346536366262353565303233303433303932393036303030303030303030303030180028ffffffff0f3a470a044a81a01310001a1976a9143aaebaf2cac6975116ea6f48a4b82a331ab581b488ac222231364d485762675a5a416b6e325176676155315431783979765762615575676142534000"
	testTxPacked2 = "0a20b715206ffb15b02d8b55e826f62048accac8c4bc083c832d53fc40c044506ec812c101010000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff1b03ee6d09043d74b55e087102008138712300082f6666706f6f6c2f0000000002bd82814a0000000017a9148ba7c2aa02d973d7cf403604eee3ad7a98e0a535870000000000000000266a24aa21a9edba8f1b231c491442f6d23b9233c65ed26daa901e7350861fdcfc8d2f12fe02b3012000000000000000000000000000000000000000000000000000000000000000000000000018bde8d5f505200028eedb2532400a36303365653664303930343364373462353565303837313032303038313338373132333030303832663636363637303666366636633266180028feffffff0f3a450a044a8182bd10001a17a9148ba7c2aa02d973d7cf403604eee3ad7a98e0a53587222233455253705155647932535835726642514b634e6e484735544d666a753842786a514000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0c00000000000000000000000000000000000000000000000000000000000000000000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff1503ec6d0904e66bb55e023043092906000000000000ffffffff0213a0814a000000001976a9143aaebaf2cac6975116ea6f48a4b82a331ab581b488ac0000000000000000266a24aa21a9ede8d2d526595d1dc32b5c07b921dd8b7366a6051603e20282b88abb40a82f4c7b0120000000000000000000000000000000000000000000000000000000000000000000000000",
		Blocktime: 1588947942,
		Time:      1588947942,
		Txid:      "92e15e581f1df1f66fc4cda0b9ffe21af614417e9cdd98accbaa87f2c8cd42a4",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				Coinbase: "03ec6d0904e66bb55e023043092906000000000000",
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1250009107),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9143aaebaf2cac6975116ea6f48a4b82a331ab581b488ac",
					Addresses: []string{
						"16MHWbgZZAkn2QvgaU1T1x9yvWbaUugaBS",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "010000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff1b03ee6d09043d74b55e087102008138712300082f6666706f6f6c2f0000000002bd82814a0000000017a9148ba7c2aa02d973d7cf403604eee3ad7a98e0a535870000000000000000266a24aa21a9edba8f1b231c491442f6d23b9233c65ed26daa901e7350861fdcfc8d2f12fe02b30120000000000000000000000000000000000000000000000000000000000000000000000000",
		Blocktime: 1588950077,
		Time:      1588950077,
		Txid:      "b715206ffb15b02d8b55e826f62048accac8c4bc083c832d53fc40c044506ec8",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				Coinbase: "03ee6d09043d74b55e087102008138712300082f6666706f6f6c2f",
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1250001597),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9148ba7c2aa02d973d7cf403604eee3ad7a98e0a53587",
					Addresses: []string{
						"3ERSpQUdy2SX5rfBQKcNnHG5TMfju8BxjQ",
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
		parser *BdiamondParser
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "bcd-1",
			args: args{
				tx:     testTx1,
				parser: NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
		{
			name: "bcd-2",
			args: args{
				tx:     testTx2,
				parser: NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser    *BdiamondParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "bcd-1",
			args: args{
				tx:        testTx1,
				height:    617964,
				blockTime: 1588947942,
				parser:    NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "bcd-2",
			args: args{
				tx:        testTx2,
				height:    617966,
				blockTime: 1588950077,
				parser:    NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
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
			fmt.Println("Encoded:", h)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("packTx() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestUnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *BdiamondParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "bcd-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   617964,
			wantErr: false,
		},
		{
			name: "bcd-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewBdiamondParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   617966,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		fmt.Println("DecodeString:")
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
