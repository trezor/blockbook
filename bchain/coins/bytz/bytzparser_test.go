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
			args:    args{address: "sT52jNJNZtnw5aCkEV44PfFWVSzzzhRC5D"},
			want:    "76a91460c326d60d9b97f362692443dc8fcbd5468ab3e788ac",
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
			args:    args{script: "76a914df5c0d39a08b7bc2fd8b35fa73f501b527ac4b8488ac"},
			want:    []string{"secR3DMvMhjAXABDXjpdJtd2M6TCv496yD"},
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
	testTxPacked1 = "0a20613645f4326f6c99bbd43d0379379c5fe792c835f8198f427b064a57cb3769e012be020100000001e06937cb574a067b428f19f835c892e75f9c3779033dd4bb996c6f32f4453661010000004847304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b01ffffffff04000000000000000000c0bf766161000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac007afd9128000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac008888cf380000001976a914dadd93ebde4cd80ab6c26c0e1b7b82a2be4ba3a688ac0000000018ecbadef505200028cc8c3f32760a001220eb12a3c698b0d26c08ed4372e3cda64cdda5e4cd164e4988d60b4e2a6f80fe141801224847304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b0128ffffffff0f3a0210003a720a0583aa20ebc010011a434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac22227350643369744c733153314e506255413964665a4a4a6e5152354b335543597a44703a720a054ada9863c010021a434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac22227350643369744c733153314e506255413964665a4a4a6e5152354b335543597a44703a480a0538cf88880010031a1976a9141add5533ceac3245a1d2fa22f76a3cac15b2b4d488ac2222734c68536f333331444b6445354c6168594472534544364e6d3667763655354e73384000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "0100000001e06937cb574a067b428f19f835c892e75f9c3779033dd4bb996c6f32f4453661010000004847304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b01ffffffff04000000000000000000c0bf766161000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac007afd9128000000434104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac008888cf380000001976a914dadd93ebde4cd80ab6c26c0e1b7b82a2be4ba3a688ac00000000",
		Txid:     "613645f4326f6c99bbd43d0379379c5fe792c835f8198f427b064a57cb3769e0",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402207e112ce470ca311278dd01410a58da9f8be1e501994d313fbf604f98f4bf7104022002754097698743cf9825f08a5993b3b53c5f81ec1229b73b16d5215d607ed94b01",
				},
				Txid:     "eb12a3c698b0d26c08ed4372e3cda64cdda5e4cd164e4988d60b4e2a6f80fe14",
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
				ValueSat: *big.NewInt(565495000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "4104fe8490b77c7aaaf08b3e9a7305691dc1ed03a92aaa89804b0e5c9fc18741936e77cdc9f0d9250083a873dc934402038d025050e4ed9836caaeaabc3d6e47bdd3ac",
					Addresses: []string{
						"sPd3itLs1S1NPbUA9dfZJJnQR5K3UCYzDp",
					},
				},
			},
			{
				ValueSat: *big.NewInt(321495000000),
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
							Hex: "76a9141add5533ceac3245a1d2fa22f76a3cac15b2b4d488ac",
							Addresses: []string{
								"sLhSo331DKdE5LahYDrSED6Nm6gv6U5Ns8",
					},
				},
			},
		},
		Blocktime: 1589091692,
		Time:      1589091692,
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
				height:    1033804,
				blockTime: 1589091692,
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
			want1:   1033804,
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
	1033804: {
		size: 569,
		time: 1589091692,
		txs: []string{
          "b76af5d1c51955aa59f50079d76ffdbda94b8ba4ec81b7f9d116dbb4d2ae8cb3",
  				"613645f4326f6c99bbd43d0379379c5fe792c835f8198f427b064a57cb3769e0",
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
