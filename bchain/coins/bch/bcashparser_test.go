//go:build unittest

package bch

import (
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"strings"
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

func Test_GetAddrDescFromAddress(t *testing.T) {
	mainParserCashAddr, mainParserLegacy, testParserCashAddr, _ := setupParsers(t)
	tests := []struct {
		name      string
		parser    *BCashParser
		addresses []string
		hex       string
		wantErr   bool
	}{
		{
			name:      "test-P2PKH-0",
			parser:    testParserCashAddr,
			addresses: []string{"mnnAKPTSrWjgoi3uEYaQkHA1QEC5btFeBr"},
			hex:       "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			wantErr:   false,
		},
		{
			name:      "test-P2PKH-1",
			parser:    testParserCashAddr,
			addresses: []string{"bchtest:qp86jfla8084048rckpv85ht90falr050s03ejaesm"},
			hex:       "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			wantErr:   false,
		},
		{
			name:      "main-P2PKH-0",
			parser:    mainParserLegacy,
			addresses: []string{"129HiRqekqPVucKy2M8zsqvafGgKypciPp"},
			hex:       "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:   false,
		},
		{
			name:      "main-P2PKH-0",
			parser:    mainParserCashAddr,
			addresses: []string{"bitcoincash:qqxgjelx8qk85t9xfk8g2zlunxmhxms6p55xarv2r5"},
			hex:       "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:   false,
		},
		{
			name:      "main-P2SH-0",
			parser:    mainParserCashAddr,
			addresses: []string{"3EBEFWPtDYWCNszQ7etoqtWmmygccayLiH"},
			hex:       "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:   false,
		},
		{
			name:      "main-P2SH-1",
			parser:    mainParserLegacy,
			addresses: []string{"bitcoincash:pzy0wuj9pjps5v8dmlwq32fatu4wrgcwzuayq5nfhh"},
			hex:       "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.parser.GetAddrDescFromAddress(tt.addresses[0])
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.hex) {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.hex)
			}
		})
	}
}

func Test_GetAddressesFromAddrDesc(t *testing.T) {
	mainParserCashAddr, mainParserLegacy, testParserCashAddr, testParserLegacy := setupParsers(t)
	tests := []struct {
		name       string
		parser     *BCashParser
		addresses  []string
		searchable bool
		hex        string
		wantErr    bool
	}{
		{
			name:       "test-P2PKH-0",
			parser:     testParserLegacy,
			addresses:  []string{"mnnAKPTSrWjgoi3uEYaQkHA1QEC5btFeBr"},
			searchable: true,
			hex:        "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",

			wantErr: false,
		},
		{
			name:       "test-P2PKH-1",
			parser:     testParserCashAddr,
			addresses:  []string{"bchtest:qp86jfla8084048rckpv85ht90falr050s03ejaesm"},
			searchable: true,
			hex:        "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2PKH-0",
			parser:     mainParserLegacy,
			addresses:  []string{"129HiRqekqPVucKy2M8zsqvafGgKypciPp"},
			searchable: true,
			hex:        "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2PKH-0",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:qqxgjelx8qk85t9xfk8g2zlunxmhxms6p55xarv2r5"},
			searchable: true,
			hex:        "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2SH-0",
			parser:     mainParserLegacy,
			addresses:  []string{"3EBEFWPtDYWCNszQ7etoqtWmmygccayLiH"},
			searchable: true,
			hex:        "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2SH-1",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:pzy0wuj9pjps5v8dmlwq32fatu4wrgcwzuayq5nfhh"},
			searchable: true,
			hex:        "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2PK",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:qqr95pwp0w5jqnh9vcjl4qm4x45atr0er57n49pq75"},
			searchable: true,
			hex:        "2103db3c3977c5165058bf38c46f72d32f4e872112dbafc13083a948676165cd1603ac",
			wantErr:    false,
		},
		{
			name:       "OP_RETURN ascii",
			parser:     mainParserCashAddr,
			addresses:  []string{"OP_RETURN (ahoj)"},
			searchable: false,
			hex:        "6a0461686f6a",
			wantErr:    false,
		},
		{
			name:       "OP_RETURN hex",
			parser:     mainParserCashAddr,
			addresses:  []string{"OP_RETURN 2020f1686f6a20"},
			searchable: false,
			hex:        "6a072020f1686f6a20",
			wantErr:    false,
		},
		{
			name:       "empty",
			parser:     mainParserCashAddr,
			addresses:  []string{},
			searchable: false,
			hex:        "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.hex)
			got, got2, err := tt.parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.addresses) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got, tt.addresses)
			}
			if !reflect.DeepEqual(got2, tt.searchable) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got2, tt.searchable)
			}
		})
	}
}

var (
	testTx1, testTx2 bchain.Tx
	testTxPacked1    = "0001e2408ba8d7af5401000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700"
	testTxPacked2    = "0007c91a899ab7da6a010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000"
)

func setupParsers(t *testing.T) (mainParserCashAddr, mainParserLegacy, testParserCashAddr, testParserLegacy *BCashParser) {
	parser1, err := NewBCashParser(GetChainParams("main"), &btc.Configuration{AddressFormat: "cashaddr"})
	if err != nil {
		t.Fatalf("NewBCashParser() error = %v", err)
	}
	parser2, err := NewBCashParser(GetChainParams("main"), &btc.Configuration{AddressFormat: "legacy"})
	if err != nil {
		t.Fatalf("NewBCashParser() error = %v", err)
	}
	parser3, err := NewBCashParser(GetChainParams("test"), &btc.Configuration{AddressFormat: "cashaddr"})
	if err != nil {
		t.Fatalf("NewBCashParser() error = %v", err)
	}
	parser4, err := NewBCashParser(GetChainParams("test"), &btc.Configuration{AddressFormat: "legacy"})
	if err != nil {
		t.Fatalf("NewBCashParser() error = %v", err)
	}
	return parser1, parser2, parser3, parser4
}

func init() {

	testTx1 = bchain.Tx{
		Hex:       "01000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700",
		Blocktime: 1519053802,
		Txid:      "056e3d82e5ffd0e915fb9b62797d76263508c34fe3e5dbed30dd3e943930f204",
		LockTime:  512115,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80",
				},
				Txid:     "425fed43ba74e9205875eb934d5bcf7bf338f146f70d4002d94bf5cbc9229a7f",
				Vout:     4,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(38812),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9146144d57c8aff48492c9dfb914e120b20bad72d6f87",
					Addresses: []string{
						"bitcoincash:pps5f4tu3tl5sjfvnhaeznsjpvst44eddugfcnqpy9",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000",
		Blocktime: 1235678901,
		Txid:      "474e6795760ebe81cb4023dc227e5a0efe340e1771c89a0035276361ed733de7",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "160014550da1f5d25a9dae2eafd6902b4194c4c6500af6",
				},
				Txid:     "c13e32a4428e31f85d7aee4ec7344504b12e72aaffcbde0160200d2ac7f0649d",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(10000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914cd668d781ece600efa4b2404dc91fd26b8b8aed887",
					Addresses: []string{
						"bchtest:prxkdrtcrm8xqrh6fvjqfhy3l5nt3w9wmq9fmsvkmz",
					},
				},
			},
			{
				ValueSat: *big.NewInt(920081157),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a87",
					Addresses: []string{
						"bchtest:pqjxv4dah42v0erh6r4zxa0gdcxm9w8cpg0qw8tqf6",
					},
				},
			},
		},
	}
}

func Test_UnpackTx(t *testing.T) {
	mainParser, _, testParser, _ := setupParsers(t)

	type args struct {
		packedTx string
		parser   *BCashParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "bcash-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   mainParser,
			},
			want:    &testTx1,
			want1:   123456,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				packedTx: testTxPacked2,
				parser:   testParser,
			},
			want:    &testTx2,
			want1:   510234,
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
			// ignore witness unpacking
			for i := range got.Vin {
				got.Vin[i].Witness = nil
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

// verbose getrawtransaction of mainnet tx 7cb443cd...9338 (block 968742) as returned by BCHN:
// vout 5 carries a CashToken whose prefix is missing from scriptPubKey.hex but present in hex
const testTxCashTokenJSON = `{"txid":"7cb443cd358ca4073cc7d00acdedca411c8e2b66f89046898af51da2ee799338","version":2,"size":1221,"locktime":0,"vin":[{"txid":"8c99d0d9c0b5b39c45252a6662bae03a7609b37ada3afb38cee3e6622c9d1952","vout":0,"sequence":4294967295},{"txid":"8c99d0d9c0b5b39c45252a6662bae03a7609b37ada3afb38cee3e6622c9d1952","vout":1,"sequence":4294967295},{"txid":"8c99d0d9c0b5b39c45252a6662bae03a7609b37ada3afb38cee3e6622c9d1952","vout":2,"sequence":4294967295},{"txid":"8c99d0d9c0b5b39c45252a6662bae03a7609b37ada3afb38cee3e6622c9d1952","vout":3,"sequence":4294967295},{"txid":"8c99d0d9c0b5b39c45252a6662bae03a7609b37ada3afb38cee3e6622c9d1952","vout":4,"sequence":4294967295},{"txid":"3ff3dfafa5dc208c18b3d91979e7c704e1f2c89908310b430f7e247e5107d75d","vout":2,"sequence":4294967295}],"vout":[{"value":240.53007729,"n":0,"scriptPubKey":{"asm":"OP_HASH256 e76e18a75e8cf6aacaacae3d0b07324796df232d8054875d7a7fd4a199beff43 OP_EQUAL","hex":"aa20e76e18a75e8cf6aacaacae3d0b07324796df232d8054875d7a7fd4a199beff4387","reqSigs":1,"type":"scripthash","addresses":["bitcoincash:p0nkux98t6x0d2k24jhr6zc8xfredher9kq9fp6a0flafgvehml5xqswpc96w"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"5278989"}},{"value":134.81564445,"n":1,"scriptPubKey":{"asm":"OP_HASH256 1c74d78d275e27604de2ba5d416bac0b4e021c1da30a4ab9e0df5f7b469f2c0c OP_EQUAL","hex":"aa201c74d78d275e27604de2ba5d416bac0b4e021c1da30a4ab9e0df5f7b469f2c0c87","reqSigs":1,"type":"scripthash","addresses":["bitcoincash:pvw8f4udya0zwczdu2a96stt4s95uqsurk3s5j4eur04776xnukqcu7lhg7xu"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"2958841"}},{"value":57.83630628,"n":2,"scriptPubKey":{"asm":"OP_HASH256 a55ed15bc3fc73e1ff0ce38733da7cf2757d7f2bf929f6cbb95294a8aed0f5e2 OP_EQUAL","hex":"aa20a55ed15bc3fc73e1ff0ce38733da7cf2757d7f2bf929f6cbb95294a8aed0f5e287","reqSigs":1,"type":"scripthash","addresses":["bitcoincash:pwj4a52mc0788c0lpn3cwv760ne82ltl90ujnakth9fff29w6r67yqak8f6ge"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"1269352"}},{"value":23.64034221,"n":3,"scriptPubKey":{"asm":"OP_HASH256 472d1974998e751377bb0bfd16d44750d84b4c44fde3db389ec3eef2c762bcf2 OP_EQUAL","hex":"aa20472d1974998e751377bb0bfd16d44750d84b4c44fde3db389ec3eef2c762bcf287","reqSigs":1,"type":"scripthash","addresses":["bitcoincash:pdrj6xt5nx882ymhhv9l69k5gagdsj6vgn778kecnmp7auk8v270yhwewdl3y"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"518842"}},{"value":16.97119661,"n":4,"scriptPubKey":{"asm":"OP_HASH256 01cf30e5780e948b7e2eb807fe418d23fca3fb0652646d3c9e3f1735b36f50bc OP_EQUAL","hex":"aa2001cf30e5780e948b7e2eb807fe418d23fca3fb0652646d3c9e3f1735b36f50bc87","reqSigs":1,"type":"scripthash","addresses":["bitcoincash:pvqu7v890q8ffzm796uq0ljp353leglmqefxgmfuncl3wddndagtck644hjl6"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"372473"}},{"value":8e-06,"n":5,"scriptPubKey":{"asm":"OP_DUP OP_HASH160 f6ba3624436026f9cece52df1023d541241702e5 OP_EQUALVERIFY OP_CHECKSIG","hex":"76a914f6ba3624436026f9cece52df1023d541241702e588ac","reqSigs":1,"type":"pubkeyhash","addresses":["bitcoincash:qrmt5d3ygdszd7wweefd7ypr64qjg9czu52chgdm3z"]},"tokenData":{"category":"2469acc5afa4b10cb5b5c04afb89c3a3ffd61c5da9c01e26d00951cae2a02544","amount":"13765"}},{"value":0.18679189,"n":6,"scriptPubKey":{"asm":"OP_DUP OP_HASH160 f6ba3624436026f9cece52df1023d541241702e5 OP_EQUALVERIFY OP_CHECKSIG","hex":"76a914f6ba3624436026f9cece52df1023d541241702e588ac","reqSigs":1,"type":"pubkeyhash","addresses":["bitcoincash:qrmt5d3ygdszd7wweefd7ypr64qjg9czu52chgdm3z"]}}],"hex":"020000000652199d2c62e6e3ce38fb3ada7ab309763ae0ba62662a25459cb3b5c0d9d0998c000000004544746376a91412effae58abba049d5402634a709e1e6a15c048088ac67c0d1c0ce88c25288c0cdc0c788c0c6c0d095c0c6c0cc9490539502e80396c0cc7c94c0d3957ca268ffffffff52199d2c62e6e3ce38fb3ada7ab309763ae0ba62662a25459cb3b5c0d9d0998c010000004544746376a914ab0de1203d427481b247faa0b434af9fd49a3ee888ac67c0d1c0ce88c25288c0cdc0c788c0c6c0d095c0c6c0cc9490539502e80396c0cc7c94c0d3957ca268ffffffff52199d2c62e6e3ce38fb3ada7ab309763ae0ba62662a25459cb3b5c0d9d0998c020000004544746376a9142ceb9595368b37469a883092dfa02b79f31ec53888ac67c0d1c0ce88c25288c0cdc0c788c0c6c0d095c0c6c0cc9490539502e80396c0cc7c94c0d3957ca268ffffffff52199d2c62e6e3ce38fb3ada7ab309763ae0ba62662a25459cb3b5c0d9d0998c030000004544746376a914b503af5d5d0e63cfe5e128bb8dfdfd94f2fb34df88ac67c0d1c0ce88c25288c0cdc0c788c0c6c0d095c0c6c0cc9490539502e80396c0cc7c94c0d3957ca268ffffffff52199d2c62e6e3ce38fb3ada7ab309763ae0ba62662a25459cb3b5c0d9d0998c040000004544746376a914c004928ff41d49c8649f098fdde048d84afe5f8f88ac67c0d1c0ce88c25288c0cdc0c788c0c6c0d095c0c6c0cc9490539502e80396c0cc7c94c0d3957ca268ffffffff5dd707517e247e0f430b310899c8f2e104c7e77919d9b3188c20dca5afdff33f02000000644190299e0cb9cd7f7cf720761355172fceb749fd688354141f96ea483925922b6d9ce761f95a93e33c70d1836de3e14ee9b64c9ab6963c03479eb066d54cbe395d412103edb0801ae8733876c6303764d7e625fc74a4e612fbe59d633eb2db1d034c83edffffffff0771c5ab99050000004aef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410fe0d8d5000aa20e76e18a75e8cf6aacaacae3d0b07324796df232d8054875d7a7fd4a199beff43871d599023030000004aef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410fef9252d00aa201c74d78d275e27604de2ba5d416bac0b4e021c1da30a4ab9e0df5f7b469f2c0c872433bb58010000004aef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410fe685e1300aa20a55ed15bc3fc73e1ff0ce38733da7cf2757d7f2bf929f6cbb95294a8aed0f5e287ad4ce88c000000004aef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410febaea0700aa20472d1974998e751377bb0bfd16d44750d84b4c44fde3db389ec3eef2c762bcf287adfd2765000000004aef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410fef9ae0500aa2001cf30e5780e948b7e2eb807fe418d23fca3fb0652646d3c9e3f1735b36f50bc8720030000000000003eef4425a0e2ca5109d0261ec0a95d1cd6ffa3c389fb4ac0b5b50cb1a4afc5ac692410fdc53576a914f6ba3624436026f9cece52df1023d541241702e588ac95051d01000000001976a914f6ba3624436026f9cece52df1023d541241702e588ac00000000"}`

func Test_ParseTxFromJson_CashToken(t *testing.T) {
	parser, _, _, _ := setupParsers(t)

	got, err := parser.ParseTxFromJson([]byte(testTxCashTokenJSON))
	if err != nil {
		t.Fatalf("ParseTxFromJson() error = %v", err)
	}
	raw, _ := hex.DecodeString(got.Hex)
	want, err := parser.ParseTx(raw)
	if err != nil {
		t.Fatalf("ParseTx() error = %v", err)
	}
	if len(got.Vout) != 7 || len(want.Vout) != 7 {
		t.Fatalf("unexpected vout count json=%d hex=%d", len(got.Vout), len(want.Vout))
	}
	for i := range got.Vout {
		if !reflect.DeepEqual(got.Vout[i].ScriptPubKey, want.Vout[i].ScriptPubKey) {
			t.Errorf("vout %d ScriptPubKey = %+v, want %+v", i, got.Vout[i].ScriptPubKey, want.Vout[i].ScriptPubKey)
		}
	}
	token := got.Vout[5].ScriptPubKey.Hex
	if !strings.HasPrefix(token, "ef") || !strings.HasSuffix(token, "76a914f6ba3624436026f9cece52df1023d541241702e588ac") {
		t.Errorf("vout 5 hex = %v, want CashToken prefix restored", token)
	}
	if got.Vout[6].ScriptPubKey.Hex != "76a914f6ba3624436026f9cece52df1023d541241702e588ac" {
		t.Errorf("vout 6 hex = %v, want plain P2PKH unchanged", got.Vout[6].ScriptPubKey.Hex)
	}
	// the token output must resolve exactly as the index does: no address
	addrDesc, err := parser.GetAddrDescFromVout(&got.Vout[5])
	if err != nil {
		t.Fatalf("GetAddrDescFromVout() error = %v", err)
	}
	addresses, isAddress, err := parser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil || len(addresses) != 0 || isAddress {
		t.Errorf("token vout resolved to %v, %v, %v; want no address", addresses, isAddress, err)
	}
	if got.Vout[5].ValueSat.Int64() != 800 {
		t.Errorf("vout 5 value = %v, want 800", got.Vout[5].ValueSat.String())
	}
}
