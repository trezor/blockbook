// //go:build unittest

package bchain

import (
	"encoding/hex"
	"testing"
)

func getCommonAddressDescriptors() []AddressDescriptor {
	return []AddressDescriptor{
		// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
		hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
		// bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav
		hexToBytes("5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f"),
		// 39ECUF8YaFRX7XfttfAiLa5ir43bsrQUZJ
		hexToBytes("a91452ae9441d9920d9eb4a3c0a877ca8d8de547ce6587"),
	}
}

func TestGolombFilter(t *testing.T) {
	tests := []struct {
		name               string
		p                  uint8
		useZeroedKey       bool
		filterScripts      string
		key                string
		addressDescriptors []AddressDescriptor
		wantError          bool
		wantEnabled        bool
		want               string
	}{
		{
			name:               "taproot",
			p:                  20,
			useZeroedKey:       false,
			filterScripts:      "taproot",
			key:                "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: getCommonAddressDescriptors(),
			wantEnabled:        true,
			wantError:          false,
			want:               "0235dddcce5d60",
		},
		{
			name:               "taproot-zeroed-key",
			p:                  20,
			useZeroedKey:       true,
			filterScripts:      "taproot",
			key:                "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: getCommonAddressDescriptors(),
			wantEnabled:        true,
			wantError:          false,
			want:               "0218c23a013600",
		},
		{
			name:               "taproot p=21",
			p:                  21,
			useZeroedKey:       false,
			filterScripts:      "taproot",
			key:                "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: getCommonAddressDescriptors(),
			wantEnabled:        true,
			wantError:          false,
			want:               "0235ddda672eb0",
		},
		{
			name:               "all",
			p:                  20,
			useZeroedKey:       false,
			filterScripts:      "",
			key:                "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: getCommonAddressDescriptors(),
			wantEnabled:        true,
			wantError:          false,
			want:               "0350ccc61ac611976c80",
		},
		{
			name:               "taproot-noordinals",
			p:                  20,
			useZeroedKey:       false,
			filterScripts:      "taproot-noordinals",
			key:                "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: getCommonAddressDescriptors(),
			wantEnabled:        true,
			wantError:          false,
			want:               "0235dddcce5d60",
		},
		{
			name:          "not supported filter",
			p:             20,
			useZeroedKey:  false,
			filterScripts: "notsupported",
			wantEnabled:   false,
			wantError:     true,
			want:          "",
		},
		{
			name:          "not enabled",
			p:             0,
			useZeroedKey:  false,
			filterScripts: "",
			wantEnabled:   false,
			wantError:     false,
			want:          "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gf, err := NewGolombFilter(tt.p, tt.filterScripts, tt.key, tt.useZeroedKey)
			if err != nil && !tt.wantError {
				t.Errorf("TestGolombFilter.NewGolombFilter() got unexpected error '%v'", err)
				return
			}
			if err == nil && tt.wantError {
				t.Errorf("TestGolombFilter.NewGolombFilter() wanted error, got none")
				return
			}
			if gf == nil && tt.wantError {
				return
			}
			if gf.Enabled != tt.wantEnabled {
				t.Errorf("TestGolombFilter.NewGolombFilter() got gf.Enabled %v, want %v", gf.Enabled, tt.wantEnabled)
				return
			}
			for _, ad := range tt.addressDescriptors {
				gf.AddAddrDesc(ad, nil)
			}
			f := gf.Compute()
			got := hex.EncodeToString(f)
			if got != tt.want {
				t.Errorf("TestGolombFilter Compute() got %v, want %v", got, tt.want)
			}
		})
	}
}

// Preparation transaction, locking BTC redeemable by ordinal witness - parent of the reveal transaction
func getOrdinalCommitTx() (Tx, []AddressDescriptor) {
	tx := Tx{
		// https://mempool.space/tx/11111c17cbe86aebab146ee039d4e354cb55a9fb226ebdd2e30948630e7710ad
		Txid: "11111c17cbe86aebab146ee039d4e354cb55a9fb226ebdd2e30948630e7710ad",
		Vin: []Vin{
			{
				// https://mempool.space/tx/c4cae52a6e681b66c85c12feafb42f3617f34977032df1ee139eae07370863ef
				Txid: "c163fe1fdc21269cb05621adec38045e46a65289a356f9354df6010bce064916",
				Vout: 0,
				Witness: [][]byte{
					hexToBytes("0371633164dd16345c02e80c9963042f9a502aa2c8109c0f61da333ac1503c3ce2a1b79895359bbdee5979ab2cb44f3395892e1c419c3a8f67d31d33d7e764c9"),
				},
			},
		},
		Vout: []Vout{
			{
				ScriptPubKey: ScriptPubKey{
					Hex: "51206a711358bac6ca8f7ddfdf8f733546e658208122939f0bf7a3727f8143dfbbff",
					Addresses: []string{
						"bc1pdfc3xk96cm9g7lwlm78hxd2xuevzpqfzjw0shaarwflczs7lh0lstksdn0",
					},
				},
			},
			{
				ScriptPubKey: ScriptPubKey{
					Hex: "a9144390d0b3d2b6d48b8c205ffbe40b2d84c40de07f87",
					Addresses: []string{
						"37rGgLSLX6C6LS9am4KWd6GT1QCEP4H4py",
					},
				},
			},
			{
				ScriptPubKey: ScriptPubKey{
					Hex: "76a914ba6b046dd832aa8bc41c158232bcc18211387c4388ac",
					Addresses: []string{
						"1HzgtNdRCXszf95rFYemsDSHJQBbs9rbZf",
					},
				},
			},
		},
	}
	addressDescriptors := []AddressDescriptor{
		// bc1pdfc3xk96cm9g7lwlm78hxd2xuevzpqfzjw0shaarwflczs7lh0lstksdn0
		hexToBytes("51206a711358bac6ca8f7ddfdf8f733546e658208122939f0bf7a3727f8143dfbbff"),
		// 37rGgLSLX6C6LS9am4KWd6GT1QCEP4H4py
		hexToBytes("a9144390d0b3d2b6d48b8c205ffbe40b2d84c40de07f87"),
		// 1HzgtNdRCXszf95rFYemsDSHJQBbs9rbZf
		hexToBytes("76a914ba6b046dd832aa8bc41c158232bcc18211387c4388ac"),
	}
	return tx, addressDescriptors
}

// Transaction containing the actual ordinal data in witness - child of the commit transaction
func getOrdinalRevealTx() (Tx, []AddressDescriptor) {
	tx := Tx{
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
	}
	addressDescriptors := []AddressDescriptor{
		// bc1pdpgtz7trphc0wqf2u2c3rwh62t4mnd2wzs6lcnucl0s27mu4qa4q4md9ta
		hexToBytes("51206850b179630df0f7012ae2b111bafa52ebb9b54e1435fc4f98fbe0af6f95076a"),
	}
	return tx, addressDescriptors
}

func TestGolombIsOrdinal(t *testing.T) {
	revealTx, _ := getOrdinalRevealTx()
	if txContainsOrdinal(&revealTx) != true {
		t.Error("Ordinal not found in reveal Tx")
	}
	commitTx, _ := getOrdinalCommitTx()
	if txContainsOrdinal(&commitTx) != false {
		t.Error("Ordinal found in commit Tx, but should not be there")
	}
}

func TestGolombOrdinalTransactions(t *testing.T) {
	tests := []struct {
		name          string
		filterScripts string
		want          string
	}{
		{
			name:          "all",
			filterScripts: "",
			want:          "04256e660160e42ff40ee320", // take all four descriptors
		},
		{
			name:          "taproot",
			filterScripts: "taproot",
			want:          "0212b734c2ebe0", // filter out two non-taproot ones
		},
		{
			name:          "taproot-noordinals",
			filterScripts: "taproot-noordinals",
			want:          "", // ignore everything
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gf, err := NewGolombFilter(20, tt.filterScripts, "", true)
			if err != nil {
				t.Errorf("TestGolombOrdinalTransactions.NewGolombFilter() got unexpected error '%v'", err)
				return
			}

			commitTx, addressDescriptorsCommit := getOrdinalCommitTx()
			revealTx, addressDescriptorsReveal := getOrdinalRevealTx()

			for _, ad := range addressDescriptorsCommit {
				gf.AddAddrDesc(ad, &commitTx)
			}
			for _, ad := range addressDescriptorsReveal {
				gf.AddAddrDesc(ad, &revealTx)
			}

			f := gf.Compute()
			got := hex.EncodeToString(f)
			if got != tt.want {
				t.Errorf("TestGolombOrdinalTransactions Compute() got %v, want %v", got, tt.want)
			}
		})
	}
}
