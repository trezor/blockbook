package fourbyte

import (
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func Test_parseSignatureFromText(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		want      bchain.FourByteSignature
	}{
		{
			name:      "_gonsPerFragment",
			signature: "_gonsPerFragment()",
			want: bchain.FourByteSignature{
				Name: "_gonsPerFragment",
			},
		},
		{
			name:      "vestingDeposits",
			signature: "vestingDeposits(address)",
			want: bchain.FourByteSignature{
				Name:       "vestingDeposits",
				Parameters: []string{"address"},
			},
		},
		{
			name:      "batchTransferTokenB",
			signature: "batchTransferTokenB(address[],uint256)",
			want: bchain.FourByteSignature{
				Name:       "batchTransferTokenB",
				Parameters: []string{"address[]", "uint256"},
			},
		},
		{
			name:      "transmitAndSellTokenForEth",
			signature: "transmitAndSellTokenForEth(address,uint256,uint256,uint256,address,(uint8,bytes32,bytes32),bytes)",
			want: bchain.FourByteSignature{
				Name:       "transmitAndSellTokenForEth",
				Parameters: []string{"address", "uint256", "uint256", "uint256", "address", "(uint8,bytes32,bytes32)", "bytes"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSignatureFromText(tt.signature); !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("parseSignatureFromText() = %v, want %v", *got, tt.want)
			}
		})
	}
}
