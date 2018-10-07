// +build unittest

package digibyte

import (
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"github.com/jakm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func TestAddressToOutputScript_Mainnet(t *testing.T) {
	type args struct {
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "pubkeyhash1",
			args:    args{address: "DFDe9ne77eEUKUijjG4EpDwW9vDxckGgHN"},
			want:    "76a9146e8d4f7f0dfeb5d69b9a2cf914a1a2e276312b2188ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "DPUnoXeaSDnNtQTa7U3nEMTYBVgJ6wVgCh"},
			want:    "76a914c92bc70927a752deb91cf0361dcdb60bdac6a1d588ac",
			wantErr: false,
		},
		// TODO - complete
		// {
		// 	name:    "scripthash1",
		// 	args:    args{address: "36c8VAv74dPZZa4cFayb92hzozkPL4fBPe"},
		// 	want:    "a91435ec06fa05f2d3b16e88cd7eda7651a10ca2e01987",
		// 	wantErr: false,
		// },
		// {
		// 	name:    "scripthash2",
		// 	args:    args{address: "38A1RNvbA5c9wNRfyLVn1FCH5TPKJVG8YR"},
		// 	want:    "a91446eb90e002f137f05385896c882fe000cc2e967f87",
		// 	wantErr: false,
		// },
		// {
		// 	name:    "witness_v0_keyhash",
		// 	args:    args{address: "vtc1qd80qaputavyhtvszlz9zprueqch0qd003g520j"},
		// 	want:    "001469de0e878beb0975b202f88a208f99062ef035ef",
		// 	wantErr: false,
		// },
	}
	parser := NewDigiByteParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}
