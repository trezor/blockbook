// build unittest

package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"
)

func TestEthParser_GetAddrDescFromAddress(t *testing.T) {
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
			p := NewEthereumParser()
			got, err := p.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthParser.GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("EthParser.GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

var (
	testTx1, testTx2 bchain.Tx
	testTxPacked1    = "08aebf0a1205012a05f20018a0f73622081234567890abcdef2a24f025caaf00000000000000000000000000000000000000000000000000000000000002253220e6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d38f095af014092f4c1d5054a14682b7903a11098cf770c7aef4aa02a85b3f3601a5214dacc9c61754a0c4616fc5323dc946e89eb272302580162011b6a201bd40a31122c03918df6d166d740a6a3a22f08a25934ceb1688c62977661c80c7220607fbc15c1f7995a4258f5a9bccc63b040362d1991d5efe1361c56222e4ca89f"
	testTxPacked2    = "08ece40212050430e234001888a4012201213220cd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b38889eaf0140fa83c3d5054a14555ee11fbddc0e49a9bab358a8941ad95ffdb48f52143e3a3d69dc66ba10737f531ed088954a9ec89d97580a6201296a20f7161c170d43573ad9c8d701cdaf714ff2a548a562b0dc639230d17889fcd40572203c4977fc90385a27efa0032e17b49fd575b2826cb56e3d1ecf21524f2a94f915"
)

func init() {

	testTx1 = bchain.Tx{
		Blocktime: 1521515026,
		Hex:       "7b226e6f6e6365223a2230783239666165222c226761735072696365223a223078313261303566323030222c22676173223a2230786462626130222c22746f223a22307836383262373930336131313039386366373730633761656634616130326138356233663336303161222c2276616c7565223a22307831323334353637383930616263646566222c22696e707574223a223078663032356361616630303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030323235222c2268617368223a22307865366231363864366262336438656437386530336462663832386236626664316662363133663665313239636261363234393634393834353533373234633564222c22626c6f636b4e756d626572223a223078326263616630222c2266726f6d223a22307864616363396336313735346130633436313666633533323364633934366538396562323732333032222c227472616e73616374696f6e496e646578223a22307831222c2276223a2230783162222c2272223a22307831626434306133313132326330333931386466366431363664373430613661336132326630386132353933346365623136383863363239373736363163383063222c2273223a22307836303766626331356331663739393561343235386635613962636363363362303430333632643139393164356566653133363163353632323265346361383966227d",
		Time:      1521515026,
		Txid:      "0xe6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0xdacc9c61754a0c4616fc5323dc946e89eb272302"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1311768467294899695),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x682b7903a11098cf770c7aef4aa02a85b3f3601a"},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Blocktime: 1521533434,
		Hex:       "7b226e6f6e6365223a22307862323663222c226761735072696365223a223078343330653233343030222c22676173223a22307835323038222c22746f223a22307835353565653131666264646330653439613962616233353861383934316164393566666462343866222c2276616c7565223a2230783231222c22696e707574223a223078222c2268617368223a22307863643634373135313535326235313332623261656637633962653030646336663733616663353930316464653135376161623133313333356261616138353362222c22626c6f636b4e756d626572223a223078326263663038222c2266726f6d223a22307833653361336436396463363662613130373337663533316564303838393534613965633839643937222c227472616e73616374696f6e496e646578223a22307861222c2276223a2230783239222c2272223a22307866373136316331373064343335373361643963386437303163646166373134666632613534386135363262306463363339323330643137383839666364343035222c2273223a22307833633439373766633930333835613237656661303033326531376234396664353735623238323663623536653364316563663231353234663261393466393135227d",
		Time:      1521533434,
		Txid:      "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x3e3a3d69dc66ba10737f531ed088954a9ec89d97"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(33),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x555ee11fbddc0e49a9bab358a8941ad95ffdb48f"},
				},
			},
		},
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
				tx:        &testTx1,
				height:    2870000,
				blockTime: 1521515026,
			},
			want: testTxPacked1,
		},
		{
			name: "2",
			args: args{
				tx:        &testTx2,
				height:    2871048,
				blockTime: 1521533434,
			},
			want: testTxPacked2,
		},
	}
	p := NewEthereumParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestEthereumParser_UnpackTx(t *testing.T) {
	type args struct {
		hex string
	}
	tests := []struct {
		name    string
		p       *EthereumParser
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name:  "1",
			args:  args{hex: testTxPacked1},
			want:  &testTx1,
			want1: 2870000,
		},
		{
			name:  "2",
			args:  args{hex: testTxPacked2},
			want:  &testTx2,
			want1: 2871048,
		},
	}
	p := NewEthereumParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := hex.DecodeString(tt.args.hex)
			if err != nil {
				panic(err)
			}
			got, got1, err := p.UnpackTx(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthereumParser.UnpackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EthereumParser.UnpackTx() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("EthereumParser.UnpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
