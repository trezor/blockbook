package xcb

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/core-coin/go-core/v2/common"
	"github.com/cryptohub-digital/blockbook-fork/bchain"
	"github.com/cryptohub-digital/blockbook-fork/tests/dbtestdata"
)

func TestXrc20_xrc20GetTransfersFromLog(t *testing.T) {
	tests := []struct {
		name    string
		args    []*RpcLog
		want    bchain.TokenTransfers
		wantErr bool
	}{
		{
			name: "1",
			args: []*RpcLog{
				{
					Address: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
					Topics: []string{
						"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
						"0x00000000000000000000ab44de35413ee2b672d322938e2fcc932d5c0cf8ec88",
						"0x00000000000000000000ab27b691efe91718cb73207207d92dbd175e6b10c756",
					},
					Data: "0x000000000000000000000000000000000000000000000002b5e3af16b1880000",
				},
			},
			want: bchain.TokenTransfers{
				{
					Contract: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
					From:     "ab44de35413ee2b672d322938e2fcc932d5c0cf8ec88",
					To:       "ab27b691efe91718cb73207207d92dbd175e6b10c756",
					Value:    *big.NewInt(0).SetBytes([]byte{0x02, 0xB5, 0xE3, 0xAF, 0x16, 0xB1, 0x88, 0x00, 0x00}),
					Type:     bchain.FungibleToken,
				},
			},
		},
		{
			name: "2",
			args: []*RpcLog{
				{ // Transfer
					Address: "ab788e6c1e3bb2174aa05358a903dd93b2f3d361e2b6",
					Topics: []string{
						"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
						"0x00000000000000000000ab325f035d28ec052ea69198a1089064e9c4244eec3f",
						"0x00000000000000000000ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					},
					Data: "0x000000000000000000000000000000000000000000000000000051c821a88000",
				},
				{ // Transfer
					Address: "ab788e6c1e3bb2174aa05358a903dd93b2f3d361e2b6",
					Topics: []string{
						"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
						"0x00000000000000000000ab06fc08a2b40a25883f2e91bca36db89a3dd79989ba",
						"0x00000000000000000000ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					},
					Data: "0x0000000000000000000000000000000000000000000000000000a39043510000",
				},
				{ // not Transfer
					Address: "ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					Topics: []string{
						"0xd5e9cd7af10b71d0052610468eba244b812851c4e582629a8e52faa2484c56aa",
						"0x0000000000000000000000000000000000000000000000000000000000000021",
						"0x00000000000000000000ab325f035d28ec052ea69198a1089064e9c4244eec3f",
						"0x00000000000000000000ab06fc08a2b40a25883f2e91bca36db89a3dd79989ba",
					},
					Data: "0x000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000000000024e1cc78e2b605b726e9da5c9d497c38f95567defbd1953c034596206560189adafe80b37d53a8b6b516635fa5588638169f062a55db158e70bc226f5f3fd5dff0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000028e410d44000000000000000000000000000000000000000000000000000000028e410d44000",
				},
				{ // not Transfer
					Address: "ab788e6c1e3bb2174aa05358a903dd93b2f3d361e2b6",
					Topics: []string{
						"0xafa504e0962ad93dec232a2c88581b4028671c11f4571f9edec54fb75bd7293d",
						"0x00000000000000000000ab06fc08a2b40a25883f2e91bca36db89a3dd79989ba",
						"0x00000000000000000000ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					},
					Data: "0x0000000000000000000000000000000000000000000000000000000000000000",
				},
			},
			want: bchain.TokenTransfers{
				{
					Contract: "ab788e6c1e3bb2174aa05358a903dd93b2f3d361e2b6",
					From:     "ab325f035d28ec052ea69198a1089064e9c4244eec3f",
					To:       "ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					Value:    *big.NewInt(0).SetBytes([]byte{0x51, 0xc8, 0x21, 0xa8, 0x80, 0x00}),
					Type:     bchain.FungibleToken,
				},
				{
					Contract: "ab788e6c1e3bb2174aa05358a903dd93b2f3d361e2b6",
					From:     "ab06fc08a2b40a25883f2e91bca36db89a3dd79989ba",
					To:       "ab8497b008b094d916aa63e60e1f4e626c7334c4eb62",
					Value:    *big.NewInt(0).SetBytes([]byte{0xa3, 0x90, 0x43, 0x51, 0x00, 0x00}),
					Type:     bchain.FungibleToken,
				},
			},
		},
		{
			name: "2",
			args: []*RpcLog{
				{ // Transfer
					Address: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
					Topics: []string{
						"0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1",
						"0x00000000000000000000ab228a4d4263e067df56b1dd226acb939f532ff7ab5b",
						"0x00000000000000000000ab094a15c3dc43095c7450c59bf56263e9827065f306",
					},
					Data: "0x0000000000000000000000000000000000000000000000000de0b6b3a7640000",
				},
			},
			want: bchain.TokenTransfers{
				{
					Contract: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
					From:     "ab228a4d4263e067df56b1dd226acb939f532ff7ab5b",
					To:       "ab094a15c3dc43095c7450c59bf56263e9827065f306",
					Value:    *big.NewInt(0).SetBytes([]byte{0x0D, 0xE0, 0xB6, 0xB3, 0xA7, 0x64, 0x00, 0x00}),
					Type:     bchain.FungibleToken,
				},
			},
		},
	}
	common.DefaultNetworkID = common.NetworkID(3)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xrc20GetTransfersFromLog(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("xrc20GetTransfersFromLog error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// the addresses could have different case
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("xrc20GetTransfersFromLog = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestXrc20_parsexrc20StringProperty(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "1",
			args: "0x0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000758504c4f44444500000000000000000000000000000000000000000000000000",
			want: "XPLODDE",
		},
		{
			name: "2",
			args: "0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000022426974436c617665202d20436f6e73756d657220416374697669747920546f6b656e00000000000000",
			want: "BitClave - Consumer Activity Token",
		},
		{
			name: "short",
			args: "0x44616920537461626c65636f696e2076312e3000000000000000000000000000",
			want: "Dai Stablecoin v1.0",
		},
		{
			name: "short2",
			args: "0x44616920537461626c65636f696e2076312e3020444444444444444444444444",
			want: "Dai Stablecoin v1.0 DDDDDDDDDDDD",
		},
		{
			name: "long",
			args: "0x556e6973776170205631000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			want: "Uniswap V1",
		},
		{
			name: "garbage",
			args: "0x2234880850896048596206002535425366538144616734015984380565810000",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsexrc20StringProperty(nil, tt.args)
			// the addresses could have different case
			if got != tt.want {
				t.Errorf("parsexrc20StringProperty = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestXrc20_xrc20GetTransfersFromTx(t *testing.T) {
	common.DefaultNetworkID = common.NetworkID(3)
	p := NewCoreCoinParser(1)
	b := dbtestdata.GetTestCoreCoinTypeBlock1(p)
	bn, _ := new(big.Int).SetString("1000000000000000000", 10)
	tests := []struct {
		name string
		args *RpcTransaction
		want bchain.TokenTransfers
	}{
		{
			name: "0",
			args: (b.Txs[0].CoinSpecificData.(CoreCoinSpecificData)).Tx,
			want: bchain.TokenTransfers{},
		},
		{
			name: "1",
			args: (b.Txs[1].CoinSpecificData.(CoreCoinSpecificData)).Tx,
			want: bchain.TokenTransfers{
				{
					Contract: "ab98e5e2ba00469ce51440c22d4d4b79a56da712297f",
					From:     "ab228a4d4263e067df56b1dd226acb939f532ff7ab5b",
					To:       "ab094a15c3dc43095c7450c59bf56263e9827065f306",
					Value:    *bn,
					Type:     bchain.FungibleToken,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xrc20GetTransfersFromTx(tt.args)
			if err != nil {
				t.Errorf("xrc20GetTransfersFromTx error = %v", err)
				return
			}
			// the addresses could have different case
			if len(got) > 0 && len(tt.want) > 0 && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("xrc20GetTransfersFromTx = %+v, want %+v", got, tt.want)
			}
		})
	}
}
