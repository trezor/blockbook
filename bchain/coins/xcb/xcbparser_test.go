package xcb

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/core-coin/go-core/v2/common"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestXcbParser_GetAddrDescFromAddress(t *testing.T) {
	type args struct {
		address string
	}
	tests := []struct {
		name      string
		args      args
		want      string
		wantErr   bool
		networkID int32
	}{
		{
			name:      "with ab prefix",
			args:      args{address: "ab57dde1a47041fc3c570c0318a713128ced55fd2ada"},
			want:      "ab57dde1a47041fc3c570c0318a713128ced55fd2ada",
			networkID: 3,
		},
		{
			name:      "with cb prefix",
			args:      args{address: "cb79fbc0290a1a3cf017f702e604ba234568533110af"},
			want:      "cb79fbc0290a1a3cf017f702e604ba234568533110af",
			networkID: 1,
		},
		{
			name:    "address of wrong length",
			args:    args{address: "7526228d673e9f079630d6cdaff5a2ed13e0e60"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "ErrAddressMissing",
			args:    args{address: ""},
			want:    "",
			wantErr: true,
		},
		{
			name:    "error - not xcb address",
			args:    args{address: "1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		if tt.networkID != 0 {
			common.DefaultNetworkID = common.NetworkID(tt.networkID)
		}
		t.Run(tt.name, func(t *testing.T) {
			p := NewCoreCoinParser(1)
			got, err := p.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("XcbParser.GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("XcbParser.GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

var testTx1, testTx2, testTx3, testTx1Failed, testTx1NoStatus bchain.Tx

func init() {

	testTx1 = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"cb656dadee521bea601692312454a655a0f49051ddc9"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"cb79fbc0290a1a3cf017f702e604ba234568533110af"},
				},
			},
		},
		CoinSpecificData: CoreCoinSpecificData{
			Tx: &RpcTransaction{
				AccountNonce:     "0xb26c",
				EnergyPrice:      "0x430e23400",
				EnergyLimit:      "0x5208",
				To:               "cb79fbc0290a1a3cf017f702e604ba234568533110af",
				Value:            "0x1bc0159d530e6000",
				Payload:          "0x",
				Hash:             "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
				BlockNumber:      "0x41eee8",
				From:             "cb656dadee521bea601692312454a655a0f49051ddc9",
				TransactionIndex: "0xa",
			},
			Receipt: &RpcReceipt{
				EnergyUsed: "0x5208",
				Status:     "0x1",
				Logs:       []*RpcLog{},
			},
		},
	}

	testTx2 = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0x6fc698f1f6037551826fd86fa1b77c27a16c62f8916f9fe9942cd89b2fc8118a",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"ab44de35413ee2b672d322938e2fcc932d5c0cf8ec88"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"ab98e5e2ba00469ce51440c22d4d4b79a56da712297f"},
				},
			},
		},
		CoinSpecificData: CoreCoinSpecificData{
			Tx: &RpcTransaction{
				AccountNonce:     "0x3a0",
				EnergyPrice:      "0x3b9aca00",
				EnergyLimit:      "0x941a",
				To:               "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
				Value:            "0x0",
				Payload:          "0xe86e7c5f00000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000100000000000000000000ab27b691efe91718cb73207207d92dbd175e6b10c7560000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000002b5e3af16b1880000",
				Hash:             "0x6fc698f1f6037551826fd86fa1b77c27a16c62f8916f9fe9942cd89b2fc8118a",
				BlockNumber:      "0x48b929",
				From:             "ab44de35413ee2b672d322938e2fcc932d5c0cf8ec88",
				TransactionIndex: "0x0",
			},
			Receipt: &RpcReceipt{
				EnergyUsed: "0x941a",
				Status:     "0x1",
				Logs: []*RpcLog{
					{
						Address: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
						Data:    "0x000000000000000000000000000000000000000000000002b5e3af16b1880000",
						Topics: []string{
							"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
							"0x00000000000000000000ab44de35413ee2b672d322938e2fcc932d5c0cf8ec88",
							"0x00000000000000000000ab27b691efe91718cb73207207d92dbd175e6b10c756",
						},
					},
				},
			},
		},
	}

	testTx3 = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0x4f65e846f570bb121b959bd37fbe57f4a6a61598095cbc4c6eaaa66aed7f66bd",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"ab228a4d4263e067df56b1dd226acb939f532ff7ab5b	"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"ab98e5e2ba00469ce51440c22d4d4b79a56da712297f"},
				},
			},
		},
		CoinSpecificData: CoreCoinSpecificData{
			Tx: &RpcTransaction{
				AccountNonce:     "0x3",
				EnergyPrice:      "0x3b9aca00",
				EnergyLimit:      "0x8c80",
				To:               "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
				Value:            "0x0",
				Payload:          "0x4b40e90100000000000000000000ab094a15c3dc43095c7450c59bf56263e9827065f3060000000000000000000000000000000000000000000000000de0b6b3a7640000",
				Hash:             "0x4f65e846f570bb121b959bd37fbe57f4a6a61598095cbc4c6eaaa66aed7f66bd",
				BlockNumber:      "0x48c147",
				From:             "ab228a4d4263e067df56b1dd226acb939f532ff7ab5b",
				TransactionIndex: "0x0",
			},
			Receipt: &RpcReceipt{
				EnergyUsed: "0x8c80",
				Status:     "0x1",
				Logs: []*RpcLog{
					{
						Address: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
						Data:    "0x0000000000000000000000000000000000000000000000000de0b6b3a7640000",
						Topics: []string{
							"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
							"0x00000000000000000000ab228a4d4263e067df56b1dd226acb939f532ff7ab5b",
							"0x00000000000000000000ab094a15c3dc43095c7450c59bf56263e9827065f306",
						},
					},
				},
			},
		},
	}

	testTx1Failed = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"cb656dadee521bea601692312454a655a0f49051ddc9"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"cb79fbc0290a1a3cf017f702e604ba234568533110af"},
				},
			},
		},
		CoinSpecificData: CoreCoinSpecificData{
			Tx: &RpcTransaction{
				AccountNonce:     "0xb26c",
				EnergyPrice:      "0x430e23400",
				EnergyLimit:      "0x5208",
				To:               "cb79fbc0290a1a3cf017f702e604ba234568533110af",
				Value:            "0x1bc0159d530e6000",
				Payload:          "0x",
				Hash:             "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
				BlockNumber:      "0x41eee8",
				From:             "cb656dadee521bea601692312454a655a0f49051ddc9",
				TransactionIndex: "0xa",
			},
			Receipt: &RpcReceipt{
				EnergyUsed: "0x5208",
				Status:     "0x0",
				Logs:       []*RpcLog{},
			},
		},
	}

	testTx1NoStatus = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"cb656dadee521bea601692312454a655a0f49051ddc9"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"cb79fbc0290a1a3cf017f702e604ba234568533110af"},
				},
			},
		},
		CoinSpecificData: CoreCoinSpecificData{
			Tx: &RpcTransaction{
				AccountNonce:     "0xb26c",
				EnergyPrice:      "0x430e23400",
				EnergyLimit:      "0x5208",
				To:               "cb79fbc0290a1a3cf017f702e604ba234568533110af",
				Value:            "0x1bc0159d530e6000",
				Payload:          "0x",
				Hash:             "0x998d535fb50fc55eafc591c20acf9ae13cebb96676fe90fcd136ea1f94113520",
				BlockNumber:      "0x41eee8",
				From:             "cb656dadee521bea601692312454a655a0f49051ddc9",
				TransactionIndex: "0xa",
			},
			Receipt: &RpcReceipt{
				EnergyUsed: "0x5208",
				Status:     "",
				Logs:       []*RpcLog{},
			},
		},
	}

}

func TestCoreCoinParser_PackTx(t *testing.T) {
	type args struct {
		tx        *bchain.Tx
		height    uint32
		blockTime int64
	}
	tests := []struct {
		name      string
		p         *CoreCoinParser
		args      args
		want      string
		wantErr   bool
		networkID common.NetworkID
	}{
		{
			name: "1",
			args: args{
				tx:        &testTx1,
				height:    4321000,
				blockTime: 1534858022,
			},
			want:      dbtestdata.XcbTx1Packed,
			networkID: common.NetworkID(1),
		},
		{
			name: "2",
			args: args{
				tx:        &testTx2,
				height:    4321000,
				blockTime: 1534858022,
			},
			want:      dbtestdata.XcbTx2Packed,
			networkID: common.NetworkID(3),
		},
		{
			name: "3",
			args: args{
				tx:        &testTx1Failed,
				height:    4321000,
				blockTime: 1534858022,
			},
			want:      dbtestdata.XcbTx1FailedPacked,
			networkID: common.NetworkID(1),
		},
		{
			name: "4",
			args: args{
				tx:        &testTx1NoStatus,
				height:    4321000,
				blockTime: 1534858022,
			},
			want:      dbtestdata.XcbTx1NoStatusPacked,
			networkID: common.NetworkID(1),
		},
		{
			name: "4",
			args: args{
				tx:        &testTx3,
				height:    4321000,
				blockTime: 1534858022,
			},
			want:      dbtestdata.XcbTx3Packed,
			networkID: common.NetworkID(3),
		},
	}
	p := NewCoreCoinParser(1)
	for _, tt := range tests {
		if tt.networkID != 0 {
			common.DefaultNetworkID = tt.networkID
		}
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.PackTx(tt.args.tx, tt.args.height, tt.args.blockTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("CoreCoinParser.PackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("CoreCoinParser.PackTx() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestCoreCoinParser_UnpackTx(t *testing.T) {
	type args struct {
		hex string
	}
	tests := []struct {
		name      string
		p         *CoreCoinParser
		args      args
		want      *bchain.Tx
		want1     uint32
		wantErr   bool
		networkID common.NetworkID
	}{
		{
			name:      "1",
			args:      args{hex: dbtestdata.XcbTx1Packed},
			want:      &testTx1,
			want1:     4321000,
			networkID: common.NetworkID(1),
		},
		{
			name:      "2",
			args:      args{hex: dbtestdata.XcbTx2Packed},
			want:      &testTx2,
			want1:     4765993,
			networkID: common.NetworkID(3),
		},
		{
			name:      "3",
			args:      args{hex: dbtestdata.XcbTx1FailedPacked},
			want:      &testTx1Failed,
			want1:     4321000,
			networkID: common.NetworkID(1),
		},
		{
			name:      "4",
			args:      args{hex: dbtestdata.XcbTx1NoStatusPacked},
			want:      &testTx1NoStatus,
			want1:     4321000,
			networkID: common.NetworkID(1),
		},
	}
	p := NewCoreCoinParser(1)
	for _, tt := range tests {
		if tt.networkID != 0 {
			common.DefaultNetworkID = tt.networkID
		}
		t.Run(tt.name, func(t *testing.T) {
			b, err := hex.DecodeString(tt.args.hex)
			if err != nil {
				panic(err)
			}
			got, got1, err := p.UnpackTx(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("CoreCoinParser.UnpackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// DeepEqual has problems with pointers in CoreCoinSpecificData
			gs := got.CoinSpecificData.(CoreCoinSpecificData)
			ws := tt.want.CoinSpecificData.(CoreCoinSpecificData)
			gc := *got
			wc := *tt.want
			gc.CoinSpecificData = nil
			wc.CoinSpecificData = nil
			if fmt.Sprint(gc) != fmt.Sprint(wc) {
				// if !reflect.DeepEqual(gc, wc) {
				t.Errorf("CoreCoinParser.UnpackTx() gc got = %+v, want %+v", gc, wc)
			}
			if !reflect.DeepEqual(gs.Tx, ws.Tx) {
				t.Errorf("CoreCoinParser.UnpackTx() gs.Tx got = %+v, want %+v", gs.Tx, ws.Tx)
			}
			if !reflect.DeepEqual(gs.Receipt, ws.Receipt) {
				t.Errorf("CoreCoinParser.UnpackTx() gs.Receipt got = %+v, want %+v", gs.Receipt, ws.Receipt)
			}
			if got1 != tt.want1 {
				t.Errorf("CoreCoinParser.UnpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestCoreCoinParser_GetCoreblockchainTxData(t *testing.T) {
	tests := []struct {
		name string
		tx   *bchain.Tx
		want string
	}{
		{
			name: "Test empty data",
			tx:   &testTx1,
			want: "0x",
		},
		{
			name: "Test non empty data",
			tx:   &testTx2,
			want: "0xe86e7c5f00000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000100000000000000000000ab27b691efe91718cb73207207d92dbd175e6b10c7560000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000002b5e3af16b1880000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCoreCoinTxData(tt.tx)
			if got.Data != tt.want {
				t.Errorf("CoreCoinParser.GetCoreCoinTxData() = %v, want %v", got.Data, tt.want)
			}
		})
	}
}
