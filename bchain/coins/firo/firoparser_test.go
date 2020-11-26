// +build unittest

package firo

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

var (
	testTx1, testTx2, testTx3, testTx4, testTx5, testTx6                                     bchain.Tx
	rawTestTx1, rawTestTx2, rawTestTx3, rawTestTx4, rawTestTx5, rawTestTx6                   string
	testTxPacked1, testTxPacked2, testTxPacked3, testTxPacked4, testTxPacked5, testTxPacked6 string
	rawBlock1, rawBlock2, rawBlock3                                                          string
	jsonTx                                                                                   json.RawMessage
)

func readHexs(path string) []string {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	rawStr := string(raw)
	raws := strings.Split(rawStr, "\n")
	return raws
}

func init() {
	rawBlocks := readHexs("./testdata/rawblock.hex")
	rawBlock1 = rawBlocks[0]
	rawBlock2 = rawBlocks[1]
	rawBlock3 = rawBlocks[2]

	hextxs := readHexs("./testdata/txs.hex")
	rawTestTx1 = hextxs[0]
	rawTestTx2 = hextxs[1]
	rawTestTx3 = hextxs[2]
	rawTestTx4 = hextxs[3]
	rawTestTx5 = hextxs[4]
	rawTestTx6 = hextxs[5]

	rawSpendHex := readHexs("./testdata/rawspend.hex")[0]

	rawSpendTx, err := ioutil.ReadFile("./testdata/spendtx.json")
	if err != nil {
		panic(err)
	}
	jsonTx = json.RawMessage(rawSpendTx)

	testTxPackeds := readHexs("./testdata/packedtxs.hex")
	testTxPacked1 = testTxPackeds[0]
	testTxPacked2 = testTxPackeds[1]
	testTxPacked3 = testTxPackeds[2]
	testTxPacked4 = testTxPackeds[3]
	testTxPacked5 = testTxPackeds[4]
	testTxPacked6 = testTxPackeds[5]

	testTx1 = bchain.Tx{
		Hex:       rawTestTx1,
		Blocktime: 1533980594,
		Time:      1533980594,
		Txid:      "9d9e759dd970d86df9e105a7d4f671543bc16a03b6c5d2b48895f2a00aa7dd23",
		LockTime:  99688,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402205b7d9c9aae790b69017651e10134735928df3b4a4a2feacc9568eb4fa133ed5902203f21a399385ce29dd79831ea34aa535612aa4314c5bd0b002bbbc9bcd2de1436012102b8d462740c99032a00083ac7028879acec244849e54ad0a04ea87f632f54b1d2",
				},
				Txid:     "463a2d66b04636a014da35724909425f3403d9d786dd4f79780de50d47b18716",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(100000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "c10280004c80f767f3ee79953c67a7ed386dcccf1243619eb4bbbe414a3982dd94a83c1b69ac52d6ab3b653a3e05c4e4516c8dfe1e58ada40461bc5835a4a0d0387a51c29ac11b72ae25bbcdef745f50ad08f08b3e9bc2c31a35444398a490e65ac090e9f341f1abdebe47e57e8237ac25d098e951b4164a35caea29f30acb50b12e4425df28",
				},
			},
			{
				ValueSat: *big.NewInt(871824000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914c963f917c7f23cb4243e079db33107571b87690588ac",
					Addresses: []string{
						"aK5KKi8qqDbspcXFfDjx8UBGMouhYbYZVp",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       rawTestTx2,
		Blocktime: 1481277009,
		Time:      1481277009,
		Txid:      "3d721fdce2855e2b4a54b74a26edd58a7262e1f195b5acaaae7832be6e0b3d32",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				Coinbase: rawSpendHex,
				Txid:     "0000000000000000000000000000000000000000000000000000000000000000",
				Vout:     4294967295,
				Sequence: 2,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(5000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b9e262e30df03e88ccea312652bc83ca7290c8fc88ac",
					Addresses: []string{
						"aHfKwzFZMiSxDuNL4jts819nh57t2yJG1h",
					},
				},
			},
		},
	}

	testTx3 = bchain.Tx{
		Hex:       rawTestTx3,
		Blocktime: 1547091829,
		Time:      1547091829,
		Txid:      "96ae951083651f141d1fb2719c76d47e5a3ad421b81905f679c0edb60f2de0ff",
		LockTime:  126200,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100bdc6b51c114617e29e28390dc9b3ad95b833ca3d1f0429ba667c58a667f9124702204ca2ed362dd9ef723ddbdcf4185b47c28b127a36f46bc4717662be863309b3e601210387e7ff08b953e3736955408fc6ebcd8aa84a04cc4b45758ea29cc2cfe1820535",
				},
				Txid:     "448ccfd9c3f375be8701b86aff355a230dbe240334233f2ed476fcae6abd295d",
				Vout:     1,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(42000000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91429bef7962c5c65a2f0f4f7d9ec791866c54f851688ac",
					Addresses: []string{
						"a4XCDQ7AnRH9opZ4h6LcG3g7ocSV2SbBmS",
					},
				},
			},
			{
				ValueSat: *big.NewInt(107300000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e2cee7b71c3a4637dbdfe613f19f4b4f2d070d7f88ac",
					Addresses: []string{
						"aMPiKHB3E1AGPi8kKLknx6j1L4JnKCGkLw",
					},
				},
			},
		},
	}

	testTx4 = bchain.Tx{
		Hex:       rawTestTx4,
		Blocktime: 1533977563,
		Time:      1533977563,
		Txid:      "914ccbdb72f593e5def15978cf5891e1384a1b85e89374fc1c440c074c6dd286",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				Coinbase: "03a1860104dba36e5b082a00077c00000000052f6d70682f",
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(2800200000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91436e086acf6561a68ba64196e7b92b606d0b8516688ac",
					Addresses: []string{
						"a5idCcHN8WYxvFCeBXSXvMPrZHuBkZmqEJ",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1500000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914381a5dd1a279e8e63e67cde39ecfa61a99dd2ba288ac",
					Addresses: []string{
						"a5q7Ad4okSFFVh5adyqx5DT21RTxJykpUM",
					},
				},
			},
			{
				ValueSat: *big.NewInt(100000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9147d9ed014fc4e603fca7c2e3f9097fb7d0fb487fc88ac",
					Addresses: []string{
						"aCAgTPgtYcA4EysU4UKC86EQd5cTtHtCcr",
					},
				},
			},
			{
				ValueSat: *big.NewInt(100000000),
				N:        3,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914bc7e5a5234db3ab82d74c396ad2b2af419b7517488ac",
					Addresses: []string{
						"aHu897ivzmeFuLNB6956X6gyGeVNHUBRgD",
					},
				},
			},
			{
				ValueSat: *big.NewInt(100000000),
				N:        4,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ff71b0c9c2a90c6164a50a2fb523eb54a8a6b55088ac",
					Addresses: []string{
						"aQ18FBVFtnueucZKeVg4srhmzbpAeb1KoN",
					},
				},
			},
			{
				ValueSat: *big.NewInt(300000000),
				N:        5,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9140654dd9b856f2ece1d56cb4ee5043cd9398d962c88ac",
					Addresses: []string{
						"a1HwTdCmQV3NspP2QqCGpehoFpi8NY4Zg3",
					},
				},
			},
			{
				ValueSat: *big.NewInt(100000000),
				N:        6,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9140b4bfb256ef4bfa360e3b9e66e53a0bd84d196bc88ac",
					Addresses: []string{
						"a1kCCGddf5pMXSipLVD9hBG2MGGVNaJ15U",
					},
				},
			},
			// TODO: test segwit
		},
	}

	testTx5 = bchain.Tx{
		Hex:       rawTestTx5,
		Blocktime: 1591752749,
		Time:      1591752749,
		Txid:      "8d1f32f35c32d2c127a7400dc1ec52049fbf0b8bcdf284cfaa3da59b6169a22d",
		LockTime:  0,
		Vin:       []bchain.Vin{},
		Vout:      []bchain.Vout{},
	}

	testTx6 = bchain.Tx{
		Hex:       rawTestTx6,
		Blocktime: 1591762049,
		Time:      1591762049,
		Txid:      "e5767d3606230a65f150837a6f28b4f0e4c2702a683045df3883d57702739c61",
		LockTime:  0,
		Vin: []bchain.Vin{
			{
				Coinbase: "02b4140101",
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1400000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "2103fb09a216761d5e7f248294970c2370f7f84ce1ad564b8e7096b1e19116af1d52ac",
					Addresses: []string{
						"TAn9Ghkp31myXRgejCj11wWVHT14Lsj349",
					},
				},
			},
			{
				ValueSat: *big.NewInt(50000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914296134d2415bf1f2b518b3f673816d7e603b160088ac",
					Addresses: []string{
						"TDk19wPKYq91i18qmY6U9FeTdTxwPeSveo",
					},
				},
			},
			{
				ValueSat: *big.NewInt(50000000),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e1e1dc06a889c1b6d3eb00eef7a96f6a7cfb884888ac",
					Addresses: []string{
						"TWZZcDGkNixTAMtRBqzZkkMHbq1G6vUTk5",
					},
				},
			},
			{
				ValueSat: *big.NewInt(50000000),
				N:        3,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914ab03ecfddee6330497be894d16c29ae341c123aa88ac",
					Addresses: []string{
						"TRZTFdNCKCKbLMQV8cZDkQN9Vwuuq4gDzT",
					},
				},
			},
			{
				ValueSat: *big.NewInt(150000000),
				N:        4,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9144281a58a1d5b2d3285e00cb45a8492debbdad4c588ac",
					Addresses: []string{
						"TG2ruj59E5b1u9G3F7HQVs6pCcVDBxrQve",
					},
				},
			},
			{
				ValueSat: *big.NewInt(50000000),
				N:        5,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9141fd264c0bb53bd9fef18e2248ddf1383d6e811ae88ac",
					Addresses: []string{
						"TCsTzQZKVn4fao8jDmB9zQBk9YQNEZ3XfS",
					},
				},
			},
			{
				ValueSat: *big.NewInt(750000000),
				N:        6,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91471a3892d164ffa3829078bf9ad5f114a3908ce5588ac",
					Addresses: []string{
						"TLL5GQULX4uBfz7yXL6VcZyvzdKVv1RGxm",
					},
				},
			},
		},
	}
}

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func TestGetAddrDesc(t *testing.T) {
	type args struct {
		tx     bchain.Tx
		parser *FiroParser
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "firo-1",
			args: args{
				tx:     testTx1,
				parser: NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
		// FIXME: work around handle zerocoin spend as coinbase
		// {
		// 	name: "firo-2",
		// 	args: args{
		// 		tx:     testTx2,
		// 		parser: NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
		// 	},
		// },
		{
			name: "firo-3",
			args: args{
				tx:     testTx3,
				parser: NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for n, vout := range tt.args.tx.Vout {
				got1, err := tt.args.parser.GetAddrDescFromVout(&vout)
				if err != nil {
					t.Errorf("getAddrDescFromVout() error = %v, vout = %d", err, n)
					return
				}

				// normal vout
				if len(vout.ScriptPubKey.Addresses) >= 1 {
					got2, err := tt.args.parser.GetAddrDescFromAddress(vout.ScriptPubKey.Addresses[0])
					if err != nil {
						t.Errorf("getAddrDescFromAddress() error = %v, vout = %d", err, n)
						return
					}
					if !bytes.Equal(got1, got2) {
						t.Errorf("Address descriptors mismatch: got1 = %v, got2 = %v", got1, got2)
					}
				}
			}
		})
	}
}

func TestGetAddrDescFromVoutForMint(t *testing.T) {
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
			name:    "OP_RETURN",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "6a072020f1686f6a20"}}},
			want:    "6a072020f1686f6a20",
			wantErr: false,
		},
		{
			name:    "OP_ZEROCOINMINT",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "c10280004c80f767f3ee79953c67a7ed386dcccf1243619eb4bbbe414a3982dd94a83c1b69ac52d6ab3b653a3e05c4e4516c8dfe1e58ada40461bc5835a4a0d0387a51c29ac11b72ae25bbcdef745f50ad08f08b3e9bc2c31a35444398a490e65ac090e9f341f1abdebe47e57e8237ac25d098e951b4164a35caea29f30acb50b12e4425df28"}}},
			want:    "c10280004c80f767f3ee79953c67a7ed386dcccf1243619eb4bbbe414a3982dd94a83c1b69ac52d6ab3b653a3e05c4e4516c8dfe1e58ada40461bc5835a4a0d0387a51c29ac11b72ae25bbcdef745f50ad08f08b3e9bc2c31a35444398a490e65ac090e9f341f1abdebe47e57e8237ac25d098e951b4164a35caea29f30acb50b12e4425df28",
			wantErr: false,
		},
		{
			name:    "OP_SIGMAMINT",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "c317dcee5b8b2c5b79728abc3a39abc54682b31a4e18f5abb6f34dc8089544763b0000"}}},
			want:    "c317dcee5b8b2c5b79728abc3a39abc54682b31a4e18f5abb6f34dc8089544763b0000",
			wantErr: false,
		},
	}
	parser := NewFiroParser(GetChainParams("main"), &btc.Configuration{})

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

func TestGetAddressesFromAddrDescForMint(t *testing.T) {
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
			name:    "OP_RETURN hex",
			args:    args{script: "6a072020f1686f6a20"},
			want:    []string{"OP_RETURN 2020f1686f6a20"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_ZEROCOINMINT size hex",
			args:    args{script: "c10280004c80f767f3ee79953c67a7ed386dcccf1243619eb4bbbe414a3982dd94a83c1b69ac52d6ab3b653a3e05c4e4516c8dfe1e58ada40461bc5835a4a0d0387a51c29ac11b72ae25bbcdef745f50ad08f08b3e9bc2c31a35444398a490e65ac090e9f341f1abdebe47e57e8237ac25d098e951b4164a35caea29f30acb50b12e4425df28"},
			want:    []string{"Zeromint"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_SIGMAMINT size hex",
			args:    args{script: "c317dcee5b8b2c5b79728abc3a39abc54682b31a4e18f5abb6f34dc8089544763b0000"},
			want:    []string{"Sigmamint"},
			want2:   false,
			wantErr: false,
		},
	}
	parser := NewFiroParser(GetChainParams("main"), &btc.Configuration{})

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

func TestPackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *FiroParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "firo-1",
			args: args{
				tx:        testTx1,
				height:    100002,
				blockTime: 1533980594,
				parser:    NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		// FIXME: work around handle zerocoin spend as coinbase
		// {
		// 	name: "firo-2",
		// 	args: args{
		// 		tx:        testTx2,
		// 		height:    11002,
		// 		blockTime: 1481277009,
		// 		parser:    NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
		// 	},
		// 	want:    testTxPacked2,
		// 	wantErr: true,
		// },
		{
			name: "firo-3",
			args: args{
				tx:        testTx3,
				height:    126202,
				blockTime: 1547091829,
				parser:    NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked3,
			wantErr: false,
		},
		{
			name: "firo-coinbase",
			args: args{
				tx:        testTx4,
				height:    100001,
				blockTime: 1533977563,
				parser:    NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked4,
			wantErr: false,
		},
		{
			name: "firo-quorum-commitment-tx",
			args: args{
				tx:        testTx5,
				height:    5268,
				blockTime: 1591752749,
				parser:    NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked5,
			wantErr: false,
		},
		{
			name: "firo-special-coinbase-tx",
			args: args{
				tx:        testTx6,
				height:    5300,
				blockTime: 1591762049,
				parser:    NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked6,
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
			if !tt.wantErr {
				h := hex.EncodeToString(got)
				if !reflect.DeepEqual(h, tt.want) {
					t.Errorf("packTx() = %v, want %v", h, tt.want)
				}
			}
		})
	}
}

func TestUnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *FiroParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "firo-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   100002,
			wantErr: false,
		},
		// FIXME: work around handle zerocoin spend as coinbase
		// {
		// 	name: "firo-2",
		// 	args: args{
		// 		packedTx: testTxPacked2,
		// 		parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
		// 	},
		// 	want:    &testTx2,
		// 	want1:   11002,
		// 	wantErr: true,
		// },
		{
			name: "firo-3",
			args: args{
				packedTx: testTxPacked3,
				parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx3,
			want1:   126202,
			wantErr: false,
		},
		{
			name: "firo-coinbase",
			args: args{
				packedTx: testTxPacked4,
				parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx4,
			want1:   100001,
			wantErr: false,
		},
		{
			name: "firo-special-tx",
			args: args{
				packedTx: testTxPacked5,
				parser:   NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx5,
			want1:   5268,
			wantErr: false,
		},
		{
			name: "firo-special-coinbase-tx",
			args: args{
				packedTx: testTxPacked6,
				parser:   NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx6,
			want1:   5300,
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

			if !tt.wantErr {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("unpackTx() got = %v, want %v", got, tt.want)
				}
				if got1 != tt.want1 {
					t.Errorf("unpackTx() got1 = %v, want %v", got1, tt.want1)
				}
			}
		})
	}
}

func TestParseBlock(t *testing.T) {
	type args struct {
		rawBlock string
		parser   *FiroParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Block
		wantTxs int
		wantErr bool
	}{
		{
			name: "normal-block",
			args: args{
				rawBlock: rawBlock1,
				parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: &bchain.Block{
				BlockHeader: bchain.BlockHeader{
					Size: 200286,
					Time: 1547120622,
				},
			},
			wantTxs: 3,
			wantErr: false,
		},
		{
			name: "spend-block",
			args: args{
				rawBlock: rawBlock2,
				parser:   NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: &bchain.Block{
				BlockHeader: bchain.BlockHeader{
					Size: 25298,
					Time: 1482107572,
				},
			},
			wantTxs: 4,
			wantErr: false,
		},
		{
			name: "special-tx-block",
			args: args{
				rawBlock: rawBlock3,
				parser:   NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want: &bchain.Block{
				BlockHeader: bchain.BlockHeader{
					Size: 200062,
					Time: 1591752749,
				},
			},
			wantTxs: 3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.rawBlock)
			got, err := tt.args.parser.ParseBlock(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBlock() error = %+v", err)
			}

			if got != nil {

				if !reflect.DeepEqual(got.BlockHeader, tt.want.BlockHeader) {
					t.Errorf("parseBlock() got = %v, want %v", got.BlockHeader, tt.want.BlockHeader)
				}

				if len(got.Txs) != tt.wantTxs {
					t.Errorf("parseBlock() txs length got = %d, want %d", len(got.Txs), tt.wantTxs)
				}
			}
		})
	}
}

func TestDecodeTransaction(t *testing.T) {
	type args struct {
		enc            wire.MessageEncoding
		rawTransaction string
		parser         *FiroParser
		privacyType    byte // 0 as non privacy
	}
	tests := []struct {
		name    string
		args    args
		want    bchain.Tx
		wantErr bool
	}{
		{
			name: "normal-transaction",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx1,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: testTx1,
		},
		{
			name: "coinbase-firospend",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx2,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
				privacyType:    OpSigmaSpend,
			},
			want: testTx2,
		},
		{
			name: "normal-transaction-2",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx3,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: testTx3,
		},
		{
			name: "coinbase-transaction",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx4,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: testTx4,
		},
		{
			name: "quorum-commitment-transaction",
			args: args{
				enc:            wire.BaseEncoding,
				rawTransaction: rawTestTx5,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want: testTx5,
		},
		{
			name: "quorum-commitment-transaction-witness",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx5,
				parser:         NewFiroParser(GetChainParams("main"), &btc.Configuration{}),
			},
			wantErr: true,
		},
		{
			name: "special-coinbase",
			args: args{
				enc:            wire.WitnessEncoding,
				rawTransaction: rawTestTx6,
				parser:         NewFiroParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want: testTx6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.rawTransaction)
			r := bytes.NewReader(b)

			msg := FiroMsgTx{}
			err := msg.FiroDecode(r, 0, tt.args.enc)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Want error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			got := tt.args.parser.TxFromFiroMsgTx(&msg, true)
			if pErr := tt.args.parser.parseFiroTx(&got); pErr != nil {
				t.Fatal(pErr)
			}

			if r.Len() != 0 {
				t.Errorf("Expected EOF but there are remaining %d bytes to read", r.Len())
			}

			if len(got.Vin) != len(tt.want.Vin) {
				t.Errorf("Check vin size, got %v, want %v", len(got.Vin), len(tt.want.Vin))
			}

			for i := 0; i != len(got.Vin); i++ {
				if !reflect.DeepEqual(got.Vin[i].Addresses, tt.want.Vin[i].Addresses) {
					t.Errorf("Check Addresses at input %d, got %v, want %v",
						i, got.Vin[i].Addresses, tt.want.Vin[i].Addresses)
				}

				if !reflect.DeepEqual(got.Vin[i].Coinbase, tt.want.Vin[i].Coinbase) {
					t.Errorf("Check Coinbase at input %d, got %v, want %v",
						i, got.Vin[i].Coinbase, tt.want.Vin[i].Coinbase)
				}

				if !reflect.DeepEqual(got.Vin[i].ScriptSig, tt.want.Vin[i].ScriptSig) {
					t.Errorf("Check ScriptSig at input %d, got %v, want %v",
						i, got.Vin[i].ScriptSig, tt.want.Vin[i].ScriptSig)
				}

				if !reflect.DeepEqual(got.Vin[i].Sequence, tt.want.Vin[i].Sequence) {
					t.Errorf("Check Sequence at input %d, got %v, want %v",
						i, got.Vin[i].Sequence, tt.want.Vin[i].Sequence)
				}

				if tt.args.privacyType == 0 && !reflect.DeepEqual(got.Vin[i].Txid, tt.want.Vin[i].Txid) {
					t.Errorf("Check Txid at input %d, got %v, want %v",
						i, got.Vin[i].Txid, tt.want.Vin[i].Txid)
				}

				if tt.args.privacyType == 0 && !reflect.DeepEqual(got.Vin[i].Vout, tt.want.Vin[i].Vout) {
					t.Errorf("Check Vout at input %d, got %v, want %v",
						i, got.Vin[i].Vout, tt.want.Vin[i].Vout)
				}
			}

			if len(got.Vout) != len(tt.want.Vout) {
				t.Errorf("Check vout size, got %v, want %v", len(got.Vout), len(tt.want.Vout))
			}

			for i := 0; i != len(got.Vout); i++ {
				if !reflect.DeepEqual(got.Vout[i].JsonValue, tt.want.Vout[i].JsonValue) {
					t.Errorf("Check JsonValue at output %d, got %v, want %v",
						i, got.Vout[i].JsonValue, tt.want.Vout[i].JsonValue)
				}

				if !reflect.DeepEqual(got.Vout[i].N, tt.want.Vout[i].N) {
					t.Errorf("Check N at output %d, got %v, want %v",
						i, got.Vout[i].N, tt.want.Vout[i].N)
				}

				// empty addresses and null should be the same
				if !((len(got.Vout[i].ScriptPubKey.Addresses) == 0 && len(got.Vout[i].ScriptPubKey.Addresses) == len(tt.want.Vout[i].ScriptPubKey.Addresses)) ||
					reflect.DeepEqual(got.Vout[i].ScriptPubKey.Addresses, tt.want.Vout[i].ScriptPubKey.Addresses)) {
					t.Errorf("Check ScriptPubKey.Addresses at output %d, got %v, want %v",
						i, got.Vout[i].ScriptPubKey.Addresses, tt.want.Vout[i].ScriptPubKey.Addresses)
				}

				if !reflect.DeepEqual(got.Vout[i].ScriptPubKey.Hex, tt.want.Vout[i].ScriptPubKey.Hex) {
					t.Errorf("Check ScriptPubKey.Hex at output %d, got %v, want %v",
						i, got.Vout[i].ScriptPubKey.Hex, tt.want.Vout[i].ScriptPubKey.Hex)
				}

				if !reflect.DeepEqual(got.Vout[i].ValueSat, tt.want.Vout[i].ValueSat) {
					t.Errorf("Check ValueSat at output %d, got %v, want %v",
						i, got.Vout[i].ValueSat, tt.want.Vout[i].ValueSat)
				}
			}

			if got.LockTime != tt.want.LockTime {
				t.Errorf("Check LockTime, got %v, want %v", got.LockTime, tt.want.LockTime)
			}

			if got.Txid != tt.want.Txid {
				t.Errorf("Check TxId, got %v, want %v", got.Txid, tt.want.Txid)
			}
		})
	}
}
