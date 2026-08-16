package eth

import (
	"testing"
	"time"
)

func TestInfuraFeeStaleDuration(t *testing.T) {
	got := infuraFeeStaleDuration(60)
	want := 30 * time.Minute
	if got != want {
		t.Fatalf("infuraFeeStaleDuration(60) = %s, want %s", got, want)
	}
}

func TestInfuraFeeProviderUsesCachedFeesDuringStaleWindow(t *testing.T) {
	provider := &infuraFeeProvider{
		alternativeFeeProvider: &alternativeFeeProvider{
			staleSyncDuration: infuraFeeStaleDuration(60),
		},
	}

	provider.processData(&infuraFeesResult{
		BaseFee: "10",
		Low: infuraFeeResult{
			MaxPriorityFeePerGas: "1",
			MaxFeePerGas:         "11",
		},
		Medium: infuraFeeResult{
			MaxPriorityFeePerGas: "2",
			MaxFeePerGas:         "12",
		},
		High: infuraFeeResult{
			MaxPriorityFeePerGas: "3",
			MaxFeePerGas:         "13",
		},
	})

	provider.mux.Lock()
	provider.lastSync = time.Now().Add(-29 * time.Minute)
	provider.mux.Unlock()

	fees, err := provider.GetEip1559Fees()
	if err != nil {
		t.Fatalf("GetEip1559Fees() error = %v", err)
	}
	if fees == nil {
		t.Fatal("GetEip1559Fees() returned nil fees inside stale window")
	}

	provider.mux.Lock()
	provider.lastSync = time.Now().Add(-31 * time.Minute)
	provider.mux.Unlock()

	fees, err = provider.GetEip1559Fees()
	if err != nil {
		t.Fatalf("GetEip1559Fees() error = %v", err)
	}
	if fees != nil {
		t.Fatal("GetEip1559Fees() returned fees after stale window")
	}
}
