// +build unittest

package eth

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestErc20_erc20GetTransfersFromLog(t *testing.T) {
	tests := []struct {
		name    string
		args    []*rpcLog
		want    []bchain.Erc20Transfer
		wantErr bool
	}{
		{
			name: "1",
			args: []*rpcLog{
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
			want: []bchain.Erc20Transfer{
				{
					Contract: "0x76a45e8976499ab9ae223cc584019341d5a84e96",
					From:     "0x2aacf811ac1a60081ea39f7783c0d26c500871a8",
					To:       "0xe9a5216ff992cfa01594d43501a56e12769eb9d2",
					Tokens:   *big.NewInt(0x123),
				},
			},
		},
		{
			name: "2",
			args: []*rpcLog{
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
			want: []bchain.Erc20Transfer{
				{
					Contract: "0x0d0f936ee4c93e25944694d6c121de94d9760f11",
					From:     "0x6f44cceb49b4a5812d54b6f494fc2febf25511ed",
					To:       "0x4bda106325c335df99eab7fe363cac8a0ba2a24d",
					Tokens:   *big.NewInt(0x6a8313d60b1f606b),
				},
				{
					Contract: "0xc778417e063141139fce010982780140aa0cd5ab",
					From:     "0x4bda106325c335df99eab7fe363cac8a0ba2a24d",
					To:       "0x6f44cceb49b4a5812d54b6f494fc2febf25511ed",
					Tokens:   *big.NewInt(0x308fd0e798ac0),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := erc20GetTransfersFromLog(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("erc20GetTransfersFromLog error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// the addresses could have different case
			if strings.ToLower(fmt.Sprint(got)) != strings.ToLower(fmt.Sprint(tt.want)) {
				t.Errorf("erc20GetTransfersFromLog = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestErc20_parseErc20StringProperty(t *testing.T) {
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
			got := parseErc20StringProperty(nil, tt.args)
			// the addresses could have different case
			if got != tt.want {
				t.Errorf("parseErc20StringProperty = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErc20_erc20GetTransfersFromTx(t *testing.T) {
	p := NewEthereumParser(1)
	b := dbtestdata.GetTestEthereumTypeBlock1(p)
	bn, _ := new(big.Int).SetString("21e19e0c9bab2400000", 16)
	tests := []struct {
		name string
		args *rpcTransaction
		want []bchain.Erc20Transfer
	}{
		{
			name: "0",
			args: (b.Txs[0].CoinSpecificData.(completeTransaction)).Tx,
			want: []bchain.Erc20Transfer{},
		},
		{
			name: "1",
			args: (b.Txs[1].CoinSpecificData.(completeTransaction)).Tx,
			want: []bchain.Erc20Transfer{
				{
					Contract: "0x4af4114f73d1c1c903ac9e0361b379d1291808a2",
					From:     "0x20cd153de35d469ba46127a0c8f18626b59a256a",
					To:       "0x555ee11fbddc0e49a9bab358a8941ad95ffdb48f",
					Tokens:   *bn,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := erc20GetTransfersFromTx(tt.args)
			if err != nil {
				t.Errorf("erc20GetTransfersFromTx error = %v", err)
				return
			}
			// the addresses could have different case
			if strings.ToLower(fmt.Sprint(got)) != strings.ToLower(fmt.Sprint(tt.want)) {
				t.Errorf("erc20GetTransfersFromTx = %+v, want %+v", got, tt.want)
			}
		})
	}
}
