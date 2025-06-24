//go:build unittest

package tron

import (
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestParseInputData(t *testing.T) {
	signatures := []bchain.FourByteSignature{
		{
			Name:       "safeTransferFrom",
			Parameters: []string{"address", "address", "uint256", "uint256", "bytes"},
		},
		{
			Name:       "transfer",
			Parameters: []string{"address", "uint256"},
		},
	}
	tests := []struct {
		name       string
		signatures *[]bchain.FourByteSignature
		data       string
		want       *bchain.EthereumParsedInputData
		wantErr    bool
	}{
		{
			name:       "TRC 1155 transfer",
			signatures: &signatures,
			data:       "0xf242432a00000000000000000000000046f67edfe3080971e39c7e099d50ec5d86f2cb060000000000000000000000008227ecc55945f98c3dd10a8f461a4d7db126fdba000000000000000000000000000000000000000019efcdb92505463d0bebd400000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000",
			want: &bchain.EthereumParsedInputData{
				MethodId: "0xf242432a",
				Name:     "Safe Transfer From",
				Function: "safeTransferFrom(address, address, uint256, uint256, bytes)",
				Params: []bchain.EthereumParsedInputParam{
					{
						Type:   "address",
						Values: []string{"TGSRbJTwpyNtjnefQJG1ZwVF1CSDaGYGDy"},
					},
					{
						Type:   "address",
						Values: []string{"TMqQg2W2UEEB8cdR35AvpEfU7QbVMihiRn"},
					},
					{
						Type:   "uint256",
						Values: []string{"8027030016865780586704000000"},
					},
					{
						Type:   "uint256",
						Values: []string{"1"},
					},
					{
						Type:   "bytes",
						Values: []string{""},
					},
				},
			},
		},
		{
			name:       "TRC20 transfer",
			signatures: &signatures,
			data:       "0xa9059cbb000000000000000000000000d54f9e3b484b372f83aecd67b3772368af4268be0000000000000000000000000000000000000000000000000000000000a7d8c0",
			want: &bchain.EthereumParsedInputData{
				MethodId: "0xa9059cbb",
				Name:     "Transfer",
				Function: "transfer(address, uint256)",
				Params: []bchain.EthereumParsedInputParam{
					{
						Type:   "address",
						Values: []string{"TVR6Jt3bTZhpsQer2DoH2RMDHoe5LS61Kz"},
					},
					{
						Type:   "uint256",
						Values: []string{"11000000"},
					},
				},
			},
		},
		{
			name: "Return Energy (dab0fe27)",
			signatures: &[]bchain.FourByteSignature{
				{
					Name:       "returnEnergy",
					Parameters: []string{"address", "uint256"},
				},
			},
			data: "0xdab0fe27000000000000000000000000e18657b3968394ae9a68f7dc93c110d84f2b079e000000000000000000000000000000000000000000000000000000016139cc53",
			want: &bchain.EthereumParsedInputData{
				MethodId: "0xdab0fe27",
				Name:     "Return Energy",
				Function: "returnEnergy(address, uint256)",
				Params: []bchain.EthereumParsedInputParam{
					{
						Type:   "address",
						Values: []string{"TWXfyWNZCeewDCpATk7i6E3X5CwGrFEkg6"},
					},
					{
						Type:   "uint256",
						Values: []string{"5926145107"},
					},
				},
			},
		},
	}
	parser := NewTronParser(1, false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.ParseInputData(tt.signatures, tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInputData() = %v, want %v", got, tt.want)
			}
		})
	}
}
