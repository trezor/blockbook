//go:build unittest

package polygon

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const transferEventSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

func transferLog(contract string, value int64) *bchain.RpcLog {
	return &bchain.RpcLog{
		Address: contract,
		Topics: []string{
			transferEventSignature,
			"0x0000000000000000000000002aacf811ac1a60081ea39f7783c0d26c500871a8",
			"0x000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d2",
		},
		Data: "0x" + big.NewInt(value).Text(16),
	}
}

func txWithLogs(logs ...*bchain.RpcLog) *bchain.Tx {
	return &bchain.Tx{
		Txid:             "0xtxid",
		CoinSpecificData: bchain.EthereumSpecificData{Receipt: &bchain.RpcReceipt{Logs: logs}},
	}
}

func newTestParser() *PolygonParser {
	return &PolygonParser{EthereumParser: eth.NewEthereumParser(1, false)}
}

// The MRC20 Transfer mirrors a native move that is already an internal transfer, so it is not a
// token transfer, while the other logs of the transaction stay untouched.
func TestPolygonParser_EthereumTypeGetTokenTransfersFromTx(t *testing.T) {
	const erc20 = "0x76a45e8976499ab9ae223cc584019341d5a84e96"
	tests := []struct {
		name string
		logs []*bchain.RpcLog
		want []string
	}{
		{name: "native contract only", logs: []*bchain.RpcLog{transferLog(nativeTokenContract, 1)}, want: []string{}},
		{name: "native contract between tokens", logs: []*bchain.RpcLog{transferLog(erc20, 1), transferLog(nativeTokenContract, 2), transferLog(erc20, 3)}, want: []string{erc20, erc20}},
		{name: "native contract first", logs: []*bchain.RpcLog{transferLog(nativeTokenContract, 1), transferLog(erc20, 2)}, want: []string{erc20}},
		{name: "no native contract", logs: []*bchain.RpcLog{transferLog(erc20, 1), transferLog(erc20, 2)}, want: []string{erc20, erc20}},
	}
	p := newTestParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.EthereumTypeGetTokenTransfersFromTx(txWithLogs(tt.logs...))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d transfers %+v, want %d", len(got), got, len(tt.want))
			}
			for i, tr := range got {
				if tr.Contract != eth.EIP55AddressFromAddress(tt.want[i]) {
					t.Errorf("transfer %d contract = %s, want %s", i, tr.Contract, tt.want[i])
				}
			}
		})
	}
}

// Without a receipt the transfer comes from the call payload, and a transfer() call to the native
// contract is a native move as well.
func TestPolygonParser_EthereumTypeGetTokenTransfersFromTx_Payload(t *testing.T) {
	tx := &bchain.Tx{
		Txid: "0xtxid",
		CoinSpecificData: bchain.EthereumSpecificData{Tx: &bchain.RpcTransaction{
			From:    "0x2aacf811ac1a60081ea39f7783c0d26c500871a8",
			To:      nativeTokenContract,
			Payload: "0xa9059cbb000000000000000000000000e9a5216ff992cfa01594d43501a56e12769eb9d20000000000000000000000000000000000000000000000000000000000000123",
		}},
	}
	got, err := newTestParser().EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no transfers", got)
	}
}

func TestPolygonParser_EthereumTypeIsIgnoredContract(t *testing.T) {
	p := newTestParser()
	if !p.EthereumTypeIsIgnoredContract(mustAddrDesc(nativeTokenContract)) {
		t.Error("native token contract not ignored")
	}
	if p.EthereumTypeIsIgnoredContract(mustAddrDesc("0x0000000000000000000000000000000000001011")) {
		t.Error("neighbouring address ignored")
	}
	if p.EthereumTypeIsIgnoredContract(nil) {
		t.Error("nil descriptor ignored")
	}
}
