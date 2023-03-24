//go:build unittest

package dash

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

type testBlock struct {
	size int
	time int64
	txs  []string
}

var testParseBlockTxs = map[int]testBlock{
	500001: {
		size: 6521,
		time: 1468043164,
		txs: []string{
			"6d45c761892eb07b7651140aa42901b03cb501a82585fb360ff8f155d46727b0",
			"5f6cbefca8a48cccd40805013e5f6c602e0d35c3511ccb7a2ae25e51dd95d38f",
			"0eb37faa8b2f24c68c8e4a3009a051caded005d5f13f0dc216ff6422256f6b7b",
			"2565cfacb7f8cbc73dc1053b06ca527d9798bf6bf29a778cb5924a17dd167a39",
			"85f93911e8a4d8d9bbf3de009a666ed594d62aa41a34a9e3763058067e64f084",
			"46712f0f32a392c71df443798e120d0f40eb93063631a992b3dedd4d4afc04e4",
			"1790efaa05caad7ab546ef479041c3cb1cb9bca7b9cc7992566d2e2344701167",
		},
	},
	// last block without special transactions, valid for bitcoin parser
	1028159: {
		size: 8608,
		time: 1551246608,
		txs: []string{
			"a800f5b2dde5d48bda08d9d6fc5647c41cec902ce690a5a2be0665e6acf77c35",
			"981d6668e65b70fcd97ddd68319f3c5e5163e510cc0ed479be5667bf1782f036",
			"b9fd19d37ec97d038da2ccad9414ef311275d5fd3762bdec3e76f535e2295f4c",
			"1b4051d02c9919ef8d482cadf6ca2002442d9436b444923cd295fe56009ec52b",
			"6f1ec8472624b8e7481024ee8b228086b9b32606790e94f161589d3fe2b3a826",
			"b12c512803d733f3f7afce846e18c6a46c713533cdb18a13392cbda88866523c",
			"69e6d67946ed660b440c8e457933ae594ce60acdbe17ded091ce0ac6f41ed186",
			"755c3b7cad9b569def3f69f897da2ee7732ee2e0a165965512680b4fe9086e12",
			"63de772ff400789c2c3ad9be653817bebf92551e139d80c8e735bb0610865500",
			"1566bd9bb2413c63412a13d16c7017814363f668d8b3bfe66a5478734e73f010",
			"d7f441b0abca7df6530a0620661c839244bb6f26e1b4a53b783fb3acc1f5f42a",
			"ec7762d0e02a87e311b128662db8ef4161dcc9d9f2831250c7366eed98fc744b",
			"b977669bfc0ac3b9ca9de7512fda564a69fe49dde8a286fcb7ea99147db54b5f",
		},
	},
	// first block with special transactions, invalid for bitcoin parser
	// 1028160: {
	// 	size: 2347,
	// 	time: 1551246710,
	// 	txs: []string{
	// 		"71d6975e3b79b52baf26c3269896a34f3bedfb04561c692ffa31f64dada1f9c4",
	// 		"ed732a404cdfd4e0475a7a016200b7eef191f2c9de0ffdef8a20091c0499299c",
	// 		"99d0613f82ea1f928bbc98318665adbdf5b40d206bd487fe77d542e86c903f55",
	// 		"05cbf334a563468d0e378c56b43fb5254ee9c0e35ca8fab5cb242ebd825ae97b",
	// 		"ee91fae7be36b3b81bc60992e904e1ae91e7dffdd5751ccaef557ba62ea80a4f",
	// 	},
	// },
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
	p := NewDashParser(GetChainParams("main"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs {
		b := helperLoadBlock(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Errorf("ParseBlock() error %v", err)
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

var (
	testTx1 = bchain.Tx{
		Blocktime:     1551246710,
		Confirmations: 0,
		Hex:           "0100000001f85264d11a747bdba77d411e5e4a3d35e3aeb5843b34a95234a2121ac65496bd000000006b483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190ffffffff02ec3f8a2a010000001976a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac3116491d000000001976a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac00000000",
		LockTime:      0,
		Time:          1551246710,
		Txid:          "ed732a404cdfd4e0475a7a016200b7eef191f2c9de0ffdef8a20091c0499299c",
		Version:       1,
		Vin: []bchain.Vin{
			{
				Txid: "bd9654c61a12a23452a9343b84b5aee3353d4a5e1e417da7db7b741ad16452f8",
				Vout: 0,
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190",
				},
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				N: 0,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"XkycBX1ykVXXs92pAi6ZQwZPEre9kSHHKH"},
					Hex:       "76a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac",
				},
				ValueSat: *big.NewInt(5008670700),
			},
			{
				N: 1,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"Xm1R9thKBm2EZKZevXsmMX4DVwQQuTohZu"},
					Hex:       "76a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac",
				},
				ValueSat: *big.NewInt(491329073),
			},
		},
	}
	testTxPacked1 = "0a20ed732a404cdfd4e0475a7a016200b7eef191f2c9de0ffdef8a20091c0499299c12e2010100000001f85264d11a747bdba77d411e5e4a3d35e3aeb5843b34a95234a2121ac65496bd000000006b483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190ffffffff02ec3f8a2a010000001976a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac3116491d000000001976a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac0000000018f6cad8e30528c0e03e3295011220bd9654c61a12a23452a9343b84b5aee3353d4a5e1e417da7db7b741ad16452f8226b483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c219028ffffffff0f3a460a05012a8a3fec1a1976a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac2222586b7963425831796b565858733932704169365a51775a50457265396b5348484b483a470a041d49163110011a1976a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac2222586d31523974684b426d32455a4b5a657658736d4d5834445677515175546f685a754001"

	testTx2 = bchain.Tx{
		Blocktime:     1551246710,
		Confirmations: 0,
		Hex:           "03000500010000000000000000000000000000000000000000000000000000000000000000ffffffff170340b00f1291af3c09542bc8349901000000002f4e614effffffff024181f809000000001976a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac3581f809000000001976a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac0000000026010040b00f000000000000000000000000000000000000000000000000000000000000000000",
		LockTime:      0,
		Time:          1551246710,
		Txid:          "71d6975e3b79b52baf26c3269896a34f3bedfb04561c692ffa31f64dada1f9c4",
		Version:       3,
		Vin: []bchain.Vin{
			{
				Coinbase: "0340b00f1291af3c09542bc8349901000000002f4e614e",
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				N: 0,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"XkNPrBSJtrHZUvUqb3JF4g5rMB3uzaJfEL"},
					Hex:       "76a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac",
				},
				ValueSat: *big.NewInt(167280961),
			},
			{
				N: 1,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"XbswPXhcLqm5AN5gwcTTyiUGSP2YndWwk9"},
					Hex:       "76a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac",
				},
				ValueSat: *big.NewInt(167280949),
			},
		},
	}

	testTxPacked2 = "0a2071d6975e3b79b52baf26c3269896a34f3bedfb04561c692ffa31f64dada1f9c412b50103000500010000000000000000000000000000000000000000000000000000000000000000ffffffff170340b00f1291af3c09542bc8349901000000002f4e614effffffff024181f809000000001976a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac3581f809000000001976a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac0000000026010040b00f00000000000000000000000000000000000000000000000000000000000000000018f6cad8e30528c0e03e32360a2e3033343062303066313239316166336330393534326263383334393930313030303030303030326634653631346528ffffffff0f3a450a0409f881411a1976a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac2222586b4e507242534a7472485a5576557162334a46346735724d4233757a614a66454c3a470a0409f8813510011a1976a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac222258627377505868634c716d35414e35677763545479695547535032596e6457776b394003"
)

func TestBaseParser_ParseTxFromJson(t *testing.T) {
	p := NewDashParser(GetChainParams("main"), &btc.Configuration{})
	tests := []struct {
		name    string
		msg     string
		want    *bchain.Tx
		wantErr bool
	}{
		{
			name: "normal tx",
			msg:  `{"hex":"0100000001f85264d11a747bdba77d411e5e4a3d35e3aeb5843b34a95234a2121ac65496bd000000006b483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190ffffffff02ec3f8a2a010000001976a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac3116491d000000001976a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac00000000","txid":"ed732a404cdfd4e0475a7a016200b7eef191f2c9de0ffdef8a20091c0499299c","size":226,"version":1,"type":0,"locktime":0,"vin":[{"txid":"bd9654c61a12a23452a9343b84b5aee3353d4a5e1e417da7db7b741ad16452f8","vout":0,"scriptSig":{"asm":"3045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6[ALL]03093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190","hex":"483045022100dfa158fbd9773fab4f6f329c807e040af0c3a40967cbe01667169b914ed5ad960220061c5876364caa3e3c9c990ad2b4cc8b1a53d4f954dbda8434b0e67cc8348ff6012103093865e1e132b33a2a5ed01c79d2edba3473826a66cb26b8311bfa42749c2190"},"value":55.00000000,"valueSat":5500000000,"address":"Xgcv4bKAXaWf5sjX9KR49L98jeMwNgeXWh","sequence":4294967295}],"vout":[{"value":50.08670700,"valueSat":5008670700,"n":0,"scriptPubKey":{"asm":"OP_DUPOP_HASH16070dcef2a22575d7a8f0779fb1d6cdd48135bd227OP_EQUALVERIFYOP_CHECKSIG","hex":"76a91470dcef2a22575d7a8f0779fb1d6cdd48135bd22788ac","reqSigs":1,"type":"pubkeyhash","addresses":["XkycBX1ykVXXs92pAi6ZQwZPEre9kSHHKH"]}},{"value":4.91329073,"valueSat":491329073,"n":1,"scriptPubKey":{"asm":"OP_DUPOP_HASH16071348f7780e955a2a60eba17ecc4c826ebc23a98OP_EQUALVERIFYOP_CHECKSIG","hex":"76a91471348f7780e955a2a60eba17ecc4c826ebc23a9888ac","reqSigs":1,"type":"pubkeyhash","addresses":["Xm1R9thKBm2EZKZevXsmMX4DVwQQuTohZu"]}}],"blockhash":"000000000000002099caaf1a877911d99a5980ede9b981280eecb291afedf87b","height":1028160,"confirmations":0,"time":1551246710,"blocktime":1551246710,"instantlock":false}`,
			want: &testTx1,
		},
		{
			name: "special tx - DIP2",
			msg:  `{"hex":"03000500010000000000000000000000000000000000000000000000000000000000000000ffffffff170340b00f1291af3c09542bc8349901000000002f4e614effffffff024181f809000000001976a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac3581f809000000001976a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac0000000026010040b00f000000000000000000000000000000000000000000000000000000000000000000","txid":"71d6975e3b79b52baf26c3269896a34f3bedfb04561c692ffa31f64dada1f9c4","size":181,"version":3,"type":5,"locktime":0,"vin":[{"coinbase":"0340b00f1291af3c09542bc8349901000000002f4e614e","sequence":4294967295}],"vout":[{"value":1.67280961,"valueSat":167280961,"n":0,"scriptPubKey":{"asm":"OP_DUPOP_HASH1606a341485a9444b35dc9cb90d24e7483de7d37e00OP_EQUALVERIFYOP_CHECKSIG","hex":"76a9146a341485a9444b35dc9cb90d24e7483de7d37e0088ac","reqSigs":1,"type":"pubkeyhash","addresses":["XkNPrBSJtrHZUvUqb3JF4g5rMB3uzaJfEL"]}},{"value":1.67280949,"valueSat":167280949,"n":1,"scriptPubKey":{"asm":"OP_DUPOP_HASH1600d1156f6026bf975ea3553b03fb534d0959c294cOP_EQUALVERIFYOP_CHECKSIG","hex":"76a9140d1156f6026bf975ea3553b03fb534d0959c294c88ac","reqSigs":1,"type":"pubkeyhash","addresses":["XbswPXhcLqm5AN5gwcTTyiUGSP2YndWwk9"]}}],"extraPayloadSize":38,"extraPayload":"010040b00f000000000000000000000000000000000000000000000000000000000000000000","cbTx":{"version":1,"height":1028160,"merkleRootMNList":"0000000000000000000000000000000000000000000000000000000000000000"},"blockhash":"000000000000002099caaf1a877911d99a5980ede9b981280eecb291afedf87b","height":1028160,"confirmations":0,"time":1551246710,"blocktime":1551246710,"instantlock":false}`,
			want: &testTx2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseTxFromJson([]byte(tt.msg))
			if (err != nil) != tt.wantErr {
				t.Errorf("DashParser.ParseTxFromJson() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DashParser.ParseTxFromJson() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *DashParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "dash-1",
			args: args{
				tx:        testTx1,
				height:    1028160,
				blockTime: 1551246710,
				parser:    NewDashParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "dash-2",
			args: args{
				tx:        testTx2,
				height:    1028160,
				blockTime: 1551246710,
				parser:    NewDashParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *DashParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "dash-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewDashParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   1028160,
			wantErr: false,
		},
		{
			name: "dash-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewDashParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   1028160,
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
