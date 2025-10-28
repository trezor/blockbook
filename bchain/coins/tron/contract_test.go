// //go:build unittest
package tron

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// Receipt != nil so we are testing getting transfers from og
func TestTronParser_EthereumTypeGetTokenTransfersFromLog(t *testing.T) {
	parser := NewTronParser(1, false)

	tests := []struct {
		name     string
		tx       *bchain.Tx
		expected bchain.TokenTransfers
	}{
		{
			name: "TRC20 transfer",
			tx: &bchain.Tx{
				Txid: "0xtesttxid",
				CoinSpecificData: bchain.EthereumSpecificData{
					Tx: &bchain.RpcTransaction{
						From:    "0xc88bb5a4636463d7eb2af02ccabb8b790fb200a9",
						To:      "0xa614f803b6fd780986a42c78ec9c7f77e6ded13c",                                                                                                 // contract
						Payload: "0xa9059cbb0000000000000000000000418da98894069283ddf2379e0b27bfea76fc9b73990000000000000000000000000000000000000000000000000000000022eda680", // transfer(address,uint256)
					},
					Receipt: &bchain.RpcReceipt{
						Logs: []*bchain.RpcLog{
							{
								Address: "0xa614f803b6fd780986a42c78ec9c7f77e6ded13c", // USDT
								Topics: []string{
									"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
									"0x000000000000000000000000c88bb5a4636463d7eb2af02ccabb8b790fb200a9",
									"0x0000000000000000000000008da98894069283ddf2379e0b27bfea76fc9b7399",
								},
								Data: "0x0000000000000000000000000000000000000000000000000000000022eda680",
							},
						},
					},
				},
			},
			expected: bchain.TokenTransfers{
				{
					Standard: bchain.FungibleToken,
					Contract: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
					From:     "TUFbWcZzvLy2LbxkxFAraojZRTB8vewjsz",
					To:       "TNtFNW4EoQJanSczatPpU2kETN3WbVFVHR",
					Value:    *big.NewInt(586000000),
				},
			},
		},
		{
			name: "TRC721 transfer",
			tx: &bchain.Tx{
				Txid: "0x49ced31cd0fd6d8e1126775f53ade165fe7ca43e9cc968d64a9ce1aff597423c",
				CoinSpecificData: bchain.EthereumSpecificData{
					Tx: &bchain.RpcTransaction{
						From:    "0x34627862d50389c8d7a1ab5ef074b84ab4ddb9e9",
						To:      "0x0b17822171ee88e98d4a61029f97c9f8edc15fcd",
						Payload: "0x23b872dd00000000000000000000000034627862d50389c8d7a1ab5ef074b84ab4ddb9e90000000000000000000000000cecca0e53477d2b6c562ab68c3452fc99f7817e000000000000000000000000000000000000000000000000000000000000067f",
					},
					Receipt: &bchain.RpcReceipt{
						Logs: []*bchain.RpcLog{
							{
								Address: "0x0b17822171ee88e98d4a61029f97c9f8edc15fcd",
								Topics: []string{
									"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
									"0x00000000000000000000000034627862d50389c8d7a1ab5ef074b84ab4ddb9e9",
									"0x0000000000000000000000000cecca0e53477d2b6c562ab68c3452fc99f7817e",
									"0x000000000000000000000000000000000000000000000000000000000000067f",
								},
								Data: "0x",
							},
						},
					},
				},
			},
			expected: bchain.TokenTransfers{
				{
					Standard: bchain.NonFungibleToken,
					Contract: "TAyrbZCme4jVBnHnALvoKbE6ewLd2VGD77",
					From:     "TEkC6sH3rPjwXzXm58p9dRVVMHiz2wTcub",
					To:       "TB9YmmXyQuhZ4dvG4T2EAzeksVme6RSvWA",
					Value:    *big.NewInt(1663),
				},
			},
		},
		{
			name: "TRC1155 transfer",
			tx: &bchain.Tx{
				Txid: "0x1c5273ced427e4dcad8f6ad7441a0e247dadec0d7e24583ba0f292feeba463b1",
				CoinSpecificData: bchain.EthereumSpecificData{
					Tx: &bchain.RpcTransaction{
						From:    "0x46f67edfe3080971e39c7e099d50ec5d86f2cb06",
						To:      "0xec3dc0f7b89a6463eb05527fdaf3634db481fe61",
						Payload: "0xf242432a00000000000000000000000046f67edfe3080971e39c7e099d50ec5d86f2cb060000000000000000000000008227ecc55945f98c3dd10a8f461a4d7db126fdba000000000000000000000000000000000000000019efcdb92505463d0bebd400000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000",
					},
					Receipt: &bchain.RpcReceipt{
						Logs: []*bchain.RpcLog{
							{
								Address: "0xec3dc0f7b89a6463eb05527fdaf3634db481fe61",
								Topics: []string{
									"0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62",
									"0x00000000000000000000000046f67edfe3080971e39c7e099d50ec5d86f2cb06",
									"0x00000000000000000000000046f67edfe3080971e39c7e099d50ec5d86f2cb06",
									"0x0000000000000000000000008227ecc55945f98c3dd10a8f461a4d7db126fdba",
								},
								Data: "0x000000000000000000000000000000000000000019efcdb92505463d0bebd4000000000000000000000000000000000000000000000000000000000000000001",
							},
						},
					},
				},
			},
			expected: bchain.TokenTransfers{
				{
					Standard: bchain.MultiToken,
					Contract: "TXWLT4N9vDcmNHDnSuKv2odhBtizYuEMKJ",
					From:     "TGSRbJTwpyNtjnefQJG1ZwVF1CSDaGYGDy",
					To:       "TMqQg2W2UEEB8cdR35AvpEfU7QbVMihiRn",
					MultiTokenValues: []bchain.MultiTokenValue{
						{
							Id:    bi("802703001686578058670400000"),
							Value: *big.NewInt(1),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transfers, err := parser.EthereumTypeGetTokenTransfersFromTx(tt.tx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tt.expected) != len(transfers) {
				t.Fatalf("expected %d transfers, got %d", len(tt.expected), len(transfers))
			}

			for i := range tt.expected {
				if tt.expected[i].Contract != transfers[i].Contract ||
					tt.expected[i].Standard != transfers[i].Standard ||
					tt.expected[i].From != transfers[i].From ||
					tt.expected[i].To != transfers[i].To ||
					tt.expected[i].Value.Cmp(&transfers[i].Value) != 0 {
					t.Errorf("transfer %d mismatch:\ngot  %+v\nwant %+v", i, transfers[i], tt.expected[i])
				}
			}

		})
	}
}

func TestTronParser_EthereumTypeGetTokenTransfersFromTx(t *testing.T) {
	parser := NewTronParser(1, false)

	tests := []struct {
		name     string
		tx       *bchain.Tx
		expected bchain.TokenTransfers
	}{
		{
			name: "TRC20 transfer",
			tx: &bchain.Tx{
				Txid: "0xtesttxid",
				CoinSpecificData: bchain.EthereumSpecificData{
					Tx: &bchain.RpcTransaction{
						From:    "0xc88bb5a4636463d7eb2af02ccabb8b790fb200a9",
						To:      "0xa614f803b6fd780986a42c78ec9c7f77e6ded13c",                                                                                                 // contract
						Payload: "0xa9059cbb0000000000000000000000418da98894069283ddf2379e0b27bfea76fc9b73990000000000000000000000000000000000000000000000000000000022eda680", // transfer(address,uint256)
					},
				},
			},
			expected: bchain.TokenTransfers{
				{
					Standard: bchain.FungibleToken,
					Contract: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", // Base58
					From:     "TUFbWcZzvLy2LbxkxFAraojZRTB8vewjsz", // Base58
					To:       "TNtFNW4EoQJanSczatPpU2kETN3WbVFVHR", // Base58
					Value:    *big.NewInt(586000000),
				},
			},
		},
		{
			name: "TRC721 transfer",
			tx: &bchain.Tx{
				Txid: "0x49ced31cd0fd6d8e1126775f53ade165fe7ca43e9cc968d64a9ce1aff597423c",
				CoinSpecificData: bchain.EthereumSpecificData{
					Tx: &bchain.RpcTransaction{
						From:    "0x34627862d50389c8d7a1ab5ef074b84ab4ddb9e9",
						To:      "0x0b17822171ee88e98d4a61029f97c9f8edc15fcd",
						Payload: "0x23b872dd00000000000000000000000034627862d50389c8d7a1ab5ef074b84ab4ddb9e90000000000000000000000000cecca0e53477d2b6c562ab68c3452fc99f7817e000000000000000000000000000000000000000000000000000000000000067f",
					},
				},
			},
			expected: bchain.TokenTransfers{
				{
					Standard: bchain.NonFungibleToken,
					Contract: "TAyrbZCme4jVBnHnALvoKbE6ewLd2VGD77",
					From:     "TEkC6sH3rPjwXzXm58p9dRVVMHiz2wTcub",
					To:       "TB9YmmXyQuhZ4dvG4T2EAzeksVme6RSvWA",
					Value:    *big.NewInt(1663),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transfers, err := parser.EthereumTypeGetTokenTransfersFromTx(tt.tx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tt.expected) != len(transfers) {
				t.Fatalf("expected %d transfers, got %d", len(tt.expected), len(transfers))
			}

			for i := range tt.expected {
				if tt.expected[i].Contract != transfers[i].Contract ||
					tt.expected[i].Standard != transfers[i].Standard ||
					tt.expected[i].From != transfers[i].From ||
					tt.expected[i].To != transfers[i].To ||
					tt.expected[i].Value.Cmp(&transfers[i].Value) != 0 {
					t.Errorf("transfer %d mismatch:\ngot  %+v\nwant %+v", i, transfers[i], tt.expected[i])
				}
			}

		})
	}
}

// convert number longer than uint64 to big.Int
func bi(s string) big.Int {
	n := big.NewInt(0)
	_, ok := n.SetString(s, 10)
	if !ok {
		panic("invalid big.Int string: " + s)
	}
	return *n
}
