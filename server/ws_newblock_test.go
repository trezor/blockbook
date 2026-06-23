//go:build unittest

package server

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/trezor/blockbook/api"
)

// TestWsNewBlockJSON pins the subscribeNewBlock wire shape: non-EVM chains serialize evm_data as
// null (height/hash unchanged), while EVM chains carry decimal-string gas figures for the EIP-1559 projection.
func TestWsNewBlockJSON(t *testing.T) {
	tests := []struct {
		name string
		in   WsNewBlock
		want string
	}{
		{
			name: "non-EVM emits evm_data null",
			in:   WsNewBlock{Height: 5, Hash: "0xabc"},
			want: `{"height":5,"hash":"0xabc","evm_data":null}`,
		},
		{
			name: "EVM emits decimal gas figures",
			in: WsNewBlock{
				Height: 5,
				Hash:   "0xabc",
				EVMData: &EthereumGasData{
					BaseFeePerGas: (*api.Amount)(big.NewInt(7)),
					BlockGasUsed:  (*api.Amount)(big.NewInt(21000)),
					BlockGasLimit: (*api.Amount)(big.NewInt(30000000)),
				},
			},
			want: `{"height":5,"hash":"0xabc","evm_data":{"baseFeePerGas":"7","blockGasUsed":"21000","blockGasLimit":"30000000"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(&tt.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("got  %s\nwant %s", b, tt.want)
			}
		})
	}
}
