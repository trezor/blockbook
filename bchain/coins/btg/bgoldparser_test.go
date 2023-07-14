//go:build unittest

package btg

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

type testBlock struct {
	size int
	time int64
	txs  []string
}

var testParseBlockTxs = map[int]testBlock{
	104000: {
		size: 15776,
		time: 1295705889,
		txs: []string{
			"331d4ef64118e9e5be75f0f51f1a4c5057550c3320e22ff7206f3e1101f113d0",
			"1f4817d8e91c21d8c8d163dabccdd1875f760fd2dc34a1c2b7b8fa204e103597",
			"268163b1a1092aa0996d118a6027b0b6f1076627e02addc4e66ae30239936818",
			"27277a1049fafa2a46368ad02961d37da633416b014bcd42a1f1391753cbf559",
			"276d2d331585d0968762d9788f71ae71524ccba3494f638b2328ac51e52edd3d",
			"28d9f85089834c20507cc5e4ec54aaaf5b79feab80cad24a48b8296b6d327a43",
			"2a2d66d3d9e8b6f154958a388377f681abd82ce98785a5bbf2e27d0ca454da3f",
			"39c9e995a12b638b541d21ed9f9e555716709add6a97c8ba63fe26e4d26bdc3f",
			"4a768efc6cc0716932800115989376c2ce3e5e17668b08bd2f43031105a6ac6e",
			"4bc41fa0188d988d853a176eb847a541c5adf35348eab21d128904aab36e8574",
			"4cad1bd38cc7be880f3a968af6f13a3aeb5dbdb51a774b7d1ae3d6d6bfd114e4",
			"6bc558801583bfdb656106889c4b8bd783168133784666338c57e5b2a675a922",
			"75eb5c1aa89b18ce71f363f95147942b46645ca9b1e472784fcb8c9a096f4f5c",
			"91755365cff22c4eed09a57a8fb7b2faa5c1caa5fa750c89f830a2bb56fa4c68",
			"9417d34969891f2a0b9aa3a1226505edf3b429fa1acd21a140d358a217d11d55",
			"950fbb5f413af9f7c6e5dabfacf68b715c9851b5cf6ab6806960f7cc7cad2f9d",
			"99b134dae55ddfab1f5d5243c2e60030a9ed969ba5915f98840b877f8af28ce0",
			"9d7b15eaaccce66e25efe7e2979454ae4968b61281d50f32be9872d2d256c244",
			"a54df5296f1d1a6101cee0869ffda03502e417fc72475b7902a6dfd5b9329399",
			"adba400f14b476f0c2b11340ee1fa440208b49fd99c1072136198b5c43664826",
			"bd7d8fee068bd45b06b4c17ccdf577b4bc2b21c9c4e0cee8453409b0e63b4f5d",
			"beabd2d68b66a9b47b6aff23b569c1b59e429074f06bdd4993e9d3c2cb69c992",
			"bfa81174d549eb7ed15be3f965686aff3084f22523c52fbed03c3fc3e18b7cda",
			"e42472099cb039b1c2248b611ce212580338550977e02bd77accdf29bfd86e96",
			"f056e02b12d99377f724a4987cde68ecf6f234fd7e2bdf4324172c03d015ba25",
			"f1815cfb1ef4cfe13ff5ec2c15b5bc55fde043db85daca1bb34cc1b491193308",
			"f225abce64f75383686fa08abe47242e59e97809a31c8fd7a88acce1e6cbcd27",
			"f93f1b125bfa2da5ccaaf30ff96635b905b657d48a5962c24be93884a82ef354",
			"fef75d015f2e9926d1d4bf82e567b91e51af66a6e636d03a072f203dd3062ae5",
			"051b60a6accead85da54b8d18f4b2360ea946da948d3a27836306d2927fed13e",
			"28e47b050ec4064cdbd3364f3be9445d52635e9730691ef71ed1db0f0147afd6",
			"446ebde2457102bcbc2c86cac6ff582c595b00346fd0b27ea5a645240020504b",
			"46c8fafe2b7bb1646aeefa229b18fa7ffe20fee0a30c4a9ef3e63c36c808f6f7",
			"550d96cf82fbe91dcc9b96e7aa404f392ee47400c22a98a7631d29eee43fceaa",
			"59b6b78a72cc33cd29741894b3007b1330fc7f7945bdc0a7a4044ed1dd289c19",
			"5a3aa07474338cf193c1d7aacdc37f3311c971857ba8cfd308e8fabf5e473882",
			"82e014b1a9c6cb7729274653ce748c66953de6abb3d1411471515b41b727cf75",
			"8d70af4f135696da396c9aa9f936b54195bfbe6ff0e08b3b210ca0b52bc167d2",
			"9949c2f2f3b96a557ef6e14004cbd239a0744c056faca34bdff01e125b4380e8",
			"d09a8c83123ce1cb6ff837e7670aab5f50c5155d9706dd26f7e0761fd20c5536",
			"f601482efc5b2dd3d0031e318e840cd06f7cab0ffff8cc37a5bf471b515ddfb7",
			"f88d3c0ebe8b294f11e70d2ae6d2f0048437bfb20dae7e69d545a4c72d3432dd",
			"2b9e574b90556250b20a79d9c94ceaff3dfd062291c34c3fa79c7ca8d85a3500",
			"b9484ef8e38ceafe8d2b09ecf59562d262b15d185844b2d00db362718d52b2c2",
			"07a4af0a81b55313a1c16da7d808829d689436fd078fa9559b6d1603dd72474e",
			"3393bdcc3a7232b37d0fb6b56d603a2b9b0419e461bf989f1c375859a5d0156a",
			"33ad36d79d63b575c7532c516f16b19541f5c637caf7073beb7ddf604c3f39cc",
		},
	},
	532144: {
		size: 12198,
		time: 1528372417,
		txs: []string{
			"574348e23301cc89535408b6927bf75f2ac88fadf8fdfb181c17941a5de02fe0",
			"9f048446401e7fac84963964df045b1f3992eda330a87b02871e422ff0a3fd28",
			"9516c320745a227edb07c98087b1febea01c3ba85122a34387896fc82ba965e4",
			"9d37e1ab5a28c49ce5e7ece4a2b9df740fb4c3a84bdec93b3023148cf20f0de7",
			"a3cd0481b983ba402fed8805ef9daf5063d6d4e5085b82eca5b4781c9e362d6a",
			"7f2c2567e8de0321744817cfeb751922d7e8d82ef71aa01164c84fb74a463a53",
			"cd064315e3f5d07920b3d159160c218d0bb5b7b4be606265767b208ae7e256eb",
			"a9523400f341aa425b0fcc00656ec1fa5421bf3545433bff98a8614fc9a99d1f",
			"ec766daacbb05a8f48a3205e5c6494a7c817bd35eefff9aaf55e0dd47fe6e8fc",
			"0837a4116872abf52caa52d1ff7608674ba5b09a239a1f43f3a25ba4052e4c77",
			"a3e23a0344fe6ba7083fc6afb940517cdb666dce00389cbd8598bd599199cdda",
			"048d951cef84d35d68f0bc3b575662caf23fee692e8035bd5efe38ab67e0d6c2",
			"11307491b24d42ddd7ea27fc795d444b65c3936be31b904a97da68fabc85e5b8",
			"84ad99dc0884e03fc71f163eebf515a1eb79d00b1aad7a1126b22747960a8275",
			"728c8d0858e506d4a1a9b506f7b974b335e6c54047af9f40d4cb1a0561f783e1",
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
	p := NewBGoldParser(GetChainParams("main"), &btc.Configuration{})

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
