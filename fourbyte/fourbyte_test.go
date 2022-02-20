package fourbyte

import (
	"reflect"
	"testing"

	"github.com/trezor/blockbook/db"
)

func Test_parseSignatureFromText(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		want      db.FourByteSignature
	}{
		{
			name:      "_gonsPerFragment",
			signature: "_gonsPerFragment()",
			want: db.FourByteSignature{
				Name: "_gonsPerFragment",
			},
		},
		{
			name:      "vestingDeposits",
			signature: "vestingDeposits(address)",
			want: db.FourByteSignature{
				Name:       "vestingDeposits",
				Parameters: []string{"address"},
			},
		},
		{
			name:      "batchTransferTokenB",
			signature: "batchTransferTokenB(address[],uint256)",
			want: db.FourByteSignature{
				Name:       "batchTransferTokenB",
				Parameters: []string{"address[]", "uint256"},
			},
		},
		{
			name:      "transmitAndSellTokenForEth",
			signature: "transmitAndSellTokenForEth(address,uint256,uint256,uint256,address,(uint8,bytes32,bytes32),bytes)",
			want: db.FourByteSignature{
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
