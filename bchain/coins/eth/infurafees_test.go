package eth

import (
	"testing"
	"time"
)

func TestFeeStaleDurationDefaultsToTenMinutes(t *testing.T) {
	got := feeStaleDuration(0)
	want := 10 * time.Minute
	if got != want {
		t.Fatalf("feeStaleDuration(0) = %s, want %s", got, want)
	}
}

func TestFeeStaleDurationHonorsConfig(t *testing.T) {
	got := feeStaleDuration(90)
	want := 90 * time.Second
	if got != want {
		t.Fatalf("feeStaleDuration(90) = %s, want %s", got, want)
	}
}

func TestInfuraFeeProviderUsesCachedFeesDuringStaleWindow(t *testing.T) {
	provider := &infuraFeeProvider{
		alternativeFeeProvider: &alternativeFeeProvider{
			staleSyncDuration: feeStaleDuration(0),
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
	provider.lastSync = time.Now().Add(-9 * time.Minute)
	provider.mux.Unlock()

	fees, err := provider.GetEip1559Fees()
	if err != nil {
		t.Fatalf("GetEip1559Fees() error = %v", err)
	}
	if fees == nil {
		t.Fatal("GetEip1559Fees() returned nil fees inside stale window")
	}

	provider.mux.Lock()
	provider.lastSync = time.Now().Add(-11 * time.Minute)
	provider.mux.Unlock()

	fees, err = provider.GetEip1559Fees()
	if err != nil {
		t.Fatalf("GetEip1559Fees() error = %v", err)
	}
	if fees != nil {
		t.Fatal("GetEip1559Fees() returned fees after stale window")
	}
}
