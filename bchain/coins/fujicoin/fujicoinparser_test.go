//go:build unittest

package fujicoin

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
			args:    args{address: "FgKfZA7QB5xHGTtpPqTF1RQRyHCCTr5NXg"},
			want:    "76a9147a5ab68256ac6bd8dc6d78b6b94af69e414dffd588ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "FsjXKBTE9nQinJnc1r2iLEb8SX7ok1eC7V"},
			want:    "76a914f787334ddbda3e65fe7d579f03842b5607cf0db888ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "7ojX5QT534BBDPwNoYXjx1MdbHWS2ueMk3"},
			want:    "a914e9ec22a509631fa3c8c09ebb1b41f59c81ec669f87",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "7XJah1ejjogvQS7UkfiH577R1yzkVzNBn4"},
			want:    "a91435b2bd909a0b9b6ca35fef9276675f568f42a4e387",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "fc1q2ze0qtna2a42qqam3k9xsu3xamrrcp8rsczdve"},
			want:    "001450b2f02e7d576aa003bb8d8a687226eec63c04e3",
			wantErr: false,
		},
	}
	parser := NewFujicoinParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "0023f3648bc2d3dd5c0100000001ec5666d4e842926c6e872fd7f12a87183b298464716dec1b03fc956777e06a70010000006a473044022062949ffe184af6bdc8d77e07c2caf1bd4e36d977bc27e24ab6d24c9e735d76b802201f42950f6fa005126f1fbd5cb9d6f0838d94adb70a05517b3eebd144a0ec2fd6012102534f95a0a13bbc3e261f966edd057a1dc9819a4e6bb21d81a633d5500d184a7bfeffffff0200e876481700000017a9146b8a69f8b43bc06b886c0668c93197be5152e8598740494d552e0000001976a914df783f980b38d7a64cee23202af95609b547fde888ac62f32300"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0100000001ec5666d4e842926c6e872fd7f12a87183b298464716dec1b03fc956777e06a70010000006a473044022062949ffe184af6bdc8d77e07c2caf1bd4e36d977bc27e24ab6d24c9e735d76b802201f42950f6fa005126f1fbd5cb9d6f0838d94adb70a05517b3eebd144a0ec2fd6012102534f95a0a13bbc3e261f966edd057a1dc9819a4e6bb21d81a633d5500d184a7bfeffffff0200e876481700000017a9146b8a69f8b43bc06b886c0668c93197be5152e8598740494d552e0000001976a914df783f980b38d7a64cee23202af95609b547fde888ac62f32300",
		Blocktime: 1546286958,
		Txid:      "7367db1a3073146f0b6060ce4a6dbb96b67b7aadcbaa450bff8d7dedda52ec13",
		LockTime:  2356066,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022062949ffe184af6bdc8d77e07c2caf1bd4e36d977bc27e24ab6d24c9e735d76b802201f42950f6fa005126f1fbd5cb9d6f0838d94adb70a05517b3eebd144a0ec2fd6012102534f95a0a13bbc3e261f966edd057a1dc9819a4e6bb21d81a633d5500d184a7b",
				},
				Txid:     "706ae0776795fc031bec6d716484293b18872af1d72f876e6c9242e8d46656ec",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(100000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9146b8a69f8b43bc06b886c0668c93197be5152e85987",
					Addresses: []string{
						"7cDGsS8Dum191ZMD19UaLzstMtf29YMXBL",
					},
				},
			},
			{
				ValueSat: *big.NewInt(198999624000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914df783f980b38d7a64cee23202af95609b547fde888ac",
					Addresses: []string{
						"FqYKBhEF4zKkDGkVyufGVGajhttuD4Usc2",
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
		parser    *FujicoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "fujicoin-1",
			args: args{
				tx:        testTx1,
				height:    2356068,
				blockTime: 1546286958,
				parser:    NewFujicoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *FujicoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "fujicoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewFujicoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   2356068,
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
