// build unittest

package viacoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"reflect"
	"testing"
)

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
			args:    args{address: "VqWr3gWBuKpABimDDBnsbxge26MSqUs5rg"},
			want:    "76a914aa3750aa18b8a0f3f0590731e1fab934856680cf88ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "VhyGT8kJU9x28dHwjf1jEDG8gMY8yhckDR"},
			want:    "76a91457757edd001d16528c7aa337b314a7bab303ee8088ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "ESqJAgbiBMEt578P5gqKVMaQyyxH8if6Gh"},
			want:    "a9146a2c482f4985f57e702f325816c90e3723ca81ae87",Viacoin
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "EYFswAbuZyWoSmc9p7q23taz83FtabyTAt"},
			want:    "a914a5ab14c9804d0d8bf02f1aea4e82780733ad0a8387",
			wantErr: false,
		},
		
	}
	parser := NewViacoinParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.AddressToOutputScript(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressToOutputScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("AddressToOutputScript() = %v, want %v", h, tt.want)
			}
		})
	}
}

var (
	testTx1 bchain.Tx

	testTxPacked1 = "000e87768bb386b878010000000146fd781834a34e0399ccda1edf9ec47d715e17d904ad0958d533a240b3605ad6000000006a473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3cfdffffff020882a400000000001976a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac80f0fa020000000017a91446eb90e002f137f05385896c882fe000cc2e967f8774870e00"
)

func init() {
	var (
		addr1, addr2 bchain.Address
		err          error
	)
	addr1, err = bchain.NewBaseAddress("Vp1UqzsmVecaexfbWFGSFFL5x1g2XQnrGR")
	if err == nil {
		addr2, err = bchain.NewBaseAddress("38A1RNvbA5c9wNRfyLVn1FCH5TPKJVG8YR")
	}
	if err != nil {
		panic(err)
	}

	testTx1 = bchain.Tx{
		Hex:       "010000000146fd781834a34e0399ccda1edf9ec47d715e17d904ad0958d533a240b3605ad6000000006a473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3cfdffffff020882a400000000001976a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac80f0fa020000000017a91446eb90e002f137f05385896c882fe000cc2e967f8774870e00",
		Blocktime: 1529925180,
		Txid:      "d58c11aa970449c3e0ee5e0cdf78532435a9d2b28a2da284a8dd4dd6bdd0331c",
		LockTime:  952180,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022026b352a0c35c232342339e2b50ec9f04587b990d5213174e368cc76dc82686f002207d0787461ad846825872a50d3d6fc748d5a836575c1daf6ad0ca602f9c4a8826012103d36b6b829c571ed7caa565eca9bdc2aa36519b7ab8551ace5edb0356d477ad3c",
				},
				Txid:     "d65a60b340a233d55809ad04d9175e717dc49edf1edacc99034ea3341878fd46",
				Vout:     0,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				Value: 0.10781192,
				N:     0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91499b16da88a7e29b913b6131df2644d6d06cb331b88ac",
					Addresses: []string{
						"Vp1UqzsmVecaexfbWFGSFFL5x1g2XQnrGR",
					},
				},
				Address: addr1,
			},
			{
				Value: 0.5000000,
				N:     1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a91446eb90e002f137f05385896c882fe000cc2e967f87",
					Addresses: []string{
						"38A1RNvbA5c9wNRfyLVn1FCH5TPKJVG8YR",
					},
				},
				Address: addr2,
			},
		},
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *ViacoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "viacoin-1",
			args: args{
				tx:        testTx1,
				height:    952182,
				blockTime: 1529925180,
				parser:    NewViacoinParser(GetChainParams("main"), &btc.Configuration{}),
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
		parser   *ViacoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "viacoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewViacoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   952182,
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
