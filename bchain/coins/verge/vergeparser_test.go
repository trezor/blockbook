// +build unittest

package verge

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func TestAddressToOutputScript_Mainnet(t *testing.T) {
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
			name:    "pubkeyhash1",
			args:    args{address: "DPw9hfaW4FJVE1Xy55NeUHNcukaAnnZLWj"},
			want:    "76a914ce2809bbb7fedefa334740afc6b37b499880c2e488ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "D9CkQHjZa1qVSer2e1iUNNwskrGVTReNJG"},
			want:    "76a9142c915b6cc7aafcc10cd5e81c3322a3e26a30144588ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "ESJcHWmXLAuteMNY3n6jLmF7WfLZui5cKj"},
			want:    "a914645e82d7e183697aa89e1115f604fe8325e2bec187",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "ESJcHWmXLAuteMNY3n6jLmF7WfLZui5cKj"},
			want:    "a914645e82d7e183697aa89e1115f604fe8325e2bec187",
			wantErr: false,
		},
	}
	parser := NewVergeParser(GetChainParams("main"), &btc.Configuration{})

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

var (
	testTx1 bchain.Tx

	testTxPacked1 = "0a209f6e7e5d86a1b30b6007f80c63a6400c1ff2fc4cdb227dbd400658b6cd2c849212f9050100000080d9eb5d068a9a8c71c3dd5a466efc9f7fe58da1d1b6aee623dcd37f5d6b23819557ec8068000000004847304402206005ba9744db429df13e4d9bea9f15e7ccd303c9b3e5f18fc5ced835bfcfacf30220450a4d3a403355d9af6e2ffb8bddc851c57c28df57de19e12a5cf6de6c2cd79f01feffffff39d52b7194cba1cd2dccba3f44b28a9ab606cdd0bc8225ed10c113e6bca917b70000000048473044022043c5266ea6f327abba0b1f3c3335c387a7de039a243cd2faf04ca83f8501965f02200cdc507081abcbb94c416fb741619471bca173244a140c821644afe8be89850901feffffffff2a9a5671b569d68e0111e05c1373536c37c8fc4c170ae24deced9f13d984540000000049483045022100ca35746adfd219f13fdf3880242862b3c481f0ac4d99bcdfbb06956cc7e5759c022031d997b002b3031f02986ad86a12ca0e47e07c3c74a010e8c72f53a86c1caf8501feffffffeb7fa51ba265b831c0950d680a113eced2f2c095c02aad563fbf7d99501d80be00000000484730440220294a7c105fd8d0ae0ab8ac96365653b76db59d74679281e278355a5192aec8a602200138606184ec251fec97f3e985afdce82f9f79d17ef3f55e4ec6be815bd3ccee01feffffff94d0227258e0986d8238b1c4ad6882c4e54a6df6a23514538102ddda5d6b9c4c0000000048473044022079a11767dd28bc5e9a3b65b1153d8425ee8cfd5ed5035135237d62b33fbd544102203c06c710231f3f124447a5a775302261110fec22b7fc07339ea7f69e9e40cce301feffffffb9151c33d05473df326ab9c9536d6524007e324e7afe2e3c0f137466e410b7f900000000484730440220204eeb4e35dd6842913ac530b73a762119b3364aeab93228924e6a7cecb5c882022025b8a993c19e8fe2e6987bbbe26f966b5d8579eb85e1a491da57b979dd75a42701feffffff02fe924de7000000001976a91435126d123ade9c71cb0ec4af3efb3368a5f6678f88acf23bc51d000000001976a914dea175a278d53c05963bae3392801b08bf9187c688accbc9370018a8b9ccee0520cb93df0128cd93df0132760a0012206880ec579581236b5d7fd3dc23e6aeb6d1a18de57f9ffc6e465addc3718c9a8a1800224847304402206005ba9744db429df13e4d9bea9f15e7ccd303c9b3e5f18fc5ced835bfcfacf30220450a4d3a403355d9af6e2ffb8bddc851c57c28df57de19e12a5cf6de6c2cd79f0128feffffff0f32760a001220b717a9bce613c110ed2582bcd0cd06b69a8ab2443fbacc2dcda1cb94712bd53918002248473044022043c5266ea6f327abba0b1f3c3335c387a7de039a243cd2faf04ca83f8501965f02200cdc507081abcbb94c416fb741619471bca173244a140c821644afe8be8985090128feffffff0f32770a0012205484d9139fedec4de20a174cfcc8376c5373135ce011018ed669b571569a2aff18002249483045022100ca35746adfd219f13fdf3880242862b3c481f0ac4d99bcdfbb06956cc7e5759c022031d997b002b3031f02986ad86a12ca0e47e07c3c74a010e8c72f53a86c1caf850128feffffff0f32760a001220be801d50997dbf3f56ad2ac095c0f2d2ce3e110a680d95c031b865a21ba57feb180022484730440220294a7c105fd8d0ae0ab8ac96365653b76db59d74679281e278355a5192aec8a602200138606184ec251fec97f3e985afdce82f9f79d17ef3f55e4ec6be815bd3ccee0128feffffff0f32760a0012204c9c6b5ddadd0281531435a2f66d4ae5c48268adc4b138826d98e0587222d09418002248473044022079a11767dd28bc5e9a3b65b1153d8425ee8cfd5ed5035135237d62b33fbd544102203c06c710231f3f124447a5a775302261110fec22b7fc07339ea7f69e9e40cce30128feffffff0f32760a001220f9b710e46674130f3c2efe7a4e327e0024656d53c9b96a32df7354d0331c15b9180022484730440220204eeb4e35dd6842913ac530b73a762119b3364aeab93228924e6a7cecb5c882022025b8a993c19e8fe2e6987bbbe26f966b5d8579eb85e1a491da57b979dd75a4270128feffffff0f3a470a04e74d92fe10001a1976a91435126d123ade9c71cb0ec4af3efb3368a5f6678f88ac22224439796952683137504c4842427239526b507067434c5a6a3763597352324436434e3a470a041dc53bf210011a1976a914dea175a278d53c05963bae3392801b08bf9187c688ac22224452534679464d4d4b32684d33387664424c684569463463705259666f614d6d7a634001"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0100000080d9eb5d068a9a8c71c3dd5a466efc9f7fe58da1d1b6aee623dcd37f5d6b23819557ec8068000000004847304402206005ba9744db429df13e4d9bea9f15e7ccd303c9b3e5f18fc5ced835bfcfacf30220450a4d3a403355d9af6e2ffb8bddc851c57c28df57de19e12a5cf6de6c2cd79f01feffffff39d52b7194cba1cd2dccba3f44b28a9ab606cdd0bc8225ed10c113e6bca917b70000000048473044022043c5266ea6f327abba0b1f3c3335c387a7de039a243cd2faf04ca83f8501965f02200cdc507081abcbb94c416fb741619471bca173244a140c821644afe8be89850901feffffffff2a9a5671b569d68e0111e05c1373536c37c8fc4c170ae24deced9f13d984540000000049483045022100ca35746adfd219f13fdf3880242862b3c481f0ac4d99bcdfbb06956cc7e5759c022031d997b002b3031f02986ad86a12ca0e47e07c3c74a010e8c72f53a86c1caf8501feffffffeb7fa51ba265b831c0950d680a113eced2f2c095c02aad563fbf7d99501d80be00000000484730440220294a7c105fd8d0ae0ab8ac96365653b76db59d74679281e278355a5192aec8a602200138606184ec251fec97f3e985afdce82f9f79d17ef3f55e4ec6be815bd3ccee01feffffff94d0227258e0986d8238b1c4ad6882c4e54a6df6a23514538102ddda5d6b9c4c0000000048473044022079a11767dd28bc5e9a3b65b1153d8425ee8cfd5ed5035135237d62b33fbd544102203c06c710231f3f124447a5a775302261110fec22b7fc07339ea7f69e9e40cce301feffffffb9151c33d05473df326ab9c9536d6524007e324e7afe2e3c0f137466e410b7f900000000484730440220204eeb4e35dd6842913ac530b73a762119b3364aeab93228924e6a7cecb5c882022025b8a993c19e8fe2e6987bbbe26f966b5d8579eb85e1a491da57b979dd75a42701feffffff02fe924de7000000001976a91435126d123ade9c71cb0ec4af3efb3368a5f6678f88acf23bc51d000000001976a914dea175a278d53c05963bae3392801b08bf9187c688accbc93700",
		Blocktime: 1574116520,
		Time:      1574116520,
		Txid:      "9f6e7e5d86a1b30b6007f80c63a6400c1ff2fc4cdb227dbd400658b6cd2c8492",
		LockTime:  3656139,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402206005ba9744db429df13e4d9bea9f15e7ccd303c9b3e5f18fc5ced835bfcfacf30220450a4d3a403355d9af6e2ffb8bddc851c57c28df57de19e12a5cf6de6c2cd79f01",
				},
				Txid:     "6880ec579581236b5d7fd3dc23e6aeb6d1a18de57f9ffc6e465addc3718c9a8a",
				Vout:     0,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022043c5266ea6f327abba0b1f3c3335c387a7de039a243cd2faf04ca83f8501965f02200cdc507081abcbb94c416fb741619471bca173244a140c821644afe8be89850901",
				},
				Txid:     "b717a9bce613c110ed2582bcd0cd06b69a8ab2443fbacc2dcda1cb94712bd539",
				Vout:     0,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100ca35746adfd219f13fdf3880242862b3c481f0ac4d99bcdfbb06956cc7e5759c022031d997b002b3031f02986ad86a12ca0e47e07c3c74a010e8c72f53a86c1caf8501",
				},
				Txid:     "5484d9139fedec4de20a174cfcc8376c5373135ce011018ed669b571569a2aff",
				Vout:     0,
				Sequence: 4294967294,
			},

			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220294a7c105fd8d0ae0ab8ac96365653b76db59d74679281e278355a5192aec8a602200138606184ec251fec97f3e985afdce82f9f79d17ef3f55e4ec6be815bd3ccee01",
				},
				Txid:     "be801d50997dbf3f56ad2ac095c0f2d2ce3e110a680d95c031b865a21ba57feb",
				Vout:     0,
				Sequence: 4294967294,
			},

			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022079a11767dd28bc5e9a3b65b1153d8425ee8cfd5ed5035135237d62b33fbd544102203c06c710231f3f124447a5a775302261110fec22b7fc07339ea7f69e9e40cce301",
				},
				Txid:     "4c9c6b5ddadd0281531435a2f66d4ae5c48268adc4b138826d98e0587222d094",
				Vout:     0,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220204eeb4e35dd6842913ac530b73a762119b3364aeab93228924e6a7cecb5c882022025b8a993c19e8fe2e6987bbbe26f966b5d8579eb85e1a491da57b979dd75a42701",
				},
				Txid:     "f9b710e46674130f3c2efe7a4e327e0024656d53c9b96a32df7354d0331c15b9",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(3880620798),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91435126d123ade9c71cb0ec4af3efb3368a5f6678f88ac",
					Addresses: []string{
						"D9yiRh17PLHBBr9RkPpgCLZj7cYsR2D6CN",
					},
				},
			},
			{
				ValueSat: *big.NewInt(499465202),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914dea175a278d53c05963bae3392801b08bf9187c688ac",
					Addresses: []string{
						"DRSFyFMMK2hM38vdBLhEiF4cpRYfoaMmzc",
					},
				},
			},
		},
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *VergeParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "verge-1",
			args: args{
				tx:        testTx1,
				blockTime: 1574116520,
				height:    3656141,
				parser:    NewVergeParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *VergeParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "verge-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewVergeParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   3656141,
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
