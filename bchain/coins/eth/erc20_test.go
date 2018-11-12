// +build unittest

package eth

import (
	"math/big"
	"reflect"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

func TestErc20_erc20GetTransfersFromLog(t *testing.T) {
	tests := []struct {
		name    string
		args    []*rpcLog
		want    []erc20Transfer
		wantErr bool
	}{
		{
			name: "1",
			args: []*rpcLog{
				&rpcLog{
					Address: ethcommon.HexToAddress("0x76a45e8976499ab9ae223cc584019341d5a84e96"),
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000002aacf811ac1a60081ea39f7783c0d26c500871a8",
						"0x000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d2",
					},
					Data: "0x0000000000000000000000000000000000000000000000000000000000000123",
				},
			},
			want: []erc20Transfer{
				{
					Contract: ethcommon.HexToAddress("0x76a45e8976499ab9ae223cc584019341d5a84e96"),
					From:     ethcommon.HexToAddress("0x2aacf811ac1a60081ea39f7783c0d26c500871a8"),
					To:       ethcommon.HexToAddress("0xe9a5216ff992cfa01594d43501a56e12769eb9d2"),
					Tokens:   *big.NewInt(0x123),
				},
			},
		},
		{
			name: "2",
			args: []*rpcLog{
				&rpcLog{ // Transfer
					Address: ethcommon.HexToAddress("0x0d0f936ee4c93e25944694d6c121de94d9760f11"),
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
						"0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d",
					},
					Data: "0x0000000000000000000000000000000000000000000000006a8313d60b1f606b",
				},
				&rpcLog{ // Transfer
					Address: ethcommon.HexToAddress("0xc778417e063141139fce010982780140aa0cd5ab"),
					Topics: []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
						"0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
					},
					Data: "0x000000000000000000000000000000000000000000000000000308fd0e798ac0",
				},
				&rpcLog{ // not Transfer
					Address: ethcommon.HexToAddress("0x479cc461fecd078f766ecc58533d6f69580cf3ac"),
					Topics: []string{
						"0x0d0b9391970d9a25552f37d436d2aae2925e2bfe1b2a923754bada030c498cb3",
						"0x0000000000000000000000006f44cceb49b4a5812d54b6f494fc2febf25511ed",
						"0x0000000000000000000000000000000000000000000000000000000000000000",
						"0x5af266c0a89a07c1917deaa024414577e6c3c31c8907d079e13eb448c082594f",
					},
					Data: "0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d0000000000000",
				},
				&rpcLog{ // not Transfer
					Address: ethcommon.HexToAddress("0x0d0f936ee4c93e25944694d6c121de94d9760f11"),
					Topics: []string{
						"0x0d0b9391970d9a25552f37d436d2aae2925e2bfe1b2a923754bada030c498cb3",
						"0x0000000000000000000000007b62eb7fe80350dc7ec945c0b73242cb9877fb1b",
						"0xb0b69dad58df6032c3b266e19b1045b19c87acd2c06fb0c598090f44b8e263aa",
					},
					Data: "0x0000000000000000000000004bda106325c335df99eab7fe363cac8a0ba2a24d000000000000000000000000c778417e063141139fce010982780140aa0cd5ab0000000000000000000000000d0f936ee4c93e25944694d6c121de94d9760f1100000000000000000000000000000000000000000000000000031855667df7a80000000000000000000000000000000000000000000000006a8313d60b1f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				},
			},
			want: []erc20Transfer{
				{
					Contract: ethcommon.HexToAddress("0x0d0f936ee4c93e25944694d6c121de94d9760f11"),
					From:     ethcommon.HexToAddress("0x6f44cceb49b4a5812d54b6f494fc2febf25511ed"),
					To:       ethcommon.HexToAddress("0x4bda106325c335df99eab7fe363cac8a0ba2a24d"),
					Tokens:   *big.NewInt(0x6a8313d60b1f606b),
				},
				{
					Contract: ethcommon.HexToAddress("0xc778417e063141139fce010982780140aa0cd5ab"),
					From:     ethcommon.HexToAddress("0x4bda106325c335df99eab7fe363cac8a0ba2a24d"),
					To:       ethcommon.HexToAddress("0x6f44cceb49b4a5812d54b6f494fc2febf25511ed"),
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("erc20GetTransfersFromLog = %+v, want %+v", got, tt.want)
			}
		})
	}

}
