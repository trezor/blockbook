//go:build unittest

package eth

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func Test_contractGetTransfersFromLog(t *testing.T) {
	tests := []struct {
		name    string
		args    []*bchain.RpcLog
		want    bchain.TokenTransfers
		wantErr bool
	}{
		{
			name: "ERC20 transfer 1",
			args: []*bchain.RpcLog{
				{
					Address: "0x76a45e8976499ab9ae223cc584019341d5a84e96",
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000002aacf811ac1a60081ea39f7783c0d26c500871a8",
						"0x000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d2",
					},
					Data: "0x0000000000000000000000000000000000000000000000000000000000000123",
				},
			},
			want: bchain.TokenTransfers{
				{
					Contract: "0x76a45e8976499ab9ae223cc584019341d5a84e96",
					From:     "0x2aacf811ac1a60081ea39f7783c0d26c500871a8",
					To:       "0xe9a5216ff992cfa01594d43501a56e12769eb9d2",
					Value:    *big.NewInt(0x123),
				},
			},
		},
		{
			name: "ERC20 transfer 2",
			args: []*bchain.RpcLog{
				{ // Transfer
					Address: "0x0d0f936ee4c93e25944694d6c121de94d9760f11",
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
						"0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d",
					},
					Data: "0x0000000000000000000000000000000000000000000000006a8313d60b1f606b",
				},
				{ // Transfer
					Address: "0xc778417e063141139fce010982780140aa0cd5ab",
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
					},
					Data: "0x000000000000000000000000000000000000000000000000000308fd0e798ac0",
				},
				{ // not Transfer
					Address: "0x479cc461fecd078f766ecc58533d6f69580cf3ac",
					Topics: []string{
						"0x0d0b9391970d9a25552f37d436d2aae2925e2bfe1b2a923754bada030c498cb3",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
						"0x0000000000000000000000000000000000000000000000000000000000000000",
						"0x5af266c0a89a07c1917deaa024414577e6c3c31c8907d079e13eb448c082594f",
					},
					Data: "0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d0000000000000",
				},
				{ // not Transfer
					Address: "0x0d0f936ee4c93e25944694d6c121de94d9760f11",
					Topics: []string{
						"0x0d0b9391970d9a25552f37d436d2aae2925e2bfe1b2a923754bada030c498cb3",
						"0x0000000000000000000000007b62eb7fe80350dc7ec945c0b73242cb9877fb1b",
						"0xb0b69dad58df6032c3b266e19b1045b19c87acd2c06fb0c598090f44b8e263aa",
					},
					Data: "0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d000000000000000000000000c778417e063141139fce010982780140aa0cd5ab0000000000000000000000000d0f936ee4c93e25944694d6c121de94d9760f1100000000000000000000000000000000000000000000000000031855667df7a80000000000000000000000000000000000000000000000006a8313d60b1f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				},
			},
			want: bchain.TokenTransfers{
				{
					Contract: "0x0d0f936ee4c93e25944694d6c121de94d9760f11",
					From:     "0x6f44cceb49b4a5812d54b6f494fc2febf25511ed",
					To:       "0x4bda106325c335df99eab7fe363cac8a0ba2a24d",
					Value:    *big.NewInt(0x6a8313d60b1f606b),
				},
				{
					Contract: "0xc778417e063141139fce010982780140aa0cd5ab",
					From:     "0x4bda106325c335df99eab7fe363cac8a0ba2a24d",
					To:       "0x6f44cceb49b4a5812d54b6f494fc2febf25511ed",
					Value:    *big.NewInt(0x308fd0e798ac0),
				},
			},
		},
		{
			name: "ERC721 transfer 1",
			args: []*bchain.RpcLog{
				{ // Approval
					Address: "0x5689b918D34C038901870105A6C7fc24744D31eB",
					Topics: []string{
						"0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925",
						"0x0000000000000000000000000a206d4d5ff79cb5069def7fe3598421cff09391",
						"0x0000000000000000000000000000000000000000000000000000000000000000",
						"0x0000000000000000000000000000000000000000000000000000000000001396",
					},
					Data: "0x",
				},
				{ // Transfer
					Address: "0x5689b918D34C038901870105A6C7fc24744D31eB",
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000000a206d4d5ff79cb5069def7fe3598421cff09391",
						"0x0000000000000000000000006a016d7eec560549ffa0fbdb7f15c2b27302087f",
						"0x0000000000000000000000000000000000000000000000000000000000001396",
					},
					Data: "0x",
				},
				{ // OrdersMatched
					Address: "0x7Be8076f4EA4A4AD08075C2508e481d6C946D12b",
					Topics: []string{
						"0xc4109843e0b7d514e4c093114b863f8e7d8d9a458c372cd51bfe526b588006c9",
						"0x0000000000000000000000000a206d4d5ff79cb5069def7fe3598421cff09391",
						"0x0000000000000000000000006a016d7eec560549ffa0fbdb7f15c2b27302087f",
						"0x0000000000000000000000000000000000000000000000000000000000000000",
					},
					Data: "0x000000000000000000000000000000000000000000000000000000000000000069d3f0cc25f121f2aa96215f51ec4b4f1966f2d2ffbd3d8d8a45ad27b1c90323000000000000000000000000000000000000000000000000008e1bc9bf040000",
				},
			},
			want: bchain.TokenTransfers{
				{
					Type:     bchain.NonFungibleToken,
					Contract: "0x5689b918D34C038901870105A6C7fc24744D31eB",
					From:     "0x0a206d4d5ff79cb5069def7fe3598421cff09391",
					To:       "0x6a016d7eec560549ffa0fbdb7f15c2b27302087f",
					Value:    *big.NewInt(0x1396),
				},
			},
		},
		{
			name: "ERC1155 TransferSingle",
			args: []*bchain.RpcLog{
				{ // Transfer
					Address: "0x6Fd712E3A5B556654044608F9129040A4839E36c",
					Topics: []string{
						"0x5f9832c7244497a64c11c4a4f7597934bdf02b0361c54ad8e90091c2ce1f9e3c",
					},
					Data: "0x000000000000000000000000a3950b823cb063dd9afc0d27f35008b805b3ed530000000000000000000000004392faf3bb96b5694ecc6ef64726f61cdd4bb0ec000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000c00000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000009600000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000001",
				},
				{ // TransferSingle
					Address: "0x6Fd712E3A5B556654044608F9129040A4839E36c",
					Topics: []string{
						"0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62",
						"0x0000000000000000000000009248a6048a58db9f0212dc7cd85ee8741128be72",
						"0x000000000000000000000000a3950b823cb063dd9afc0d27f35008b805b3ed53",
						"0x0000000000000000000000004392faf3bb96b5694ecc6ef64726f61cdd4bb0ec",
					},
					Data: "0x00000000000000000000000000000000000000000000000000000000000000960000000000000000000000000000000000000000000000000000000000000011",
				},
				{ // unknown
					Address: "0x9248A6048a58db9f0212dC7CD85eE8741128be72",
					Topics: []string{
						"0x0b7bef9468bee71526deef3cbbded0ec1a0aa3d5a3e81eaffb0e758552b33199",
					},
					Data: "0x0000000000000000000000000000000000000000000000000000000000000060000000000000000000000000a3950b823cb063dd9afc0d27f35008b805b3ed530000000000000000000000004392faf3bb96b5694ecc6ef64726f61cdd4bb0ec0000000000000000000000000000000000000000000000000000000000000001",
				},
			},
			want: bchain.TokenTransfers{
				{
					Type:             bchain.MultiToken,
					Contract:         "0x6Fd712E3A5B556654044608F9129040A4839E36c",
					From:             "0xa3950b823cb063dd9afc0d27f35008b805b3ed53",
					To:               "0x4392faf3bb96b5694ecc6ef64726f61cdd4bb0ec",
					MultiTokenValues: []bchain.MultiTokenValue{{Id: *big.NewInt(150), Value: *big.NewInt(0x11)}},
				},
			},
		},
		{
			name: "ERC1155 TransferBatch",
			args: []*bchain.RpcLog{
				{ // TransferBatch
					Address: "0x6c42C26a081c2F509F8bb68fb7Ac3062311cCfB7",
					Topics: []string{
						"0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb",
						"0x0000000000000000000000005dc6288b35e0807a3d6feb89b3a2ff4ab773168e",
						"0x0000000000000000000000000000000000000000000000000000000000000000",
						"0x0000000000000000000000005dc6288b35e0807a3d6feb89b3a2ff4ab773168e",
					},
					Data: "0x000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000a0000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000006f0000000000000000000000000000000000000000000000000000000000000076a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000a",
				},
			},
			want: bchain.TokenTransfers{
				{
					Type:     bchain.MultiToken,
					Contract: "0x6c42c26a081c2f509f8bb68fb7ac3062311ccfb7",
					From:     "0x0000000000000000000000000000000000000000",
					To:       "0x5dc6288b35e0807a3d6feb89b3a2ff4ab773168e",
					MultiTokenValues: []bchain.MultiTokenValue{
						{Id: *big.NewInt(1776), Value: *big.NewInt(1)},
						{Id: *big.NewInt(1898), Value: *big.NewInt(10)},
					},
				},
			},
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := contractGetTransfersFromLog(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("contractGetTransfersFromLog error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("contractGetTransfersFromLog len not same, %+v, want %+v", got, tt.want)
			}
			for i := range got {
				// the addresses could have different case
				if strings.ToLower(fmt.Sprint(got[i])) != strings.ToLower(fmt.Sprint(tt.want[i])) {
					t.Errorf("contractGetTransfersFromLog %d = %+v, want %+v", i, got[i], tt.want[i])
				}

			}
		})
	}
}

func Test_contractGetTransfersFromTx(t *testing.T) {
	p := NewEthereumParser(1, false)
	b1 := dbtestdata.GetTestEthereumTypeBlock1(p)
	b2 := dbtestdata.GetTestEthereumTypeBlock2(p)
	bn, _ := new(big.Int).SetString("21e19e0c9bab2400000", 16)
	tests := []struct {
		name string
		args *bchain.RpcTransaction
		want bchain.TokenTransfers
	}{
		{
			name: "no contract transfer",
			args: (b1.Txs[0].CoinSpecificData.(bchain.EthereumSpecificData)).Tx,
			want: bchain.TokenTransfers{},
		},
		{
			name: "ERC20 transfer",
			args: (b1.Txs[1].CoinSpecificData.(bchain.EthereumSpecificData)).Tx,
			want: bchain.TokenTransfers{
				{
					Type:     bchain.FungibleToken,
					Contract: "0x4af4114f73d1c1c903ac9e0361b379d1291808a2",
					From:     "0x20cd153de35d469ba46127a0c8f18626b59a256a",
					To:       "0x555ee11fbddc0e49a9bab358a8941ad95ffdb48f",
					Value:    *bn,
				},
			},
		},
		{
			name: "ERC721 transferFrom",
			args: (b2.Txs[2].CoinSpecificData.(bchain.EthereumSpecificData)).Tx,
			want: bchain.TokenTransfers{
				{
					Type:     bchain.NonFungibleToken,
					Contract: "0xcda9fc258358ecaa88845f19af595e908bb7efe9",
					From:     "0x837e3f699d85a4b0b99894567e9233dfb1dcb081",
					To:       "0x7b62eb7fe80350dc7ec945c0b73242cb9877fb1b",
					Value:    *big.NewInt(1),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := contractGetTransfersFromTx(tt.args)
			if err != nil {
				t.Errorf("contractGetTransfersFromTx error = %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("contractGetTransfersFromTx len not same, %+v, want %+v", got, tt.want)
			}
			for i := range got {
				// the addresses could have different case
				if strings.ToLower(fmt.Sprint(got[i])) != strings.ToLower(fmt.Sprint(tt.want[i])) {
					t.Errorf("contractGetTransfersFromTx %d = %+v, want %+v", i, got[i], tt.want[i])
				}

			}
		})
	}
}
