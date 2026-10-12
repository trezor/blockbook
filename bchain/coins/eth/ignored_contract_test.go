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
	if p.EthereumTypeIsIgnoredContract(desc("0x0000000000000000000000000000000000001011")) {
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
