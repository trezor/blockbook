//go:build unittest

package eth

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

const testIgnoredContract = "0x0000000000000000000000000000000000001010"

func ignoredTestTransferLog(contract string, value int64) *bchain.RpcLog {
	return &bchain.RpcLog{
		Address: contract,
		Topics: []string{
			tokenTransferEventSignature,
			"0x0000000000000000000000002aacf811ac1a60081ea39f7783c0d26c500871a8",
			"0x000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d2",
		},
		Data: "0x" + big.NewInt(value).Text(16),
	}
}

func ignoredTestTransferSingleLog(contract string) *bchain.RpcLog {
	return &bchain.RpcLog{
		Address: contract,
		Topics: []string{
			tokenERC1155TransferSingleEventSignature,
			"0x0000000000000000000000009248a6048a58db9f0212dc7cd85ee8741128be72",
			"0x000000000000000000000000a3950b823cb063dd9afc0d27f35008b805b3ed53",
			"0x0000000000000000000000004392faf3bb96b5694ecc6ef64726f61cdd4bb0ec",
		},
		Data: "0x00000000000000000000000000000000000000000000000000000000000000960000000000000000000000000000000000000000000000000000000000000011",
	}
}

func txWithReceiptLogs(logs ...*bchain.RpcLog) *bchain.Tx {
	return &bchain.Tx{
		Txid:             "0xtxid",
		CoinSpecificData: bchain.EthereumSpecificData{Receipt: &bchain.RpcReceipt{Logs: logs}},
	}
}

func newIgnoringTestParser(t *testing.T) *EthereumParser {
	p := NewEthereumParser(1, false)
	if err := p.IgnoreContract(testIgnoredContract); err != nil {
		t.Fatal(err)
	}
	return p
}

func requireTransferContracts(t *testing.T, got bchain.TokenTransfers, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d transfers %+v, want %d", len(got), got, len(want))
	}
	for i, tr := range got {
		if tr.Contract != EIP55AddressFromAddress(want[i]) {
			t.Errorf("transfer %d contract = %s, want %s", i, tr.Contract, want[i])
		}
	}
}

// Post-Glamsterdam ETH -> token swap on Sepolia (tx 0x83b1a2e8…, block 11856400): the two
// EIP-7708 logs mirror the ETH paid into the router and wrapped into WETH, only the WETH and
// the bought token transfers are token transfers.
func TestEthereumParser_SkipsEIP7708TransferLogs(t *testing.T) {
	const (
		weth   = "0x7b79995e5f793A07Bc00c21412e50Ecae098E7f9"
		bought = "0x5f2E83cCDEa73D60aF400e03F1Cd8Fb9eaB07b20"
	)
	tx := txWithReceiptLogs(
		&bchain.RpcLog{
			Address: "0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE",
			Topics: []string{
				tokenTransferEventSignature,
				"0x0000000000000000000000007757d12199d98cf46b9af7be5072495fca4c983c",
				"0x0000000000000000000000008f6eb7870334b1fd8006fd52413f01689f4e57e9",
			},
			Data: "0x0000000000000000000000000000000000000000000000000000033ce0891000",
		},
		&bchain.RpcLog{
			Address: "0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE",
			Topics: []string{
				tokenTransferEventSignature,
				"0x0000000000000000000000008f6eb7870334b1fd8006fd52413f01689f4e57e9",
				"0x0000000000000000000000007b79995e5f793a07bc00c21412e50ecae098e7f9",
			},
			Data: "0x0000000000000000000000000000000000000000000000000000033ce0891000",
		},
		&bchain.RpcLog{ // WETH Deposit
			Address: weth,
			Topics: []string{
				"0xe1fffcc4923d04b559f4d29a8bfc6cda04eb5b0d3c460751c2402c5c5cc9109c",
				"0x0000000000000000000000008f6eb7870334b1fd8006fd52413f01689f4e57e9",
			},
			Data: "0x0000000000000000000000000000000000000000000000000000033ce0891000",
		},
		&bchain.RpcLog{
			Address: weth,
			Topics: []string{
				tokenTransferEventSignature,
				"0x0000000000000000000000008f6eb7870334b1fd8006fd52413f01689f4e57e9",
				"0x000000000000000000000000f32e24b7f739c7c17544cb972833ab551121a72b",
			},
			Data: "0x0000000000000000000000000000000000000000000000000000033ce0891000",
		},
		&bchain.RpcLog{
			Address: bought,
			Topics: []string{
				tokenTransferEventSignature,
				"0x000000000000000000000000f32e24b7f739c7c17544cb972833ab551121a72b",
				"0x0000000000000000000000007757d12199d98cf46b9af7be5072495fca4c983c",
			},
			Data: "0x0000000000000000000000000000000000000000000000000000000014318826",
		},
	)
	got, err := NewEthereumParser(1, false).EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	requireTransferContracts(t, got, weth, bought)
	if got[1].Value.Int64() != 0x14318826 {
		t.Errorf("bought token value = %s, want %d", got[1].Value.String(), 0x14318826)
	}
}

func TestEthereumParser_SkipsIgnoredContractLogs(t *testing.T) {
	const erc20 = "0x76a45e8976499ab9ae223cc584019341d5a84e96"
	const erc1155 = "0x6Fd712E3A5B556654044608F9129040A4839E36c"
	tests := []struct {
		name string
		logs []*bchain.RpcLog
		want []string
	}{
		{name: "ignored only", logs: []*bchain.RpcLog{ignoredTestTransferLog(testIgnoredContract, 1)}},
		{name: "ignored between tokens", logs: []*bchain.RpcLog{ignoredTestTransferLog(erc20, 1), ignoredTestTransferLog(testIgnoredContract, 2), ignoredTestTransferLog(erc20, 3)}, want: []string{erc20, erc20}},
		{name: "ignored first", logs: []*bchain.RpcLog{ignoredTestTransferLog(testIgnoredContract, 1), ignoredTestTransferLog(erc20, 2)}, want: []string{erc20}},
		{name: "ignored ERC1155", logs: []*bchain.RpcLog{ignoredTestTransferSingleLog(testIgnoredContract), ignoredTestTransferSingleLog(erc1155)}, want: []string{erc1155}},
		{name: "ignored in mixed case", logs: []*bchain.RpcLog{ignoredTestTransferLog("0X0000000000000000000000000000000000001010", 1), ignoredTestTransferLog(erc20, 2)}, want: []string{erc20}},
		{name: "nothing ignored", logs: []*bchain.RpcLog{ignoredTestTransferLog(erc20, 1), ignoredTestTransferLog(erc20, 2)}, want: []string{erc20, erc20}},
	}
	p := newIgnoringTestParser(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.EthereumTypeGetTokenTransfersFromTx(txWithReceiptLogs(tt.logs...))
			if err != nil {
				t.Fatal(err)
			}
			requireTransferContracts(t, got, tt.want...)
		})
	}
}

// Without a receipt the transfer comes from the call payload; a transfer() call to an ignored
// contract moves the native coin as well.
func TestEthereumParser_SkipsIgnoredContractPayload(t *testing.T) {
	const payload = "0xa9059cbb000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d20000000000000000000000000000000000000000000000000000000000000123"
	pendingTx := func(to string) *bchain.Tx {
		return &bchain.Tx{
			Txid: "0xtxid",
			CoinSpecificData: bchain.EthereumSpecificData{Tx: &bchain.RpcTransaction{
				From:    "0x2aacf811ac1a60081ea39f7783c0d26c500871a8",
				To:      to,
				Payload: payload,
			}},
		}
	}
	p := newIgnoringTestParser(t)
	got, err := p.EthereumTypeGetTokenTransfersFromTx(pendingTx(testIgnoredContract))
	if err != nil {
		t.Fatal(err)
	}
	requireTransferContracts(t, got)

	const erc20 = "0x76a45e8976499ab9ae223cc584019341d5a84e96"
	got, err = p.EthereumTypeGetTokenTransfersFromTx(pendingTx(erc20))
	if err != nil {
		t.Fatal(err)
	}
	requireTransferContracts(t, got, erc20)
}

func TestEthereumParser_EthereumTypeIsIgnoredContract(t *testing.T) {
	p := NewEthereumParser(1, false)
	desc := func(address string) bchain.AddressDescriptor {
		d, err := p.GetAddrDescFromAddress(address)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if !p.EthereumTypeIsIgnoredContract(desc(eip7708SystemAddress)) {
		t.Error("EIP-7708 system address not ignored by default")
	}
	if p.EthereumTypeIsIgnoredContract(desc("0xffffffffffffffffffffffffffffffffffffffff")) {
		t.Error("neighbouring address ignored")
	}
	if p.EthereumTypeIsIgnoredContract(nil) {
		t.Error("nil descriptor ignored")
	}
	if p.EthereumTypeIsIgnoredContract(desc(testIgnoredContract)) {
		t.Error("contract ignored before IgnoreContract")
	}
	if err := p.IgnoreContract("0x1010"); err == nil {
		t.Error("malformed address accepted")
	}
	if err := p.IgnoreContract(testIgnoredContract); err != nil {
		t.Fatal(err)
	}
	if !p.EthereumTypeIsIgnoredContract(desc(testIgnoredContract)) {
		t.Error("contract not ignored after IgnoreContract")
	}
	if !p.EthereumTypeIsIgnoredContract(desc(eip7708SystemAddress)) {
		t.Error("EIP-7708 system address no longer ignored after IgnoreContract")
	}
	// each parser has its own set
	if NewEthereumParser(1, false).EthereumTypeIsIgnoredContract(desc(testIgnoredContract)) {
		t.Error("IgnoreContract leaked into another parser")
	}
}

func Test_isIgnoredContract(t *testing.T) {
	ignored := newIgnoringTestParser(t).ignoredContracts
	tests := []struct {
		address string
		want    bool
	}{
		{address: testIgnoredContract, want: true},
		{address: "0000000000000000000000000000000000001010"},
		{address: "0x1010"},
		{address: "0x000000000000000000000000000000000000101g"},
		{address: ""},
	}
	for _, tt := range tests {
		if got := isIgnoredContract(ignored, tt.address); got != tt.want {
			t.Errorf("isIgnoredContract(%q) = %v, want %v", tt.address, got, tt.want)
		}
	}
	if isIgnoredContract(nil, testIgnoredContract) {
		t.Error("empty set ignores a contract")
	}
}
