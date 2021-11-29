//go:build unittest

package block

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

func TestGetAddrDescFromAddress(t *testing.T) {
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
			name:    "P2PKH",
			args:    args{address: "BoYhQh9zJ8Kw5grXEK4pLwpyTtEyq1bC7p"},
			want:    "76a914d8bd5de47c77b9064d87bade0a040e217de454cb88ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{address: "CdbiHEWZLpVqyRTQYSMaLU3ckLdff9QFhT"},
			want:    "a914e7d561324d0c61329db604437d2d3dd7280a3f1087",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{address: "block1q5n5y22txfkew99qmkppl4dkjl5pz888n22mad2"},
			want:    "0014a4e84529664db2e2941bb043fab6d2fd02239cf3",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{address: "block1qsa5xq0f65d28u77tt4sdlacz7fjkcausrapk4zaa83jtj04tx2nq0duxn4"},
			want:    "00208768603d3aa3547e7bcb5d60dff702f2656c77901f436a8bbd3c64b93eab32a6",
			wantErr: false,
		},
	}
	parser := NewBlocknetParser(GetChainParams("main"), &btc.Configuration{})

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

func TestGetAddrDescFromVout(t *testing.T) {
	type args struct {
		vout bchain.Vout
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "P2PKH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac"}}},
			want:    "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac",
			wantErr: false,
		},
		{
			name:    "P2PK compressed",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "21020e46e79a2a8d12b9b5d12c7a91adb4e454edfae43c0a0cb805427d2ac7613fd9ac"}}},
			want:    "76a914f1dce4182fce875748c4986b240ff7d7bc3fffb088ac",
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "41041057356b91bfd3efeff5fc0fa8b865faafafb67bd653c5da2cd16ce15c7b86db0e622c8e1e135f68918a23601eb49208c1ac72c7b64a4ee99c396cf788da16ccac"}}},
			want:    "76a914b563933904dceba5c234e978bea0e9eb8b7e721b88ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "a9140394b3cf9a44782c10105b93962daa8dba304d7f87"}}},
			want:    "a9140394b3cf9a44782c10105b93962daa8dba304d7f87",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "00141c12afc6b2602607fdbc209f2a053c54ecd2c673"}}},
			want:    "00141c12afc6b2602607fdbc209f2a053c54ecd2c673",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29"}}},
			want:    "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29",
			wantErr: false,
		},
	}
	parser := NewBlocknetParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromVout(&tt.args.vout)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromVout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromVout() = %v, want %v", h, tt.want)
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
			args:    args{script: "76a91440f825e2eb4ecaf96edfbb4d1a38cebdca62257188ac"},
			want:    []string{"BZiD7j4tGmKBtkiXw8qMHBoE1Eot62by4w"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PK compressed",
			args:    args{script: "21020e46e79a2a8d12b9b5d12c7a91adb4e454edfae43c0a0cb805427d2ac7613fd9ac"},
			want:    []string{"BqqY4q8EUWpPHTLkq3HXaQg1w8G7qyBDs9"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed",
			args:    args{script: "41041057356b91bfd3efeff5fc0fa8b865faafafb67bd653c5da2cd16ce15c7b86db0e622c8e1e135f68918a23601eb49208c1ac72c7b64a4ee99c396cf788da16ccac"},
			want:    []string{"BkKnC9JBhyA4WJsJmdRDbyTjhxw2qLWf89"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a914926c6dd4fb1da987d316d58d38fd0ee52f61bcf687"},
			want:    []string{"CVp77q5EZbMg74FJF2NPEeuTxfBTVpsmmm"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "0014256b96c294600d06038b70670a180b768e50f4be"},
			want:    []string{"block1qy44eds55vqxsvqutwpns5xqtw689pa97jqux5w"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "0020306b3f04e29adf526463c7a71880b7afa4934adba04d465fa77fae7c62515b6c"},
			want:    []string{"block1qxp4n7p8znt04yerrc7n33q9h47jfxjkm5px5vha807h8ccj3tdkq875ajy"},
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
			name:    "OP_RETURN OP_PUSHDATA1 ascii",
			args:    args{script: "6a4c0b446c6f7568792074657874"},
			want:    []string{"OP_RETURN (Dlouhy text)"},
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

	parser := NewBlocknetParser(GetChainParams("main"), &btc.Configuration{})

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
	testTx1, testTx2 bchain.Tx
	testTxPacked1    = "000a928d8bc0cfb71a01000000036ba6efd4a411cedce9cbac527437671f49305cbb50a90b9d1e432262d137f56a010000004847304402205aacd5e0d16d2b21f299ca3a53f9d9e29d16eb2decad3a55892d8a6584ab6257022039a93630696a2535acf4b6cf77db017e91a7a14aac30b3276325b1632c0ca3de01ffffffffb7b6bec11d34f44e98e71932662d3f9ee7d585cf8d8760b2f21d50f5d5848683000000006a47304402203f132c37fd68a5e67ddf7926aed2bb31cf7cb94c970fccf4d2bd17415000fc6402206c720786d0df0b83c9b52bb4148bd2ccb1d3c66358c653bc270b88bf8908c998012102713d227b8714a5986b336b4fa9821ce460f85ed3e0a43fd26e80bb05187dc919ffffffff127e121b174e10adbbe1f881b35bdb8c1babcd2b2f1d784619907c91e8bf47fa010000006a473044022014d00f28233052a24e2ff5c05f1e95eb7768b3e3db5f7bbbb94bc89c3a0fc5cd022012d0a51b2bfa02305c7ae4dff165956bf0063d25a239f134b71dddd423fdea7e012103722bd26c8ee2efcdb9e9bd59ae6e0465f16aa4446a0cfad3185bff9372f26dc6ffffffff02eef20000000000001976a914c29650971756810bdf2b34ff30d037bf9a1b10d288acc03b956c160000001976a914ccb546b5be09438cf6b868b40f086b9b261bd46988ac00000000"
	testTxPacked2    = "0001369e8c8dc1d73c01000000017ead8519ab8848ba62ac1aff62c36ca2ec93c805ac3a072e0b38d91633b5da46010000004847304402207970483f33f61899997d98672c2934d643f1bd35704afc31bcb8bf7b2eec70e602206e834bd611d1222bb27b84500733530ab9d5ab4f7059af4a1fd720af03d297d601feffffff0206081a838b0000001976a9142cb4d2fc255cc88d0a6fca88fb4b2ef3518e211d88ac00943577000000001976a914b60dfcb590be22ed373535ccdcc5eb090fbc451988ac9d360100"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000036ba6efd4a411cedce9cbac527437671f49305cbb50a90b9d1e432262d137f56a010000004847304402205aacd5e0d16d2b21f299ca3a53f9d9e29d16eb2decad3a55892d8a6584ab6257022039a93630696a2535acf4b6cf77db017e91a7a14aac30b3276325b1632c0ca3de01ffffffffb7b6bec11d34f44e98e71932662d3f9ee7d585cf8d8760b2f21d50f5d5848683000000006a47304402203f132c37fd68a5e67ddf7926aed2bb31cf7cb94c970fccf4d2bd17415000fc6402206c720786d0df0b83c9b52bb4148bd2ccb1d3c66358c653bc270b88bf8908c998012102713d227b8714a5986b336b4fa9821ce460f85ed3e0a43fd26e80bb05187dc919ffffffff127e121b174e10adbbe1f881b35bdb8c1babcd2b2f1d784619907c91e8bf47fa010000006a473044022014d00f28233052a24e2ff5c05f1e95eb7768b3e3db5f7bbbb94bc89c3a0fc5cd022012d0a51b2bfa02305c7ae4dff165956bf0063d25a239f134b71dddd423fdea7e012103722bd26c8ee2efcdb9e9bd59ae6e0465f16aa4446a0cfad3185bff9372f26dc6ffffffff02eef20000000000001976a914c29650971756810bdf2b34ff30d037bf9a1b10d288acc03b956c160000001976a914ccb546b5be09438cf6b868b40f086b9b261bd46988ac00000000",
		Blocktime: 1544154573,
		Txid:      "ef85eaf687e8a003dec557036bde92011f3c17e0a54a2a174f4e9959c6a2fbb1",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402205aacd5e0d16d2b21f299ca3a53f9d9e29d16eb2decad3a55892d8a6584ab6257022039a93630696a2535acf4b6cf77db017e91a7a14aac30b3276325b1632c0ca3de01",
				},
				Txid:     "6af537d16222431e9d0ba950bb5c30491f67377452accbe9dcce11a4d4efa66b",
				Vout:     1,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402203f132c37fd68a5e67ddf7926aed2bb31cf7cb94c970fccf4d2bd17415000fc6402206c720786d0df0b83c9b52bb4148bd2ccb1d3c66358c653bc270b88bf8908c998012102713d227b8714a5986b336b4fa9821ce460f85ed3e0a43fd26e80bb05187dc919",
				},
				Txid:     "838684d5f5501df2b260878dcf85d5e79e3f2d663219e7984ef4341dc1beb6b7",
				Vout:     0,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022014d00f28233052a24e2ff5c05f1e95eb7768b3e3db5f7bbbb94bc89c3a0fc5cd022012d0a51b2bfa02305c7ae4dff165956bf0063d25a239f134b71dddd423fdea7e012103722bd26c8ee2efcdb9e9bd59ae6e0465f16aa4446a0cfad3185bff9372f26dc6",
				},
				Txid:     "fa47bfe8917c901946781d2f2bcdab1b8cdb5bb381f8e1bbad104e171b127e12",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(62190),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914c29650971756810bdf2b34ff30d037bf9a1b10d288ac",
					Addresses: []string{
						"BmXZm6Tjn3YoAceauPczSPHiANn3Spcszu",
					},
				},
			},
			{
				ValueSat: *big.NewInt(96311000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ccb546b5be09438cf6b868b40f086b9b261bd46988ac",
					Addresses: []string{
						"BnT5cFRK47dTxWKEmYaET23aw3sy3zj8gv",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "01000000017ead8519ab8848ba62ac1aff62c36ca2ec93c805ac3a072e0b38d91633b5da46010000004847304402207970483f33f61899997d98672c2934d643f1bd35704afc31bcb8bf7b2eec70e602206e834bd611d1222bb27b84500733530ab9d5ab4f7059af4a1fd720af03d297d601feffffff0206081a838b0000001976a9142cb4d2fc255cc88d0a6fca88fb4b2ef3518e211d88ac00943577000000001976a914b60dfcb590be22ed373535ccdcc5eb090fbc451988ac9d360100",
		Blocktime: 1624782302,
		Txid:      "c69ed365e1db3490200dbca40b5137d7d85ce0c583c288fc36ad8b13e9a05602",
		LockTime:  79517,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402207970483f33f61899997d98672c2934d643f1bd35704afc31bcb8bf7b2eec70e602206e834bd611d1222bb27b84500733530ab9d5ab4f7059af4a1fd720af03d297d601",
				},
				Txid:     "46dab53316d9380b2e073aac05c893eca26cc362ff1aac62ba4888ab1985ad7e",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(599199975430),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9142cb4d2fc255cc88d0a6fca88fb4b2ef3518e211d88ac",
					Addresses: []string{
						"y14EDRC4wPJmV8q1kY66WTv262zuvmr7Ps",
					},
				},
			},
			{
				ValueSat: *big.NewInt(2000000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b60dfcb590be22ed373535ccdcc5eb090fbc451988ac",
					Addresses: []string{
						"yDaTbrLjVK2jX54JhsKtRNkDcrQ6sTdBXD",
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
		parser    *BlocknetParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "block-1",
			args: args{
				tx:        testTx1,
				height:    692877,
				blockTime: 1544154573,
				parser:    NewBlocknetParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				tx:        testTx2,
				height:    79518,
				blockTime: 1624782302,
				parser:    NewBlocknetParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked2,
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
		parser   *BlocknetParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "block-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBlocknetParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   692877,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewBlocknetParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   79518,
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
