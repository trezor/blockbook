// +build unittest

package gobyte

import (
	"encoding/hex"
  "os"
	"math/big"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)


var (
	testnetParser, mainnetParser *GoByteParser

	testTx1 = bchain.Tx{
		Hex:       "03000500010000000000000000000000000000000000000000000000000000000000000000ffffffff1f02a017044046b960082ffffff4000000000d2f6e6f64655374726174756d2f000000000240cf2318000000001976a914ed22e769238dd0f934eea51073629f92dcc28af688ac408e5338000000001976a914e8d55cde2e27389fdc8f04b831d59e39931f6f5c88ac00000000460200a01700004231359f788b64986d4a2c80161e3b688d9c8cdb1099b13ad5aab0972f0e67f80000000000000000000000000000000000000000000000000000000000000000",
		Blocktime: 1622754880,
		Time:      1622754880,
		Txid:      "8d64738add8ca229f247fbfe1263c84d124d80bbf6f08dc6f17c7da7f1f2da74",
		LockTime:  0,
		Version:   3,
		Vin: []bchain.Vin{
			{
				Coinbase:     "02a017044046b960082ffffff4000000000d2f6e6f64655374726174756d2f",
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(405000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ed22e769238dd0f934eea51073629f92dcc28af688ac",
					Addresses: []string{
						"nSUREo5TJ5dWqk3kdMHAcCDhctSSwGTqYb",
					},
				},
			},
			{
				ValueSat: *big.NewInt(945000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e8d55cde2e27389fdc8f04b831d59e39931f6f5c88ac",
					Addresses: []string{
						"nS5feNXxDejftdJKaGwdefQMYxkcYAhkot",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff1e02a0170422c41d5a0881004cd9df000000416c744d696e65722e4e65740000000000021c26b42c000000001976a9141c333960eff1c1104be135e792d5540d0f21300e88ac1c26b42c000000001976a9144e789acae87934eecd346718aca849b0d8408f6d88ac00000000",
		Txid:      "2a771edce522e87ba092ba1be99fbc3a6cf632d81e6a4f5f7b1b6fa3ee21678d",
		Blocktime: 1511900194,
		Time:      1511900194,
		LockTime:  0,
		Version:   2,
		Vin: []bchain.Vin{
			{
				Coinbase:     "02a0170422c41d5a0881004cd9df000000416c744d696e65722e4e657400",
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(750003740),
				N: 0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9141c333960eff1c1104be135e792d5540d0f21300e88ac",
					Addresses: []string{
						"GLR2hcYnWQwY7DXGerpr7DmZnSWGRkkg2o",
					},
				},
			},
			{
				ValueSat: *big.NewInt(750003740),
				N: 1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9144e789acae87934eecd346718aca849b0d8408f6d88ac",
					Addresses: []string{
						"GQzqbrAcGWvL1niRh65SLaayTRVq92YmNH",
					},
				},
			},
		},
	}

	testTx3 = bchain.Tx{
		Hex:       "03000500010000000000000000000000000000000000000000000000000000000000000000ffffffff1f02a117044c46b960082ffffff4000000000d2f6e6f64655374726174756d2f000000000240cf2318000000001976a914ed22e769238dd0f934eea51073629f92dcc28af688ac408e5338000000001976a914e8d55cde2e27389fdc8f04b831d59e39931f6f5c88ac00000000460200a11700004231359f788b64986d4a2c80161e3b688d9c8cdb1099b13ad5aab0972f0e67f80000000000000000000000000000000000000000000000000000000000000000",
		Blocktime: 1622754892,
		Time:      1622754892,
		Txid:      "da83cc6fad2eae8d7454e595eb2d1e9f48b264add40a6ce742be92756c23f6e4",
		LockTime:  0,
		Version:   3,
		Vin: []bchain.Vin{
			{
				Coinbase:     "02a117044c46b960082ffffff4000000000d2f6e6f64655374726174756d2f",
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(405000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ed22e769238dd0f934eea51073629f92dcc28af688ac",
					Addresses: []string{
						"nSUREo5TJ5dWqk3kdMHAcCDhctSSwGTqYb",
					},
				},
			},
			{
				ValueSat: *big.NewInt(945000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e8d55cde2e27389fdc8f04b831d59e39931f6f5c88ac",
					Addresses: []string{
						"nS5feNXxDejftdJKaGwdefQMYxkcYAhkot",
					},
				},
			},
		},
	}
)

func TestMain(m *testing.M) {
	testnetParser = NewGoByteParser(GetChainParams("testnet3"), &btc.Configuration{Slip44: 1})
	mainnetParser = NewGoByteParser(GetChainParams("mainnet"), &btc.Configuration{Slip44: 176})
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestGetAddrDescFromAddress(t *testing.T) {
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
			name:    "P2PKH",
			args:    args{address: "nSUREo5TJ5dWqk3kdMHAcCDhctSSwGTqYb"},
			want:    "6e535552456f35544a356457716b336b644d48416343446863745353774754715962",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "nS5feNXxDejftdJKaGwdefQMYxkcYAhkot"},
			want:    "6e533566654e587844656a6674644a4b614777646566514d59786b635941686b6f74",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testnetParser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestGetAddrDescFromVout(t *testing.T) {
	type args struct {
		vout bchain.Vout
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a914ed22e769238dd0f934eea51073629f92dcc28af688ac"}}},
			want:    "6e535552456f35544a356457716b336b644d48416343446863745353774754715962",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a914e8d55cde2e27389fdc8f04b831d59e39931f6f5c88ac"}}},
			want:    "6e533566654e587844656a6674644a4b614777646566514d59786b635941686b6f74",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testnetParser.GetAddrDescFromVout(&tt.args.vout)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddrDescFromVout() error = %v, wantErr %v", err, tt.wantErr)
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromVout() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestGetAddressesFromAddrDesc(t *testing.T) {
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
			args:    args{script: "6e535552456f35544a356457716b336b644d48416343446863745353774754715962"},
			want:    []string{"nSUREo5TJ5dWqk3kdMHAcCDhctSSwGTqYb"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{script: "6e533566654e587844656a6674644a4b614777646566514d59786b635941686b6f74"},
			want:    []string{"nS5feNXxDejftdJKaGwdefQMYxkcYAhkot"},
			want2:   true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := testnetParser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
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

func TestPackAndUnpack(t *testing.T) {
	tests := []struct {
		name   string
		txInfo *bchain.Tx
		height uint32
		parser *GoByteParser
	}{
		{
			name:   "Test_1",
			txInfo: &testTx1,
			height: 6048,
			parser: testnetParser,
		},
		{
			name:   "Test_2",
			txInfo: &testTx2,
			height: 6048,
			parser: mainnetParser,
		},
		{
			name:   "Test_3",
			txInfo: &testTx3,
			height: 6049,
			parser: testnetParser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packedTx, err := tt.parser.PackTx(tt.txInfo, tt.height, tt.txInfo.Blocktime)
			if err != nil {
				t.Errorf("PackTx() expected no error but got %v", err)
				return
			}

			unpackedtx, gotHeight, err := tt.parser.UnpackTx(packedTx)
			if err != nil {
				t.Errorf("PackTx() expected no error but got %v", err)
				return
			}

			if !reflect.DeepEqual(tt.txInfo, unpackedtx) {
				t.Errorf("TestPackAndUnpack() expected the raw tx and the unpacked tx to match but they didn't")
			}

			if gotHeight != tt.height {
				t.Errorf("TestPackAndUnpack() = got height %v, but want %v", gotHeight, tt.height)
			}
		})
	}
}
