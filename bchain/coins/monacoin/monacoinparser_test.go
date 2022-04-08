//go:build unittest

package monacoin

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
			args:    args{address: "mptvgSbAs4iwxQ7JQZdEN6Urpt3dtjbawd"},
			want:    "76a91466e0ef980c8ff8129e8d0f716b2ce1df2f97bbbf88ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "pJwLxfRRUhAaYJsKzKCk9cATAn8Do2SS7L"},
			want:    "a91492e825fa92f4aa873c6caf4b20f6c7e949b456a987",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "pHNnBm6ECsh5QsUyXMzdoAXV8qV68wj2M4"},
			want:    "a91481c75a711f23443b44d70b10ddf856e39a6b254d87",
			wantErr: false,
		},
	}
	parser := NewMonacoinParser(GetChainParams("test"), &btc.Configuration{})

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
			args:    args{address: "MFMy9FwJsV6HiN5eZDqDETw4pw52q3UGrb"},
			want:    "76a91451dadacc7021440cbe4ca148a5db563b329b4c0388ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "MVELZC3ks1Xk59kvKWuSN3mpByNwaxeaBJ"},
			want:    "76a914e9fb298e72e29ebc2b89864a5e4ae10e0b84726088ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "PHjTKtgYLTJ9D2Bzw2f6xBB41KBm2HeGfg"},
			want:    "a9146449f568c9cd2378138f2636e1567112a184a9e887",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "PUfP1H9f2RhFf6wbuK16RKHDKBeTmRMMcU"},
			want:    "a914dc2e124d590b4dd34b9c564fe281474531cdc21987",
			wantErr: false,
		},
		{
			name:    "witness_v0_keyhash",
			args:    args{address: "mona1q49knemcefarfkvuqr7rgajdu3x5gx8pzdnurgq"},
			want:    "0014a96d3cef194f469b33801f868ec9bc89a8831c22",
			wantErr: false,
		},
		{
			name:    "witness_v0_scripthashx",
			args:    args{address: "mona1qp8f842ywwr9h5rdxyzggex7q3trvvvaarfssxccju52rj6htfzfsqr79j2"},
			want:    "002009d27aa88e70cb7a0da620908c9bc08ac6c633bd1a61036312e514396aeb4893",
			wantErr: false,
		},
	}
	parser := NewMonacoinParser(GetChainParams("main"), &btc.Configuration{})

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
			args:    args{script: "76a91451dadacc7021440cbe4ca148a5db563b329b4c0388ac"},
			want:    []string{"MFMy9FwJsV6HiN5eZDqDETw4pw52q3UGrb"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9146449f568c9cd2378138f2636e1567112a184a9e887"},
			want:    []string{"PHjTKtgYLTJ9D2Bzw2f6xBB41KBm2HeGfg"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "0014a96d3cef194f469b33801f868ec9bc89a8831c22"},
			want:    []string{"mona1q49knemcefarfkvuqr7rgajdu3x5gx8pzdnurgq"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "002009d27aa88e70cb7a0da620908c9bc08ac6c633bd1a61036312e514396aeb4893"},
			want:    []string{"mona1qp8f842ywwr9h5rdxyzggex7q3trvvvaarfssxccju52rj6htfzfsqr79j2"},
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

	parser := NewMonacoinParser(GetChainParams("main"), &btc.Configuration{})

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

	testTxPacked1 = "0014fd278bb3fde2620200000003e44ef4e5fe2e4345f1e1340afe396c780773e3834a5bffb153a2faf510e2845e000000006a47304402205ebd735621eaaf512441998727a37e99be94e5ecded54601ea3eebac9282bc2502207d48da44e1c883579c6cd8c2b8ccfb5380e5ac71affe70b475d2b558e0f7bd4b01210391f72b34c04855ce16b97dd79b0ba78fc4b26f40abce853c33788e348cb79c3bfeffffff0ad690a74c43c0df9527c516d26e31fa47e15471a2ead65757b672522888e920010000006b48304502210091a473124bf506edbb095951aa1a32c76bea7eba4020ae2858314961b1a83de602205c3818e517cf830a95a1208fc84aa343faaeeaaa96eab76238379769598ab2d40121038c217e5de8e375ed6cf648e96ec6bfb9e0fbcf5ae3945a5ea60d16919d9c8b68feffffffb9aa4aed4ad4c4b95419e132a43db34aa03a7ec35ef0beecdd627f9ca07bda03010000006a47304402204906d973ac9b4786403f8f8fc2b2ad2e6745ea01a93336b4b67af1d7d1b625cc022016820be905ffd6e11949da79e7a1c7eb97939421a04e0645c8caef8fc585f7ca012102b5f647c4eb677e952913c0b6934c12b29dc50afba8b558b1677ffd2d78c84a88feffffff02f6da4601000000001976a914fb69fe6dcfe88557dc0ce0ea65bd7cf02f5e4f5b88ac8bfd8c57000000001976a914628d603ac50d656e3311ff0cd5490b4c5cdd92ea88ac25fd1400"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0200000003e44ef4e5fe2e4345f1e1340afe396c780773e3834a5bffb153a2faf510e2845e000000006a47304402205ebd735621eaaf512441998727a37e99be94e5ecded54601ea3eebac9282bc2502207d48da44e1c883579c6cd8c2b8ccfb5380e5ac71affe70b475d2b558e0f7bd4b01210391f72b34c04855ce16b97dd79b0ba78fc4b26f40abce853c33788e348cb79c3bfeffffff0ad690a74c43c0df9527c516d26e31fa47e15471a2ead65757b672522888e920010000006b48304502210091a473124bf506edbb095951aa1a32c76bea7eba4020ae2858314961b1a83de602205c3818e517cf830a95a1208fc84aa343faaeeaaa96eab76238379769598ab2d40121038c217e5de8e375ed6cf648e96ec6bfb9e0fbcf5ae3945a5ea60d16919d9c8b68feffffffb9aa4aed4ad4c4b95419e132a43db34aa03a7ec35ef0beecdd627f9ca07bda03010000006a47304402204906d973ac9b4786403f8f8fc2b2ad2e6745ea01a93336b4b67af1d7d1b625cc022016820be905ffd6e11949da79e7a1c7eb97939421a04e0645c8caef8fc585f7ca012102b5f647c4eb677e952913c0b6934c12b29dc50afba8b558b1677ffd2d78c84a88feffffff02f6da4601000000001976a914fb69fe6dcfe88557dc0ce0ea65bd7cf02f5e4f5b88ac8bfd8c57000000001976a914628d603ac50d656e3311ff0cd5490b4c5cdd92ea88ac25fd1400",
		Blocktime: 1530902705,
		Txid:      "7533fa6651cc96762e27bf496e00262671312244ff0c8bfe56a3c0ef688a49b5",
		LockTime:  1375525,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402205ebd735621eaaf512441998727a37e99be94e5ecded54601ea3eebac9282bc2502207d48da44e1c883579c6cd8c2b8ccfb5380e5ac71affe70b475d2b558e0f7bd4b01210391f72b34c04855ce16b97dd79b0ba78fc4b26f40abce853c33788e348cb79c3b",
				},
				Txid:     "5e84e210f5faa253b1ff5b4a83e37307786c39fe0a34e1f145432efee5f44ee4",
				Vout:     0,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "48304502210091a473124bf506edbb095951aa1a32c76bea7eba4020ae2858314961b1a83de602205c3818e517cf830a95a1208fc84aa343faaeeaaa96eab76238379769598ab2d40121038c217e5de8e375ed6cf648e96ec6bfb9e0fbcf5ae3945a5ea60d16919d9c8b68",
				},
				Txid:     "20e988285272b65757d6eaa27154e147fa316ed216c52795dfc0434ca790d60a",
				Vout:     1,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402204906d973ac9b4786403f8f8fc2b2ad2e6745ea01a93336b4b67af1d7d1b625cc022016820be905ffd6e11949da79e7a1c7eb97939421a04e0645c8caef8fc585f7ca012102b5f647c4eb677e952913c0b6934c12b29dc50afba8b558b1677ffd2d78c84a88",
				},
				Txid:     "03da7ba09c7f62ddecbef05ec37e3aa04ab33da432e11954b9c4d44aed4aaab9",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(21420790),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914fb69fe6dcfe88557dc0ce0ea65bd7cf02f5e4f5b88ac",
					Addresses: []string{
						"MWpWpANNQRskQHcuY5ZQpN4BVynQxmSxRb",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1468857739),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914628d603ac50d656e3311ff0cd5490b4c5cdd92ea88ac",
					Addresses: []string{
						"MGtFpCVyKEHNtpVNesxPMxYuQayoEBX5yZ",
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
		parser    *MonacoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Monacoin-1",
			args: args{
				tx:        testTx1,
				height:    1375527,
				blockTime: 1530902705,
				parser:    NewMonacoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *MonacoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "Monacoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewMonacoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   1375527,
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
