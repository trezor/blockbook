//go:build unittest

package bellcoin

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
			args:    args{address: "BSQmMGWpjwP5Lu8feSypSaPFiTTrC3EdEx"},
			want:    "76a914f0e2aff6730b53b9986a5db8ca17c59426134a0988ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "BLpETLYW1vs8csYSoYeCvPwsiCSTJUjx6T"},
			want:    "76a914b3822026c7f758b43a0882d7f2cbfa954702e45688ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "bNn9Y9YfgNUHXopXJEesS9M8noJzzrWTmP"},
			want:    "a9146e3c881d51d62a668362a5bba28be438f9c548e287",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "bXCT73hNPSwCeVbpvo3wJJU3erRjawUGSu"},
			want:    "a914ca962f788569443b33ec673208ccdcfaac6873b487",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "bm1q3msdh3npg5ufvwm2sxltxcet6hll9tjzkzxzqt"},
			want:    "00148ee0dbc6614538963b6a81beb3632bd5fff2ae42",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{address: "bm1q0uvgqxwqt0u4esawwcff7652jreqye30kmp4sj5dtydee50pze0sjx6wn5"},
			want:    "00207f188019c05bf95cc3ae76129f6a8a90f202662fb6c3584a8d591b9cd1e1165f",
			wantErr: false,
		},
	}
	parser := NewBellcoinParser(GetChainParams("main"), &btc.Configuration{})

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
			want:    []string{"BSQmMGWpjwP5Lu8feSypSaPFiTTrC3EdEx"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9146e3c881d51d62a668362a5bba28be438f9c548e287"},
			want:    []string{"bNn9Y9YfgNUHXopXJEesS9M8noJzzrWTmP"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00148ee0dbc6614538963b6a81beb3632bd5fff2ae42"},
			want:    []string{"bm1q3msdh3npg5ufvwm2sxltxcet6hll9tjzkzxzqt"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "00207f188019c05bf95cc3ae76129f6a8a90f202662fb6c3584a8d591b9cd1e1165f"},
			want:    []string{"bm1q0uvgqxwqt0u4esawwcff7652jreqye30kmp4sj5dtydee50pze0sjx6wn5"},
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

	parser := NewBellcoinParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "0003238c8bc5aac144020000000001026f3631c4db09d48354d12cd2e780b2aa0f92198fca7d044a6b073e9a06b4135d0200000017160014dfc67c36c26ea8036042c56b8ce5d5027d1fb81ffeffffffac13cfbe663797aced3134006adec863e5b3f9480ab65a4dda112de875f8dd6e010000006a473044022066e4d5f99fec12f076d7d912d1e530e4b950d23cc3de2b8127f98109f510c9d502207a74f2e7a753262fdd29756193493d47ead7880871cbc6c55cc6d20e229c0c1201210294fc5b3928335caddc7f4c536e0db85c736cbc7c164de0319976da90b65d288ffeffffff05f4c16020000000001976a91416e20046e69c396aed18c6663ddcbbf1dde9082f88ac0aa47113000000001976a91426fa5e6c4e579058d3f4d1dd83d99e291d4dc0c588acbcb11a00000000001976a9145a53bd436b5c19a42b1518ef18443e8403bcaeed88ac0c04693b0000000017a9140576053f982117afcff024013ed61270189c4fbc87358bad07000000001976a914e00ec357dfee124ac68b3c10dcf82e2ed0993ccc88ac02483045022100ed4b0e9b140850951ffbc10349e3ac56a18b80c600a77b95cfac274e10228eb602207c876b9b134e63b8a01ba28720dc4c1c2c67bb7e547fb71d31440cd365c674260121027aa4243e82c73c9c15d544a0b61a828eed1128464952bfbdc9235d4380f2767d008a230300"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "020000000001026f3631c4db09d48354d12cd2e780b2aa0f92198fca7d044a6b073e9a06b4135d0200000017160014dfc67c36c26ea8036042c56b8ce5d5027d1fb81ffeffffffac13cfbe663797aced3134006adec863e5b3f9480ab65a4dda112de875f8dd6e010000006a473044022066e4d5f99fec12f076d7d912d1e530e4b950d23cc3de2b8127f98109f510c9d502207a74f2e7a753262fdd29756193493d47ead7880871cbc6c55cc6d20e229c0c1201210294fc5b3928335caddc7f4c536e0db85c736cbc7c164de0319976da90b65d288ffeffffff05f4c16020000000001976a91416e20046e69c396aed18c6663ddcbbf1dde9082f88ac0aa47113000000001976a91426fa5e6c4e579058d3f4d1dd83d99e291d4dc0c588acbcb11a00000000001976a9145a53bd436b5c19a42b1518ef18443e8403bcaeed88ac0c04693b0000000017a9140576053f982117afcff024013ed61270189c4fbc87358bad07000000001976a914e00ec357dfee124ac68b3c10dcf82e2ed0993ccc88ac02483045022100ed4b0e9b140850951ffbc10349e3ac56a18b80c600a77b95cfac274e10228eb602207c876b9b134e63b8a01ba28720dc4c1c2c67bb7e547fb71d31440cd365c674260121027aa4243e82c73c9c15d544a0b61a828eed1128464952bfbdc9235d4380f2767d008a230300",
		Blocktime: 1549095010,
		Txid:      "e7ef52bbf3d9cb1ca5dfdb02eabf108e2b0b7757b009d1cfb24a06e4126e67f2",
		LockTime:  205706,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "160014dfc67c36c26ea8036042c56b8ce5d5027d1fb81f",
				},
				Txid:     "5d13b4069a3e076b4a047dca8f19920faab280e7d22cd15483d409dbc431366f",
				Vout:     2,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022066e4d5f99fec12f076d7d912d1e530e4b950d23cc3de2b8127f98109f510c9d502207a74f2e7a753262fdd29756193493d47ead7880871cbc6c55cc6d20e229c0c1201210294fc5b3928335caddc7f4c536e0db85c736cbc7c164de0319976da90b65d288f",
				},
				Txid:     "6eddf875e82d11da4d5ab60a48f9b3e563c8de6a003431edac973766becf13ac",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(543212020),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91416e20046e69c396aed18c6663ddcbbf1dde9082f88ac",
					Addresses: []string{
						"B6Y5DmPr1LPUP95YDEnX6FbJHERVipkRcg",
					},
				},
			},
			{
				ValueSat: *big.NewInt(326214666),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91426fa5e6c4e579058d3f4d1dd83d99e291d4dc0c588ac",
					Addresses: []string{
						"B81BDwJTasemPgnHBxQ67wX2WV48b2XmEc",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1749436),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9145a53bd436b5c19a42b1518ef18443e8403bcaeed88ac",
					Addresses: []string{
						"BCggkHk8ZTVd9T4yseNmURj6w56XY47EUG",
					},
				},
			},
			{
				ValueSat: *big.NewInt(996738060),
				N:        3,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9140576053f982117afcff024013ed61270189c4fbc87",
					Addresses: []string{
						"bDE9TQ3W5zF4ej8hLwYVdK5w8n5zhd9Qxj",
					},
				},
			},
			{
				ValueSat: *big.NewInt(128813877),
				N:        4,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e00ec357dfee124ac68b3c10dcf82e2ed0993ccc88ac",
					Addresses: []string{
						"BQsnfSXNonZZMs1G6ndrVLbUJAK5tFG2bN",
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
		parser    *BellcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Bellcoin-1",
			args: args{
				tx:        testTx1,
				height:    205708,
				blockTime: 1549095010,
				parser:    NewBellcoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *BellcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "Bellcoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBellcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   205708,
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
