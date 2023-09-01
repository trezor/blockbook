// //go:build unittest

package bchain

import (
	"encoding/hex"
	"testing"
)

func TestGolombFilter(t *testing.T) {
	tests := []struct {
		name               string
		p                  uint8
		filterScripts      string
		key                string
		addressDescriptors [][]byte
		wantError          bool
		wantEnabled        bool
		want               string
	}{
		{
			name:          "taproot",
			p:             20,
			filterScripts: "taproot",
			key:           "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: [][]byte{
				// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
				hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
				// bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav
				hexToBytes("5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f"),
				// 39ECUF8YaFRX7XfttfAiLa5ir43bsrQUZJ
				hexToBytes("a91452ae9441d9920d9eb4a3c0a877ca8d8de547ce6587"),
			},
			wantEnabled: true,
			wantError:   false,
			want:        "0235dddcce5d60",
		},
		{
			name:          "taproot p=21",
			p:             21,
			filterScripts: "taproot",
			key:           "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: [][]byte{
				// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
				hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
				// bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav
				hexToBytes("5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f"),
				// 39ECUF8YaFRX7XfttfAiLa5ir43bsrQUZJ
				hexToBytes("a91452ae9441d9920d9eb4a3c0a877ca8d8de547ce6587"),
			},
			wantEnabled: true,
			wantError:   false,
			want:        "0235ddda672eb0",
		},
		{
			name:          "all",
			p:             20,
			filterScripts: "",
			key:           "86336c62a63f509a278624e3f400cdd50838d035a44e0af8a7d6d133c04cc2d2",
			addressDescriptors: [][]byte{
				// bc1pgeqrcq5capal83ypxczmypjdhk4d9wwcea4k66c7ghe07p2qt97sqh8sy5
				hexToBytes("512046403c0298e87bf3c4813605b2064dbdaad2b9d8cf6b6d6b1e45f2ff0540597d"),
				// bc1p7en40zu9hmf9d3luh8evmfyg655pu5k2gtna6j7zr623f9tz7z0stfnwav
				hexToBytes("5120f667578b85bed256c7fcb9f2cda488d5281e52ca42e7dd4bc21e95149562f09f"),
				// 39ECUF8YaFRX7XfttfAiLa5ir43bsrQUZJ
				hexToBytes("a91452ae9441d9920d9eb4a3c0a877ca8d8de547ce6587"),
			},
			wantEnabled: true,
			wantError:   false,
			want:        "0350ccc61ac611976c80",
		},
		{
			name:          "not supported filter",
			p:             20,
			filterScripts: "notsupported",
			wantEnabled:   false,
			wantError:     true,
			want:          "",
		},
		{
			name:          "not enabled",
			p:             0,
			filterScripts: "",
			wantEnabled:   false,
			wantError:     false,
			want:          "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO add tests for useZeroedKey
			gf, err := NewGolombFilter(tt.p, tt.filterScripts, tt.key, false)
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
