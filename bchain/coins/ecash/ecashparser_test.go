//go:build unittest

package ecash

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

func Test_GetAddrDescFromAddress(t *testing.T) {
	mainParserCashAddr, mainParserLegacy, testParserCashAddr, _ := setupParsers(t)
	tests := []struct {
		name      string
		parser    *ECashParser
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
			addresses: []string{"ectest:qp86jfla8084048rckpv85ht90falr050s59h7rejp"},
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
			name:      "main-P2PKH-1",
			parser:    mainParserCashAddr,
			addresses: []string{"ecash:qqxgjelx8qk85t9xfk8g2zlunxmhxms6p5dtfghs9r"},
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
			addresses: []string{"ecash:pzy0wuj9pjps5v8dmlwq32fatu4wrgcwzuyf5lgn3q"},
			hex:       "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.parser.GetAddrDescFromAddress(tt.addresses[0])
			if (err != nil) != tt.wantErr {
				t.Errorf("%v", tt.addresses[0])
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
		parser     *ECashParser
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
			addresses:  []string{"ectest:qp86jfla8084048rckpv85ht90falr050s59h7rejp"},
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
			addresses:  []string{"ecash:qqxgjelx8qk85t9xfk8g2zlunxmhxms6p5dtfghs9r"},
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
			addresses:  []string{"ecash:pzy0wuj9pjps5v8dmlwq32fatu4wrgcwzuyf5lgn3q"},
			searchable: true,
			hex:        "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2PK",
			parser:     mainParserCashAddr,
			addresses:  []string{"ecash:qqr95pwp0w5jqnh9vcjl4qm4x45atr0er587pw66cr"},
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

func setupParsers(t *testing.T) (mainParserCashAddr, mainParserLegacy, testParserCashAddr, testParserLegacy *ECashParser) {
	parser1, err := NewECashParser(GetChainParams("main"), &btc.Configuration{AddressFormat: "cashaddr"})
	if err != nil {
		t.Fatalf("NewECashParser() error = %v", err)
	}
	parser2, err := NewECashParser(GetChainParams("main"), &btc.Configuration{AddressFormat: "legacy"})
	if err != nil {
		t.Fatalf("NewECashParser() error = %v", err)
	}
	parser3, err := NewECashParser(GetChainParams("test"), &btc.Configuration{AddressFormat: "cashaddr"})
	if err != nil {
		t.Fatalf("NewECashParser() error = %v", err)
	}
	parser4, err := NewECashParser(GetChainParams("test"), &btc.Configuration{AddressFormat: "legacy"})
	if err != nil {
		t.Fatalf("NewECashParser() error = %v", err)
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
						"ecash:pps5f4tu3tl5sjfvnhaeznsjpvst44eddu3yvcmmzj",
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
						"ectest:prxkdrtcrm8xqrh6fvjqfhy3l5nt3w9wmq7a4ujkec",
					},
				},
			},
			{
				ValueSat: *big.NewInt(920081157),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a87",
					Addresses: []string{
						"ectest:pqjxv4dah42v0erh6r4zxa0gdcxm9w8cpg55qt4qtq",
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
		parser   *ECashParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "ecash-1",
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unpackTx() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("unpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
