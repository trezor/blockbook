package eth

import "testing"

// The cached tip is advanced by the newHeads subscription and the watchdog poll, while the
// block height passed in is read straight from the backend, so the height can be ahead of
// the tip. That must not underflow the unsigned subtraction into a bogus finality count.
func TestComputeConfirmations_TipBehindHeight(t *testing.T) {
	const tip = 18000000
	tests := []struct {
		name   string
		height uint64
		want   uint32
	}{
		{"height below tip", tip - 1, 2},
		{"height at tip", tip, 1},
		{"tip behind by 1 block", tip + 1, 1},
		{"tip behind by 2 blocks", tip + 2, 1},
		{"tip behind by many blocks", tip + 300, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &EthereumRPC{bestHeader: stubHeader{n: tip}}
			got, err := b.computeConfirmations(tt.height)
			if err != nil {
				t.Fatalf("computeConfirmations(%d) error %v", tt.height, err)
			}
			if got != tt.want {
				t.Errorf("computeConfirmations(%d) with tip %d = %d, want %d", tt.height, tip, got, tt.want)
			}
		})
	}
}
