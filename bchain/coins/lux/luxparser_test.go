// +build unittest

 package lux

 import (
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

 	"github.com/martinboehm/btcutil/chaincfg"
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
			name:    "P2PKH1",
			args:    args{address: "LViS5Ziwf5kh97A3dRQxT68gK7mtErJoAJ"},
			want:    "76a91473141fef01a297fe8555aa749f83988b0f89f15588ac",
			wantErr: false,
		}
	}
	parser := NewLuxParser(GetChainParams("main"), &btc.Configuration{})

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