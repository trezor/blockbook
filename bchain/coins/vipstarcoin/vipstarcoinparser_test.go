//go:build unittest

package vipstarcoin

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
			args:    args{address: "VLhjpFadC3pkyydKfMTbdg6JFaYojkubik"},
			want:    "76a9146e2b0a8655786c8c5ea7b9ce478f03e00ecb2f5588ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "VEUMnC1iPpr64u4HwE9DRjhwnF98xbS6n4"},
			want:    "76a91429d27c0c33c87830dda711aa878574bcd9c5247188ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "MAyVr99hsthBJin9sosjdyfeAP3ByxXFxz"},
			want:    "a91421ba6a62ac1d74d2ba921bbc8c9a3ca6e1420a0087",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "MV71zj5jJ1b9ijt198JAJrCwDHxu3tCn6g"},
			want:    "a914e898c1f90c736de7c3570c3391bf5e726c8b59aa87",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{address: "vips1q0v3tadxj6pm3ym9j06v9rfyw0jeh5f8s87sgtw"},
			want:    "00147b22beb4d2d077126cb27e9851a48e7cb37a24f0",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{address: "vips1qumpyvyxz25kfjjrvyxn3zlyc2wfc0m3l3gm5pg99c4mxylemfqhsnj023l"},
			want:    "0020e6c24610c2552c99486c21a7117c98539387ee3f8a3740a0a5c576627f3b482f",
			wantErr: false,
		},
	}
	parser := NewVIPSTARCOINParser(GetChainParams("main"), &btc.Configuration{})

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
			want:    []string{"VLhjpFadC3pkyydKfMTbdg6JFaYojkubik"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a91421ba6a62ac1d74d2ba921bbc8c9a3ca6e1420a0087"},
			want:    []string{"MAyVr99hsthBJin9sosjdyfeAP3ByxXFxz"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00147b22beb4d2d077126cb27e9851a48e7cb37a24f0"},
			want:    []string{"vips1q0v3tadxj6pm3ym9j06v9rfyw0jeh5f8s87sgtw"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "0020e6c24610c2552c99486c21a7117c98539387ee3f8a3740a0a5c576627f3b482f"},
			want:    []string{"vips1qumpyvyxz25kfjjrvyxn3zlyc2wfc0m3l3gm5pg99c4mxylemfqhsnj023l"},
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

	parser := NewVIPSTARCOINParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "000831508bcbe1ae6002000000000103a733281698ae7d80da17d5a201ab8b6077e83f780cb417c8d12eb9da8343735900000000171600141ee6fc3b04fdd080c03eeb513498b38c2621f4fcfdffffff3976295b67ed10a28016171da4f9a5833fe4ca1f83ffb56f95856aa445ebf46a000000006a473044022012315ff56ad254ed8b099623bd84a106819770970c41c2a138b6cfe4bb332aa602206f5679570c968b77a3f7e8dd14663d0af9aba5b354f113d3a4321b3aeafc03080121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3fdffffffdd5a7a9852c8ebe1c417a26c54bb58339eb6e1ea1416b9b11314f05f93e69ea1010000006a4730440220149942b3971fb655bd5d76630bb1c8993d3083e4ad631c972156927173e6afb802204590399049f77530251c86ab569f45d2248aa222372b9d1d95963bd67213a5f80121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3fdffffff0200ca9a3b000000001976a91493c052c292e366221f9ee709c36a9ea441eb984488acb0b20e000000000017a9149695605a5a5e9349e1c99e01b175c5c3baf39bc4870247304402207a2a1cc2f314c8c659a4bcbce099c5adfb217c03fa2b0cfc95bef48c1507901a0220324ab06cf2fe4c9e446a3a12c00fa611a479b5734f62c20b66e919e173a2c699012102bbe6f37b4c44303b2186de6784d02cc5b86a65ca1203821b06a98e243b44c76400004e310800"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "02000000000103a733281698ae7d80da17d5a201ab8b6077e83f780cb417c8d12eb9da8343735900000000171600141ee6fc3b04fdd080c03eeb513498b38c2621f4fcfdffffff3976295b67ed10a28016171da4f9a5833fe4ca1f83ffb56f95856aa445ebf46a000000006a473044022012315ff56ad254ed8b099623bd84a106819770970c41c2a138b6cfe4bb332aa602206f5679570c968b77a3f7e8dd14663d0af9aba5b354f113d3a4321b3aeafc03080121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3fdffffffdd5a7a9852c8ebe1c417a26c54bb58339eb6e1ea1416b9b11314f05f93e69ea1010000006a4730440220149942b3971fb655bd5d76630bb1c8993d3083e4ad631c972156927173e6afb802204590399049f77530251c86ab569f45d2248aa222372b9d1d95963bd67213a5f80121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3fdffffff0200ca9a3b000000001976a91493c052c292e366221f9ee709c36a9ea441eb984488acb0b20e000000000017a9149695605a5a5e9349e1c99e01b175c5c3baf39bc4870247304402207a2a1cc2f314c8c659a4bcbce099c5adfb217c03fa2b0cfc95bef48c1507901a0220324ab06cf2fe4c9e446a3a12c00fa611a479b5734f62c20b66e919e173a2c699012102bbe6f37b4c44303b2186de6784d02cc5b86a65ca1203821b06a98e243b44c76400004e310800",
		Blocktime: 1555835824,
		Txid:      "93aae65e87ec46cd13b3032e1588c7db75e2b712514696efca5f2bfd80c16632",
		LockTime:  536910,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "1600141ee6fc3b04fdd080c03eeb513498b38c2621f4fc",
				},
				Txid:     "59734383dab92ed1c817b40c783fe877608bab01a2d517da807dae98162833a7",
				Vout:     0,
				Sequence: 4294967293,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022012315ff56ad254ed8b099623bd84a106819770970c41c2a138b6cfe4bb332aa602206f5679570c968b77a3f7e8dd14663d0af9aba5b354f113d3a4321b3aeafc03080121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3",
				},
				Txid:     "6af4eb45a46a85956fb5ff831fcae43f83a5f9a41d171680a210ed675b297639",
				Vout:     0,
				Sequence: 4294967293,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220149942b3971fb655bd5d76630bb1c8993d3083e4ad631c972156927173e6afb802204590399049f77530251c86ab569f45d2248aa222372b9d1d95963bd67213a5f80121030d52fc12b11b9288490ed78b8b07ff025a33fe1577f402ddf30c0f73769363a3",
				},
				Txid:     "a19ee6935ff01413b1b91614eae1b69e3358bb546ca217c4e1ebc852987a5add",
				Vout:     1,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91493c052c292e366221f9ee709c36a9ea441eb984488ac",
					Addresses: []string{
						"VQ8Tef3y1hKQnAq5baiyxiScsVnEcPqjdy",
					},
				},
			},
			{
				ValueSat: *big.NewInt(963248),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9149695605a5a5e9349e1c99e01b175c5c3baf39bc487",
					Addresses: []string{
						"MMdNY4HfcntfZ1EB6zn5Zz7i25yYfocaFT",
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
		parser    *VIPSTARCOINParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "VIPSTARCOIN-1",
			args: args{
				tx:        testTx1,
				height:    536912,
				blockTime: 1555835824,
				parser:    NewVIPSTARCOINParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *VIPSTARCOINParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "VIPSTARCOIN-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewVIPSTARCOINParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   536912,
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
			// ignore witness unpacking
			for i := range got.Vin {
				got.Vin[i].Witness = nil
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
