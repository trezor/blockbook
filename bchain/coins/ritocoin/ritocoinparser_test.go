//go:build unittest

package ritocoin

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
			args:    args{address: "BPXJUs9jo15TTJA6EhS6WAJkEMWtXtWqGf"},
			want:    "76a914d136cae89e81911acd8c7b47b4cfc7ea7e56537188ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "BGmHxkXaaRMNdNin7a13MipcbZNHZzeEsi"},
			want:    "76a91487134d5b626c428db9518f7e83f839852b0947c588ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "jdRbWCgeJ6hJXbPLLvABkydaBWkS73rX8J"},
			want:    "a914f09db8fe493b1f36806e90de7bde9a4e6c1f57d787",
			wantErr: false,
		},
	}
	parser := NewRitocoinParser(GetChainParams("main"), &btc.Configuration{})

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
	testTxPacked1 = "0003d1478bceb6c13802000000019d5b660bcf1e33116fc5b7991da0cbdd848d3bb8fe2d98fd8efb1c2002f5acdc010000006b483045022100e97ac763e605e9605159ef7a12279e275cc455dbb7db54511e41384f49918b1c0220470f11c765a8262d4b941a87cbc3407f175ba0f3465f44dfdcf29977e2aea259012103eabae25fd82dfa35f66490a8ad5619ccd2492092c8653a79a3fb7a97f2153f43feffffff0240ac8b460e0000001976a9148f87576aad8ba0b42387b6c46f4ec3701eae1b6288ac82c89c40010000001976a914adeec4782f2d95082ceb04d34be90063bc2f221088ac46d10300"

	testTx2       bchain.Tx
	testTxPacked2 = "0003d10d8bceb69108020000000213aa0749de3e9139cd5352d7037b2cc0ee1eedc89bd74cd698d87f2a62b41127010000006b483045022100a64c3a3df8d5e74e28a19401e0eab65c713ddf04ba9666584e9157ed4648bf4a022009d857df4b8a36d371316daf72db4bcaa002bcf7318f898baa71039b52330095012102355d477bfe43e3a884f3fc81e4c81bd7a79371457974479f901222392c883cadfeffffffac4ebcdaa62454a2bf77e73db68e81452dc2e63f01bb787c63f62a9abd4a7406010000006b483045022100f0fa2cf9134acfdd0124543fc5bc1db7bc7f60023dffdca0f6f87bdd8849cb6e0220319840af61ad2b37294e3532ffd191c57954982a2f1d4738cc639999e9752622012103a7e716d712aa4d6075507daf9b62717d89c2e624236a4bdee7a97c4cf34b6b69feffffff0260b9b543ba0000001976a91456b3aa9d5d2bdc1658406d3e8272d92f1cb05a2088ac06502200000000001976a914b328e94bb96666f774d387afe329588b5e2fc35d88ac0cd10300"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "02000000019d5b660bcf1e33116fc5b7991da0cbdd848d3bb8fe2d98fd8efb1c2002f5acdc010000006b483045022100e97ac763e605e9605159ef7a12279e275cc455dbb7db54511e41384f49918b1c0220470f11c765a8262d4b941a87cbc3407f175ba0f3465f44dfdcf29977e2aea259012103eabae25fd82dfa35f66490a8ad5619ccd2492092c8653a79a3fb7a97f2153f43feffffff0240ac8b460e0000001976a9148f87576aad8ba0b42387b6c46f4ec3701eae1b6288ac82c89c40010000001976a914adeec4782f2d95082ceb04d34be90063bc2f221088ac46d10300",
		Blocktime: 1558630492,
		Txid:      "535e470daf1a4eb2097e6adaddd81972b010e33417747536f19ed29371f9713f",
		LockTime:  250182,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100e97ac763e605e9605159ef7a12279e275cc455dbb7db54511e41384f49918b1c0220470f11c765a8262d4b941a87cbc3407f175ba0f3465f44dfdcf29977e2aea259012103eabae25fd82dfa35f66490a8ad5619ccd2492092c8653a79a3fb7a97f2153f43",
				},
				Txid:     "dcacf502201cfb8efd982dfeb83b8d84ddcba01d99b7c56f11331ecf0b665b9d",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(61313100864),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9148f87576aad8ba0b42387b6c46f4ec3701eae1b6288ac",
					Addresses: []string{
						"BHXzNqXn3rj8WCKg3mzdZCha7YPyXJnqta",
					},
				},
			},
			{
				ValueSat: *big.NewInt(5378984066),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914adeec4782f2d95082ceb04d34be90063bc2f221088ac",
					Addresses: []string{
						"BLJkYkobSHGonwfRpo3Tu9bAFHoZpTSbfH",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "020000000213aa0749de3e9139cd5352d7037b2cc0ee1eedc89bd74cd698d87f2a62b41127010000006b483045022100a64c3a3df8d5e74e28a19401e0eab65c713ddf04ba9666584e9157ed4648bf4a022009d857df4b8a36d371316daf72db4bcaa002bcf7318f898baa71039b52330095012102355d477bfe43e3a884f3fc81e4c81bd7a79371457974479f901222392c883cadfeffffffac4ebcdaa62454a2bf77e73db68e81452dc2e63f01bb787c63f62a9abd4a7406010000006b483045022100f0fa2cf9134acfdd0124543fc5bc1db7bc7f60023dffdca0f6f87bdd8849cb6e0220319840af61ad2b37294e3532ffd191c57954982a2f1d4738cc639999e9752622012103a7e716d712aa4d6075507daf9b62717d89c2e624236a4bdee7a97c4cf34b6b69feffffff0260b9b543ba0000001976a91456b3aa9d5d2bdc1658406d3e8272d92f1cb05a2088ac06502200000000001976a914b328e94bb96666f774d387afe329588b5e2fc35d88ac0cd10300",
		Blocktime: 1558627396,
		Txid:      "7f0745611b4cf48f611a26873cc3c5c01eff7bdf8df7427f379bc7963792f966",
		LockTime:  250124,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100a64c3a3df8d5e74e28a19401e0eab65c713ddf04ba9666584e9157ed4648bf4a022009d857df4b8a36d371316daf72db4bcaa002bcf7318f898baa71039b52330095012102355d477bfe43e3a884f3fc81e4c81bd7a79371457974479f901222392c883cad",
				},
				Txid:     "2711b4622a7fd898d64cd79bc8ed1eeec02c7b03d75253cd39913ede4907aa13",
				Vout:     1,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100f0fa2cf9134acfdd0124543fc5bc1db7bc7f60023dffdca0f6f87bdd8849cb6e0220319840af61ad2b37294e3532ffd191c57954982a2f1d4738cc639999e9752622012103a7e716d712aa4d6075507daf9b62717d89c2e624236a4bdee7a97c4cf34b6b69",
				},
				Txid:     "06744abd9a2af6637c78bb013fe6c22d45818eb63de777bfa25424a6dabc4eac",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(799999900000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91456b3aa9d5d2bdc1658406d3e8272d92f1cb05a2088ac",
					Addresses: []string{
						"BCMWxf6dvLXQv3ZNXAxeQLMvrXvSVYfubt",
					},
				},
			},
			{
				ValueSat: *big.NewInt(2248710),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b328e94bb96666f774d387afe329588b5e2fc35d88ac",
					Addresses: []string{
						"BLnPacy2LCSEgBT2Wi2WgjGhaMfpV1ykf3",
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
		parser    *RitocoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "ritocoin-1",
			args: args{
				tx:        testTx1,
				height:    250183,
				blockTime: 1558630492,
				parser:    NewRitocoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "ritocoin-2",
			args: args{
				tx:        testTx2,
				height:    250125,
				blockTime: 1558627396,
				parser:    NewRitocoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *RitocoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "ritocoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewRitocoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   250183,
			wantErr: false,
		},
		{
			name: "ritocoin-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewRitocoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   250125,
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
