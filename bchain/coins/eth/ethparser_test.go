//go:build unittest

package eth

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestEthParser_GetAddrDescFromAddress(t *testing.T) {
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
			name: "with 0x prefix",
			args: args{address: "0x81b7e08f65bdf5648606c89998a9cc8164397647"},
			want: "81b7e08f65bdf5648606c89998a9cc8164397647",
		},
		{
			name: "without 0x prefix",
			args: args{address: "47526228d673e9f079630d6cdaff5a2ed13e0e60"},
			want: "47526228d673e9f079630d6cdaff5a2ed13e0e60",
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
			name:    "error - not eth address",
			args:    args{address: "1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewEthereumParser(1, false)
			got, err := p.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthParser.GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("EthParser.GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

var testTx1, testTx2, testTx1Failed, testTx1NoStatus bchain.Tx

func init() {

	testTx1 = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x3E3a3D69dc66bA10737F531ed088954a9EC89d97"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f"},
				},
			},
		},
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{
				AccountNonce:         "0xb26c",
				GasPrice:             "0x430e23400",
				MaxPriorityFeePerGas: "0x430e23401",
				MaxFeePerGas:         "0x430e23402",
				BaseFeePerGas:        "0x430e23403",
				GasLimit:             "0x5208",
				To:                   "0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f",
				Value:                "0x1bc0159d530e6000",
				Payload:              "0x",
				Hash:                 "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
				BlockNumber:          "0x41eee8",
				From:                 "0x3E3a3D69dc66bA10737F531ed088954a9EC89d97",
				TransactionIndex:     "0xa",
			},
			Receipt: &bchain.RpcReceipt{
				GasUsed: "0x5208",
				Status:  "0x1",
				Logs:    []*bchain.RpcLog{},
			},
		},
	}

	testTx2 = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0xa9cd088aba2131000da6f38a33c20169baee476218deea6b78720700b895b101",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x20cD153de35D469BA46127A0C8F18626b59a256A"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x4af4114F73d1c1C903aC9E0361b379D1291808A2"},
				},
			},
		},
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{
				AccountNonce:         "0xd0",
				GasPrice:             "0x9502f9000",
				MaxPriorityFeePerGas: "0x9502f9001",
				MaxFeePerGas:         "0x9502f9002",
				BaseFeePerGas:        "0x9502f9003",
				GasLimit:             "0x130d5",
				To:                   "0x4af4114F73d1c1C903aC9E0361b379D1291808A2",
				Value:                "0x0",
				Payload:              "0xa9059cbb000000000000000000000000555ee11fbddc0e49a9bab358a8941ad95ffdb48f00000000000000000000000000000000000000000000021e19e0c9bab2400000",
				Hash:                 "0xa9cd088aba2131000da6f38a33c20169baee476218deea6b78720700b895b101",
				BlockNumber:          "0x41eee8",
				From:                 "0x20cD153de35D469BA46127A0C8F18626b59a256A",
				TransactionIndex:     "0x0"},
			Receipt: &bchain.RpcReceipt{
				GasUsed: "0xcb39",
				Status:  "0x1",
				Logs: []*bchain.RpcLog{
					{
						Address: "0x4af4114F73d1c1C903aC9E0361b379D1291808A2",
						Data:    "0x00000000000000000000000000000000000000000000021e19e0c9bab2400000",
						Topics: []string{
							"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
							"0x00000000000000000000000020cd153de35d469ba46127a0c8f18626b59a256a",
							"0x000000000000000000000000555ee11fbddc0e49a9bab358a8941ad95ffdb48f",
						},
					},
				},
			},
		},
	}

	testTx1Failed = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x3E3a3D69dc66bA10737F531ed088954a9EC89d97"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f"},
				},
			},
		},
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{
				AccountNonce:     "0xb26c",
				GasPrice:         "0x430e23400",
				GasLimit:         "0x5208",
				To:               "0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f",
				Value:            "0x1bc0159d530e6000",
				Payload:          "0x",
				Hash:             "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
				BlockNumber:      "0x41eee8",
				From:             "0x3E3a3D69dc66bA10737F531ed088954a9EC89d97",
				TransactionIndex: "0xa",
			},
			Receipt: &bchain.RpcReceipt{
				GasUsed: "0x5208",
				Status:  "0x0",
				Logs:    []*bchain.RpcLog{},
			},
		},
	}

	testTx1NoStatus = bchain.Tx{
		Blocktime: 1534858022,
		Time:      1534858022,
		Txid:      "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x3E3a3D69dc66bA10737F531ed088954a9EC89d97"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f"},
				},
			},
		},
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{
				AccountNonce:     "0xb26c",
				GasPrice:         "0x430e23400",
				GasLimit:         "0x5208",
				To:               "0x555Ee11FBDDc0E49A9bAB358A8941AD95fFDB48f",
				Value:            "0x1bc0159d530e6000",
				Payload:          "0x",
				Hash:             "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
				BlockNumber:      "0x41eee8",
				From:             "0x3E3a3D69dc66bA10737F531ed088954a9EC89d97",
				TransactionIndex: "0xa",
			},
			Receipt: &bchain.RpcReceipt{
				GasUsed: "0x5208",
				Status:  "",
				Logs:    []*bchain.RpcLog{},
			},
		},
	}

}

func TestEthereumParser_PackTx(t *testing.T) {
	type args struct {
		tx        *bchain.Tx
		height    uint32
		blockTime int64
	}
	tests := []struct {
		name    string
		p       *EthereumParser
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "1",
			args: args{
				tx:        &testTx1,
				height:    4321000,
				blockTime: 1534858022,
			},
			want: dbtestdata.EthTx1Packed,
		},
		{
			name: "2",
			args: args{
				tx:        &testTx2,
				height:    4321000,
				blockTime: 1534858022,
			},
			want: dbtestdata.EthTx2Packed,
		},
		{
			name: "3",
			args: args{
				tx:        &testTx1Failed,
				height:    4321000,
				blockTime: 1534858022,
			},
			want: dbtestdata.EthTx1FailedPacked,
		},
		{
			name: "4",
			args: args{
				tx:        &testTx1NoStatus,
				height:    4321000,
				blockTime: 1534858022,
			},
			want: dbtestdata.EthTx1NoStatusPacked,
		},
	}
	p := NewEthereumParser(1, false)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.PackTx(tt.args.tx, tt.args.height, tt.args.blockTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthereumParser.PackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("EthereumParser.PackTx() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestEthereumParser_UnpackTx(t *testing.T) {
	type args struct {
		hex string
	}
	tests := []struct {
		name    string
		p       *EthereumParser
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name:  "1",
			args:  args{hex: dbtestdata.EthTx1Packed},
			want:  &testTx1,
			want1: 4321000,
		},
		{
			name:  "2",
			args:  args{hex: dbtestdata.EthTx2Packed},
			want:  &testTx2,
			want1: 4321000,
		},
		{
			name:  "3",
			args:  args{hex: dbtestdata.EthTx1FailedPacked},
			want:  &testTx1Failed,
			want1: 4321000,
		},
		{
			name:  "4",
			args:  args{hex: dbtestdata.EthTx1NoStatusPacked},
			want:  &testTx1NoStatus,
			want1: 4321000,
		},
	}
	p := NewEthereumParser(1, false)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := hex.DecodeString(tt.args.hex)
			if err != nil {
				panic(err)
			}
			got, got1, err := p.UnpackTx(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthereumParser.UnpackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// DeepEqual has problems with pointers in completeTransaction
			gs := got.CoinSpecificData.(bchain.EthereumSpecificData)
			ws := tt.want.CoinSpecificData.(bchain.EthereumSpecificData)
			gc := *got
			wc := *tt.want
			gc.CoinSpecificData = nil
			wc.CoinSpecificData = nil
			if fmt.Sprint(gc) != fmt.Sprint(wc) {
				// if !reflect.DeepEqual(gc, wc) {
				t.Errorf("EthereumParser.UnpackTx() gc got = %+v, want %+v", gc, wc)
			}
			if !reflect.DeepEqual(gs.Tx, ws.Tx) {
				t.Errorf("EthereumParser.UnpackTx() gs.Tx got = %+v, want %+v", gs.Tx, ws.Tx)
			}
			if !reflect.DeepEqual(gs.Receipt, ws.Receipt) {
				t.Errorf("EthereumParser.UnpackTx() gs.Receipt got = %+v, want %+v", gs.Receipt, ws.Receipt)
			}
			if got1 != tt.want1 {
				t.Errorf("EthereumParser.UnpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestEthereumParser_GetEthereumTxData(t *testing.T) {
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
			want: "0xa9059cbb000000000000000000000000555ee11fbddc0e49a9bab358a8941ad95ffdb48f00000000000000000000000000000000000000000000021e19e0c9bab2400000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetEthereumTxData(tt.tx)
			if got.Data != tt.want {
				t.Errorf("EthereumParser.GetEthereumTxData() = %v, want %v", got.Data, tt.want)
			}
		})
	}
}

func TestEthereumParser_ParseErrorFromOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "ParseErrorFromOutput 1",
			output: "0x08c379a000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000031546f74616c206e756d626572206f662067726f757073206d7573742062652067726561746572207468616e207a65726f2e000000000000000000000000000000",
			want:   "Total number of groups must be greater than zero.",
		},
		{
			name:   "ParseErrorFromOutput 2",
			output: "0x08c379a0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000126e6f7420656e6f7567682062616c616e63650000000000000000000000000000",
			want:   "not enough balance",
		},
		{
			name:   "ParseErrorFromOutput empty",
			output: "",
			want:   "",
		},
		{
			name:   "ParseErrorFromOutput short",
			output: "0x08c379a000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000012",
			want:   "",
		},
		{
			name:   "ParseErrorFromOutput invalid signature",
			output: "0x08c379b0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000126e6f7420656e6f7567682062616c616e63650000000000000000000000000000",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseErrorFromOutput(tt.output)
			if got != tt.want {
				t.Errorf("EthereumParser.ParseErrorFromOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthereumParser_PackInternalTransactionError_UnpackInternalTransactionError(t *testing.T) {
	tests := []struct {
		name     string
		original string
		packed   string
		unpacked string
	}{
		{
			name:     "execution reverted",
			original: "execution reverted",
			packed:   "\x01",
			unpacked: "Reverted.",
		},
		{
			name:     "out of gas",
			original: "out of gas",
			packed:   "\x02",
			unpacked: "Out of gas.",
		},
		{
			name:     "contract creation code storage out of gas",
			original: "contract creation code storage out of gas",
			packed:   "\x03",
			unpacked: "Contract creation code storage out of gas.",
		},
		{
			name:     "max code size exceeded",
			original: "max code size exceeded",
			packed:   "\x04",
			unpacked: "Max code size exceeded.",
		},
		{
			name:     "unknown error",
			original: "unknown error",
			packed:   "unknown error",
			unpacked: "unknown error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := PackInternalTransactionError(tt.original)
			if packed != tt.packed {
				t.Errorf("EthereumParser.PackInternalTransactionError() = %v, want %v", packed, tt.packed)
			}
			unpacked := UnpackInternalTransactionError([]byte(packed))
			if unpacked != tt.unpacked {
				t.Errorf("EthereumParser.UnpackInternalTransactionError() = %v, want %v", unpacked, tt.unpacked)
			}
		})
	}
}
