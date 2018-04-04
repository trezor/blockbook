package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestEthParser_GetAddrIDFromAddress(t *testing.T) {
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
			name: "with 0x prefix",
			args: args{address: "0x81b7e08f65bdf5648606c89998a9cc8164397647"},
			want: "81b7e08f65bdf5648606c89998a9cc8164397647",
		},
		{
			name: "without 0x prefix",
			args: args{address: "47526228d673e9f079630d6cdaff5a2ed13e0e60"},
			want: "47526228d673e9f079630d6cdaff5a2ed13e0e60",
		},
		{
			name: "odd address",
			args: args{address: "7526228d673e9f079630d6cdaff5a2ed13e0e60"},
			want: "07526228d673e9f079630d6cdaff5a2ed13e0e60",
		},
		{
			name:    "ErrAddressMissing",
			args:    args{address: ""},
			want:    "",
			wantErr: true,
		},
		{
			name:    "error - not eth address",
			args:    args{address: "1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &EthereumParser{}
			got, err := p.GetAddrIDFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthParser.GetAddrIDFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("EthParser.GetAddrIDFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestEthereumParser_PackTx(t *testing.T) {
	type args struct {
		tx        *bchain.Tx
		height    uint32
		blockTime int64
	}
	tests := []struct {
		name    string
		p       *EthereumParser
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "1",
			args: args{
				tx: &bchain.Tx{
					Blocktime: 1521515026,
					Hex:       "7b226e6f6e6365223a2230783239666165222c226761735072696365223a223078313261303566323030222c22676173223a2230786462626130222c22746f223a22307836383262373930336131313039386366373730633761656634616130326138356233663336303161222c2276616c7565223a22307830222c22696e707574223a223078663032356361616630303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030323235222c2268617368223a22307865366231363864366262336438656437386530336462663832386236626664316662363133663665313239636261363234393634393834353533373234633564222c22426c6f636b4e756d626572223a223078326263616630222c22426c6f636b48617368223a22307865636364366230303331303135613139636237643465313066323835393062613635613661353461643162616133323262353066653561643136393033383935222c2246726f6d223a22307864616363396336313735346130633436313666633533323364633934366538396562323732333032222c227472616e73616374696f6e496e646578223a22307831222c2276223a2230783162222c2272223a22307831626434306133313132326330333931386466366431363664373430613661336132326630386132353933346365623136383863363239373736363163383063222c2273223a22307836303766626331356331663739393561343235386635613962636363363362303430333632643139393164356566653133363163353632323265346361383966227d",
					Time:      1521515026,
					Txid:      "e6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d",
					Vin: []bchain.Vin{
						{
							Addresses: []string{"dacc9c61754a0c4616fc5323dc946e89eb272302"},
						},
					},
					Vout: []bchain.Vout{
						{
							ScriptPubKey: bchain.ScriptPubKey{
								Addresses: []string{"682b7903a11098cf770c7aef4aa02a85b3f3601a"},
							},
						},
					},
				},
				height:    2870000,
				blockTime: 1521515026,
			},
			want: "08aebf0a1205012a05f20018a0f7362a24f025caaf00000000000000000000000000000000000000000000000000000000000002253220e6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d38f095af014092f4c1d5054a14682b7903a11098cf770c7aef4aa02a85b3f3601a5214dacc9c61754a0c4616fc5323dc946e89eb272302580162011b6a201bd40a31122c03918df6d166d740a6a3a22f08a25934ceb1688c62977661c80c7220607fbc15c1f7995a4258f5a9bccc63b040362d1991d5efe1361c56222e4ca89f",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &EthereumParser{}
			got, err := p.PackTx(tt.args.tx, tt.args.height, tt.args.blockTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthereumParser.PackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("EthereumParser.PackTx() = %v, want %v", h, tt.want)
			}
		})
	}
}
