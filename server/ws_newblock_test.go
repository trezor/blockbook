//go:build unittest

package server

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// gaugeFloat reads a single gauge's value via a throwaway registry, avoiding the
// prometheus/testutil dependency (and its transitive modules).
func gaugeFloat(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(g); err != nil {
		t.Fatalf("register gauge: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather gauge: %v", err)
	}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

func TestObserveNewBlockGas(t *testing.T) {
	m := &common.Metrics{
		EthBlockGasUsedRatio: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_eth_block_gas_used_ratio"}),
	}
	s := &WebsocketServer{metrics: m}

	// half-full post-London block -> ratio 0.5
	s.observeNewBlockGas(&bchain.Block{CoinSpecificData: &bchain.EthereumBlockSpecificData{
		BaseFeePerGas: big.NewInt(7), GasUsed: big.NewInt(15000000), GasLimit: big.NewInt(30000000),
	}})
	if got := gaugeFloat(t, m.EthBlockGasUsedRatio); got != 0.5 {
		t.Errorf("gas-used ratio = %v, want 0.5", got)
	}

	// non-EVM block and nil metrics must be safe no-ops
	s.observeNewBlockGas(&bchain.Block{})
	(&WebsocketServer{}).observeNewBlockGas(&bchain.Block{CoinSpecificData: &bchain.EthereumBlockSpecificData{
		GasUsed: big.NewInt(1), GasLimit: big.NewInt(2),
	}})
}

// TestNewBlockNotification covers the block -> subscribeNewBlock payload mapping: only EVM
// post-London blocks (EthereumBlockSpecificData with BaseFeePerGas set) carry evmData; non-EVM,
// pre-London, and missing coin-specific data leave it nil.
func TestNewBlockNotification(t *testing.T) {
	t.Run("non-EVM block has nil EVMData", func(t *testing.T) {
		got := newBlockNotification(&bchain.Block{BlockHeader: bchain.BlockHeader{Height: 7, Hash: "0xh"}})
		if got.Height != 7 || got.Hash != "0xh" {
			t.Errorf("height/hash = %d/%q, want 7/0xh", got.Height, got.Hash)
		}
		if got.EVMData != nil {
			t.Errorf("expected nil EVMData for non-EVM block, got %+v", got.EVMData)
		}
	})

	t.Run("EVM post-London block carries gas data from the block header", func(t *testing.T) {
		block := &bchain.Block{
			BlockHeader: bchain.BlockHeader{Height: 7, Hash: "0xh"},
			CoinSpecificData: &bchain.EthereumBlockSpecificData{
				BaseFeePerGas: big.NewInt(7),
				GasUsed:       big.NewInt(21000),
				GasLimit:      big.NewInt(30000000),
			},
		}
		got := newBlockNotification(block)
		if got.EVMData == nil {
			t.Fatal("expected EVMData, got nil")
		}
		if (*big.Int)(got.EVMData.BaseFeePerGas).Cmp(big.NewInt(7)) != 0 {
			t.Errorf("BaseFeePerGas = %v, want 7", got.EVMData.BaseFeePerGas)
		}
		if (*big.Int)(got.EVMData.BlockGasUsed).Cmp(big.NewInt(21000)) != 0 {
			t.Errorf("BlockGasUsed = %v, want 21000", got.EVMData.BlockGasUsed)
		}
		if (*big.Int)(got.EVMData.BlockGasLimit).Cmp(big.NewInt(30000000)) != 0 {
			t.Errorf("BlockGasLimit = %v, want 30000000", got.EVMData.BlockGasLimit)
		}
	})

	t.Run("EVM pre-London block (no base fee) has nil EVMData", func(t *testing.T) {
		block := &bchain.Block{
			BlockHeader:      bchain.BlockHeader{Height: 7, Hash: "0xh"},
			CoinSpecificData: &bchain.EthereumBlockSpecificData{GasUsed: big.NewInt(21000), GasLimit: big.NewInt(30000000)},
		}
		if got := newBlockNotification(block); got.EVMData != nil {
			t.Errorf("expected nil EVMData pre-London, got %+v", got.EVMData)
		}
	})
}

// TestWsNewBlockJSON pins the subscribeNewBlock wire shape: non-EVM chains serialize evmData as
// null (height/hash unchanged), while EVM chains carry decimal-string gas figures for the EIP-1559 projection.
func TestWsNewBlockJSON(t *testing.T) {
	tests := []struct {
		name string
		in   WsNewBlock
		want string
	}{
		{
			name: "non-EVM emits evmData null",
			in:   WsNewBlock{Height: 5, Hash: "0xabc"},
			want: `{"height":5,"hash":"0xabc","evmData":null}`,
		},
		{
			name: "EVM emits decimal gas figures",
			in: WsNewBlock{
				Height: 5,
				Hash:   "0xabc",
				EVMData: &EthereumGasData{
					BaseFeePerGas: (*api.Amount)(big.NewInt(7)),
					BlockGasUsed:  (*api.Amount)(big.NewInt(21000)),
					BlockGasLimit: (*api.Amount)(big.NewInt(30000000)),
				},
			},
			want: `{"height":5,"hash":"0xabc","evmData":{"baseFeePerGas":"7","blockGasUsed":"21000","blockGasLimit":"30000000"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(&tt.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("got  %s\nwant %s", b, tt.want)
			}
		})
	}
}
