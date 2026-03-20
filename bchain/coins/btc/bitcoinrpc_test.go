package btc

import (
	"encoding/json"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestDecodeBatchRawTransactions(t *testing.T) {
	const txid = "ca211af71c54c3d90b83851c1d35a73669040b82742dd7f95e39953b032f7d39"
	const rawTx = "01000000014ce1dd2c07c07524ed102b5bf67d9eb601f65ccd848952042ed538c7bcf5ef830b0000006b483045022100f0beea3fada8a71b7dba04357112474e089bc1bd6726b520065a3ba244dc0dcc02200126f8cbbec0c21ea8fed38481391a4df43603c89736cbdc007e5280100f5fd401210242b47391c5b851486b7113ce30cbf60c45a8e8d2a6f7145a972100015e690a25ffffffff02d0b3fb02000000001976a914d39c85c954ae3002137fe718c2af835175352b5f88ac141b0000000000001976a914198ec3f7a57bc6a1dc929dc68464149108e272bf88ac00000000"

	responses := []rpcBatchResponse{
		{ID: 1, Result: json.RawMessage("\"" + rawTx + "\"")},
		{ID: 2, Error: &bchain.RPCError{Code: -5, Message: "No such mempool or blockchain transaction"}},
	}
	idToTxid := map[int]string{1: txid, 2: "missing"}

	parser := NewBitcoinParser(GetChainParams("main"), &Configuration{})
	got, err := decodeBatchRawTransactions(responses, idToTxid, parser)
	if err != nil {
		t.Fatalf("decodeBatchRawTransactions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(got))
	}
	if got[txid] == nil {
		t.Fatalf("missing tx %s", txid)
	}
	if got[txid].Txid != txid {
		t.Fatalf("expected txid %s, got %s", txid, got[txid].Txid)
	}
}
