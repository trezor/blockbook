//go:build unittest

package tron

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
)

func TestTronParser_GetAddrDescFromAddress(t *testing.T) {
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
			name:    "Base58 Tron Address",
			args:    args{address: "TJngGWiRMLgNFScEybQxLEKQMNdB4nR6Vx"},
			want:    "60bb513e91aa723a10a4020ae6fcce39bce7e240", // Hexadecimal format with prefix 41
			wantErr: false,
		},
		{
			name:    "Hex Tron Address as from JSON-RPC",
			args:    args{address: "0xef51c82ea6336ba1544c4a182a7368e9fbe28274"},
			want:    "ef51c82ea6336ba1544c4a182a7368e9fbe28274", // descriptor without prefix and checksum -> len = 20
			wantErr: false,
		},
		{
			name:    "Invalid Tron Address",
			args:    args{address: "invalidAddress"},
			want:    "",
			wantErr: true,
		},
	}
	parser := NewTronParser(1, false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if h != tt.want {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestTronParser_GetAddressesFromAddrDesc(t *testing.T) {
	type args struct {
		desc string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name:    "Desc to Base58 Tron Address",
			args:    args{desc: "f3f1c189594e2642e5d42d7669b4ec60a69802a9"},
			want:    []string{"TYD4pB7wGi1p8zK67rBTV3KdfEb9nvNDXh"},
			wantErr: false,
		},
		{
			name:    "Desc to Base58 Tron Address 2",
			args:    args{desc: "ef51c82ea6336ba1544c4a182a7368e9fbe28274"},
			want:    []string{"TXncUDXYkRCmwhFikxYMutwAy93fbhPbbv"},
			wantErr: false,
		},
		{
			name:    "Invalid Hex Address",
			args:    args{desc: "invalidHex"},
			want:    nil,
			wantErr: true,
		},
	}
	parser := NewTronParser(1, false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := hex.DecodeString(tt.args.desc)
			if err != nil && !tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v", err)
				return
			}

			got, _, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeHexUint64String(t *testing.T) {
	tests := map[string]string{
		"0x0000000000000000": "0x0",
		"0x0000000000000001": "0x1",
		"0x":                 "0x0",
		"0x01":               "0x1",
		"0xa":                "0xa",
	}
	for input, expected := range tests {
		got := SanitizeHexUint64String(input)
		if got != expected {
			t.Errorf("SanitizeHexUint64String(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFromTronAddressToHex(t *testing.T) {
	parser := NewTronParser(1, false)

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid Base58 Tron address",
			input:       "TJngGWiRMLgNFScEybQxLEKQMNdB4nR6Vx",
			expected:    "0x60bb513e91aa723a10a4020ae6fcce39bce7e240",
			expectError: false,
		},
		{
			name:        "Invalid Tron address",
			input:       "INVALID_ADDRESS",
			expected:    "", // should return empty string on error
			expectError: true,
		},
		{
			name:        "Already hex address",
			input:       "0x60bb513e91aa723a10a4020ae6fcce39bce7e240",
			expected:    "0x60bb513e91aa723a10a4020ae6fcce39bce7e240",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.FromTronAddressToHex(tt.input)

			if (err != nil) != tt.expectError {
				t.Errorf("FromTronAddressToHex(%s) unexpected error state: got err=%v, wantError=%v", tt.input, err, tt.expectError)
				return
			}

			if result != tt.expected {
				t.Errorf("FromTronAddressToHex(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTronParser_PackUnpackRoundtrip(t *testing.T) {
	original := &bchain.Tx{
		Txid: "0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
		Vin: []bchain.Vin{
			{
				Addresses: []string{
					"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt",
				},
			},
		},
		Vout: []bchain.Vout{
			{
				N: 0,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{
						"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
					},
				},
			},
		},
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{
				AccountNonce:     "0x0",
				GasPrice:         "0xd2",
				GasLimit:         "0x393a",
				To:               "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
				Value:            "0x0",
				Payload:          "0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e",
				Hash:             "0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
				BlockNumber:      "0x348d2a7",
				BlockHash:        "0x000000000348d2a70c64b102b21699f7f561fffbc67d50ed5f540db5ad631913",
				From:             "TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt",
				TransactionIndex: "0x0",
			},
			Receipt: &bchain.RpcReceipt{
				GasUsed: "0x393a",
				Status:  "0x1",
				Logs: []*bchain.RpcLog{
					{
						Address: "0xeca9bc828a3005b9a3b909f2cc5c2a54794de05f",
						Topics: []string{
							"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
							"0x000000000000000000000000ff324071970b2b08822caa310c1bb458e63a5033",
							"0x000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab",
						},
						Data: "0x0000000000000000000000000000000000000000000000000000000000ab604e",
					},
				},
			},
		},
	}

	parser := NewTronParser(1, false)

	packed, err := parser.PackTx(original, original.BlockHeight, original.Blocktime)
	require.NoError(t, err)

	unpacked, _, err := parser.UnpackTx(packed)
	require.NoError(t, err)

	origJSON, err := json.Marshal(original)
	require.NoError(t, err)
	unpkJSON, err := json.Marshal(unpacked)
	require.NoError(t, err)

	if !bytes.Equal(origJSON, unpkJSON) {
		t.Errorf("Transactions are not equal \nOriginal: %s\nUnpacked: %s", origJSON, unpkJSON)
	}

}
