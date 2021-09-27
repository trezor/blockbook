// +build unittest

package trezarcoin

import (
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func Test_GetAddrDescFromAddress_Mainnet(t *testing.T) {
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
			args:    args{address: "TovkYkEtp73t4KYEJxMxXhBMKVdDPmr7Hv"},
			want:    "76a914a05ddd8e3268846a3e7a4ddf505adb942cc6557488ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "TuEoL199onxdg7z69D12AmhMnEEAH742Ro"},
			want:    "76a914daa04d741763566e77a9df316f6cf755e8e77d3088ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "4Nx2k3S57z4PbUoP9M6BpQBCpizn8critB"},
			want:    "a9146568dc26eb0054c19042114cae9cff56e816a06c87",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "4XvMi1G8rXtgZnz5G8S9yA3QzTtaC8TLrY"},
			want:    "a914c7d0fdbdc654f7154b014f83b9d607f3adfbf4f887",
			wantErr: false,
		},
	}
	parser := NewTrezarcoinParser(GetChainParams("main"), &btc.Configuration{})

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
