//go:build unittest

package digibyte

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

func TestAddressToOutputScript_Mainnet(t *testing.T) {
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
			args:    args{address: "DFDe9ne77eEUKUijjG4EpDwW9vDxckGgHN"},
			want:    "76a9146e8d4f7f0dfeb5d69b9a2cf914a1a2e276312b2188ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "DPUnoXeaSDnNtQTa7U3nEMTYBVgJ6wVgCh"},
			want:    "76a914c92bc70927a752deb91cf0361dcdb60bdac6a1d588ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "SgbK2hJXBUccpQgj41fR4VMZqVPesPZgzC"},
			want:    "a914d3b07c1aaea886f8ceddedec440623f812e49ddc87",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "SRrevBM5bfZNpFJ4MhzaNfkTghYKoTB6LV"},
			want:    "a914320d7056c33fd8d0f5bb9cf42d74133dc28d89bb87",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "dgb1qwdzu3cxvcte7g5f8ze4dsn83wf6x8zdvw927dx"},
			want:    "00147345c8e0ccc2f3e45127166ad84cf172746389ac",
			wantErr: false,
		},
	}
	parser := NewDigiByteParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "006acfc28bb5a0fe3c01000000015952b77fedb7233936a48df90e5423d62539efcd3f61a2466501d6ca26554d5e000000006b483045022100fad176fc354e976e0250ca880085f98c7072d5ccd2978d462f981c7c14495a9102207e60e2fa7b58991909f7528aaf3efb4acc5000da299b84b5d7b065b2ef1a2ac601210322e516129a8e55c043a7ac1d01f3c8fc192b88c32f03746f558432fe6a601e05ffffffff0217930054020000001976a914d9eff329b03348487fb9688a77c6b8081794fd2288ac29a9750f0a0000001976a914ad554bde320ae4b3d755ec32e706d8b327ecdd5688acc0cf6a00"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000015952b77fedb7233936a48df90e5423d62539efcd3f61a2466501d6ca26554d5e000000006b483045022100fad176fc354e976e0250ca880085f98c7072d5ccd2978d462f981c7c14495a9102207e60e2fa7b58991909f7528aaf3efb4acc5000da299b84b5d7b065b2ef1a2ac601210322e516129a8e55c043a7ac1d01f3c8fc192b88c32f03746f558432fe6a601e05ffffffff0217930054020000001976a914d9eff329b03348487fb9688a77c6b8081794fd2288ac29a9750f0a0000001976a914ad554bde320ae4b3d755ec32e706d8b327ecdd5688acc0cf6a00",
		Blocktime: 1532239774,
		Txid:      "0dcf2530419b9ef525a69f6a15e4d699be1dc9a4ac643c9581b6c57acf25eabf",
		LockTime:  7000000,
		VSize:     226,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100fad176fc354e976e0250ca880085f98c7072d5ccd2978d462f981c7c14495a9102207e60e2fa7b58991909f7528aaf3efb4acc5000da299b84b5d7b065b2ef1a2ac601210322e516129a8e55c043a7ac1d01f3c8fc192b88c32f03746f558432fe6a601e05",
				},
				Txid:     "5e4d5526cad6016546a2613fcdef3925d623540ef98da4363923b7ed7fb75259",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(9999258391),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914d9eff329b03348487fb9688a77c6b8081794fd2288ac",
					Addresses: []string{
						"DR1Scto7rRpTCAyNY6DFhRsZgG2vZ6rxLx",
					},
				},
			},
			{
				ValueSat: *big.NewInt(43209042217),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ad554bde320ae4b3d755ec32e706d8b327ecdd5688ac",
					Addresses: []string{
						"DLwbcwUY5dwsdF9ezk8Uoga9V8eMB1Kb6B",
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
		parser    *DigiByteParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "digibyte-1",
			args: args{
				tx:        testTx1,
				height:    7000002,
				blockTime: 1532239774,
				parser:    NewDigiByteParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *DigiByteParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "digibyte-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewDigiByteParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   7000002,
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
