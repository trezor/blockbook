// +build unittest

package iocoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
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
			args:    args{address: "n2g8y2p6cTnTkc85Dv4VPq3KkMThe4njwX"},
			want:    "76a914e8174b2eb9b4e98f246e7e0d7508baffe054069e88ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "mtwoH47yhXpaYoBQZroeEaTfL8BoumHJAi"},
			want:    "76a914934c81cc9c34edf61ed38455569fb1daf76a58f088ac",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "n2KbtwJSiy9SRvDWwD8AFMCiMNKKRhyCFQ"},
			want:    "76a914e43509bb27cbb57f526b5d80b64e587361ea0c0d88ac",
			wantErr: false,
		},
	}
	parser := NewIocoinParser(GetChainParams("test"), &btc.Configuration{})

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
			args:    args{address: "iZEKH2CYJWvjLVMJ8PL1uQEM55aYNtfMkf"},
			want:    "76a9144663cd78e94a1db21d0ca930867f27738b3cad2388ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "iaNjzujDJaifihYp48Ln2ygi6XAkg3Q5U5"},
			want:    "76a91452f3e4a6c1bc498f05bd8553c132503262b0b58088ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "in4TTYRvnNXqcA7TMLLrbPjx4Zp8zwCc7b"},
			want:    "76a914d3201b9157fac49c850b1cab16974bf37081056488ac",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "iT9kjJ2yzTQZfrNKh87YCTTJZeG9PtGRg4"},
			want:    "76a91403b637170387d18c6e73cc3b840c4fafab0cc3a488ac",
			wantErr: false,
		},
	}
	parser := NewIocoinParser(GetChainParams("main"), &btc.Configuration{})

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

//	testTxPacked1 = "01000000e0e9255401860e16929f8a7d3bf71746e881a6c55754550bd160951ed3d2e6f98c4ee2832d0200000048473044022008d7e21c321fd918d462c1cb223f37bd0508f6fd51e764c6315b130ce90807bb022075b9d62f3decec0c31437fa954f79f076ba9f9e484a12f00ca104f4e2af9cb1901ffffffff03000000000000000000809d526d0400000023210249690df08ead343f07c32660f1b27befd3465c02d953df8b0cc59e6473fd9dc8acdce25d6d0400000023210249690df08ead343f07c32660f1b27befd3465c02d953df8b0cc59e6473fd9dc8ac00000000"
	testTxPacked1 = "002281bd8bc9f2b14001000000934b9e5c0148bd22bbe1f0e700121d11380b6ccba11d0b5b7b10af3241b53514a6d7c1a050000000006a4730440220744503f4c1f7dd5bc585c2b5ae93f1d8b4831ac9cc1e1a454078315eaf489f29022019c74bf3caff3d31795a5d636d94bea7c0144c650269230c01ea3fafddf6ddb301210301f27030e7b0b825e68ad924bff69ab9521115a7bf1927f9723b8893140beb0effffffff0297ad7c47060000001976a9147e6efb8717c64ceff55e0ebbc58d7efa506afaaa88ac007cdcac040000001976a914622aa5df3e1dabe518f28adbcb6ea95d626548f188ac00000000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000934b9e5c0148bd22bbe1f0e700121d11380b6ccba11d0b5b7b10af3241b53514a6d7c1a050000000006a4730440220744503f4c1f7dd5bc585c2b5ae93f1d8b4831ac9cc1e1a454078315eaf489f29022019c74bf3caff3d31795a5d636d94bea7c0144c650269230c01ea3fafddf6ddb301210301f27030e7b0b825e68ad924bff69ab9521115a7bf1927f9723b8893140beb0effffffff0297ad7c47060000001976a9147e6efb8717c64ceff55e0ebbc58d7efa506afaaa88ac007cdcac040000001976a914622aa5df3e1dabe518f28adbcb6ea95d626548f188ac00000000",
		Blocktime: 1553878112,
		Txid:      "768f11e50fb5137fd5605f5a43b914bb8a88f07015583ade5fedb15299aa5ab3",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					//Asm : "3044022008d7e21c321fd918d462c1cb223f37bd0508f6fd51e764c6315b130ce90807bb022075b9d62f3decec0c31437fa954f79f076ba9f9e484a12f00ca104f4e2af9cb1901",
					Hex: "4730440220744503f4c1f7dd5bc585c2b5ae93f1d8b4831ac9cc1e1a454078315eaf489f29022019c74bf3caff3d31795a5d636d94bea7c0144c650269230c01ea3fafddf6ddb301210301f27030e7b0b825e68ad924bff69ab9521115a7bf1927f9723b8893140beb0e",
				},
				Txid:     "50a0c1d7a61435b54132af107b5b0b1da1cb6c0b38111d1200e7f0e1bb22bd48",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(26969157015),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9147e6efb8717c64ceff55e0ebbc58d7efa506afaaa88ac",
					Addresses: []string{
						"ieLeWZH7vbXZAzNk3VGavNDwxP8WPyVz5H",
					},
				},
			},
			{
				ValueSat: *big.NewInt(20080000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					//Asm: "0249690df08ead343f07c32660f1b27befd3465c02d953df8b0cc59e6473fd9dc8",
					Hex: "76a914622aa5df3e1dabe518f28adbcb6ea95d626548f188ac",
					Addresses: []string{
						"ibmBjDSj4rjywWXJommFoAVkjLqGxxNPe8",
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
		parser    *IocoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "iocoin-1",
			args: args{
				tx:        testTx1,
				height:    2261437,
				blockTime: 1553878112,
				parser:    NewIocoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *IocoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "iocoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewIocoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   2261437,
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
