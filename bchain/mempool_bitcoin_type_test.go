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
		filterScripts: "taproot",
	}
	golombFilterM := GetGolombParamM(m.golombFilterP)
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
			got := m.computeGolombFilter(&tt.mtx, nil)
			if got != tt.want {
				t.Errorf("MempoolBitcoinType.computeGolombFilter() = %v, want %v", got, tt.want)
			}
			if got != "" {
				// build the filter from computed value
				filter, err := gcs.FromNBytes(m.golombFilterP, golombFilterM, hexToBytes(got))
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
					if match != tt.mtx.Vin[i].AddrDesc.IsTaproot() {
						t.Errorf("filter.Match vin[%d] got %v, want %v", i, match, tt.mtx.Vin[i].AddrDesc.IsTaproot())
					}
				}
				// check that the vout scripts match the filter
				for i := range tt.mtx.Vout {
					s := hexToBytes(tt.mtx.Vout[i].ScriptPubKey.Hex)
					match, err := filter.Match(*(*[gcs.KeySize]byte)(b[:gcs.KeySize]), s)
					if err != nil {
						t.Errorf("filter.Match vout[%d] unexpected error %v", i, err)
					}
					if match != AddressDescriptor(s).IsTaproot() {
						t.Errorf("filter.Match vout[%d] got %v, want %v", i, match, AddressDescriptor(s).IsTaproot())
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

func TestMempoolBitcoinType_computeGolombFilter_taproot_noordinals(t *testing.T) {
	m := &MempoolBitcoinType{
		golombFilterP: 20,
		filterScripts: "taproot-noordinals",
	}
	tests := []struct {
		name string
		mtx  MempoolTx
		tx   Tx
		want string
	}{
		{
			name: "taproot-no-ordinals normal taproot tx",
			mtx: MempoolTx{
				Txid: "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
				Vin: []MempoolVin{
					{
						// bc1pdfc3xk96cm9g7lwlm78hxd2xuevzpqfzjw0shaarwflczs7lh0lstksdn0
						AddrDesc: hexToBytes("51206a711358bac6ca8f7ddfdf8f733546e658208122939f0bf7a3727f8143dfbbff"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51206850b179630df0f7012ae2b111bafa52ebb9b54e1435fc4f98fbe0af6f95076a",
							Addresses: []string{
								"bc1pdpgtz7trphc0wqf2u2c3rwh62t4mnd2wzs6lcnucl0s27mu4qa4q4md9ta",
							},
						},
					},
				},
			},
			tx: Tx{
				Vin: []Vin{
					{
						Witness: [][]byte{
							hexToBytes("737ad2835962e3d147cd74a578f1109e9314eac9d00c9fad304ce2050b78fac21a2d124fd886d1d646cf1de5d5c9754b0415b960b1319526fa25e36ca1f650ce"),
						},
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51206850b179630df0f7012ae2b111bafa52ebb9b54e1435fc4f98fbe0af6f95076a",
							Addresses: []string{
								"bc1pdpgtz7trphc0wqf2u2c3rwh62t4mnd2wzs6lcnucl0s27mu4qa4q4md9ta",
							},
						},
					},
				},
			},
			want: "02899e8c952b40",
		},
		{
			name: "taproot-no-ordinals ordinal tx",
			mtx: MempoolTx{
				Txid: "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
				Vin: []MempoolVin{
					{
						// bc1pdfc3xk96cm9g7lwlm78hxd2xuevzpqfzjw0shaarwflczs7lh0lstksdn0
						AddrDesc: hexToBytes("51206a711358bac6ca8f7ddfdf8f733546e658208122939f0bf7a3727f8143dfbbff"),
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51206850b179630df0f7012ae2b111bafa52ebb9b54e1435fc4f98fbe0af6f95076a",
							Addresses: []string{
								"bc1pdpgtz7trphc0wqf2u2c3rwh62t4mnd2wzs6lcnucl0s27mu4qa4q4md9ta",
							},
						},
					},
				},
			},
			tx: Tx{
				// https://mempool.space/tx/c4cae52a6e681b66c85c12feafb42f3617f34977032df1ee139eae07370863ef
				Txid: "c4cae52a6e681b66c85c12feafb42f3617f34977032df1ee139eae07370863ef",
				Vin: []Vin{
					{
						Txid: "11111c17cbe86aebab146ee039d4e354cb55a9fb226ebdd2e30948630e7710ad",
						Vout: 0,
						Witness: [][]byte{
							hexToBytes("737ad2835962e3d147cd74a578f1109e9314eac9d00c9fad304ce2050b78fac21a2d124fd886d1d646cf1de5d5c9754b0415b960b1319526fa25e36ca1f650ce"),
							hexToBytes("2029f34532e043fade4471779b4955005db8fa9b64c9e8d0a2dae4a38bbca23328ac0063036f726401010a696d6167652f77656270004d08025249464650020000574542505650384c440200002f57c2950067a026009086939b7785a104699656f4f53388355445b6415d22f8924000fd83bd31d346ca69f8fcfed6d8d18231846083f90f00ffbf203883666c36463c6ba8662257d789935e002192245bd15ac00216b080052cac85b380052c60e1593859f33a7a7abff7ed88feb361db3692341bc83553aef7aec75669ffb1ffd87fec3ff61ffb8ffdc736f20a96a0fba34071d4fdf111c435381df667728f95c4e82b6872d82471bfdc1665107bb80fd46df1686425bcd2e27eb59adc9d17b54b997ee96776a7c37ca2b57b9551bcffeb71d88768765af7384c2e3ba031ca3f19c9ddb0c6ec55223fbfe3731a1e8d7bb010de8532d53293bbbb6145597ee53559a612e6de4f8fc66936ef463eea7498555643ac0dafad6627575f2733b9fb352e411e7d9df8fc80fde75f5f66f5c5381a46b9a697d9c97555c4bf41a4909b9dd071557c3dfe0bfcd6459e06514266c65756ce9f25705230df63d30fef6076b797e1f49d00b41e87b5ccecb1c237f419e4b3ca6876053c14fc979a629459a62f78d735fb078bfa0e7a1fc69ad379447d817e06b3d7f1de820f28534f85fa20469cd6f93ddc6c5f2a94878fc64a98ac336294c99d27d11742268ae1a34cd61f31e2e4aee94b0ff496f55068fa727ace6ad2ec1e6e3f59e6a8bd154f287f652fbfaa05cac067951de1bfacc0e330c3bf6dd2efde4c509646566836eb71986154731daf722a6ff585001e87f9479559a61265d6e330f3682bf87ab2598fc3fca36da778e59cee71584594ef175e6d7d5f70d6deb02c4b371e5063c35669ffb1ffd87ffe0e730068"),
							hexToBytes("c129f34532e043fade4471779b4955005db8fa9b64c9e8d0a2dae4a38bbca23328"),
						},
					},
				},
				Vout: []Vout{
					{
						ScriptPubKey: ScriptPubKey{
							Hex: "51206850b179630df0f7012ae2b111bafa52ebb9b54e1435fc4f98fbe0af6f95076a",
							Addresses: []string{
								"bc1pdpgtz7trphc0wqf2u2c3rwh62t4mnd2wzs6lcnucl0s27mu4qa4q4md9ta",
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
			got := m.computeGolombFilter(&tt.mtx, &tt.tx)
			if got != tt.want {
				t.Errorf("MempoolBitcoinType.computeGolombFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}
