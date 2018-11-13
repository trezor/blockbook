// +build unittest

package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"fmt"
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
	testTxPacked1    = "08d38388021092f4c1d5051aa20108d001120509502f900018d5e1042a44a9059cbb00000000000000000000000008e93c026b6454b7437d097aabd550f98cb89ed300000000000000000000000000000000000000000000021e19e0c9bab24000003220a9cd088aba2131000da6f38a33c20169baee476218deea6b78720700b895b1013a144af4114f73d1c1c903ac9e0361b379d1291808a2421420cd153de35d469ba46127a0c8f18626b59a256a22a8010a02cb391201011a9e010a144af4114f73d1c1c903ac9e0361b379d1291808a2122000000000000000000000000000000000000000000000021e19e0c9bab24000001a20ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef1a2000000000000000000000000020cd153de35d469ba46127a0c8f18626b59a256a1a2000000000000000000000000008e93c026b6454b7437d097aabd550f98cb89ed3"
	testTxPacked2    = "08889eaf0110fa83c3d5051a6908ece40212050430e234001888a40122081bc0159d530e60003220cd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b3a14555ee11fbddc0e49a9bab358a8941ad95ffdb48f42143e3a3d69dc66ba10737f531ed088954a9ec89d97480a22070a025208120101"
)

func init() {

	testTx1 = bchain.Tx{
		Blocktime: 1521515026,
		Hex:       "7b227478223a7b226e6f6e6365223a2230786430222c226761735072696365223a223078393530326639303030222c22676173223a2230783133306435222c22746f223a22307834616634313134663733643163316339303361633965303336316233373964313239313830386132222c2276616c7565223a22307830222c22696e707574223a22307861393035396362623030303030303030303030303030303030303030303030303038653933633032366236343534623734333764303937616162643535306639386362383965643330303030303030303030303030303030303030303030303030303030303030303030303030303030303030303032316531396530633962616232343030303030222c2268617368223a22307861396364303838616261323133313030306461366633386133336332303136396261656534373632313864656561366237383732303730306238393562313031222c22626c6f636b4e756d626572223a223078343230316433222c2266726f6d223a22307832306364313533646533356434363962613436313237613063386631383632366235396132353661222c227472616e73616374696f6e496e646578223a22307830227d2c2272656365697074223a7b2267617355736564223a22307863623339222c22737461747573223a22307831222c226c6f6773223a5b7b2261646472657373223a22307834616634313134663733643163316339303361633965303336316233373964313239313830386132222c22746f70696373223a5b22307864646632353261643162653263383962363963326230363866633337386461613935326261376631363363346131313632386635356134646635323362336566222c22307830303030303030303030303030303030303030303030303032306364313533646533356434363962613436313237613063386631383632366235396132353661222c22307830303030303030303030303030303030303030303030303030386539336330323662363435346237343337643039376161626435353066393863623839656433225d2c2264617461223a22307830303030303030303030303030303030303030303030303030303030303030303030303030303030303030303032316531396530633962616232343030303030227d5d7d7d",
		Time:      1521515026,
		Txid:      "0xa9cd088aba2131000da6f38a33c20169baee476218deea6b78720700b895b101",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x20cd153de35d469ba46127a0c8f18626b59a256a"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(0),
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: []string{"0x4af4114f73d1c1c903ac9e0361b379d1291808a2"},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Blocktime: 1521533434,
		Hex:       "7b227478223a7b226e6f6e6365223a22307862323663222c226761735072696365223a223078343330653233343030222c22676173223a22307835323038222c22746f223a22307835353565653131666264646330653439613962616233353861383934316164393566666462343866222c2276616c7565223a22307831626330313539643533306536303030222c22696e707574223a223078222c2268617368223a22307863643634373135313535326235313332623261656637633962653030646336663733616663353930316464653135376161623133313333356261616138353362222c22626c6f636b4e756d626572223a223078326263663038222c2266726f6d223a22307833653361336436396463363662613130373337663533316564303838393534613965633839643937222c227472616e73616374696f6e496e646578223a22307861227d2c2272656365697074223a7b2267617355736564223a22307835323038222c22737461747573223a22307831222c226c6f6773223a5b5d7d7d",
		Time:      1521533434,
		Txid:      "0xcd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
		Vin: []bchain.Vin{
			{
				Addresses: []string{"0x3e3a3d69dc66ba10737f531ed088954a9ec89d97"},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(1999622000000000000),
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
				height:    4325843,
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
			want1: 4325843,
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
			// CoinSpecificData are not set in want struct
			got.CoinSpecificData = nil
			// DeepEqual compares empty nil slices as not equal
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("EthereumParser.UnpackTx() got = %+v, want %+v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("EthereumParser.UnpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
