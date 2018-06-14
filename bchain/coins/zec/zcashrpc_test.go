// +build integration

package zec

import (
	"blockbook/bchain"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"testing"
)

var rpcURL = flag.String("rpcURL", "http://localhost:18032", "RPC URL of backend server")
var rpcUser = flag.String("rpcUser", "rpc", "RPC user of backend server")
var rpcPass = flag.String("rpcPass", "rpc", "RPC password of backend server")
var rpcClient *ZCashRPC

func getRPCConfig() string {
	config := `{
	"coin_name": "Zcash",
	"rpcURL": "%s",
	"rpcUser": "%s",
	"rpcPass": "%s",
	"rpcTimeout": 25,
	"parse": true
	}`

	return fmt.Sprintf(config, *rpcURL, *rpcUser, *rpcPass)
}

func getRPCClient() (*ZCashRPC, error) {
	if rpcClient == nil {
		cfg := json.RawMessage(getRPCConfig())
		c, err := NewZCashRPC(cfg, nil)
		if err != nil {
			return nil, err
		}
		cli := c.(*ZCashRPC)
		cli.Parser = NewZCashParser(cli.ChainConfig)
		rpcClient = cli
	}
	return rpcClient, nil
}

const blockHeight = 251102
const blockHash = "001335906f981bbf0633e124e2fa8afef3d882e34a0306a4e0c55162e57e673d"
const blockHex = "04000000dae072724713c60408a643775f15f58a37088bea7a51165880fbed47346b0c00cce3aae2d297be2838c5d4829b682b4fbbe8a09f9a10da8222b8256b2b1f20df0000000000000000000000000000000000000000000000000000000000000000d15b1f5baf71151f040019290a7a71036c73966bf61b685bd50dd09e095ef4680224901a53050000fd4005008d152c3b0d8adeee94b01ce9d9ebeec96098779a1a9782005ec849d1198482c1eb2c9234d8100b2d560c1bae8798c6c15add6082e9b871bc751792aedbce2f086ad7aa914537318d846af7e6c9ede26856c9440874d981c8d5f48eff38e5f81ef76585ca6c3b774f0b5ffc7676054c246d5704e9bfdc0f3e29aefa913a16bb1505f3cc44aaae9d7170569fe120d4146ad98f242c52e334e40599cdaab24455d95e12abf7fb283009647baa27a04a79ac45d0dddfc70106baa8dfb34d105b1da71fbac3c3dca4718a97b976b9e4c4b56dea0c2f0f73d258e435e3156125a3a85574b0e06a5cb22ab6d77d900c0d82a73db6b147f1378286885f4b8d11e15cb9bbee80abb90ff49addd3d995e1727c6d751c15d964ca0a199b3fd1821d97685f793ebeb82e1a173f3d4b21535004e9edc511f2f8595e467cd3dd5117df2ff8cb0e928ffadbe33a2838d914d22e709ed000f267538f88287858f062edab5f7dbe079ab6f5e8252ee356155eea1d99b8b8bc9a778f3e3f4c15716408f4d6d4b0d6739fa5900622dc72fff6d0c79ae43118727bb533c81930a3bc85dea97574ee678d7546c4060d1ee526359e8ff6b0809ff4e218e54c8233411d5d1316861c698a13bc95862adf3eda6360367fe18519eea495294990bb2f0db3a7e9dd1e064ba1d3a0fc2c4489be1d52a2fbfac665adb93d84ae806d9e1ed704aae1e140156f4f113fd2aa88db1fa98c7a903e29115b1a37b5f53b57e0b5350231630603057d7f47ab05c5a9b8e78976d73f6be1f39eef207ef665bba21910b4333592cd23af144fd32cbeb29f5d8f734f5c7f062e4a0a0d5abb236588a3a604657231adc10e07910cccd6b075c73bb4d53311b076be2ac22f2a75c98719f857b2bddfbcfbb99314cdacbdb9cbb4ea3f532f5f9a1c93256f83b7b933f7857675eb56e57d395c2a009214f706c371b13d706221161161914f61927ed3435f641b851cc2c35a56f68aa0367db22e24bf7c4608317135e65422f5023391bc2893eb45cf0f3762648bec5e2c3f649a5b2ffd69555668c1525d7f1cc19104793ccb1c14360b5f3f4790bfe0f60ed882189567058e29c0fe67428db64721e5880fef24d9b696a5f10972740e5ed8eedd365876662555ace71bbebb8b9b200621e6d2ef28e5eaac935521c621bd6533b725f001ef494f6ba94cad6e8b4810937a2642fa97fddf59606e9d43762e1eff7badf6b0274354864df0f97cfb0362c324de02d694c83356b0f6e273f20cfad339fe0883da29e58715663d03b1d6c5fa0b9615cef7377507aa43c06c70d5f1cc48a6f449be2956481b33309911edb77effc8fea728fa04544c7f04a1d3f7b365a310c7ed63034528890c6bc23118dd01cad7a096c1fb15b2df445fdfb3a146a881f424254d44dc0caa20e501f5edaf3eba4881ef6380937cb1eb50488a0aef365d49d40aeaa7bd754231973ad1cc6716a5babcfed810f60db313172583b34c237057f1a3991436f6b40c3bd2374cbf1d8b0ef2af73fbdf60623a51841a256617d0829e6b338fddeca9948a0fde177d7a04def4e858ad56388e660a25a405d6397bdc0cce6287b3906a2a2167fbb08ee5828dda75cd3dd61af2a8eb971ea72a515df331ce595b15113368fd30dd36a7ca77d2ac149f049ca55e37cdf95a7316c6686eba6e5a152a6f289983d470314681d331b36b5ff5a1a93915bc309a1f2c0be8eed10a39548ee7b48cbedcb6622c13b0f134136f1bead1dc7ae29118048064119252721b5446192ca1a77eac7f2592fc34bc8eeae98633a21a17b3220ef93d58e28fc9e9b922e4f7ea3bf10f1dd5da9e327fbdfbe79765bf6cc0e37f3c9eceed203d8f80083b8b2a2c0f6a5b0d5a52c8d541e60632a666bcad5c03030000807082c403010000000000000000000000000000000000000000000000000000000000000000ffffffff0603ded4030101ffffffff0267cc9a3b000000001976a914550cef2cf048a294415d0c5893e653ac529c404388ac80b2e60e0000000017a914a71e4588c50c86f4669e0da43db37cbd7428a9f287000000000000000000030000807082c40301601ade5f535f62f3da01e0c88b16beb916877fa0fa43299a9e2398b813d0a119020000006a4730440220770472192407cfc5c01e331442ac579a68e445a7ccb0fe8a481161519555794a02207b00b4c01af416a4972c2fd2fba8d58a6467da545400469ab57e9207a0ee3ca201210201d494a45f36f545443bafd1a9050b02f448dd236bb4ce2602f83978980b98f2feffffff02ea5e2c1c000000001976a9144ef2de10b304381a9404ccbe12f83395e7860bbd88ac00a3e1110000000017a9146a8eecfe3a7949a4a8c68754accaea75ab32cd9e8794d40300f1d4030000030000807082c40302eac0139a677c07b6fcdddd205f4bf538d9cc9fb586565fe77440f93cf1b5ca9a010000006b4830450221009ad50b4f0da90a1876af6b43c029e55f1c8b8c44e94161a7f14c812adcebeaa902204b9df40096337eb43120408d4743693b17ea27fc0f660948a0a761264e67a6b00121036ec451bd1d3d39bb8d2d0264dd5a0ff817ae306ec7ce0c7c9b86be3f3dde8727feffffff6a1ed49e62634ab5a60b23ec73f9014ad1c9d3bd3bf023333c8eb311c802f298000000006b4830450221009008a10ebebf45f1a9aeb01a5b49a5de820a1a396ca1b900ea0420a1c2c49ea7022053fad044ff42792dcad16d084d67d7c4cc565458f78a14292fe5ef5e9f7a20bb012103448301b9253e6822ae05af64c61517fe8cf8b22590cc7d0b308b7f5cbce26db1feffffff0200a3e111000000001976a9149bc587e2cdd13b78e26f601f084ea0597b357f4d88ac3176a002000000001976a91414a88f1032c8ef1c6bd9177f62cdb260b5087b5488acd2d40300f1d4030000"

var blockTxs = []string{
	"f02aa1c4c86e1d0cef6ccbbc48b2b7b38355bc3612d8f77dd58d04be1ec6ba19",
	"a9f7cc34d7e272d2d9fb68cfa1c1941e338f377e6e426ae2fea1c12616d89c63",
	"83f3db1d129a77bee9c6cf32cbc12d959cd0af8c8d734c2611db4bfddfe99202",
}

var txDetails map[string]*bchain.Tx

func init() {
	var (
		addr1, addr2, addr3, addr4 bchain.Address
		err                        error
	)
	addr1, err = bchain.NewBaseAddress("tmGunyrqeeHBKX3Jm23zKLokRogrS51Qzqh")
	if err == nil {
		addr2, err = bchain.NewBaseAddress("t2GGEpYQUKiTsb9WYKD9AKNEriao84tVsr5")
	}
	if err == nil {
		addr3, err = bchain.NewBaseAddress("tmPuzhaxL3vzTqkyJ6wn48Q4ZfAo6PBvHKC")
	}
	if err == nil {
		addr4, err = bchain.NewBaseAddress("tmBbamgVqVBTAsEzPmEymbczdoaQvE5qwaR")
	}
	if err != nil {
		panic(err)
	}

	txDetails = map[string]*bchain.Tx{
		"a9f7cc34d7e272d2d9fb68cfa1c1941e338f377e6e426ae2fea1c12616d89c63": &bchain.Tx{
			Hex:       "030000807082c40301601ade5f535f62f3da01e0c88b16beb916877fa0fa43299a9e2398b813d0a119020000006a4730440220770472192407cfc5c01e331442ac579a68e445a7ccb0fe8a481161519555794a02207b00b4c01af416a4972c2fd2fba8d58a6467da545400469ab57e9207a0ee3ca201210201d494a45f36f545443bafd1a9050b02f448dd236bb4ce2602f83978980b98f2feffffff02ea5e2c1c000000001976a9144ef2de10b304381a9404ccbe12f83395e7860bbd88ac00a3e1110000000017a9146a8eecfe3a7949a4a8c68754accaea75ab32cd9e8794d40300f1d4030000",
			Blocktime: 1528781777,
			Time:      1528781777,
			Txid:      "a9f7cc34d7e272d2d9fb68cfa1c1941e338f377e6e426ae2fea1c12616d89c63",
			LockTime:  251028,
			Vin: []bchain.Vin{
				{
					ScriptSig: bchain.ScriptSig{
						Hex: "4730440220770472192407cfc5c01e331442ac579a68e445a7ccb0fe8a481161519555794a02207b00b4c01af416a4972c2fd2fba8d58a6467da545400469ab57e9207a0ee3ca201210201d494a45f36f545443bafd1a9050b02f448dd236bb4ce2602f83978980b98f2",
					},
					Txid:     "19a1d013b898239e9a2943faa07f8716b9be168bc8e001daf3625f535fde1a60",
					Vout:     2,
					Sequence: 4294967294,
				},
			},
			Vout: []bchain.Vout{
				{
					Value: 4.72669930,
					N:     0,
					ScriptPubKey: bchain.ScriptPubKey{
						Hex: "76a9144ef2de10b304381a9404ccbe12f83395e7860bbd88ac",
						Addresses: []string{
							"tmGunyrqeeHBKX3Jm23zKLokRogrS51Qzqh",
						},
					},
					Address: addr1,
				},
				{
					Value: 3.0,
					N:     1,
					ScriptPubKey: bchain.ScriptPubKey{
						Hex: "a9146a8eecfe3a7949a4a8c68754accaea75ab32cd9e87",
						Addresses: []string{
							"t2GGEpYQUKiTsb9WYKD9AKNEriao84tVsr5",
						},
					},
					Address: addr2,
				},
			},
		},
		"83f3db1d129a77bee9c6cf32cbc12d959cd0af8c8d734c2611db4bfddfe99202": &bchain.Tx{
			Hex:       "030000807082c40302eac0139a677c07b6fcdddd205f4bf538d9cc9fb586565fe77440f93cf1b5ca9a010000006b4830450221009ad50b4f0da90a1876af6b43c029e55f1c8b8c44e94161a7f14c812adcebeaa902204b9df40096337eb43120408d4743693b17ea27fc0f660948a0a761264e67a6b00121036ec451bd1d3d39bb8d2d0264dd5a0ff817ae306ec7ce0c7c9b86be3f3dde8727feffffff6a1ed49e62634ab5a60b23ec73f9014ad1c9d3bd3bf023333c8eb311c802f298000000006b4830450221009008a10ebebf45f1a9aeb01a5b49a5de820a1a396ca1b900ea0420a1c2c49ea7022053fad044ff42792dcad16d084d67d7c4cc565458f78a14292fe5ef5e9f7a20bb012103448301b9253e6822ae05af64c61517fe8cf8b22590cc7d0b308b7f5cbce26db1feffffff0200a3e111000000001976a9149bc587e2cdd13b78e26f601f084ea0597b357f4d88ac3176a002000000001976a91414a88f1032c8ef1c6bd9177f62cdb260b5087b5488acd2d40300f1d4030000",
			Blocktime: 1528781777,
			Time:      1528781777,
			Txid:      "83f3db1d129a77bee9c6cf32cbc12d959cd0af8c8d734c2611db4bfddfe99202",
			LockTime:  251090,
			Vin: []bchain.Vin{
				{
					ScriptSig: bchain.ScriptSig{
						Hex: "4830450221009ad50b4f0da90a1876af6b43c029e55f1c8b8c44e94161a7f14c812adcebeaa902204b9df40096337eb43120408d4743693b17ea27fc0f660948a0a761264e67a6b00121036ec451bd1d3d39bb8d2d0264dd5a0ff817ae306ec7ce0c7c9b86be3f3dde8727",
					},
					Txid:     "9acab5f13cf94074e75f5686b59fccd938f54b5f20ddddfcb6077c679a13c0ea",
					Vout:     1,
					Sequence: 4294967294,
				},
				{
					ScriptSig: bchain.ScriptSig{
						Hex: "4830450221009008a10ebebf45f1a9aeb01a5b49a5de820a1a396ca1b900ea0420a1c2c49ea7022053fad044ff42792dcad16d084d67d7c4cc565458f78a14292fe5ef5e9f7a20bb012103448301b9253e6822ae05af64c61517fe8cf8b22590cc7d0b308b7f5cbce26db1",
					},
					Txid:     "98f202c811b38e3c3323f03bbdd3c9d14a01f973ec230ba6b54a63629ed41e6a",
					Vout:     0,
					Sequence: 4294967294,
				},
			},
			Vout: []bchain.Vout{
				{
					Value: 3.0,
					N:     0,
					ScriptPubKey: bchain.ScriptPubKey{
						Hex: "76a9149bc587e2cdd13b78e26f601f084ea0597b357f4d88ac",
						Addresses: []string{
							"tmPuzhaxL3vzTqkyJ6wn48Q4ZfAo6PBvHKC",
						},
					},
					Address: addr3,
				},
				{
					Value: 0.44070449,
					N:     1,
					ScriptPubKey: bchain.ScriptPubKey{
						Hex: "76a91414a88f1032c8ef1c6bd9177f62cdb260b5087b5488ac",
						Addresses: []string{
							"tmBbamgVqVBTAsEzPmEymbczdoaQvE5qwaR",
						},
					},
					Address: addr4,
				},
			},
		},
	}
}

func TestZCashRPC_GetBlockHash(t *testing.T) {
	cli, err := getRPCClient()
	if err != nil {
		t.Fatal(err)
	}

	hash, err := cli.GetBlockHash(blockHeight)
	if err != nil {
		t.Error(err)
		return
	}

	if hash != blockHash {
		t.Errorf("GetBlockHash() got %q, want %q", hash, blockHash)
	}
}

func TestZCashRPC_GetBlockRaw(t *testing.T) {
	cli, err := getRPCClient()
	if err != nil {
		t.Fatal(err)
	}

	d, err := cli.GetBlockRaw(blockHash)
	if err != nil {
		t.Error(err)
		return
	}

	blk := hex.EncodeToString(d)

	if blk != blockHex {
		t.Errorf("GetBlockRaw() got %q, want %q", blk, blockHex)
	}
}

func TestZCashRPC_GetBlock(t *testing.T) {
	cli, err := getRPCClient()
	if err != nil {
		t.Fatal(err)
	}

	blk, err := cli.GetBlock(blockHash, 0)
	if err != nil {
		t.Error(err)
		return
	}

	if len(blk.Txs) != len(blockTxs) {
		t.Errorf("GetBlock() number of transactions: got %d, want %d", len(blk.Txs), len(blockTxs))
	}

	for ti, tx := range blk.Txs {
		if tx.Txid != blockTxs[ti] {
			t.Errorf("GetBlock() transaction %d: got %s, want %s", ti, tx.Txid, blockTxs[ti])
		}
	}

}

func TestZCashRPC_GetTransaction(t *testing.T) {
	cli, err := getRPCClient()
	if err != nil {
		t.Fatal(err)
	}

	for txid, want := range txDetails {
		got, err := cli.GetTransaction(txid)
		if err != nil {
			t.Error(err)
			return
		}

		// Confirmations is variable field, we just check if is set and reset it
		if got.Confirmations > 0 {
			got.Confirmations = 0
		} else {
			t.Errorf("GetTransaction() has empty Confirmations field")
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetTransaction() got %v, want %v", got, want)
		}
	}
}
