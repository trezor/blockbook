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
			want:    "76a914dda91c0396050d660f9c0e38f78064486bbfcb2c88ac",
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
	testTxPacked1 = "0a2052b116d26f7c8b633c284f8998a431e106d837c0c5888f9ea5273d36c4556bec12f501010000000188557c816acd0a61579b701278c7dde85ea25d57877f9dbc65d3b2df2feacc42320000006b483045022100f5d0e98d064d5256852e420a4a3779527fb182c5edbfecf6143fc70eeba8eeef02202f0b2445185fbf846cca07c56c317733a9a4e46f960615f541da7aa27c33cfa201210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ffffffff03000000000000000000f06832fa0100000023210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73aca038370e000000001976a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac000000001883a8aacd0520002880ea303299010a00122042ccea2fdfb2d365bc9d7f87575da25ee8ddc77812709b57610acd6a817c55881832226b483045022100f5d0e98d064d5256852e420a4a3779527fb182c5edbfecf6143fc70eeba8eeef02202f0b2445185fbf846cca07c56c317733a9a4e46f960615f541da7aa27c33cfa201210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae7328ffffffff0f3a0210003a520a0501fa3268f010011a23210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ac2222444b4c33517a43624a71724870524b4148764571736f6d7344686b515076567a5a673a470a040e3738a010021a1976a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac2222444d634e45393855667571454b32674746664b4234597057415771627748524154484000"

)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "010000000188557c816acd0a61579b701278c7dde85ea25d57877f9dbc65d3b2df2feacc42320000006b483045022100f5d0e98d064d5256852e420a4a3779527fb182c5edbfecf6143fc70eeba8eeef02202f0b2445185fbf846cca07c56c317733a9a4e46f960615f541da7aa27c33cfa201210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ffffffff03000000000000000000f06832fa0100000023210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73aca038370e000000001976a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac00000000",
		Txid:     "a8637ca9441a7b1501dce3008415285324459c07df1dd59105fac91424b9b35a",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100f5d0e98d064d5256852e420a4a3779527fb182c5edbfecf6143fc70eeba8eeef02202f0b2445185fbf846cca07c56c317733a9a4e46f960615f541da7aa27c33cfa201210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73",
				},
				Txid:     "fb14f4b6894448bf200668f9057e0b47329beff9fc593bc6e2458b9a993c1dcc",
				Vout:     50,
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
				ValueSat: *big.NewInt(8492574960),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "210251c5555ff3c684aebfca92f5329e2f660da54856299da067060a1bcf5e8fae73ac",
					Addresses: []string{
						"sVqFFBJz11My5GrxnMuX5V1R9ujB1GEipJ",
					},
				},
			},
			{
				ValueSat: *big.NewInt(238500000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b4aa56c103b398f875bb8d15c3bb4136aa62725f88ac",
					Addresses: []string{
						"sVqFFBJz11My5GrxnMuX5V1R9ujB1GEipJ",
					},
				},
			},
		},
		Blocktime: 1504351235,
		Time:      1504351235,
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
		size: 463,
		time: 1504351235,
		txs: []string{
          "556569e1bd20ae007853d839fda5cbefed4883ac53e6327a0a8a30180d242e24",
  				"52b116d26f7c8b633c284f8998a431e106d837c0c5888f9ea5273d36c4556bec",
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
