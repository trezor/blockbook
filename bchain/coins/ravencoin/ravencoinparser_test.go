//go:build unittest

package ravencoin

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
			args:    args{address: "RAoGkGhKwzxLnstApumYPD2eTrAJ849cga"},
			want:    "76a91410a8805f1a6af1a5927088544b0b6ec7d6f0ab8b88ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "RTq37kPJqMS36tZYunxo2abrBMLeYSCAaa"},
			want:    "76a914cb78181d62d312fdb9aacca433570150dcf0dec288ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "rCzjkBoY2duVn2WizKxfBedTVWAg6UhfLZ"},
			want:    "a9144a2a40987c74578ee517d426aa2c43fc568f7e0887",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "rDzGemZkv9FbDDh5pvWfr7TWtMUnNRRE7T"},
			want:    "a914550bc2fcc1992afade4d298326ee6a03ab975a9387",
			wantErr: false,
		},
	}
	parser := NewRavencoinParser(GetChainParams("main"), &btc.Configuration{})

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
	testTx1       bchain.Tx
	testTxPacked1 = "0a20d4d3a093586eae0c3668fd288d9e24955928a894c20b551b38dd18c99b123a7c12e1010200000001c171348ffc8976074fa064e48598a816fce3798afc635fb67d99580e50b8e614000000006a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46affffffff02f8137114000000001976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ace0c48110000000001976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac0000000018c7e1b3e50528849128329401122014e6b8500e58997db65f63fc8a79e3fc16a89885e464a04f077689fc8f3471c1226a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46a28ffffffff0f3a450a04147113f81a1976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ac222252484d31746d64766b6b3776446f69477877554a414d4e4e6d447179775a3574456e3a470a041081c4e010011a1976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac2222525631463939623955424272434d38614e4b7567737173444d3869716f4371374d744002"

	testTx2       bchain.Tx
	testTxPacked2 = "0a208e480d5c1bf7f11d1cbe396ab7dc14e01ea4e1aff45de7c055924f61304ad43412f40202000000029e2e14113b2f55726eebaa440edec707fcec3a31ce28fa125afea1e755fb6850010000006a47304402204034c3862f221551cffb2aa809f621f989a75cdb549c789a5ceb3a82c0bcc21c022001b4638f5d73fdd406a4dd9bf99be3dfca4a572b8f40f09b8fd495a7756c0db70121027a32ef45aef2f720ccf585f6fb0b8a7653db89cacc3320e5b385146851aba705fefffffff3b240ae32c542786876fcf23b4b2ab4c34ef077912898ee529756ed4ba35910000000006a47304402204d442645597b13abb85e96e5acd34eff50a4418822fe6a37ed378cdd24574dff02205ae667c56eab63cc45a51063f15b72136fd76e97c46af29bd28e8c4d405aa211012102cde27d7b29331ea3fef909a8d91f6f7753e99a3dd129914be50df26eed73fab3feffffff028447bf38000000001976a9146d7badec5426b880df25a3afc50e476c2423b34b88acb26b556a740000001976a914b3020d0ab85710151fa509d5d9a4e783903d681888ac83080a0018c7e1b3e505208391282884912832960112205068fb55e7a1fe5a12fa28ce313aecfc07c7de0e44aaeb6e72552f3b11142e9e1801226a47304402204034c3862f221551cffb2aa809f621f989a75cdb549c789a5ceb3a82c0bcc21c022001b4638f5d73fdd406a4dd9bf99be3dfca4a572b8f40f09b8fd495a7756c0db70121027a32ef45aef2f720ccf585f6fb0b8a7653db89cacc3320e5b385146851aba70528feffffff0f32940112201059a34bed569752ee98289177f04ec3b42a4b3bf2fc76687842c532ae40b2f3226a47304402204d442645597b13abb85e96e5acd34eff50a4418822fe6a37ed378cdd24574dff02205ae667c56eab63cc45a51063f15b72136fd76e97c46af29bd28e8c4d405aa211012102cde27d7b29331ea3fef909a8d91f6f7753e99a3dd129914be50df26eed73fab328feffffff0f3a450a0438bf47841a1976a9146d7badec5426b880df25a3afc50e476c2423b34b88ac2222524b4735747057776a6874716464546741335168556837516d4b637576426e6842583a480a05746a556bb210011a1976a914b3020d0ab85710151fa509d5d9a4e783903d681888ac222252526268564d624c6675657a485077554d756a546d4446417a76363459396d4a71644002"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "0200000001c171348ffc8976074fa064e48598a816fce3798afc635fb67d99580e50b8e614000000006a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46affffffff02f8137114000000001976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ace0c48110000000001976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac00000000",
		Blocktime: 1554837703,
		Time:      1554837703,
		Txid:      "d4d3a093586eae0c3668fd288d9e24955928a894c20b551b38dd18c99b123a7c",
		LockTime:  0,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46a",
				},
				Txid:     "14e6b8500e58997db65f63fc8a79e3fc16a89885e464a04f077689fc8f3471c1",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(342955000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ac",
					Addresses: []string{
						"RHM1tmdvkk7vDoiGxwUJAMNNmDqywZ5tEn",
					},
				},
			},
			{
				ValueSat: *big.NewInt(276940000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac",
					Addresses: []string{
						"RV1F99b9UBBrCM8aNKugsqsDM8iqoCq7Mt",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "02000000029e2e14113b2f55726eebaa440edec707fcec3a31ce28fa125afea1e755fb6850010000006a47304402204034c3862f221551cffb2aa809f621f989a75cdb549c789a5ceb3a82c0bcc21c022001b4638f5d73fdd406a4dd9bf99be3dfca4a572b8f40f09b8fd495a7756c0db70121027a32ef45aef2f720ccf585f6fb0b8a7653db89cacc3320e5b385146851aba705fefffffff3b240ae32c542786876fcf23b4b2ab4c34ef077912898ee529756ed4ba35910000000006a47304402204d442645597b13abb85e96e5acd34eff50a4418822fe6a37ed378cdd24574dff02205ae667c56eab63cc45a51063f15b72136fd76e97c46af29bd28e8c4d405aa211012102cde27d7b29331ea3fef909a8d91f6f7753e99a3dd129914be50df26eed73fab3feffffff028447bf38000000001976a9146d7badec5426b880df25a3afc50e476c2423b34b88acb26b556a740000001976a914b3020d0ab85710151fa509d5d9a4e783903d681888ac83080a00",
		Blocktime: 1554837703,
		Time:      1554837703,
		Txid:      "8e480d5c1bf7f11d1cbe396ab7dc14e01ea4e1aff45de7c055924f61304ad434",
		LockTime:  657539,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402204034c3862f221551cffb2aa809f621f989a75cdb549c789a5ceb3a82c0bcc21c022001b4638f5d73fdd406a4dd9bf99be3dfca4a572b8f40f09b8fd495a7756c0db70121027a32ef45aef2f720ccf585f6fb0b8a7653db89cacc3320e5b385146851aba705",
				},
				Txid:     "5068fb55e7a1fe5a12fa28ce313aecfc07c7de0e44aaeb6e72552f3b11142e9e",
				Vout:     1,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402204d442645597b13abb85e96e5acd34eff50a4418822fe6a37ed378cdd24574dff02205ae667c56eab63cc45a51063f15b72136fd76e97c46af29bd28e8c4d405aa211012102cde27d7b29331ea3fef909a8d91f6f7753e99a3dd129914be50df26eed73fab3",
				},
				Txid:     "1059a34bed569752ee98289177f04ec3b42a4b3bf2fc76687842c532ae40b2f3",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(952059780),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9146d7badec5426b880df25a3afc50e476c2423b34b88ac",
					Addresses: []string{
						"RKG5tpWwjhtqddTgA3QhUh7QmKcuvBnhBX",
					},
				},
			},
			{
				ValueSat: *big.NewInt(500000189362),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914b3020d0ab85710151fa509d5d9a4e783903d681888ac",
					Addresses: []string{
						"RRbhVMbLfuezHPwUMujTmDFAzv64Y9mJqd",
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
		parser    *RavencoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "ravencoin-1",
			args: args{
				tx:        testTx1,
				height:    657540,
				blockTime: 1554837703,
				parser:    NewRavencoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "ravencoin-2",
			args: args{
				tx:        testTx2,
				height:    657540,
				blockTime: 1554837703,
				parser:    NewRavencoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *RavencoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "ravencoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewRavencoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   657540,
			wantErr: false,
		},
		{
			name: "ravencoin-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewRavencoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   657540,
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
