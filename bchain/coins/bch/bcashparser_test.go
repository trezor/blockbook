//go:build unittest

package bch

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
)

func hexToBytes(h string) []byte {
	b, _ := hex.DecodeString(h)
	return b
}

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
		{
			name:      "main-P2SH32-0",
			parser:    mainParserCashAddr,
			addresses: []string{"bitcoincash:p0wuns40rqxeac6vrg74jwvcujxx3u0mdzcrzt7ywg4kkvyg7u7g6ukpc9cf2"},
			hex:       "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:   false,
		},
		{
			name:      "test-P2SH32-1",
			parser:    testParserCashAddr,
			addresses: []string{"bchtest:p0wuns40rqxeac6vrg74jwvcujxx3u0mdzcrzt7ywg4kkvyg7u7g6l3sxatuc"},
			hex:       "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
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
			name:       "main-P2PKH-CastTokens-0",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:qqxgjelx8qk85t9xfk8g2zlunxmhxms6p55xarv2r5"},
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffff76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
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
			name:       "main-P2SH-CashTokens-0",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:pzy0wuj9pjps5v8dmlwq32fatu4wrgcwzuayq5nfhh"},
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffffa91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2SH32-0",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:p0wuns40rqxeac6vrg74jwvcujxx3u0mdzcrzt7ywg4kkvyg7u7g6ukpc9cf2"},
			searchable: true,
			hex:        "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:    false,
		},
		{
			name:       "main-P2SH32-CashTokens-0",
			parser:     mainParserCashAddr,
			addresses:  []string{"bitcoincash:p0wuns40rqxeac6vrg74jwvcujxx3u0mdzcrzt7ywg4kkvyg7u7g6ukpc9cf2"},
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffffaa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:    false,
		},
		{
			name:       "test-P2SH32-0",
			parser:     testParserCashAddr,
			addresses:  []string{"bchtest:p0wuns40rqxeac6vrg74jwvcujxx3u0mdzcrzt7ywg4kkvyg7u7g6l3sxatuc"},
			searchable: true,
			hex:        "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
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
			name:       "OP_RETURN BCMR",
			parser:     mainParserCashAddr,
			addresses:  []string{"OP_RETURN (BCMR 13f93a5fb1df5cbe30382368b39992287add714a5b682c0be02d6849536e5d6c gist.githubusercontent.com/alpsy05/55c602d49e44842fa41dfcabed237a15/raw/148812af3c25fb1cf91d3fbae4fc9e5cd1d501e5/Cash)"},
			searchable: false,
			hex:        "6a0442434d522013f93a5fb1df5cbe30382368b39992287add714a5b682c0be02d6849536e5d6c4c75676973742e67697468756275736572636f6e74656e742e636f6d2f616c70737930352f35356336303264343965343438343266613431646663616265643233376131352f7261772f313438383132616633633235666231636639316433666261653466633965356364316435303165352f43617368",
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

func Test_GetAddrDescFromVout(t *testing.T) {
	mainParserCashAddr, mainParserLegacy, testParserCashAddr, testParserLegacy := setupParsers(t)
	tests := []struct {
		name       string
		parser     *BCashParser
		wantHex    string
		searchable bool
		hex        string
		wantErr    bool
	}{
		{
			name:       "test-P2PKH-0",
			parser:     testParserLegacy,
			wantHex:    "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			searchable: true,
			hex:        "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",

			wantErr: false,
		},
		{
			name:       "test-P2PKH-1",
			parser:     testParserCashAddr,
			wantHex:    "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			searchable: true,
			hex:        "76a9144fa927fd3bcf57d4e3c582c3d2eb2bd3df8df47c88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2PKH-0",
			parser:     mainParserLegacy,
			wantHex:    "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			searchable: true,
			hex:        "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2PKH-0",
			parser:     mainParserCashAddr,
			wantHex:    "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			searchable: true,
			hex:        "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2PKH-CastTokens-0",
			parser:     mainParserCashAddr,
			wantHex:    "76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffff76a9140c8967e6382c7a2ca64d8e850bfc99b7736e1a0d88ac",
			wantErr:    false,
		},
		{
			name:       "main-P2SH-0",
			parser:     mainParserLegacy,
			wantHex:    "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			searchable: true,
			hex:        "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2SH-1",
			parser:     mainParserCashAddr,
			wantHex:    "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			searchable: true,
			hex:        "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2SH-CashTokens-0",
			parser:     mainParserCashAddr,
			wantHex:    "a91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffffa91488f772450c830a30eddfdc08a93d5f2ae1a30e1787",
			wantErr:    false,
		},
		{
			name:       "main-P2SH32-0",
			parser:     mainParserCashAddr,
			wantHex:    "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			searchable: true,
			hex:        "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:    false,
		},
		{
			name:       "main-P2SH32-CashTokens-0",
			parser:     mainParserCashAddr,
			wantHex:    "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			searchable: true,
			hex:        "efbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7201ccfdffffaa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:    false,
		},
		{
			name:       "test-P2SH32-0",
			parser:     testParserCashAddr,
			wantHex:    "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			searchable: true,
			hex:        "aa20ddc9c2af180d9ee34c1a3d593998e48c68f1fb68b0312fc4722b6b3088f73c8d87",
			wantErr:    false,
		},
		{
			name:       "main-P2PK",
			parser:     mainParserCashAddr,
			wantHex:    "76a914065a05c17ba9204ee56625fa83753569d58df91d88ac",
			searchable: true,
			hex:        "2103db3c3977c5165058bf38c46f72d32f4e872112dbafc13083a948676165cd1603ac",
			wantErr:    false,
		},
		{
			name:       "OP_RETURN ascii",
			parser:     mainParserCashAddr,
			wantHex:    "6a0461686f6a",
			searchable: false,
			hex:        "6a0461686f6a",
			wantErr:    false,
		},
		{
			name:       "OP_RETURN hex",
			parser:     mainParserCashAddr,
			wantHex:    "6a072020f1686f6a20",
			searchable: false,
			hex:        "6a072020f1686f6a20",
			wantErr:    false,
		},
		{
			name:       "empty",
			parser:     mainParserCashAddr,
			wantHex:    "",
			searchable: false,
			hex:        "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.parser.GetAddrDescFromVout(&bchain.Vout{
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: tt.hex,
				},
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotHex := hex.EncodeToString(got)
			if !reflect.DeepEqual(gotHex, tt.wantHex) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", gotHex, tt.wantHex)
			}
		})
	}
}

var (
	testTx1, testTx2, testTx3, testTx4 bchain.Tx
	testTxPacked1                      = "0001e2408ba8d7af5401000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700"
	testTxPacked2                      = "0007c91a899ab7da6a010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000"
	// p2sh32 and cashtokens
	testTxPacked3 = "000de5118d89add2200200000003269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b9000000000fd26011015ac9568b9371300a037130053e1000040ccffa8aa283534adf5178af0d325456761653648d13ab6544e8d14a26ed8d3d6bd34f28476c10f64937b0e9ee0c54739dbb57e999623a54615ceeb178c64b035004cd120561fd4a4dc7cde1b06f224ec70380e8b2718a04aa0ef249e27b287531e64ec392102d09db08af1ff4e8453919cc866a4be427d7bfe18f2c05e5444c196fcf6fd28185279009c635379827701409d537a54797bbbc0009d51ce0087916952ce0088c3539d00cc00c6a26900cd00c78800d100ce885279547f75815379587f77547f7581547a5c7f77547f758151cf567f77527f7581537a56807c52807e7b54807e7c54807e00cc00c6a26900cd00c78800d100ce8851d28851d151ce8852d10088c4539c7777677b519d00ce7b877768ffffffff269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b9001000000d2004ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868ffffffff269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b90020000006441a444684bf550d6242a5061dba25c64dddd3ee58f830e64f2aacb8e5abfec145923e972a3f104d3dac8bf93329251dc074b15f349a8306f4f484366b00720de6d41210245496ebbf3d90fa94a395a3b6cc6852fbc621ee4147bbac0580a59d803cae8f8ffffffff03e80300000000000045efc1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e20aa203b71bed23bf7e606d2f9178d85e15170d68c45a10a9e86db520ac08e3ef41f6a8730fd13000000000056ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0611015ac95680000e803a037130053e10000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a9875cb07900000000001976a914925e9bcecea0e9a129ede392a075da2ef03a22c188ac29230000"
	// cashtokens of all kinds and combinations
	testTxPacked4 = "000de5b98d89b2851402000000069eb9f37e89903bb6351d7d6aa667aade93d47231f5f6095d130453264e3999f400000000fd3b03524d370323aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f8872172c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0015279009c63c0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba2697c567f7581527901247f7781a2697c01207f77527f75810164957602e803a26900d0937c00cd00c78800d100ce8800d37ba26900cc02e8039d00d28851d17b8852d1788852d2008853d100876453d101207f75788791696854d1008855d1008755d17b879b6955d20088c4569c7777675279519c63c0009d51ce788852ce0088c3539d51cf00cf817c587f77547f7581767ba06900cd00c78800d100ce8800d300d0a26900cc02e8039d00d28851d18852d10088c4539c7777675279529c63c0009d51ce788852ce008851cf00cf8178587f77547f7581767ba269785c7f77547f75817600a06952cc529553967c950400e1f5059653d3767ba1697602e803a269760370e531a1697c00d052799400cd00c78800d100ce8800d3a16900cc02e8039d00d28851d1537a8800ce01207f7552d1788852d3009d52cd547a8852d27b7801207f77527f75810164959d7b567f75817801247f77819d01227f77527f75817600a26902bc7fa16953cc02e8039d53d1788853d2008854d1008855d100876455d101207f75788791696856d100876456d101207f757887916968c4579c7777677b539dc0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba269785c7f77547f75817600a0695279567f7581547901247f7781a269537a01207f77527f75810164957602e803a2697b00cd00c78800d100ce8800cc02e8039d00d28851d1557a8852d154798852d3009d52cd557a8852d2537a567f75817801247f77819d7601227f77527f75817600a26902bc7fa16953d200876453d154798791696854d200876454d154798791696855d200876455d154798791696856d200876456d1547987916968c4579d52cc52955396537a950400e1f505967c01207f77527f7581016495767ba1697602e803a269760370e531a16900d000d39476900370e531a1697c7b7b939c7768686800000000b4fd1c66b0d9d1467c8207c4ace530d6736fed5c8e742acae4e520ae0404902601000000d2514ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868000000006a7cba6e4e0debe1d110354cf613db2529a90a42c69ecbe0655be323cf475bf601000000644185b1de9a8106014dbb2d2136f8e602fb63a2c4e3a127c82e87e2f55474cac9b443eaf89a0a3245100f0fd3815879b0346067edfe2d807023dfcb95864a7a60db4121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d85000000009eb9f37e89903bb6351d7d6aa667aade93d47231f5f6095d130453264e3999f405000000d6004cd320c004a27a03a13d755e16d8f89564f00f44944cbeb85f4c838cfa10937f15200d78009c63c0539d02e80353cf81768b55ccc0c6547a93a26955cdc0c78855d1c0ce8855d28853ce01207f7556d1788856d27b8800d100876400d101207f75788791696851d100876451d101207f75788791696852d100876452d101207f75788791696853d100876453d101207f75788791696854d100876454d101207f757887916968c4579c777777677c519d00ce01207f757888c0519d00d18851cd51c78851d151ce8851d251cf8852d10088c4539c68000000004db735d7cfc2b8bc57c7f7ee57301bec2c87bfc9ba01b2f1ecc3be3ce2b6b6a000000000644180d0a1b1a26151bdc500241c0587ff0435f48347bef84ad87a121a5a2af48e82d9a6437b93ac3326eceef56efa14a82c74e8e58d2edf7c4f12baf654381776734121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d850000000035a520545d10a453ac1b919308622ae5de9ce61e0242543a12904f7303cbebbd01000000644113ad1a6a65269ac17234c49ae7f7e536ca8d8a4eec72161ac4fbe1688cb5ccc27b20c238cc5f0510bc028d6587432d4670f35b4b1904e6b38e39912872357d184121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d850000000007e80300000000000052ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab37203f13913ff0843b1feffff0f00aa204ca6c7c5e1a38241e15beec09220bdb49fdcc3e7c2ae3bf1ea62a1a24e58b7e587804314000000000056ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d061101b3796680000e803f139130035e40000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a987d29ba5b7000000006eef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab36028fef765e9999e6248699a45a68847c06e7d5cb36f49f42e554995a6ec1720ffbbe02e81011b379668aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f887e80300000000000040ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab310fe804f120076a91413a2b518f04ac2570c222b3264d798abcb8a336488ac8e8b2001000000001976a91413a2b518f04ac2570c222b3264d798abcb8a336488ac602b0b000000000048ef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6202dc02aa206298ff3e09c5434b76ddcb6b372209f110c69be7e44eb304cd64eca8914fa77087a3020000000000003eef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6002db0276a91413a2b518f04ac2570c222b3264d798abcb8a336488ac29230000"
	testTxHex3    = "0200000003269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b9000000000fd26011015ac9568b9371300a037130053e1000040ccffa8aa283534adf5178af0d325456761653648d13ab6544e8d14a26ed8d3d6bd34f28476c10f64937b0e9ee0c54739dbb57e999623a54615ceeb178c64b035004cd120561fd4a4dc7cde1b06f224ec70380e8b2718a04aa0ef249e27b287531e64ec392102d09db08af1ff4e8453919cc866a4be427d7bfe18f2c05e5444c196fcf6fd28185279009c635379827701409d537a54797bbbc0009d51ce0087916952ce0088c3539d00cc00c6a26900cd00c78800d100ce885279547f75815379587f77547f7581547a5c7f77547f758151cf567f77527f7581537a56807c52807e7b54807e7c54807e00cc00c6a26900cd00c78800d100ce8851d28851d151ce8852d10088c4539c7777677b519d00ce7b877768ffffffff269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b9001000000d2004ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868ffffffff269eb8730da6a0c560d82bc7ff57776260c9dca9f92bbc79d578a97b2dfb7b90020000006441a444684bf550d6242a5061dba25c64dddd3ee58f830e64f2aacb8e5abfec145923e972a3f104d3dac8bf93329251dc074b15f349a8306f4f484366b00720de6d41210245496ebbf3d90fa94a395a3b6cc6852fbc621ee4147bbac0580a59d803cae8f8ffffffff03e80300000000000045efc1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e20aa203b71bed23bf7e606d2f9178d85e15170d68c45a10a9e86db520ac08e3ef41f6a8730fd13000000000056ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0611015ac95680000e803a037130053e10000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a9875cb07900000000001976a914925e9bcecea0e9a129ede392a075da2ef03a22c188ac29230000"
	testTxHex4    = "02000000069eb9f37e89903bb6351d7d6aa667aade93d47231f5f6095d130453264e3999f400000000fd3b03524d370323aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f8872172c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0015279009c63c0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba2697c567f7581527901247f7781a2697c01207f77527f75810164957602e803a26900d0937c00cd00c78800d100ce8800d37ba26900cc02e8039d00d28851d17b8852d1788852d2008853d100876453d101207f75788791696854d1008855d1008755d17b879b6955d20088c4569c7777675279519c63c0009d51ce788852ce0088c3539d51cf00cf817c587f77547f7581767ba06900cd00c78800d100ce8800d300d0a26900cc02e8039d00d28851d18852d10088c4539c7777675279529c63c0009d51ce788852ce008851cf00cf8178587f77547f7581767ba269785c7f77547f75817600a06952cc529553967c950400e1f5059653d3767ba1697602e803a269760370e531a1697c00d052799400cd00c78800d100ce8800d3a16900cc02e8039d00d28851d1537a8800ce01207f7552d1788852d3009d52cd547a8852d27b7801207f77527f75810164959d7b567f75817801247f77819d01227f77527f75817600a26902bc7fa16953cc02e8039d53d1788853d2008854d1008855d100876455d101207f75788791696856d100876456d101207f757887916968c4579c7777677b539dc0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba269785c7f77547f75817600a0695279567f7581547901247f7781a269537a01207f77527f75810164957602e803a2697b00cd00c78800d100ce8800cc02e8039d00d28851d1557a8852d154798852d3009d52cd557a8852d2537a567f75817801247f77819d7601227f77527f75817600a26902bc7fa16953d200876453d154798791696854d200876454d154798791696855d200876455d154798791696856d200876456d1547987916968c4579d52cc52955396537a950400e1f505967c01207f77527f7581016495767ba1697602e803a269760370e531a16900d000d39476900370e531a1697c7b7b939c7768686800000000b4fd1c66b0d9d1467c8207c4ace530d6736fed5c8e742acae4e520ae0404902601000000d2514ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868000000006a7cba6e4e0debe1d110354cf613db2529a90a42c69ecbe0655be323cf475bf601000000644185b1de9a8106014dbb2d2136f8e602fb63a2c4e3a127c82e87e2f55474cac9b443eaf89a0a3245100f0fd3815879b0346067edfe2d807023dfcb95864a7a60db4121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d85000000009eb9f37e89903bb6351d7d6aa667aade93d47231f5f6095d130453264e3999f405000000d6004cd320c004a27a03a13d755e16d8f89564f00f44944cbeb85f4c838cfa10937f15200d78009c63c0539d02e80353cf81768b55ccc0c6547a93a26955cdc0c78855d1c0ce8855d28853ce01207f7556d1788856d27b8800d100876400d101207f75788791696851d100876451d101207f75788791696852d100876452d101207f75788791696853d100876453d101207f75788791696854d100876454d101207f757887916968c4579c777777677c519d00ce01207f757888c0519d00d18851cd51c78851d151ce8851d251cf8852d10088c4539c68000000004db735d7cfc2b8bc57c7f7ee57301bec2c87bfc9ba01b2f1ecc3be3ce2b6b6a000000000644180d0a1b1a26151bdc500241c0587ff0435f48347bef84ad87a121a5a2af48e82d9a6437b93ac3326eceef56efa14a82c74e8e58d2edf7c4f12baf654381776734121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d850000000035a520545d10a453ac1b919308622ae5de9ce61e0242543a12904f7303cbebbd01000000644113ad1a6a65269ac17234c49ae7f7e536ca8d8a4eec72161ac4fbe1688cb5ccc27b20c238cc5f0510bc028d6587432d4670f35b4b1904e6b38e39912872357d184121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d850000000007e80300000000000052ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab37203f13913ff0843b1feffff0f00aa204ca6c7c5e1a38241e15beec09220bdb49fdcc3e7c2ae3bf1ea62a1a24e58b7e587804314000000000056ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d061101b3796680000e803f139130035e40000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a987d29ba5b7000000006eef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab36028fef765e9999e6248699a45a68847c06e7d5cb36f49f42e554995a6ec1720ffbbe02e81011b379668aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f887e80300000000000040ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab310fe804f120076a91413a2b518f04ac2570c222b3264d798abcb8a336488ac8e8b2001000000001976a91413a2b518f04ac2570c222b3264d798abcb8a336488ac602b0b000000000048ef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6202dc02aa206298ff3e09c5434b76ddcb6b372209f110c69be7e44eb304cd64eca8914fa77087a3020000000000003eef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6002db0276a91413a2b518f04ac2570c222b3264d798abcb8a336488ac29230000"
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

	testTx3 = bchain.Tx{
		Hex:       testTxHex3,
		Blocktime: 1754641552,
		Txid:      "781dd77b1c36765c034aa46d7b38d1d5e1f4491ebb6db0238ce9abcb29d7e632",
		LockTime:  9001,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "1015ac9568b9371300a037130053e1000040ccffa8aa283534adf5178af0d325456761653648d13ab6544e8d14a26ed8d3d6bd34f28476c10f64937b0e9ee0c54739dbb57e999623a54615ceeb178c64b035004cd120561fd4a4dc7cde1b06f224ec70380e8b2718a04aa0ef249e27b287531e64ec392102d09db08af1ff4e8453919cc866a4be427d7bfe18f2c05e5444c196fcf6fd28185279009c635379827701409d537a54797bbbc0009d51ce0087916952ce0088c3539d00cc00c6a26900cd00c78800d100ce885279547f75815379587f77547f7581547a5c7f77547f758151cf567f77527f7581537a56807c52807e7b54807e7c54807e00cc00c6a26900cd00c78800d100ce8851d28851d151ce8852d10088c4539c7777677b519d00ce7b877768",
				},
				Txid:     "907bfb2d7ba978d579bc2bf9a9dcc960627757ffc72bd860c5a0a60d73b89e26",
				Vout:     0,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "004ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868",
				},
				Txid:     "907bfb2d7ba978d579bc2bf9a9dcc960627757ffc72bd860c5a0a60d73b89e26",
				Vout:     1,
				Sequence: 4294967295,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "41a444684bf550d6242a5061dba25c64dddd3ee58f830e64f2aacb8e5abfec145923e972a3f104d3dac8bf93329251dc074b15f349a8306f4f484366b00720de6d41210245496ebbf3d90fa94a395a3b6cc6852fbc621ee4147bbac0580a59d803cae8f8",
				},
				Txid:     "907bfb2d7ba978d579bc2bf9a9dcc960627757ffc72bd860c5a0a60d73b89e26",
				Vout:     2,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "efc1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e20aa203b71bed23bf7e606d2f9178d85e15170d68c45a10a9e86db520ac08e3ef41f6a87",
					Addresses: []string{
						"bitcoincash:pvahr0kj80m7vpkjlytcmp0p29cddrz95y9fapkm2g9vpr377s0k5574dr2wc",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1310000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0611015ac95680000e803a037130053e10000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a987",
					Addresses: []string{
						"bitcoincash:p0kkwvymxepycxelyhsg7xwcat2w4h5303rkukck2gx7433avh22jlv39pdzt",
					},
				},
			},
			{
				ValueSat: *big.NewInt(7975004),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914925e9bcecea0e9a129ede392a075da2ef03a22c188ac",
					Addresses: []string{
						"bitcoincash:qzf9ax7we6swngffah3e9gr4mgh0qw3zcy6kq5afau",
					},
				},
			},
		},
	}

	testTx4 = bchain.Tx{
		Hex:       testTxHex4,
		Blocktime: 1754677578,
		Txid:      "d7ad5f6336cd2c80c86e241ab2deb7b981b7dd9234b68a5b42e304829e482fb2",
		LockTime:  9001,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "524d370323aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f8872172c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d0015279009c63c0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba2697c567f7581527901247f7781a2697c01207f77527f75810164957602e803a26900d0937c00cd00c78800d100ce8800d37ba26900cc02e8039d00d28851d17b8852d1788852d2008853d100876453d101207f75788791696854d1008855d1008755d17b879b6955d20088c4569c7777675279519c63c0009d51ce788852ce0088c3539d51cf00cf817c587f77547f7581767ba06900cd00c78800d100ce8800d300d0a26900cc02e8039d00d28851d18852d10088c4539c7777675279529c63c0009d51ce788852ce008851cf00cf8178587f77547f7581767ba269785c7f77547f75817600a06952cc529553967c950400e1f5059653d3767ba1697602e803a269760370e531a1697c00d052799400cd00c78800d100ce8800d3a16900cc02e8039d00d28851d1537a8800ce01207f7552d1788852d3009d52cd547a8852d27b7801207f77527f75810164959d7b567f75817801247f77819d01227f77527f75817600a26902bc7fa16953cc02e8039d53d1788853d2008854d1008855d100876455d101207f75788791696856d100876456d101207f757887916968c4579c7777677b539dc0009d51ce7888c0ce01207f7552cf52ce5279887682770114a26951cf00cf8178587f77547f7581767ba269785c7f77547f75817600a0695279567f7581547901247f7781a269537a01207f77527f75810164957602e803a2697b00cd00c78800d100ce8800cc02e8039d00d28851d1557a8852d154798852d3009d52cd557a8852d2537a567f75817801247f77819d7601227f77527f75817600a26902bc7fa16953d200876453d154798791696854d200876454d154798791696855d200876455d154798791696856d200876456d1547987916968c4579d52cc52955396537a950400e1f505967c01207f77527f7581016495767ba1697602e803a269760370e531a16900d000d39476900370e531a1697c7b7b939c77686868",
				},
				Txid:     "f499394e265304135d09f6f53172d493deaa67a66a7d1d35b63b90897ef3b99e",
				Vout:     0,
				Sequence: 0,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "514ccf20177db58be48264cb1f1acef0ef332541f45c3e4968b06dd9cb5d2c14f137bad920c1c3f1b000630136069e86c617ab6eeecf65fd22593fda5578ba9a442673435e5279009c6300ce01207f7588c0d276827760a269c0cf78587f77547f758178587f77547f7581a0697c567f75817c567f7581a069c0ccc0c6a269c0cdc0c788c0d1c0ce877777675279519c63c0cf567f77527f7581c0ccc0c67b93a269c0cdc0c788c0d1c0ce88c0d2c0cf87777777677b529d00ce01207f757b88c0cdc0c788c0d1c0ce88c0d2c0cf87776868",
				},
				Txid:     "26900404ae20e5e4ca2a748e5ced6f73d630e5acc407827c46d1d9b0661cfdb4",
				Vout:     1,
				Sequence: 0,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4185b1de9a8106014dbb2d2136f8e602fb63a2c4e3a127c82e87e2f55474cac9b443eaf89a0a3245100f0fd3815879b0346067edfe2d807023dfcb95864a7a60db4121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d85",
				},
				Txid:     "f65b47cf23e35b65e0cb9ec6420aa92925db13f64c3510d1e1eb0d4e6eba7c6a",
				Vout:     1,
				Sequence: 0,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "004cd320c004a27a03a13d755e16d8f89564f00f44944cbeb85f4c838cfa10937f15200d78009c63c0539d02e80353cf81768b55ccc0c6547a93a26955cdc0c78855d1c0ce8855d28853ce01207f7556d1788856d27b8800d100876400d101207f75788791696851d100876451d101207f75788791696852d100876452d101207f75788791696853d100876453d101207f75788791696854d100876454d101207f757887916968c4579c777777677c519d00ce01207f757888c0519d00d18851cd51c78851d151ce8851d251cf8852d10088c4539c68",
				},
				Txid:     "f499394e265304135d09f6f53172d493deaa67a66a7d1d35b63b90897ef3b99e",
				Vout:     5,
				Sequence: 0,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4180d0a1b1a26151bdc500241c0587ff0435f48347bef84ad87a121a5a2af48e82d9a6437b93ac3326eceef56efa14a82c74e8e58d2edf7c4f12baf654381776734121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d85",
				},
				Txid:     "a0b6b6e23cbec3ecf1b201bac9bf872cec1b3057eef7c757bcb8c2cfd735b74d",
				Vout:     0,
				Sequence: 0,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4113ad1a6a65269ac17234c49ae7f7e536ca8d8a4eec72161ac4fbe1688cb5ccc27b20c238cc5f0510bc028d6587432d4670f35b4b1904e6b38e39912872357d184121039c158fc32984be692bf564f042c16975570184b84e3bac827b78017169883d85",
				},
				Txid:     "bdebcb03734f90123a5442021ee69cdee52a620893911bac53a4105d5420a535",
				Vout:     1,
				Sequence: 0,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab37203f13913ff0843b1feffff0f00aa204ca6c7c5e1a38241e15beec09220bdb49fdcc3e7c2ae3bf1ea62a1a24e58b7e587",
					Addresses: []string{
						"bitcoincash:pdx2d379ux3cys0pt0hvpy3qhk6flhxrulp2uwl3af32rgjwtzm72z8ps3c8c",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1328000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef72c96a0660a2b60b1f10301a33b89cc10057c7493e0ddece8a1882bd5c6fd4d061101b3796680000e803f139130035e40000aa20ed67309b36424c1b3f25e08f19d8ead4eade917c476e5b16520deac63d65d4a987",
					Addresses: []string{
						"bitcoincash:p0kkwvymxepycxelyhsg7xwcat2w4h5303rkukck2gx7433avh22jlv39pdzt",
					},
				},
			},
			{
				ValueSat: *big.NewInt(3081083858),
				N:        2,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab36028fef765e9999e6248699a45a68847c06e7d5cb36f49f42e554995a6ec1720ffbbe02e81011b379668aa201687336b0d6e4a58bdfe1d8a117a2dab29c2f9635dd082b177ae13efd774b4f887",
					Addresses: []string{
						"bitcoincash:pvtgwvmtp4hy5k9alcwc5yt69k4jnshevdwapq43w7hp8m7hwj60ss0qjrjyz",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1000),
				N:        3,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef66f387ed89967808344f5d58215060796e3e87cb236f9a165c4cf850f7338ab310fe804f120076a91413a2b518f04ac2570c222b3264d798abcb8a336488ac",
					Addresses: []string{
						"bitcoincash:qqf69dgc7p9vy4cvyg4nyexhnz4uhz3nvs6k0cfv98",
					},
				},
			},
			{
				ValueSat: *big.NewInt(18910094),
				N:        4,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91413a2b518f04ac2570c222b3264d798abcb8a336488ac",
					Addresses: []string{
						"bitcoincash:qqf69dgc7p9vy4cvyg4nyexhnz4uhz3nvs6k0cfv98",
					},
				},
			},
			{
				ValueSat: *big.NewInt(732000),
				N:        5,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6202dc02aa206298ff3e09c5434b76ddcb6b372209f110c69be7e44eb304cd64eca8914fa77087",
					Addresses: []string{
						"bitcoincash:pd3f3le7p8z5xjmkmh9kkdezp8c3p35muljyavcye4jwe2y3f7nhqp2f5vwvl",
					},
				},
			},
			{
				ValueSat: *big.NewInt(675),
				N:        6,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "ef9b3d0b4648de64973e69b4dfa7dae5a6c9f06e4b18b66460512d7e06ec62839c6002db0276a91413a2b518f04ac2570c222b3264d798abcb8a336488ac",
					Addresses: []string{
						"bitcoincash:qqf69dgc7p9vy4cvyg4nyexhnz4uhz3nvs6k0cfv98",
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
		name       string
		args       args
		want       *bchain.Tx
		want1      uint32
		wantErr    bool
		wantTokens []*bchain.BcashToken
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
			wantTokens: []*bchain.BcashToken{
				nil,
			},
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
			wantTokens: []*bchain.BcashToken{
				nil,
				nil,
			},
		},
		{
			name: "bcash-cashtokens-1",
			args: args{
				packedTx: testTxPacked3,
				parser:   mainParser,
			},
			want:    &testTx3,
			want1:   910609,
			wantErr: false,
			wantTokens: []*bchain.BcashToken{
				{
					Category: hexToBytes("5e437326449aba7855da3f5922fd65cfee6eab17c6869e0636016300b0f1c3c1"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("none"),
						Commitment: hexToBytes(""),
					},
				},
				{
					Category: hexToBytes("d0d46f5cbd82188acede0d3e49c75700c19cb8331a30101f0bb6a260066ac972"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("mutable"),
						Commitment: hexToBytes("15ac95680000e803a037130053e10000"),
					},
				},
				nil,
			},
		},
		{
			name: "bcash-cashtokens-2",
			args: args{
				packedTx: testTxPacked4,
				parser:   mainParser,
			},
			want:    &testTx4,
			want1:   910777,
			wantErr: false,
			wantTokens: []*bchain.BcashToken{
				{
					Category: hexToBytes("b38a33f750f84c5c169a6f23cb873e6e79605021585d4f3408789689ed87f366"),
					Amount:   (common.Amount)(*big.NewInt(4503599605433096)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("minting"),
						Commitment: hexToBytes("f13913"),
					},
				},
				{
					Category: hexToBytes("d0d46f5cbd82188acede0d3e49c75700c19cb8331a30101f0bb6a260066ac972"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("mutable"),
						Commitment: hexToBytes("1b3796680000e803f139130035e40000"),
					},
				},
				{
					Category: hexToBytes("b38a33f750f84c5c169a6f23cb873e6e79605021585d4f3408789689ed87f366"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("none"),
						Commitment: hexToBytes("fef765e9999e6248699a45a68847c06e7d5cb36f49f42e554995a6ec1720ffbbe02e81011b379668"),
					},
				},
				{
					Category: hexToBytes("b38a33f750f84c5c169a6f23cb873e6e79605021585d4f3408789689ed87f366"),
					Amount:   (common.Amount)(*big.NewInt(1200000)),
				},
				nil,
				{
					Category: hexToBytes("9c8362ec067e2d516064b6184b6ef0c9a6e5daa7dfb4693e9764de48460b3d9b"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("minting"),
						Commitment: hexToBytes("dc02"),
					},
				},
				{
					Category: hexToBytes("9c8362ec067e2d516064b6184b6ef0c9a6e5daa7dfb4693e9764de48460b3d9b"),
					Amount:   (common.Amount)(*big.NewInt(0)),
					Nft: &bchain.BcashTokenNft{
						Capability: bchain.BcashNFTCapabilityLabel("none"),
						Commitment: hexToBytes("db02"),
					},
				},
			},
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

			tokens := make([]*bchain.BcashToken, len(tt.want.Vout))
			for i, vout := range tt.want.Vout {
				script, err := hex.DecodeString(vout.ScriptPubKey.Hex)
				if err != nil {
					t.Errorf("failed to decode scriptPubKey hex for vout %d: %v", i, err)
					return
				}
				token, _, err := tt.args.parser.ParseTokenData(script)
				if err != nil {
					t.Errorf("ParseTokenData failed for vout %d: %v", i, err)
					return
				}
				tokens[i] = token
			}

			if !reflect.DeepEqual(tt.wantTokens, tokens) {
				t.Errorf("unpackTx() gotTokens = %v, want %v", tokens, tt.wantTokens)
			}
		})
	}
}

func loadFixturesFromJSON(filename string, t *testing.T) []interface{} {
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed to open fixture file: %v", err)
	}
	defer file.Close()

	var fixtures []interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&fixtures); err != nil {
		t.Fatalf("failed to decode fixture json: %v", err)
	}
	return fixtures
}

func Test_ValidTokenPrefixes(t *testing.T) {
	mainParser, _, _, _ := setupParsers(t)

	// Load the JSON fixtures
	fixtures := loadFixturesFromJSON("./fixtures/token-prefix-valid.json", t)

	for _, testCase := range fixtures {
		test, ok := testCase.(map[string]interface{})
		if !ok {
			t.Fatalf("invalid test case format")
		}
		prefixHex := test["prefix"].(string)
		data := test["data"].(map[string]interface{})

		prefix, err := hex.DecodeString(prefixHex)
		if err != nil {
			t.Fatalf("failed to decode prefix hex: %v", err)
		}

		token, _, err := mainParser.ParseTokenData(prefix)
		if err != nil {
			t.Fatalf("ParseTokenData failed: %v", err)
		}

		amount := data["amount"].(string)
		category := data["category"].(string)
		var commitment, nftCapability string
		if nft, ok := data["nft"].(map[string]interface{}); ok {
			commitment, _ = nft["commitment"].(string)
			nftCapability, _ = nft["capability"].(string)
		}

		if token == nil {
			t.Fatalf("token is nil")
		}

		if token.Amount.String() != amount {
			t.Errorf("amount: got %s, want %s", token.Amount.String(), amount)
		}
		if hex.EncodeToString(token.Category) != category {
			t.Errorf("category: got %s, want %s", hex.EncodeToString(token.Category), category)
		}

		if commitment != "" || nftCapability != "" {
			if token.Nft == nil {
				t.Fatalf("expected token to have NFT data, but got nil")
			}

			if hex.EncodeToString(token.Nft.Commitment) != commitment {
				t.Errorf("commitment: got %s, want %s", token.Nft.Commitment, commitment)
			}
			if string(token.Nft.Capability) != nftCapability {
				t.Errorf("capability: got %s, want %s", token.Nft.Capability, nftCapability)
			}
		}

		tokenData := PackTokenData(token)
		if !bytes.Equal(tokenData, prefix) {
			t.Errorf("packed token data does not match original prefix.\ngot:  %x\nwant: %x", tokenData, prefix)
		}

		unpacked, l, err := UnpackTokenData(tokenData)
		if err != nil {
			t.Fatalf("UnpackTokenData failed: %v", err)
		}
		if l != len(tokenData) {
			t.Errorf("UnpackTokenData length mismatch: got %d, want %d", l, len(tokenData))
		}
		if hex.EncodeToString(token.Category) != hex.EncodeToString(unpacked.Category) {
			t.Errorf("category: got %s, want %s", hex.EncodeToString(token.Category), hex.EncodeToString(unpacked.Category))
		}
		if !reflect.DeepEqual(unpacked, token) {
			t.Errorf("UnpackTokenData result mismatch:\ngot:  %+v\nwant: %+v", unpacked, token)
		}
	}
}

func Test_InvalidTokenPrefixes(t *testing.T) {
	mainParser, _, _, _ := setupParsers(t)

	// Load the JSON fixtures
	fixtures := loadFixturesFromJSON("./fixtures/token-prefix-invalid.json", t)

	for _, testCase := range fixtures {
		test, ok := testCase.(map[string]interface{})
		if !ok {
			t.Fatalf("invalid test case format")
		}
		prefixHex := test["prefix"].(string)
		expectedError := test["error"].(string)

		prefix, err := hex.DecodeString(prefixHex)
		if err != nil {
			t.Fatalf("failed to decode prefix hex: %v", err)
		}

		_, _, err = mainParser.ParseTokenData(prefix)
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("expected error to contain %q, got %q", expectedError, err.Error())
			return
		}
	}
}
