//go:build unittest

package digiwage

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
			args:    args{address: "DRM8TaiY38qcHbgdytp8oETreobBLHtpeE"},
			want:    "76a914dda91c0396050d660f9c0e38f78064486bbfcb2c88ac",
			wantErr: false,
		},
	}
	parser := NewDigiwageParser(GetChainParams("main"), &btc.Configuration{})

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
			want:    []string{"DRM8TaiY38qcHbgdytp8oETreobBLHtpeE"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "pubkey",
			args:    args{script: "210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ac"},
			want:    []string{"DKL3QzCbJqrHpRKAHvEqsomsDhkQPvVzZg"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewDigiwageParser(GetChainParams("main"), &btc.Configuration{})

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
	testTxPacked1 = "0a20ed32c532d67f9a6ca89f89c489e05d606b584c43eb4eaeae7788124a8e35881f12d2010100000001c1225b643116fc77b91efaff8f8ee43f44ab824b8bc1ee3c73f29cd5f48fbd0e0100000048473044022000afc06134440013c55f949b117fc77ab2257026118eee0c5fe46cf38cfec34502200922a4a9cd03f65ff84289701cd8e6f371bf6bcd4dc05ab12d8f43e9367915ff01ffffffff0300000000000000000028bc1c2458000000232102cbbe3ec4472b20c05c30f476f05e43932ab523b0e5057225679afc8d53c4072bac0050d6dc010000001976a9141b6c76434d26d81578228b31923c00868bd963d188ac0000000018dc9699d60528ba5d327412200ebd8ff4d59cf2733ceec18b4b82ab443fe48e8ffffa1eb977fc1631645b22c118012248473044022000afc06134440013c55f949b117fc77ab2257026118eee0c5fe46cf38cfec34502200922a4a9cd03f65ff84289701cd8e6f371bf6bcd4dc05ab12d8f43e9367915ff0128ffffffff0f3a003a520a0558241cbc2810011a23210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ac22224454525a75635776756742463152696b72694b4573735971794b3777316e546e786b3a480a0501dcd6500010021a1976a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac22224437653669574e684d707a3169765233534c535742653638424a39675462594d3852"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "0100000001c1225b643116fc77b91efaff8f8ee43f44ab824b8bc1ee3c73f29cd5f48fbd0e0100000048473044022000afc06134440013c55f949b117fc77ab2257026118eee0c5fe46cf38cfec34502200922a4a9cd03f65ff84289701cd8e6f371bf6bcd4dc05ab12d8f43e9367915ff01ffffffff0300000000000000000028bc1c2458000000232102cbbe3ec4472b20c05c30f476f05e43932ab523b0e5057225679afc8d53c4072bac0050d6dc010000001976a9141b6c76434d26d81578228b31923c00868bd963d188ac00000000",
		Txid:     "ed32c532d67f9a6ca89f89c489e05d606b584c43eb4eaeae7788124a8e35881f",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022000afc06134440013c55f949b117fc77ab2257026118eee0c5fe46cf38cfec34502200922a4a9cd03f65ff84289701cd8e6f371bf6bcd4dc05ab12d8f43e9367915ff01",
				},
				Txid:     "0ebd8ff4d59cf2733ceec18b4b82ab443fe48e8ffffa1eb977fc1631645b22c1",
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
				ValueSat: *big.NewInt(378562985000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ac",
					Addresses: []string{
						"DTRZucWvugBF1RikriKEssYqyK7w1nTnxk",
					},
				},
			},
			{
				ValueSat: *big.NewInt(8000000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac",
					Addresses: []string{
						"D7e6iWNhMpz1ivR3SLSWBe68BJ9gTbYM8R",
					},
				},
			},
		},
		Blocktime: 1522944860,
		Time:      1522944860,
	}

}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *DigiwageParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Digiwage-1",
			args: args{
				tx:        testTx1,
				height:    11962,
				blockTime: 1522944860,
				parser:    NewDigiwageParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *DigiwageParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "Digiwage-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewDigiwageParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   11962,
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
	11962: {
		size: 427,
		time: 1522944860,
		txs: []string{
			"ff888cfafb3953d55fa89e5759d3a8327eae9df69ab06be896dd8014ddc2e207",
			"ed32c532d67f9a6ca89f89c489e05d606b584c43eb4eaeae7788124a8e35881f",
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
	p := NewDigiwageParser(GetChainParams("main"), &btc.Configuration{})

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

		if len(blk.Txs) != len(tb.txs) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.txs))
		}

		for ti, tx := range tb.txs {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}

