// +build unittest

package verge

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
			args:    args{address: "DPw9hfaW4FJVE1Xy55NeUHNcukaAnnZLWj"},
			want:    "76a914ce2809bbb7fedefa334740afc6b37b499880c2e488ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "D9CkQHjZa1qVSer2e1iUNNwskrGVTReNJG"},
			want:    "76a9142c915b6cc7aafcc10cd5e81c3322a3e26a30144588ac",
			wantErr: false,
		},
	}
	parser := NewVergeParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "01000000971cd35d0155fd4db2798510d074d1e665b7e29de5d01882096d24c300c5cf6a0efa8c6d74010000006a47304402201350850167a4e18c4b28867aca9c35250ff6f8b3e830c26a1e0b747c1a28f73c0220188243adc5034dc27c6b50a1f20590168b068c4407cd2a5c7875f4f4c7a2c73b012102cd39c0fabc1e5ad696afb7d8df37d128810662358600280f535572b81c3fa24cfeffffff023cb63300000000001976a914ce2809bbb7fedefa334740afc6b37b499880c2e488acf64be24d000000001976a9142c915b6cc7aafcc10cd5e81c3322a3e26a30144588ac13103700"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "041800007ffa0fb1951eee28a23c18b6852186616455d72c2d744f4ae214ef31c53c16012a5aba0475f68ef4925eec889bd6885dcd29061b865fe87fe9b3f3d6fb319022a81cd35d89e8001bce2934400201000000a81cd35d010000000000000000000000000000000000000000000000000000000000000000ffffffff150317103704a81cd35d0139023c3a00000000000000ffffffff020000000000000000232103cfe60688d4d265f27a58594e31a25fceff1f566056e2019bb764d0dfdfcb098dac2071842b000000001976a91406ca227e4e7e4b9cf1116ee6c4197e80fe29179c88ac0000000001000000971cd35d0155fd4db2798510d074d1e665b7e29de5d01882096d24c300c5cf6a0efa8c6d74010000006a47304402201350850167a4e18c4b28867aca9c35250ff6f8b3e830c26a1e0b747c1a28f73c0220188243adc5034dc27c6b50a1f20590168b068c4407cd2a5c7875f4f4c7a2c73b012102cd39c0fabc1e5ad696afb7d8df37d128810662358600280f535572b81c3fa24cfeffffff023cb63300000000001976a914ce2809bbb7fedefa334740afc6b37b499880c2e488acf64be24d000000001976a9142c915b6cc7aafcc10cd5e81c3322a3e26a30144588ac13103700473045022100cf9d88febf043c49921b89f37937b6cf7d765c88eed77c2563665e5575c4dab0022065fd120ffd512763e3bc38df226b3f0a2aa6a0b8df372c67b25a0233114bd648",
		Blocktime: 1574116520,
		Txid:      "f27ed64e32e3796c2797e17de233483bea579940a9a2a3e2178a7712da62d9d4",
		LockTime:  3608595,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
				Hex: "47304402201350850167a4e18c4b28867aca9c35250ff6f8b3e830c26a1e0b747c1a28f73c0220188243adc5034dc27c6b50a1f20590168b068c4407cd2a5c7875f4f4c7a2c73b012102cd39c0fabc1e5ad696afb7d8df37d128810662358600280f535572b81c3fa24c",
				},
				Txid:     "746d8cfa0e6acfc500c3246d098218d0e59de2b765e6d174d0108579b24dfd55",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(3388988),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ce2809bbb7fedefa334740afc6b37b499880c2e488ac",
					Addresses: []string{
						"DPw9hfaW4FJVE1Xy55NeUHNcukaAnnZLWj",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1306676214),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9142c915b6cc7aafcc10cd5e81c3322a3e26a30144588ac",
					Addresses: []string{
						"D9CkQHjZa1qVSer2e1iUNNwskrGVTReNJG",
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
		parser    *VergeParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "verge-1",
			args: args{
				tx:        testTx1,
				height:    7000002,
				blockTime: 1574116520,
				parser:    NewVergeParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *VergeParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "verge-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewVergeParser(GetChainParams("main"), &btc.Configuration{}),
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