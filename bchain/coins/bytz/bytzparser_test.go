// +build unittest

package bytz

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
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
			args:    args{address: "sVqFFBJz11My5GrxnMuX5V1R9ujB1GEipJ"},
			want:    "76a9147f1026d7c5566bbdcf3aebf03b6064c6c1362a0888ac",
			wantErr: false,
		},
	}
	parser := NewBytzParser(GetChainParams("main"), &btc.Configuration{})

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
			name:    "P2PKH1",
			args:    args{script: "76a914dda91c0396050d660f9c0e38f78064486bbfcb2c88ac"},
			want:    []string{"sVqFFBJz11My5GrxnMuX5V1R9ujB1GEipJ"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "pubkey",
			args:    args{script: "4104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac"},
			want:    []string{"sPd3itLs1S1NPbUA9dfZJJnQR5K3UCYzDp"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewBytzParser(GetChainParams("main"), &btc.Configuration{})

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
	// regular transaction
	testTx1       bchain.Tx
	testTxPacked1 = "010000000114fe806f2a4e0bd688494e16cde4a5dd4ca6cde37243ed086cd2b098c6a312eb0100000048473044022001bcf2e8b90711fee4981ad594aac9c3cacdc87c17c28fcb6e5357fb75783c6602202fa2cb56a6ccc95d9b88e6d6a29edca97ea965a2892533d718028833c627002901ffffffff04000000000000000000c0eb20aa83000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3acc06398da4a000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac008888cf380000001976a9141add5533ceac3245a1d2fa22f76a3cac15b2b4d488ac00000000"

)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "0100000001e06937cb574a067b428f19f835c892e75f9c3779033dd4bb996c6f32f4453661010000004847304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b01ffffffff04000000000000000000c0bf766161000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac007afd9128000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac008888cf380000001976a914dadd93ebde4cd80ab6c26c0e1b7b82a2be4ba3a688ac00000000",
		Txid:     "f209cf0b6b159b020a333f48aaa9ac5e4bf487d2f351f902f9040e326a624a21",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b01",
				},
				Txid:     "613645f4326f6c99bbd43d0379379c5fe792c835f8198f427b064a57cb3769e0",
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
				ValueSat: *big.NewInt(418247000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "4104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac",
					Addresses: []string{
						"sPd3itLs1S1NPbUA9dfZJJnQR5K3UCYzDp",
					},
				},
			},
			{
				ValueSat: *big.NewInt(174248000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "4104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac",
					Addresses: []string{
						"sPd3itLs1S1NPbUA9dfZJJnQR5K3UCYzDp",
					},
				},
			},
					{
						ValueSat: *big.NewInt(244000000000),
						N:        3,
						ScriptPubKey: bchain.ScriptPubKey{
							Hex: "76a914dadd93ebde4cd80ab6c26c0e1b7b82a2be4ba3a688ac",
							Addresses: []string{
								"seCepta5SLiX6pBcUCdotUKzwTAQYVTsvB",
					},
				},
			},
		},
		Blocktime: 1595613206,
		Time:      1595613206,
	}
}


func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *BytzParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "bytz-1",
			args: args{
				tx:        testTx1,
				height:    800000,
				blockTime: 1575334901,
				parser:    NewBytzParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *BytzParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "bytz-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBytzParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   800000,
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

type testBlock struct {
	size int
	time int64
	txs  []string
}

var testParseBlockTxs = map[int]testBlock{
	800000: {
		size: 792,
		time: 1575334901,
		txs: []string{
          "556569e1bd20ae007853d839fda5cbefed4883ac53e6327a0a8a30180d242e24",
  				"52b116d26f7c8b633c284f8998a431e106d837c0c5888f9ea5273d36c4556bec",
					"c4b7a36756cd720e4dd5931813f762788b521c0eff27ffcd14832e26b3188340",
		},
	},
}

func helperLoadBlock(t *testing.T, height int) []byte {
	name := fmt.Sprintf("block_dump.%d", height)
	path := filepath.Join("testdata", name)

	d, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d = bytes.TrimSpace(d)

	b := make([]byte, hex.DecodedLen(len(d)))
	_, err = hex.Decode(b, d)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func TestParseBlock(t *testing.T) {
	p := NewBytzParser(GetChainParams("main"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs {
		b := helperLoadBlock(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Fatal(err)
		}

		if blk.Size != tb.size {
			t.Errorf("ParseBlock() block size: got %d, want %d", blk.Size, tb.size)
		}

		if blk.Time != tb.time {
			t.Errorf("ParseBlock() block time: got %d, want %d", blk.Time, tb.time)
		  }
	  }
  }
