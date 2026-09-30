//go:build unittest

package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/bch"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

type bcashFixtureChain struct {
	bchain.BlockChain
	txs map[string]*bchain.Tx
}

func (c *bcashFixtureChain) GetTransaction(txid string) (*bchain.Tx, error) {
	if tx := c.txs[txid]; tx != nil {
		return tx, nil
	}
	return nil, bchain.ErrTxNotFound
}

func (c *bcashFixtureChain) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	return json.Marshal(tx)
}

func bcashWebsocketRequest(t *testing.T, conn *websocket.Conn, method string, params interface{}, out interface{}) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, conn.WriteJSON(websocketReq{ID: method, Method: method, Params: params}))
	var response websocketRespWithData
	require.NoError(t, conn.ReadJSON(&response))
	require.Equal(t, method, response.ID)
	require.NoError(t, json.Unmarshal(response.Data, out))
}

func assertBcashTransaction(t *testing.T, fixture dbtestdata.BcashTxFixture, tx *api.Tx) {
	t.Helper()
	require.Equal(t, fixture.Txid, tx.Txid)
	require.Len(t, tx.Vout, len(fixture.Outputs))
	for i, want := range fixture.Outputs {
		require.Equal(t, []string{want.Address}, tx.Vout[i].Addresses)
		require.Equal(t, want.Value, tx.Vout[i].ValueSat.String())
		wantToken, err := json.Marshal(want.Token)
		require.NoError(t, err)
		gotToken, err := json.Marshal(tx.Vout[i].BcashToken)
		require.NoError(t, err)
		require.JSONEq(t, string(wantToken), string(gotToken))
		if want.Token != nil {
			require.NotNil(t, tx.BcashSpecific)
			require.Len(t, tx.BcashSpecific.TokenVouts, len(fixture.Outputs))
			gotToken, err = json.Marshal(tx.BcashSpecific.TokenVouts[i])
			require.NoError(t, err)
			require.JSONEq(t, string(wantToken), string(gotToken))
		}
	}
	if fixture.Name == "pat-empty-script-spend" {
		require.Equal(t, []string{"script-"}, tx.Vin[0].Addresses)
	}
}

// The RPC fixture supplies raw transactions; indexing, persistence and both public APIs are real.
func TestBcashIndexingAPIs(t *testing.T) {
	fixtures, err := dbtestdata.GetBcashFixtures()
	require.NoError(t, err)
	metrics, err := common.GetMetrics("BcashFixtures")
	require.NoError(t, err)
	for _, network := range []string{"main", "test"} {
		for _, extended := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/extended=%t", network, extended), func(t *testing.T) {
				parser, err := bch.NewBCashParser(bch.GetChainParams(network), &btc.Configuration{AddressFormat: "cashaddr"})
				require.NoError(t, err)
				base, err := dbtestdata.NewFakeBlockChain(parser)
				require.NoError(t, err)
				chain := &bcashFixtureChain{BlockChain: base, txs: make(map[string]*bchain.Tx)}
				config := &common.Config{CoinName: "Bcash", CoinShortcut: "BCH"}
				path := t.TempDir()
				d, err := db.NewRocksDB(path, 100000, -1, parser, nil, extended)
				require.NoError(t, err)
				is, err := d.LoadInternalState(config)
				require.NoError(t, err)
				d.SetInternalState(is)
				selected := make(map[string]dbtestdata.BcashTxFixture)
				for _, fixture := range fixtures {
					if fixture.Chain != network {
						continue
					}
					raw, err := hex.DecodeString(fixture.Hex)
					require.NoError(t, err)
					tx, err := parser.ParseTx(raw)
					require.NoError(t, err)
					tx.Confirmations = 1
					tx.BlockHeight = uint32(len(selected) + 1)
					tx.Blocktime = 1700000000 + int64(tx.BlockHeight)
					chain.txs[tx.Txid] = tx
					selected[tx.Txid] = fixture
					block := &bchain.Block{BlockHeader: bchain.BlockHeader{Height: tx.BlockHeight, Hash: fmt.Sprintf("%064x", tx.BlockHeight), Time: tx.Blocktime}, Txs: []bchain.Tx{*tx}}
					require.NoError(t, d.ConnectBlock(block))
					// Cache the parsed raw transaction as the public transaction endpoint does.
					require.NoError(t, d.PutTx(tx, tx.BlockHeight, tx.Blocktime))
				}
				is.FinishedSync(uint32(len(selected)))
				require.NoError(t, d.Close())
				d, err = db.NewRocksDB(path, 100000, -1, parser, nil, extended)
				require.NoError(t, err)
				defer d.Close()
				is, err = d.LoadInternalState(config)
				require.NoError(t, err)
				d.SetInternalState(is)
				is.FinishedSync(uint32(len(selected)))
				mempool, err := chain.CreateMempool(chain)
				require.NoError(t, err)
				cache, err := db.NewTxCache(d, chain, metrics, is, true)
				require.NoError(t, err)
				rates, err := fiat.NewFiatRates(d, config, metrics, nil)
				require.NoError(t, err)
				s, err := NewPublicServer("localhost:12345", "", d, chain, mempool, cache, "", metrics, is, rates, false)
				require.NoError(t, err)
				defer s.Shutdown(context.Background())
				s.ConnectFullPublicInterface()
				ts := httptest.NewServer(s.https.Handler)
				defer ts.Close()
				conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(ts.URL, "http", "ws", 1)+"/websocket", nil)
				require.NoError(t, err)
				defer conn.Close()
				for _, fixture := range selected {
					var rest, ws api.Tx
					mustGetJSON(t, ts.URL+"/api/v2/tx/"+fixture.Txid, http.StatusOK, &rest)
					assertBcashTransaction(t, fixture, &rest)
					bcashWebsocketRequest(t, conn, "getTransaction", map[string]string{"txid": fixture.Txid}, &ws)
					assertBcashTransaction(t, fixture, &ws)
					addresses := make(map[string]bool)
					for _, output := range fixture.Outputs {
						if output.Descriptor != "" {
							addresses[output.Address] = true
						}
					}
					for address := range addresses {
						var account api.Address
						mustGetJSON(t, ts.URL+"/api/v2/address/"+address+"?details=txslight", http.StatusOK, &account)
						require.NotEmpty(t, account.Transactions)
						for _, tx := range account.Transactions {
							assertBcashTransaction(t, selected[tx.Txid], tx)
						}
						var wsAccount api.Address
						bcashWebsocketRequest(t, conn, "getAccountInfo", map[string]string{"descriptor": address, "details": "txslight"}, &wsAccount)
						require.Equal(t, account.Transactions, wsAccount.Transactions)
						var restUtxos, wsUtxos api.Utxos
						mustGetJSON(t, ts.URL+"/api/v2/utxo/"+address, http.StatusOK, &restUtxos)
						bcashWebsocketRequest(t, conn, "getAccountUtxo", map[string]string{"descriptor": address}, &wsUtxos)
						require.Equal(t, restUtxos, wsUtxos)
						require.NotEmpty(t, restUtxos)
						for _, utxo := range restUtxos {
							want := selected[utxo.Txid].Outputs[utxo.Vout]
							require.Equal(t, want.Value, utxo.AmountSat.String())
							gotToken, err := json.Marshal(utxo.BcashToken)
							require.NoError(t, err)
							wantToken, err := json.Marshal(want.Token)
							require.NoError(t, err)
							require.JSONEq(t, string(wantToken), string(gotToken))
						}
					}
				}
			})
		}
	}
}
