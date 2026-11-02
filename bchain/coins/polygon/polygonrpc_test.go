//go:build unittest

package polygon

import (
	"encoding/json"
	"testing"

	"github.com/trezor/blockbook/bchain/coins/eth"
)

// The MRC20 Transfer mirrors a native POL move that is already an internal transfer.
func TestNewPolygonRPC_IgnoresNativeTokenContract(t *testing.T) {
	rpc, err := NewPolygonRPC(json.RawMessage(`{"coin_name":"Polygon","averageBlockTimeMs":1500}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	parser := rpc.GetChainParser()
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{address: nativeTokenContract, want: true},
		{address: "0x0000000000000000000000000000000000001011"},
	} {
		desc, err := parser.GetAddrDescFromAddress(tc.address)
		if err != nil {
			t.Fatal(err)
		}
		if got := parser.EthereumTypeIsIgnoredContract(desc); got != tc.want {
			t.Errorf("EthereumTypeIsIgnoredContract(%s) = %v, want %v", tc.address, got, tc.want)
		}
	}
	if _, ok := parser.(*eth.EthereumParser); !ok {
		t.Errorf("parser is %T, want the shared *eth.EthereumParser", parser)
	}
}
