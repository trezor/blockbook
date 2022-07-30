//go:build unittest

package feathercoin

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
			args:    args{address: "6z2aHdD9qpg5J2MW8dsbKTDTekVFG37y2S"},
			want:    "76a914de9fd965bc63fc198fd7c37de4328ce0bdec88a488ac",
			wantErr: false,
		},
		{
			name:    "pubkeyhash2",
			args:    args{address: "729vtmzauQdywB94HAJbtH3zLmUPrmfQws"},
			want:    "76a914f5f436ad39d4e2b4ecf8c45cafa068b4f7a5a05e88ac",
			wantErr: false,
		},
		{
			name:    "scripthash1",
			args:    args{address: "3FfGSMgjEdR7UK8t7WyqSZJy1LEUVTgrFx"},
			want:    "a914993d0b74ca784f2817a82b8991bb373e2e6c04eb87",
			wantErr: false,
		},
		{
			name:    "scripthash2",
			args:    args{address: "3MCEGQuc8gfCavDBDuUS4Lz1jyanSs3ijG"},
			want:    "a914d5f0c17f0f028ea834ad8fda350f8f5b745ee10387",
			wantErr: false,
		},
	}
	parser := NewFeathercoinParser(GetChainParams("main"), &btc.Configuration{})

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
