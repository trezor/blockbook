//go:build unittest

package litecoin

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
			args:    args{address: "mgPdTgEq6YqUJ4yzQgR8jH5TCX5c5yRwCP"},
			want:    "76a91409957dfdb3eb620a94b99857e13949551584c33688ac",
			wantErr: false,
		},
		{
			name:    "P2SH1-legacy",
			args:    args{address: "2MvGVySztevmycxrSmMRjJaVj2iJin7qpap"},
			want:    "a9142126232e3f47ae0f1246ec5f05fc400d83c86a0d87",
			wantErr: false,
		},
		{
			name:    "P2SH2-legacy",
			args:    args{address: "2N9a2TNzWz1FEKGFxUdMEh62V83URdZ5QAZ"},
			want:    "a914b31049e7ee51501fe19e3e0cdb803dc84cf99f9e87",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "QPdG6Ts8g2q4m9cVPTTkPGwAB6kYgXB7Hc"},
			want:    "a9142126232e3f47ae0f1246ec5f05fc400d83c86a0d87",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "QcvnaPrm17JKTT216jPFmnTvGRvFX2fWzN"},
			want:    "a914b31049e7ee51501fe19e3e0cdb803dc84cf99f9e87",
			wantErr: false,
		},
	}
	parser := NewLitecoinParser(GetChainParams("test"), &btc.Configuration{})

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
			args:    args{address: "LgJGe7aKy1wfXESKhiKeRWj6z4KjzCfXNW"},
			want:    "76a914e72ba56ab6afccac045d696b979e3b5077e88d1988ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "LiTVReQ6N8rWc2pNg2XMwCWq7A9P15teWg"},
			want:    "76a914feda50542e61108cf53b93dbffa0959f91ccb32588ac",
			wantErr: false,
		},
		{
			name:    "P2PKH3 - bech32 prefix",
			args:    args{address: "LTC1eqUzePT9uvpvb413Ejd6P8Cx1Ei8Di"},
			want:    "76a91457630115300a625f5deaab64100faa5506c1422f88ac",
			wantErr: false,
		},
		{
			name:    "P2PKH4 - bech32 prefix",
			args:    args{address: "LTC1f9gtb7bU6B4VjHXvPGDi8ACNZhkKPo"},
			want:    "76a9145763023d3f02509644dacbfc45f2c9102129749788ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "MLTQ8niHMnpJLNvK72zBeY91hQmUtoo8nX"},
			want:    "a91489ba6cf45546f91f1bdf553e695d63fc6b8795bd87",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "MAVWzxXm8KGkZTesqLtqywzrvbs96FEoKy"},
			want:    "a9141c6fbaf46d64221e80cbae182c33ddf81b9294ac87",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "ltc1q5fgkuac9s2ry56jka5s6zqsyfcugcchrqgz2yl"},
			want:    "0014a2516e770582864a6a56ed21a102044e388c62e3",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{address: "ltc1qu9dgdg330r6r84g5mw7wqshg04exv2uttmw2elfwx74h5tgntuzsk3x5nd"},
			want:    "0020e15a86a23178f433d514dbbce042e87d72662b8b5edcacfd2e37ab7a2d135f05",
			wantErr: false,
		},
	}
	parser := NewLitecoinParser(GetChainParams("main"), &btc.Configuration{})

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

func TestGetAddressesFromAddrDesc_Mainnet(t *testing.T) {
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
			name:    "P2PKH1",
			args:    args{script: "76a914feda50542e61108cf53b93dbffa0959f91ccb32588ac"},
			want:    []string{"LiTVReQ6N8rWc2pNg2XMwCWq7A9P15teWg"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{script: "76A9145763023D3F02509644DACBFC45F2C9102129749788AC"},
			want:    []string{"LTC1f9gtb7bU6B4VjHXvPGDi8ACNZhkKPo"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{script: "a9141c6fbaf46d64221e80cbae182c33ddf81b9294ac87"},
			want:    []string{"MAVWzxXm8KGkZTesqLtqywzrvbs96FEoKy"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{script: "0014a2516e770582864a6a56ed21a102044e388c62e3"},
			want:    []string{"ltc1q5fgkuac9s2ry56jka5s6zqsyfcugcchrqgz2yl"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{script: "0020e15a86a23178f433d514dbbce042e87d72662b8b5edcacfd2e37ab7a2d135f05"},
			want:    []string{"ltc1qu9dgdg330r6r84g5mw7wqshg04exv2uttmw2elfwx74h5tgntuzsk3x5nd"},
			want2:   true,
			wantErr: false,
		},
	}
	parser := NewLitecoinParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
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

var (
	testTx1 bchain.Tx

	testTxPacked1 = "0a201c50c1770374d7de2f81a87463a5225bb620d25fd467536223a5b715a47c9e3212c90102000000031e1977dc524bec5929e95d8d0946812944b7b5bda12f5b99fdf557773f2ee65e0100000000ffffffff8a398e44546dce0245452b90130e86832b21fd68f26662bc33aeb7c6c115d23c1900000000ffffffffb807ab93a7fcdff7af6d24581a4a18aa7c1db1ebecba2617a6805b009513940f0c00000000ffffffff020001a04a000000001976a9141ae882e788091732da6910595314447c9e38bd8d88ac27440f00000000001976a9146b474cbf0f6004329b630bdd4798f2c23d1751b688ac000000001890d5abd40528d3c807322a12205ee62e3f7757f5fd995b2fa1bdb5b744298146098d5de92959ec4b52dc77191e180128ffffffff0f322a12203cd215c1c6b7ae33bc6266f268fd212b83860e13902b454502ce6d54448e398a181928ffffffff0f322a12200f941395005b80a61726baecebb11d7caa184a1a58246daff7dffca793ab07b8180c28ffffffff0f3a450a044aa001001a1976a9141ae882e788091732da6910595314447c9e38bd8d88ac22224c4d67454e4e587a7a755078703776664d6a44724355343462736d72454d677176633a460a030f442710011a1976a9146b474cbf0f6004329b630bdd4798f2c23d1751b688ac22224c563142796a624a4e46544879465171777177644a584b4a7a6e59447a587a6734424002489101"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "02000000031e1977dc524bec5929e95d8d0946812944b7b5bda12f5b99fdf557773f2ee65e0100000000ffffffff8a398e44546dce0245452b90130e86832b21fd68f26662bc33aeb7c6c115d23c1900000000ffffffffb807ab93a7fcdff7af6d24581a4a18aa7c1db1ebecba2617a6805b009513940f0c00000000ffffffff020001a04a000000001976a9141ae882e788091732da6910595314447c9e38bd8d88ac27440f00000000001976a9146b474cbf0f6004329b630bdd4798f2c23d1751b688ac00000000",
		Blocktime: 1519053456,
		Time:      1519053456,
		Txid:      "1c50c1770374d7de2f81a87463a5225bb620d25fd467536223a5b715a47c9e32",
		LockTime:  0,
		Version:   2,
		VSize:     145,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "5ee62e3f7757f5fd995b2fa1bdb5b744298146098d5de92959ec4b52dc77191e",
				Vout:     1,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "3cd215c1c6b7ae33bc6266f268fd212b83860e13902b454502ce6d54448e398a",
				Vout:     25,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "0f941395005b80a61726baecebb11d7caa184a1a58246daff7dffca793ab07b8",
				Vout:     12,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1252000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9141ae882e788091732da6910595314447c9e38bd8d88ac",
					Addresses: []string{
						"LMgENNXzzuPxp7vfMjDrCU44bsmrEMgqvc",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1000487),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9146b474cbf0f6004329b630bdd4798f2c23d1751b688ac",
					Addresses: []string{
						"LV1ByjbJNFTHyFQqwqwdJXKJznYDzXzg4B",
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
		parser    *LitecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "litecoin-1",
			args: args{
				tx:        testTx1,
				height:    123987,
				blockTime: 1519053456,
				parser:    NewLitecoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *LitecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "litecoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewLitecoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   123987,
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
