// +build unittest

package monetaryunit

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

        testTxPacked1 = "0a2021d4e2d8ee1ce3e80b8e2e0b9acdfaab38713eb26c4c12ed9bdad7879da8c43c12d3010100000001ae3ae1ebb77f55d611fb9385ca84afa09eb70950f730d0d025bc4eaa37f7ecf70100$
)

func init() {
        testTx1 = bchain.Tx{
                Hex:       "0100000001ae3ae1ebb77f55d611fb9385ca84afa09eb70950f730d0d025bc4eaa37f7ecf7010000004948304502210098a4a6e595a9c4f4b807029fa35afca75a987861353aa1a9f3519$
                Blocktime: 1556386279,
                Txid:      "21d4e2d8ee1ce3e80b8e2e0b9acdfaab38713eb26c4c12ed9bdad7879da8c43c",
                LockTime:  0,
                Version:   1,
		Vin: []bchain.Vin{
                        {
                                ScriptSig: bchain.ScriptSig{
                                        Hex: "48304502210098a4a6e595a9c4f4b807029fa35afca75a987861353aa1a9f3519e450a9e99480220135c73dc4777b543ecc985689b89e1af1b4ba16b3841bbff3a0$
                                },
                                Txid:     "f7ecf737aa4ebc25d0d030f75009b79ea0af84ca8593fb11d6557fb7ebe13aae",
                                Vout:     1,
                                Sequence: 4294967295,
                        },
                },
                Vout: []bchain.Vout{
                        {
                                ValueSat: *big.NewInt(60047995839),
                                N:        1,
                                ScriptPubKey: bchain.ScriptPubKey{
                                        Hex: "2103f52407e34c2697a91b9fede95247dbb20755f9fefe78924b6502956f6e3d2961ac",
                                        Addresses: []string{
                                                "7SqFp5enga496XTHBoJhCHBqpRKpgMgPwJ",
                                        },
                                },
                        },
                        {
                                ValueSat: *big.NewInt(1800000000),
                                N:        2,
                                ScriptPubKey: bchain.ScriptPubKey{
                                        Hex: "76a91476a0ad606d14cd79ae30c3391a6eb54be4adffed88ac",
                                        Addresses: []string{
                                                "7dDtyRdQztS68iyGCfPU1YmQiiQLqN7VBW",
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
                                height:    444106,
                                blockTime: 1556386279,
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
                        want1:   444106,
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
