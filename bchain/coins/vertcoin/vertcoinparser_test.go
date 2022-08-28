//go:build unittest

package vertcoin

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
			args:    args{address: "Ve7SkjuVgVm1z3X8aqqxnzGV6GraUcde6K"},
			want:    "76a9142d20ebccc60a8b7dcba607701c6e5bda05eb6d9a88ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "Vavx3LENdGctDMsaVm426McFE5iDDrSuk5"},
			want:    "76a9140a3c6a3970a5b6b94ee0c1da82d6a87899d169da88ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "36c8VAv74dPZZa4cFayb92hzozkPL4fBPe"},
			want:    "a91435ec06fa05f2d3b16e88cd7eda7651a10ca2e01987",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "38A1RNvbA5c9wNRfyLVn1FCH5TPKJVG8YR"},
			want:    "a91446eb90e002f137f05385896c882fe000cc2e967f87",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "vtc1qd80qaputavyhtvszlz9zprueqch0qd003g520j"},
			want:    "001469de0e878beb0975b202f88a208f99062ef035ef",
			wantErr: false,
		},
	}
	parser := NewVertcoinParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "000e87768bb386b878010000000146fd781834a34e0399ccda1edf9ec47d715e17d904ad0958d533a240b3605ad6000000006a473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3cfdffffff020882a400000000001976a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac80f0fa020000000017a91446eb90e002f137f05385896c882fe000cc2e967f8774870e00"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "010000000146fd781834a34e0399ccda1edf9ec47d715e17d904ad0958d533a240b3605ad6000000006a473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3cfdffffff020882a400000000001976a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac80f0fa020000000017a91446eb90e002f137f05385896c882fe000cc2e967f8774870e00",
		Blocktime: 1529925180,
		Txid:      "d58c11aa970449c3e0ee5e0cdf78532435a9d2b28a2da284a8dd4dd6bdd0331c",
		LockTime:  952180,
		VSize:     223,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3c",
				},
				Txid:     "d65a60b340a233d55809ad04d9175e717dc49edf1edacc99034ea3341878fd46",
				Vout:     0,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(10781192),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac",
					Addresses: []string{
						"Vp1UqzsmVecaexfbWFGSFFL5x1g2XQnrGR",
					},
				},
			},
			{
				ValueSat: *big.NewInt(50000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a91446eb90e002f137f05385896c882fe000cc2e967f87",
					Addresses: []string{
						"38A1RNvbA5c9wNRfyLVn1FCH5TPKJVG8YR",
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
		parser    *VertcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "vertcoin-1",
			args: args{
				tx:        testTx1,
				height:    952182,
				blockTime: 1529925180,
				parser:    NewVertcoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *VertcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "vertcoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewVertcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   952182,
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
