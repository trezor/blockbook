//go:build unittest

package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestSystemInfoInSync(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	oldStart := now.Add(-time.Minute)

	tests := []struct {
		name          string
		inSync        bool
		initialSync   bool
		chainType     bchain.ChainType
		bestHeight    uint32
		backendBlocks int
		lastBlockTime time.Time
		startSync     time.Time
		blockPeriod   time.Duration
		want          bool
	}{
		{
			name:          "reports evm synced when active sync loop is already at backend tip",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
			want:          true,
		},
		{
			name:          "does not hide stale evm tip when heights match",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-25 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
		{
			name:          "does not report synced while local height is behind",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    90,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
		{
			name:          "reports evm synced within one block of a fresh tip",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    99,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
			want:          true,
		},
		{
			name:          "reports evm synced on a sub-second chain at the tip",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-1 * time.Second),
			startSync:     oldStart,
			blockPeriod:   250 * time.Millisecond,
			want:          true,
		},
		{
			name:          "does not report synced more than one block behind tip",
			chainType:     bchain.ChainEthereumType,
			bestHeight:    98,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
		{
			name:          "does not report synced during initial sync",
			initialSync:   true,
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
		{
			name:          "keeps startup grace for fresh regular sync",
			chainType:     bchain.ChainBitcoinType,
			backendBlocks: 100,
			startSync:     now.Add(-2 * time.Second),
			want:          true,
		},
		{
			name:          "marks already synced evm stale",
			inSync:        true,
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-25 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
		{
			name:          "keeps already synced evm fresh",
			inSync:        true,
			chainType:     bchain.ChainEthereumType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
			want:          true,
		},
		{
			name:          "does not extend tip equality rescue to bitcoin",
			chainType:     bchain.ChainBitcoinType,
			bestHeight:    100,
			backendBlocks: 100,
			lastBlockTime: now.Add(-10 * time.Second),
			startSync:     oldStart,
			blockPeriod:   2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemInfoInSync(tt.inSync, tt.initialSync, tt.chainType, tt.bestHeight, tt.backendBlocks, tt.lastBlockTime, tt.startSync, now, tt.blockPeriod)
			if got != tt.want {
				t.Fatalf("systemInfoInSync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSecondaryTicker_SkipsLookupWithoutSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return &common.CurrencyRatesTicker{}
	}

	ticker := w.getSecondaryTicker("")
	if ticker != nil {
		t.Fatalf("expected nil ticker when secondary currency is not requested, got %+v", ticker)
	}
	if calls != 0 {
		t.Fatalf("expected no ticker lookup call, got %d", calls)
	}
}

func TestGetSecondaryTicker_PerformsLookupWithSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	expected := &common.CurrencyRatesTicker{Rates: map[string]float32{"usd": 1}}
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return expected
	}

	ticker := w.getSecondaryTicker("usd")
	if ticker != expected {
		t.Fatalf("unexpected ticker returned: got %+v, want %+v", ticker, expected)
	}
	if calls != 1 {
		t.Fatalf("expected one ticker lookup call, got %d", calls)
	}
}

func TestTronBalanceHistoryOverrides(t *testing.T) {
	tests := []struct {
		name              string
		payload           string
		fallbackAmount    string
		hasFallbackAmount bool
		wantOverride      bool
		wantDirection     tronBalanceHistoryDirection
		wantAmount        string
	}{
		{
			name:              "freeze uses stake amount",
			payload:           `{"operation":"freeze","stakeAmount":"42000000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionOutgoing,
			wantAmount:        "42000000",
		},
		{
			name:              "withdraw uses unstake amount",
			payload:           `{"operation":"withdraw","unstakeAmount":"77000000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "77000000",
		},
		{
			name:              "withdraw falls back to tx value",
			payload:           `{"operation":"withdraw"}`,
			fallbackAmount:    "123",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "123",
		},
		{
			name:              "vote reward amount uses claimed vote reward",
			payload:           `{"operation":"voteRewardAmount","claimedVoteReward":"6500000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "6500000",
		},
		{
			name:              "vote reward amount falls back to tx value",
			payload:           `{"operation":"voteRewardAmount"}`,
			fallbackAmount:    "321",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "321",
		},
		{
			name:              "freeze invalid amount falls back to tx value",
			payload:           `{"operation":"freeze","stakeAmount":"not-a-number"}`,
			fallbackAmount:    "999",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionOutgoing,
			wantAmount:        "999",
		},
		{
			name:          "unfreeze has explicit no-move override",
			payload:       `{"operation":"unfreeze","unstakeAmount":"77000000"}`,
			wantOverride:  true,
			wantDirection: tronBalanceHistoryDirectionNone,
			wantAmount:    "0",
		},
		{
			name:         "non-freeze operation has no override",
			payload:      `{"operation":"transfer","stakeAmount":"42000000"}`,
			wantOverride: false,
		},
		{
			name:         "invalid json has no override",
			payload:      `{`,
			wantOverride: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fallback *big.Int
			if tt.hasFallbackAmount {
				var ok bool
				fallback, ok = new(big.Int).SetString(tt.fallbackAmount, 10)
				if !ok {
					t.Fatalf("invalid fallback amount in test: %q", tt.fallbackAmount)
				}
			}

			override, hasOverride := tronBalanceHistoryOverrideFromExtraData(json.RawMessage(tt.payload), fallback)
			if hasOverride != tt.wantOverride {
				t.Fatalf("override mismatch: got %v want %v", hasOverride, tt.wantOverride)
			}
			if !tt.wantOverride {
				return
			}
			if override.direction != tt.wantDirection {
				t.Fatalf("direction mismatch: got %v want %v", override.direction, tt.wantDirection)
			}
			if got := override.amount.String(); got != tt.wantAmount {
				t.Fatalf("amount mismatch: got %s want %s", got, tt.wantAmount)
			}
		})
	}
}

// BenchmarkTxInputResolution measures GetTransactionFromBchainTx as a function of
// a transaction's input fan-in, using the plain Bitcoin-testnet parser. Each input's
// previous output is resolved from the database, so this tracks how per-transaction
// allocation and time scale with the input count. The fan-in values span a wide range
// to make that scaling visible and to guard against regressions in input resolution.
//
//	go test -tags unittest -run x -bench TxInputResolution -benchmem ./api/
func BenchmarkTxInputResolution(b *testing.B) {
	for _, fanIn := range []int{8, 32, 128, 512, 2048} {
		b.Run("fanIn="+strconv.Itoa(fanIn), func(b *testing.B) {
			worker, subjectTx, cleanup := buildHighFanInWorker(b, fanIn)
			defer cleanup()
			addresses := make(map[string]struct{})

			tx, err := worker.GetTransactionFromBchainTx(subjectTx, 2, false, false, addresses)
			require.NoError(b, err)
			require.Len(b, tx.Vin, fanIn)
			require.True(b, tx.Vin[fanIn-1].IsAddress && tx.Vin[fanIn-1].ValueSat != nil)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := worker.GetTransactionFromBchainTx(subjectTx, 2, false, false, addresses); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// buildHighFanInWorker indexes a previous transaction with fanIn outputs and a
// subject transaction whose fanIn inputs each spend one of them (all referencing the
// same previous tx, concentrating resolution on a single large record), and returns
// a Worker plus the subject tx. Only db + parser are needed: GetTransactionFromBchainTx
// for a confirmed Bitcoin-type tx (specificJSON=false) does not touch the chain.
func buildHighFanInWorker(tb testing.TB, fanIn int) (*Worker, *bchain.Tx, func()) {
	tb.Helper()
	parser := btc.NewBitcoinParser(btc.GetChainParams("test"), &btc.Configuration{BlockAddressesToKeep: 1})
	script := dbtestdata.AddressToPubKeyHex(dbtestdata.Addr1, parser)

	prevTxid := fanInHash(1)
	prevOuts := make([]bchain.Vout, fanIn)
	subjectIns := make([]bchain.Vin, fanIn)
	for j := 0; j < fanIn; j++ {
		prevOuts[j] = fanInVout(uint32(j), script)
		subjectIns[j] = bchain.Vin{Txid: prevTxid, Vout: uint32(j), Sequence: 0xffffffff}
	}
	prevTx := fanInTx(prevTxid, 1, []bchain.Vin{{Coinbase: "01", Sequence: 0xffffffff}}, prevOuts)
	subjectTx := fanInTx(fanInHash(2), 2, subjectIns, []bchain.Vout{fanInVout(0, script), fanInVout(1, script)})

	blocks := []*bchain.Block{
		{BlockHeader: bchain.BlockHeader{Hash: fanInHash(1_000_001), Height: 1, Time: 1_700_000_001}, Txs: []bchain.Tx{*prevTx}},
		{BlockHeader: bchain.BlockHeader{Hash: fanInHash(1_000_002), Height: 2, Time: 1_700_000_002}, Txs: []bchain.Tx{*subjectTx}},
	}

	tmp, err := os.MkdirTemp("", "vin-resolution")
	require.NoError(tb, err)
	database, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	require.NoError(tb, err)
	cleanup := func() {
		require.NoError(tb, database.Close())
		require.NoError(tb, os.RemoveAll(tmp))
	}

	is, err := database.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	require.NoError(tb, err)
	database.SetInternalState(is)

	bulk, err := database.InitBulkConnect()
	require.NoError(tb, err)
	for i, block := range blocks {
		require.NoError(tb, bulk.ConnectBlock(block, i == len(blocks)-1))
	}
	require.NoError(tb, bulk.Close())
	is.FinishedSync(uint32(len(blocks)))

	return &Worker{db: database, chainParser: parser, chainType: bchain.ChainBitcoinType, is: is}, subjectTx, cleanup
}

func fanInTx(txid string, height uint32, vin []bchain.Vin, vout []bchain.Vout) *bchain.Tx {
	return &bchain.Tx{
		Txid:          txid,
		Version:       1,
		Vin:           vin,
		Vout:          vout,
		BlockHeight:   height,
		Confirmations: 1,
		Time:          int64(1_700_000_000 + height),
		Blocktime:     int64(1_700_000_000 + height),
	}
}

func fanInVout(n uint32, scriptHex string) bchain.Vout {
	return bchain.Vout{
		ValueSat:     *big.NewInt(100000),
		N:            n,
		ScriptPubKey: bchain.ScriptPubKey{Hex: scriptHex, Addresses: []string{dbtestdata.Addr1}},
	}
}

func fanInHash(n uint64) string { return fmt.Sprintf("%064x", n) }
