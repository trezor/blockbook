// +build unittest

package dcr

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"
)

var (
	parser *DecredParser

	testTx1 = bchain.Tx{
		Hex:       "01000000012372568fe80d2f9b2ab17226158dd5732d9926dc705371eaf40ab748c9e3d9720200000001ffffffff02644b252d0000000000001976a914a862f83733cc368f386a651e03d844a5bd6116d588acacdf63090000000000001976a91491dc5d18370939b3414603a0729bcb3a38e4ef7688ac000000000000000001e48d893600000000bb3d0000020000006a4730440220378e1442cc17fa7e49184518713eedd30e13e42147e077859557da6ffbbd40c702205f85563c28b6287f9c9110e6864dd18acfd92d85509ea846913c28b6e8a7f940012102bbbd7aadef33f2d2bdd9b0c5ba278815f5d66a6a01d2c019fb73f697662038b5",
		Blocktime: 1535632670,
		Time:      1535632670,
		Txid:      "132acb5b474b45b830f7961c91c87e53cce3a37a6c6f0b0933ccdf0395c81a6a",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				Txid:     "72d9e3c948b70af4ea715370dc26992d73d58d152672b12a9b2f0de88f567223",
				Vout:     2,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(757418852),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914a862f83733cc368f386a651e03d844a5bd6116d588ac",
					Addresses: []string{
						"TsgNUZKEnUhFASLESj7fVRTkgue3QR9TAeZ",
					},
				},
			},
			{
				ValueSat: *big.NewInt(157540268),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91491dc5d18370939b3414603a0729bcb3a38e4ef7688ac",
					Addresses: []string{
						"TseKNSWYbAzaGogpnNn25teTz53PTk3sgPu",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "0100000001c56d80756eaa7fc6e3542b29f596c60a9bcc959cf04d5f6e6b12749e241ece290200000001ffffffff02cf20b42d0000000000001976a9140799daa3cd36b44def220886802eb99e10c4a7c488ac0c25c7070000000000001976a9140b102deb3314213164cb6322211225365658407e88ac000000000000000001afa87b3500000000e33d0000000000006a47304402201ff342e5aa55b6030171f85729221ca0b81938826cc09449b77752e6e3b615be0220281e160b618e57326b95a0e0c3ac7a513bd041aba63cbace2f71919e111cfdba01210290a8de6665c8caac2bb8ca1aabd3dc09a334f997f97bd894772b1e51cab003d9",
		Blocktime: 1535638326,
		Time:      1535638326,
		Txid:      "caf34c934d4c36b410c0265222b069f52e2df459ebb09d6797a635ceee0edd60",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				Txid:     "29ce1e249e74126b6e5f4df09c95cc9b0ac696f5292b54e3c67faa6e75806dc5",
				Vout:     2,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(766779599),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9140799daa3cd36b44def220886802eb99e10c4a7c488ac",
					Addresses: []string{
						"TsRiKWsS9ucaqYDw9qhg6NukTthS5LwTRnv",
					},
				},
			},
			{
				ValueSat: *big.NewInt(13049166),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9140b102deb3314213164cb6322211225365658407e88ac",
					Addresses: []string{
						"TsS2dHqESY1vffjddpo1VMTbwLnDspfEj5W",
					},
				},
			},
		},
	}
)

func TestMain(m *testing.M) {
	parser = NewDecredParser(GetChainParams("testnet3"), &btc.Configuration{})
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestGetAddrDescFromAddress(t *testing.T) {
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
			name:    "P2PKH",
			args:    args{address: "TcrypGAcGCRVXrES7hWqVZb5oLJKCZEtoL1"},
			want:    "5463727970474163474352565872455337685771565a62356f4c4a4b435a45746f4c31",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "TsfDLrRkk9ciUuwfp2b8PawwnukYD7yAjGd"},
			want:    "547366444c72526b6b3963695575776670326238506177776e756b59443779416a4764",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "TsTevp3WYTiV3X1qjvZqa7nutuTqt5VNeoU"},
			want:    "547354657670335759546956335831716a765a7161376e75747554717435564e656f55",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestGetAddrDescFromVout(t *testing.T) {
	type args struct {
		vout bchain.Vout
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a914936f3a56a2dd0fb3bfde6bc820d4643e1701542a88ac"}}},
			want:    "54736554683431516f356b594c3337614c474d535167346e67636f71396a7a44583659",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a9144b31f712b03837b1303cddcb1ae9abd98da44f1088ac"}}},
			want:    "547358736a3161747744736455746e354455576b666f6d5a586e4a6151467862395139",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a9140d85a1d3f77383eb3dacfd83c46e2c7915aba91d88ac"}}},
			want:    "54735346644c79657942776e68486978737367784b34546f4664763876525931793871",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromVout(&tt.args.vout)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddrDescFromVout() error = %v, wantErr %v", err, tt.wantErr)
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("GetAddrDescFromVout() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestGetAddressesFromAddrDesc(t *testing.T) {
	type args struct {
		script string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		want2   bool
		wantErr bool
	}{
		{
			name:    "P2PKH",
			args:    args{script: "5463727970474163474352565872455337685771565a62356f4c4a4b435a45746f4c31"},
			want:    []string{"TcrypGAcGCRVXrES7hWqVZb5oLJKCZEtoL1"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{script: "547366444c72526b6b3963695575776670326238506177776e756b59443779416a4764"},
			want:    []string{"TsfDLrRkk9ciUuwfp2b8PawwnukYD7yAjGd"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{script: "547354657670335759546956335831716a765a7161376e75747554717435564e656f55"},
			want:    []string{"TsTevp3WYTiV3X1qjvZqa7nutuTqt5VNeoU"},
			want2:   true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got2, tt.want2) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got2, tt.want2)
			}
		})
	}
}
