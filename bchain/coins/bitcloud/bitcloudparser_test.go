// +build unittest

package bitcloud

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
			name:    "pubkeyhash1",
			args:    args{address: "BHUkRKz6MJZr1Y2pbffgj8jh27PDwZccrE"},
			want:    "76a9148eea715ca825567003ca5ca2cc9696a90ae8551588ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "B72KRvTCgnVnRGo9daHCVbL5YtJHPx7G3W"},
			want:    "76a9141c398273cf8a3e7a2190de650b1dcb285e26933088ac",
			wantErr: false,
		},
	}
	parser := NewBitcloudParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "0a20621ba62690e934e603b225587ac68aa74b7223b9a5946c79f909a67dd1dd558b12ff010100000001325a1066d70f69cd2012e9d3c7a51bf3f758809def510e0bc079347d3b340fb40100000049483045022100d718cd6c483741923f3ab40e67f53e7a7e69c446046cd3e1d9be7abcb6e05cb502207a6a05f3e70690d50053e14ffdef9b35b449cb54c9800f9087f11975848237ae01ffffffff04000000000000000000002cbee70000000023210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac6721b1610000000023210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac80461c86000000001976a914881e1ddf7dbc1adfb3e3080cf53541ec7b37d6af88ac0000000018b1edd1ed05200028d9e60d32770a001220b40f343b7d3479c00b0e51ef9d8058f7f31ba5c7d3e91220cd690fd766105a3218012249483045022100d718cd6c483741923f3ab40e67f53e7a7e69c446046cd3e1d9be7abcb6e05cb502207a6a05f3e70690d50053e14ffdef9b35b449cb54c9800f9087f11975848237ae0128ffffffff0f3a0210003a510a04e7be2c0010011a23210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac222242363775624c72706b37565941567974313571474c774a4a334e4e3277546d41397a3a510a0461b1216710021a23210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac222242363775624c72706b37565941567974313571474c774a4a334e4e3277546d41397a3a470a04861c468010031a1976a914881e1ddf7dbc1adfb3e3080cf53541ec7b37d6af88ac22224247726f624d426d6e4b64596d514d52346f525139504e4c4b6e43484431545735514001"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0100000001325a1066d70f69cd2012e9d3c7a51bf3f758809def510e0bc079347d3b340fb40100000049483045022100d718cd6c483741923f3ab40e67f53e7a7e69c446046cd3e1d9be7abcb6e05cb502207a6a05f3e70690d50053e14ffdef9b35b449cb54c9800f9087f11975848237ae01ffffffff04000000000000000000002cbee70000000023210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac6721b1610000000023210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac80461c86000000001976a914881e1ddf7dbc1adfb3e3080cf53541ec7b37d6af88ac00000000",
		Blocktime: 1572107953,
		Time:      1572107953,
		Txid:      "621ba62690e934e603b225587ac68aa74b7223b9a5946c79f909a67dd1dd558b",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100d718cd6c483741923f3ab40e67f53e7a7e69c446046cd3e1d9be7abcb6e05cb502207a6a05f3e70690d50053e14ffdef9b35b449cb54c9800f9087f11975848237ae01",
				},
				Txid:     "b40f343b7d3479c00b0e51ef9d8058f7f31ba5c7d3e91220cd690fd766105a32",
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
				},
			},
			{
				ValueSat: *big.NewInt(3888000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac",
					Addresses: []string{
						"B67ubLrpk7VYAVyt15qGLwJJ3NN2wTmA9z",
					},
				},
			},
                        {
                                ValueSat: *big.NewInt(1638998375),
                                N:        2,
                                ScriptPubKey: bchain.ScriptPubKey{
                                        Hex: "210259b9e8ac4014c04f38a2f3f0ab60052353bf329e83dc03debd14f8e700e300beac",
                                        Addresses: []string{
                                                "B67ubLrpk7VYAVyt15qGLwJJ3NN2wTmA9z",
                                        },
                                },
                        },
                        {
                                ValueSat: *big.NewInt(2250000000),
                                N:        3,
                                ScriptPubKey: bchain.ScriptPubKey{
                                        Hex: "76a914881e1ddf7dbc1adfb3e3080cf53541ec7b37d6af88ac",
                                        Addresses: []string{
                                                "BGrobMBmnKdYmQMR4oRQ9PNLKnCHD1TW5Q",
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
		parser    *BitcloudParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "bitcloud-1",
			args: args{
				tx:        testTx1,
				height:    226137,
				blockTime: 1572107953,
				parser:    NewBitcloudParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *BitcloudParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "bitcloud-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBitcloudParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   226137,
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

