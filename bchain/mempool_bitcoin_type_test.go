package bchain

import (
	"encoding/hex"
	"testing"

	"github.com/martinboehm/btcutil/gcs"
)

func hexToBytes(h string) []byte {
	b, _ := hex.DecodeString(h)
	return b
}

func TestMempoolBitcoinType_computeGolombFilter(t *testing.T) {
	randomScript := hexToBytes("a914ff074800343a81ada8fe86c2d5d5a0e55b93dd7a87")
	m := &MempoolBitcoinType{
		golombFilterP: 20,
		golombFilterM: uint64(1 << 20),
	}
	tests := []struct {
		name string
		mtx  MempoolTx
		want string
	}{
		{
			name: "taproot",
			mtx: MempoolTx{
				Txid: "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
				Vin: []MempoolVin{
					{
						// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
						AddrDesc: hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f",
							Addresses: []string{
								"bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav",
							},
						},
					},
				},
			},
			want: "35dddcce5d60",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.computeGolombFilter(&tt.mtx); got != tt.want {
				t.Errorf("MempoolBitcoinType.computeGolombFilter() = %v, want %v", got, tt.want)
			}
			// check that the vin script matches the filter
			b, _ := hex.DecodeString(tt.mtx.Txid)
			filter, err := gcs.BuildGCSFilter(m.golombFilterP, m.golombFilterM, *(*[gcs.KeySize]byte)(b[:gcs.KeySize]), [][]byte{tt.mtx.Vin[0].AddrDesc})
			if err != nil {
				t.Errorf("gcs.BuildGCSFilter() unexpected error %v", err)
			}
			match, err := filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), tt.mtx.Vin[0].AddrDesc)
			if err != nil {
				t.Errorf("filter.Match vin[0] unexpected error %v", err)
			}
			if match != true {
				t.Errorf("filter.Match vin[0] expected true, got false")
			}
			// check that a random script does not match the filter
			match, err = filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), randomScript)
			if err != nil {
				t.Errorf("filter.Match randomScript unexpected error %v", err)
			}
			if match != false {
				t.Errorf("filter.Match randomScript expected false, got true")
			}

		})
	}
}
