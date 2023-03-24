//go:build unittest

package monetaryunit

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
			name:    "P2PKH1",
			args:    args{address: "7cTAePV4rJoSpa6eLhUPSuPpGLdsLiWnXf"},
			want:    "76a9146e2b0a8655786c8c5ea7b9ce478f03e00ecb2f5588ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "XdmBSxuCLajxZzQQWiY2FEk6XVjiVoqLXW"},
			want:    "a91421ba6a62ac1d74d2ba921bbc8c9a3ca6e1420a0087",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "bc1q0v3tadxj6pm3ym9j06v9rfyw0jeh5f8squ3nvt"},
			want:    "00147b22beb4d2d077126cb27e9851a48e7cb37a24f0",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "bc1qumpyvyxz25kfjjrvyxn3zlyc2wfc0m3l3gm5pg99c4mxylemfqhsdf5q0k"},
			want:    "0020e6c24610c2552c99486c21a7117c98539387ee3f8a3740a0a5c576627f3b482f",
			wantErr: false,
		},
	}
	parser := NewMonetaryUnitParser(GetChainParams("main"), &btc.Configuration{})

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

func Test_GetAddressesFromAddrDesc(t *testing.T) {
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
			args:    args{script: "76a9146e2b0a8655786c8c5ea7b9ce478f03e00ecb2f5588ac"},
			want:    []string{"7cTAePV4rJoSpa6eLhUPSuPpGLdsLiWnXf"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a91421ba6a62ac1d74d2ba921bbc8c9a3ca6e1420a0087"},
			want:    []string{"XdmBSxuCLajxZzQQWiY2FEk6XVjiVoqLXW"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00147b22beb4d2d077126cb27e9851a48e7cb37a24f0"},
			want:    []string{"bc1q0v3tadxj6pm3ym9j06v9rfyw0jeh5f8squ3nvt"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "0020e6c24610c2552c99486c21a7117c98539387ee3f8a3740a0a5c576627f3b482f"},
			want:    []string{"bc1qumpyvyxz25kfjjrvyxn3zlyc2wfc0m3l3gm5pg99c4mxylemfqhsdf5q0k"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "OP_RETURN ascii",
			args:    args{script: "6a0461686f6a"},
			want:    []string{"OP_RETURN (ahoj)"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN hex",
			args:    args{script: "6a072020f1686f6a20"},
			want:    []string{"OP_RETURN 2020f1686f6a20"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewMonetaryUnitParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("outputScriptToAddresses() error = %v, wantErr %v", err, tt.wantErr)
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

	testTxPacked1 = "0a20f05ba72a05c4900ff2a00a0403697750201e41267aeea8a589a7dc7bcc57076e12d30101000000010c396f3768565c707addf85ecf47e04cefb2721d95afd977e13f25904de8336a0100000049483045022100fe1b79f38ca4b9dc2fbc50eac6c9bf050ae5a3ee37da05b950918230eb0a8c7c0220477173b60ec00a8b4b28d5db9fc5d8259eea35f91bbdd60089c5ec1be7b05f3b01ffffffff03000000000000000000dd400def140000002321025d145b77df04c40ceb88ea36828755f8275dc5fecf19d3ecccce2d8198c3407cac00d2496b000000001976a914afe70b2e1bf4199298ed8281767bae22970b415088ac0000000018e6b092e60528f48d1b327512206a33e84d90253fe177d9af951d72b2ef4ce047cf5ef8dd7a705c5668376f390c18012249483045022100fe1b79f38ca4b9dc2fbc50eac6c9bf050ae5a3ee37da05b950918230eb0a8c7c0220477173b60ec00a8b4b28d5db9fc5d8259eea35f91bbdd60089c5ec1be7b05f3b0128ffffffff0f3a0222003a520a0514ef0d40dd10011a2321025d145b77df04c40ceb88ea36828755f8275dc5fecf19d3ecccce2d8198c3407cac22223764396a4e79716835694b555a566e516a6d7a6d4541513878444b777a4536536e673a470a046b49d20010021a1976a914afe70b2e1bf4199298ed8281767bae22970b415088ac22223769536a6e57436f41556d4a347656584d6b61457561736e5658537a5a7166504c324001"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000010c396f3768565c707addf85ecf47e04cefb2721d95afd977e13f25904de8336a0100000049483045022100fe1b79f38ca4b9dc2fbc50eac6c9bf050ae5a3ee37da05b950918230eb0a8c7c0220477173b60ec00a8b4b28d5db9fc5d8259eea35f91bbdd60089c5ec1be7b05f3b01ffffffff03000000000000000000dd400def140000002321025d145b77df04c40ceb88ea36828755f8275dc5fecf19d3ecccce2d8198c3407cac00d2496b000000001976a914afe70b2e1bf4199298ed8281767bae22970b415088ac00000000",
		Blocktime: 1556387942,
		Txid:      "f05ba72a05c4900ff2a00a0403697750201e41267aeea8a589a7dc7bcc57076e",
		LockTime:  0,
		Time:      1556387942,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100fe1b79f38ca4b9dc2fbc50eac6c9bf050ae5a3ee37da05b950918230eb0a8c7c0220477173b60ec00a8b4b28d5db9fc5d8259eea35f91bbdd60089c5ec1be7b05f3b01",
				},
				Txid:     "6a33e84d90253fe177d9af951d72b2ef4ce047cf5ef8dd7a705c5668376f390c",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "",
					Addresses: []string{
						"",
					},
				},
			},
			{
				ValueSat: *big.NewInt(89909969117),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "21025d145b77df04c40ceb88ea36828755f8275dc5fecf19d3ecccce2d8198c3407cac",
					Addresses: []string{
						"7d9jNyqh5iKUZVnQjmzmEAQ8xDKwzE6Sng",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1800000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914afe70b2e1bf4199298ed8281767bae22970b415088ac",
					Addresses: []string{
						"7iSjnWCoAUmJ4vVXMkaEuasnVXSzZqfPL2",
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
		parser    *MonetaryUnitParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "monetaryunit-1",
			args: args{
				tx:        testTx1,
				height:    444148,
				blockTime: 1556387942,
				parser:    NewMonetaryUnitParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *MonetaryUnitParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "monetaryunit-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewMonetaryUnitParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   444148,
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
