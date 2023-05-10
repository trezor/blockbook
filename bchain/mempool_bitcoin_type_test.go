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

func TestMempoolBitcoinType_computeGolombFilter_taproot(t *testing.T) {
	randomScript := hexToBytes("a914ff074800343a81ada8fe86c2d5d5a0e55b93dd7a87")
	m := &MempoolBitcoinType{
		golombFilterP: 20,
		golombFilterM: uint64(1 << 20),
		filterScripts: filterScriptsTaproot,
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
			want: "0235dddcce5d60",
		},
		{
			name: "taproot multiple",
			mtx: MempoolTx{
				Txid: "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
				Vin: []MempoolVin{
					{
						// bc1pp3752xgfy39w30kggy8vvn0u68x8afwqmq6p96jzr8ffrcvjxgrqrny93y
						AddrDesc: hexToBytes("51200c7d451909244ae8bec8410ec64dfcd1cc7ea5c0d83412ea4219d291e1923206"),
					},
					{
						// bc1p5ldsz3zxnjxrwf4xluf4qu7u839c204ptacwe2k0vzfk8s63mwts3njuwr
						AddrDesc: hexToBytes("5120a7db0144469c8c3726a6ff135073dc3c4b853ea15f70ecaacf609363c351db97"),
					},
					{
						// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
						AddrDesc: hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51209ab20580f77e7cd676f896fc1794f7e8061efc1ce7494f2bb16205262aa12bdb",
							Addresses: []string{
								"bc1pn2eqtq8h0e7dvahcjm7p098haqrpalquuay572a3vgzjv24p90dszxzg40",
							},
						},
					},
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f",
							Addresses: []string{
								"bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav",
							},
						},
					},
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51201341e5a58314d89bcf5add2b2a68f109add5efb1ae774fa33c612da311f25904",
							Addresses: []string{
								"bc1pzdq7tfvrznvfhn66m54j5683pxkatma34em5lgeuvyk6xy0jtyzqjt48z3",
							},
						},
					},
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "512042b2d5c032b68220bfd6d4e26bc015129e168e87e22af743ffdc736708b7d342",
							Addresses: []string{
								"bc1pg2edtspjk6pzp07k6n3xhsq4z20pdr58ug40wsllm3ekwz9h6dpq77lhu9",
							},
						},
					},
				},
			},
			want: "071143e4ad12730965a5247ac15db8c81c89b0bc",
		},
		{
			name: "taproot duplicities",
			mtx: MempoolTx{
				Txid: "33a03f983b47725bbdd6045f2d5ee0d95dce08eaaf7104759758aabd8af27d34",
				Vin: []MempoolVin{
					{
						// bc1px2k5tu5mfq23ekkwncz5apx6ccw2nr0rne25r8t8zk7nu035ryxqn9ge8p
						AddrDesc: hexToBytes("512032ad45f29b48151cdace9e054e84dac61ca98de39e55419d6715bd3e3e34190c"),
					},
					{
						// bc1px2k5tu5mfq23ekkwncz5apx6ccw2nr0rne25r8t8zk7nu035ryxqn9ge8p
						AddrDesc: hexToBytes("512032ad45f29b48151cdace9e054e84dac61ca98de39e55419d6715bd3e3e34190c"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "512032ad45f29b48151cdace9e054e84dac61ca98de39e55419d6715bd3e3e34190c",
							Addresses: []string{
								"bc1px2k5tu5mfq23ekkwncz5apx6ccw2nr0rne25r8t8zk7nu035ryxqn9ge8p",
							},
						},
					},
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "512032ad45f29b48151cdace9e054e84dac61ca98de39e55419d6715bd3e3e34190c",
							Addresses: []string{
								"bc1px2k5tu5mfq23ekkwncz5apx6ccw2nr0rne25r8t8zk7nu035ryxqn9ge8p",
							},
						},
					},
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "512032ad45f29b48151cdace9e054e84dac61ca98de39e55419d6715bd3e3e34190c",
							Addresses: []string{
								"bc1px2k5tu5mfq23ekkwncz5apx6ccw2nr0rne25r8t8zk7nu035ryxqn9ge8p",
							},
						},
					},
				},
			},
			want: "01778db0",
		},
		{
			name: "partial taproot",
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
							Hex: "00145f997834e1135e893b7707ba1b12bcb8d74b821d",
							Addresses: []string{
								"bc1qt7vhsd8pzd0gjwmhq7apky4uhrt5hqsa2y58nl",
							},
						},
					},
				},
			},
			want: "011aeee8",
		},
		{
			name: "no taproot",
			mtx: MempoolTx{
				Txid: "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
				Vin: []MempoolVin{
					{
						// 39ECUF8YaFRX7XfttfAiLa5ir43bsrQUZJ
						AddrDesc: hexToBytes("a91452ae9441d9920d9eb4a3c0a877ca8d8de547ce6587"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "00145f997834e1135e893b7707ba1b12bcb8d74b821d",
							Addresses: []string{
								"bc1qt7vhsd8pzd0gjwmhq7apky4uhrt5hqsa2y58nl",
							},
						},
					},
				},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.computeGolombFilter(&tt.mtx)
			if got != tt.want {
				t.Errorf("MempoolBitcoinType.computeGolombFilter() = %v, want %v", got, tt.want)
			}
			if got != "" {
				// build the filter from computed value
				filter, err := gcs.FromNBytes(m.golombFilterP, m.golombFilterM, hexToBytes(got))
				if err != nil {
					t.Errorf("gcs.BuildGCSFilter() unexpected error %v", err)
				}
				// check that the vin scripts match the filter
				b, _ := hex.DecodeString(tt.mtx.Txid)
				for i := range tt.mtx.Vin {
					match, err := filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), tt.mtx.Vin[i].AddrDesc)
					if err != nil {
						t.Errorf("filter.Match vin[%d] unexpected error %v", i, err)
					}
					if match != isTaproot(tt.mtx.Vin[i].AddrDesc) {
						t.Errorf("filter.Match vin[%d] got %v, want %v", i, match, isTaproot(tt.mtx.Vin[i].AddrDesc))
					}
				}
				// check that the vout scripts match the filter
				for i := range tt.mtx.Vout {
					s := hexToBytes(tt.mtx.Vout[i].ScriptPubKey.Hex)
					match, err := filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), s)
					if err != nil {
						t.Errorf("filter.Match vout[%d] unexpected error %v", i, err)
					}
					if match != isTaproot(s) {
						t.Errorf("filter.Match vout[%d] got %v, want %v", i, match, isTaproot(s))
					}
				}
				// check that a random script does not match the filter
				match, err := filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), randomScript)
				if err != nil {
					t.Errorf("filter.Match randomScript unexpected error %v", err)
				}
				if match != false {
					t.Errorf("filter.Match randomScript got true, want false")
				}
			}
		})
	}
}
