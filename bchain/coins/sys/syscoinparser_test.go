// +build unittest

package syscoin

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
			args:    args{address: "SeqvAeauAKjrFaZKGQAHwhpdDr3PZek1rv"},
			want:    "76a914c083633e8928e5e046c3b97b7046eda00472da0c88ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "SeqvAeauAKjrFaZKGQAHwhpdDr3PZek1rv"},
			want:    "76a9148c1dbe285eb038fb34a83d3c2fdc768b281b6e3d88ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{address: "3GPo5ppEqtSTkV5yJUY34RenzJsC7nMLDJ"},
			want:    "a914a14818ddd4f921db4c22ee059c4a058320259b2187",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "sys1qtlgqnm0z94a22yn02zm5mfph909atx9nqsf3ew"},
			want:    "00145fd009ede22d7aa5126f50b74da4372bcbd598b3",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthash",
			args:    args{address: "sys1qsvdeu7gje7d2hurh0wpypwddjaqj2smmadnczutt0xwyp7q9z2csa7hp20"},
			want:    "0020831b9e7912cf9aabf0777b8240b9ad974125437beb6781716b799c40f80512b1",
			wantErr: false,
		},
	}
	parser := NewSyscoinParser(GetChainParams("main"), &btc.Configuration{})

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
	testTx1       bchain.Tx
	testTxPacked1 = "02000000000101dd76e9f66483e9e20ac64649b4b29fbfefd0cdaa802dde8b3417da2257569f3a0100000000fdffffff029cf4b9bc7b490000160014d6764e092d0d611c989ee640e49e745886d758ed00e057eb481b0000160014a967abb7bc99cde6554dafe9f54eb8462c08504102473044022071e8e6e2a0686b0a0965fd30b212b8e61c9d78c5e11b73215be079786d2f1d8b02205e60b2054e3f8f348f8f3f444727b81dac8da98fc3d483dbc9b847abdd7c298c0121036dd2e1775086670b888c67e52ea5005ca45cf9df77a7441b56f4a8f71d3bdfb437170400"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "000417398bdeb4813e02000000000101dd76e9f66483e9e20ac64649b4b29fbfefd0cdaa802dde8b3417da2257569f3a0100000000fdffffff029cf4b9bc7b490000160014d6764e092d0d611c989ee640e49e745886d758ed00e057eb481b0000160014a967abb7bc99cde6554dafe9f54eb8462c08504102473044022071e8e6e2a0686b0a0965fd30b212b8e61c9d78c5e11b73215be079786d2f1d8b02205e60b2054e3f8f348f8f3f444727b81dac8da98fc3d483dbc9b847abdd7c298c0121036dd2e1775086670b888c67e52ea5005ca45cf9df77a7441b56f4a8f71d3bdfb437170400,",
		Blocktime: 1575387231,
		Txid:      "20bdf7a7b9061124f31da34e1cb8df8bfd53e6d70de03e42df4a2488adca897c",
		LockTime:  268087,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "3a9f565722da17348bde2d80aacdd0efbf9fb2b44946c60ae2e98364f6e976dd",
				Vout:     1,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(80795796108444),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "0014d6764e092d0d611c989ee640e49e745886d758ed",
					Addresses: []string{
						"sys1q6emyuzfdp4s3exy7ueqwf8n5tzrdwk8dzu5g55",
					},
				},
			},
			{
				ValueSat: *big.NewInt(30000000000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "0014a967abb7bc99cde6554dafe9f54eb8462c085041",
					Addresses: []string{
						"sys1q49n6hdaun8x7v42d4l5l2n4cgckqs5zptp9fmf",
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
		parser    *SyscoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "syscoin-1",
			args: args{
				tx:        testTx1,
				height:    268089,
				blockTime: 1575387231,
				parser:    NewSyscoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *SyscoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "syscoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewSyscoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   268089,
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
