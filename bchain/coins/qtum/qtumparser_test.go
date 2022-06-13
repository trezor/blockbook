//go:build unittest

package qtum

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
			args:    args{address: "QiZfqrMLAtfzLCjXTHyLSiNDV6xydoocme"},
			want:    "76a914f0e2aff6730b53b9986a5db8ca17c59426134a0988ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "Qcy8wvP1StA3cB9JcPdivXvqUqwai6t1tC"},
			want:    "76a914b3822026c7f758b43a0882d7f2cbfa954702e45688ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "MHx35M7aq4FcudwVSYziTkpbm9Hz8wJL7y"},
			want:    "a9146e3c881d51d62a668362a5bba28be438f9c548e287",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "MSNLeFGHY8iY2Kio57PnKuwWdCQinjuPDC"},
			want:    "a914ca962f788569443b33ec673208ccdcfaac6873b487",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "qc1q3msdh3npg5ufvwm2sxltxcet6hll9tjzu8ym0d"},
			want:    "00148ee0dbc6614538963b6a81beb3632bd5fff2ae42",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{address: "qc1q0uvgqxwqt0u4esawwcff7652jreqye30kmp4sj5dtydee50pze0s0ljwxp"},
			want:    "00207f188019c05bf95cc3ae76129f6a8a90f202662fb6c3584a8d591b9cd1e1165f",
			wantErr: false,
		},
	}
	parser := NewQtumParser(GetChainParams("main"), &btc.Configuration{})

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
			args:    args{script: "76a914f0e2aff6730b53b9986a5db8ca17c59426134a0988ac"},
			want:    []string{"QiZfqrMLAtfzLCjXTHyLSiNDV6xydoocme"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9146e3c881d51d62a668362a5bba28be438f9c548e287"},
			want:    []string{"MHx35M7aq4FcudwVSYziTkpbm9Hz8wJL7y"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00148ee0dbc6614538963b6a81beb3632bd5fff2ae42"},
			want:    []string{"qc1q3msdh3npg5ufvwm2sxltxcet6hll9tjzu8ym0d"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "00207f188019c05bf95cc3ae76129f6a8a90f202662fb6c3584a8d591b9cd1e1165f"},
			want:    []string{"qc1q0uvgqxwqt0u4esawwcff7652jreqye30kmp4sj5dtydee50pze0s0ljwxp"},
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

	parser := NewQtumParser(GetChainParams("main"), &btc.Configuration{})

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
	testTx1       bchain.Tx
	testTxPacked1 = "00050fc08bc88ede00010000000336e691ab7f236d7c772b18e967c324b92ad1ba79e4641fd868f737d08f11857a000000006b483045022100bdef630a30ea681be3d2a66bbbc994100509effe9b85b384a0f8d75685eca97802206b7a5e58115deffe3f8f35d4c22dc52eb2cc1632ef18a48498731c09255c2fa9812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3fffffffffa3ba1bd605b4db3594a8f5fd3cdec34d3044e3e26dee66908235e8643e9f50f010000006b483045022100f1889232cae3860876025317002bbc9a7e68b172c0595df5db8a1e59a12254150220557bebe548bae1b8fe3474375caca12cab1789e8d6e2c9bd6ab1b4a2c3e3691a812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3ffffffff06fb670847bea092c352198f327e39fac3f568d57a100cb4a7db991485dda546170000006b483045022100a2fba32aebca4eaa261f9ebd2b956ac22d9c29e7f65868acd60165077dcfbc85022011864df322178f515260c9f4c098112bfbc23bf257f38005b80a34271d08149b812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3ffffffff020088526a740000001976a914b61ba6aa3cc8be40e7553c8728ab3a303cbd4f2188acec1e0923000000001976a9148e896f90d402cdb5517f7d1f32a3d9d400e4bbcb88ac00000000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "010000000336e691ab7f236d7c772b18e967c324b92ad1ba79e4641fd868f737d08f11857a000000006b483045022100bdef630a30ea681be3d2a66bbbc994100509effe9b85b384a0f8d75685eca97802206b7a5e58115deffe3f8f35d4c22dc52eb2cc1632ef18a48498731c09255c2fa9812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3fffffffffa3ba1bd605b4db3594a8f5fd3cdec34d3044e3e26dee66908235e8643e9f50f010000006b483045022100f1889232cae3860876025317002bbc9a7e68b172c0595df5db8a1e59a12254150220557bebe548bae1b8fe3474375caca12cab1789e8d6e2c9bd6ab1b4a2c3e3691a812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3ffffffff06fb670847bea092c352198f327e39fac3f568d57a100cb4a7db991485dda546170000006b483045022100a2fba32aebca4eaa261f9ebd2b956ac22d9c29e7f65868acd60165077dcfbc85022011864df322178f515260c9f4c098112bfbc23bf257f38005b80a34271d08149b812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3ffffffff020088526a740000001976a914b61ba6aa3cc8be40e7553c8728ab3a303cbd4f2188acec1e0923000000001976a9148e896f90d402cdb5517f7d1f32a3d9d400e4bbcb88ac00000000",
		Blocktime: 1552013184,
		Txid:      "40cc76f3d9747472c49a7c162628d5794e1fb3e5c28e5787b3c6c1178c794e8c",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100bdef630a30ea681be3d2a66bbbc994100509effe9b85b384a0f8d75685eca97802206b7a5e58115deffe3f8f35d4c22dc52eb2cc1632ef18a48498731c09255c2fa9812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3",
				},
				Txid:     "7a85118fd037f768d81f64e479bad12ab924c367e9182b777c6d237fab91e636",
				Vout:     0,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100f1889232cae3860876025317002bbc9a7e68b172c0595df5db8a1e59a12254150220557bebe548bae1b8fe3474375caca12cab1789e8d6e2c9bd6ab1b4a2c3e3691a812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3",
				},
				Txid:     "0ff5e943865e230869e6de263e4e04d334eccdd35f8f4a59b34d5b60bda13bfa",
				Vout:     1,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100a2fba32aebca4eaa261f9ebd2b956ac22d9c29e7f65868acd60165077dcfbc85022011864df322178f515260c9f4c098112bfbc23bf257f38005b80a34271d08149b812102cc0f9a0906c0e8dffb3262778be1b3bc75e2d636fa01a7c2c129b1f3f30f21d3",
				},
				Txid:     "46a5dd851499dba7b40c107ad568f5c3fa397e328f1952c392a0be470867fb06",
				Vout:     23,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(500000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b61ba6aa3cc8be40e7553c8728ab3a303cbd4f2188ac",
					Addresses: []string{
						"QdCtDST9o3JzQN1tchkpakAgGT4oSRhJec",
					},
				},
			},
			{
				ValueSat: *big.NewInt(587800300),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9148e896f90d402cdb5517f7d1f32a3d9d400e4bbcb88ac",
					Addresses: []string{
						"QZbehkDJekWHZeyKGwXDWqxy8m7RGWPeGn",
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
		parser    *QtumParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Qtum-1",
			args: args{
				tx:        testTx1,
				height:    331712,
				blockTime: 1552013184,
				parser:    NewQtumParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *QtumParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "Qtum-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewQtumParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   331712,
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
