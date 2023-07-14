//go:build unittest

package omotenashicoin

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

// Test getting the address details from the address hash

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
			args:    args{address: "SSpnQKkfgwKhdsp7FDm3BziRFKfX1zbmWL"},
			want:    "76a9143caafb26e97bc26f27543a523f7e2ab8b841d76988ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "8Hkth4xWKCemL8u88urWXwSqUV4WSX5FDm"},
			want:    "a9141d521dcf4983772b3c1e6ef937103ebdfaa1ad7787",
			wantErr: false,
		},
	}
	parser := NewOmotenashiCoinParser(GetChainParams("main"), &btc.Configuration{})

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
			args:    args{address: "aRimakbKh2CiG4PSTYcnFGbVzcCZFQr2kd"},
			want:    "76a914124a09dbceec6cf2060d72959ba7009649a7783088ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "8GkZahDbYJ5m6tKosGy4j7uN5pTF21ktHU"},
			want:    "a914124a09dbceec6cf2060d72959ba7009649a7783087",
			wantErr: false,
		},
	}
	parser := NewOmotenashiCoinParser(GetChainParams("test"), &btc.Configuration{})

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
			name:    "Normal",
			args:    args{script: "76a914f1684a035088c20e76ece8e4dd79bdead0e1569a88ac"},
			want:    []string{"SjJSpCzWBYacyj6wGmaFoPoj79QSQq8wcw"},
			want2:   true,
			wantErr: false,
		},
	}

	parser := NewOmotenashiCoinParser(GetChainParams("main"), &btc.Configuration{})

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

// Test the packing and unpacking of raw transaction data

var (
	// Block Height 600
	testTx1                bchain.Tx
	testTxPacked_testnet_1 = "0a2054af08185cf5c5d312ebd9865b4b224c6120801b209343cfb9dc3332af28a2a5126401000000010000000000000000000000000000000000000000000000000000000000000000ffffffff050258020101ffffffff0100e87648170000002321024a9c0d55966c7a46d8ac15830c6c26555a2b570a3e78c51534ccc8dadc7943c8ac000000001894e38ff10528d80432120a0a3032353830323031303128ffffffff0f3a500a05174876e8001a2321024a9c0d55966c7a46d8ac15830c6c26555a2b570a3e78c51534ccc8dadc7943c8ac2222616e667766545642725934795a4e54543167625a61584e664854377951544856674d"

	// Block Height 135001
	testTx2                bchain.Tx
	testTxPacked_mainnet_1 = "0a20a2eedb3990bddcace3a5211332e86f70d0195a2a7efaad2de18698172ff9fc6d128f0102000000010000000000000000000000000000000000000000000000000000000000000000ffffffff1803590f020445b8075e088104e0b22c0000007969696d70000000000002807c814a000000001976a91487bac515ab40891b58a05c913f908194c9d73bd588ac807584df000000001976a914a1441e207bd13f80b2142026ad39a58b5f47434d88ac0000000018c4f09ef00528d99e0832320a303033353930663032303434356238303735653038383130346530623232633030303030303739363936393664373030303a450a044a817c801a1976a91487bac515ab40891b58a05c913f908194c9d73bd588ac2222535a66667a6a666454486f7a394675684a5444453847704378455762544c433654743a450a04df8475801a1976a914a1441e207bd13f80b2142026ad39a58b5f47434d88ac222253627a685264475855475245556b70557a67716877615847666f4244447565366b36"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:      "01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff050258020101ffffffff0100e87648170000002321024a9c0d55966c7a46d8ac15830c6c26555a2b570a3e78c51534ccc8dadc7943c8ac00000000",
		Txid:     "54af08185cf5c5d312ebd9865b4b224c6120801b209343cfb9dc3332af28a2a5",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				Coinbase: "0258020101",
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(100000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "21024a9c0d55966c7a46d8ac15830c6c26555a2b570a3e78c51534ccc8dadc7943c8ac",
					Addresses: []string{
						"anfwfTVBrY4yZNTT1gbZaXNfHT7yQTHVgM",
					},
				},
			},
		},
		Blocktime: 1579413908,
		Time:      1579413908,
	}

	//135001
	testTx2 = bchain.Tx{
		Hex:      "02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff1803590f020445b8075e088104e0b22c0000007969696d70000000000002807c814a000000001976a91487bac515ab40891b58a05c913f908194c9d73bd588ac807584df000000001976a914a1441e207bd13f80b2142026ad39a58b5f47434d88ac00000000",
		Txid:     "a2eedb3990bddcace3a5211332e86f70d0195a2a7efaad2de18698172ff9fc6d",
		LockTime: 0,
		Vin: []bchain.Vin{
			{
				Coinbase: "03590f020445b8075e088104e0b22c0000007969696d7000",
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1250000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91487bac515ab40891b58a05c913f908194c9d73bd588ac",
					Addresses: []string{
						"SZffzjfdTHoz9FuhJTDE8GpCxEWbTLC6Tt",
					},
				},
			},
			{
				ValueSat: *big.NewInt(3750000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914a1441e207bd13f80b2142026ad39a58b5f47434d88ac",
					Addresses: []string{
						"SbzhRdGXUGREUkpUzgqhwaXGfoBDDue6k6",
					},
				},
			},
		},
		Blocktime: 1577564228,
		Time:      1577564228,
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *OmotenashiCoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "mtnstestnet_1",
			args: args{
				tx:        testTx1,
				height:    600,
				blockTime: 1579413908,
				parser:    NewOmotenashiCoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked_testnet_1,
			wantErr: false,
		},
		{
			name: "mtnsmainnet_1",
			args: args{
				tx:        testTx2,
				height:    135001,
				blockTime: 1577564228,
				parser:    NewOmotenashiCoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked_mainnet_1,
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
		parser   *OmotenashiCoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "mtns_1",
			args: args{
				packedTx: testTxPacked_testnet_1,
				parser:   NewOmotenashiCoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   600,
			wantErr: false,
		},
		{
			name: "mtns_2",
			args: args{
				packedTx: testTxPacked_mainnet_1,
				parser:   NewOmotenashiCoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   135001,
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

// Block test - looks for size, time, and transaction hashes

type testBlock struct {
	size int
	time int64
	tx   []string
}

var testParseBlockTxs = map[int]testBlock{
	600: {
		size: 181,
		time: 1579413908,
		tx: []string{
			"54af08185cf5c5d312ebd9865b4b224c6120801b209343cfb9dc3332af28a2a5",
		},
	},
	135001: {
		size: 224,
		time: 1577564228,
		tx: []string{
			"a2eedb3990bddcace3a5211332e86f70d0195a2a7efaad2de18698172ff9fc6d",
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
	p := NewOmotenashiCoinParser(GetChainParams("main"), &btc.Configuration{})

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

		if len(blk.Txs) != len(tb.tx) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.tx))
		}

		for ti, tx := range tb.tx {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}
