// +build unittest

package dcr

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"os"
	"reflect"
	"testing"
)

var (
	decredParser *DecredParser
)

func TestMain(m *testing.M) {
	decredParser = NewDecredParser(GetChainParams("mainnet"),
		&btc.Configuration{
			Slip44: 42,
		})
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
			args:    args{address: "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu"},
			want:    "76a9142789d58cfa0957d206f025c2af056fc8a77cebb088ac",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "DsU7xcg53nxaKLLcAUSKyRndjG78Z2VZnX9"},
			want:    "76a914229ebac30efd6a69eec9c1a48e048b7c975c25f288ac",
			wantErr: false,
		},
		{
			name:    "P2PH",
			args:    args{address: "DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS"},
			want:    "a914f0b4e85100aee1a996f22915eb3c3f764d53779a87",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decredParser.GetAddrDescFromAddress(tt.args.address)
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
			want:    "76a914936f3a56a2dd0fb3bfde6bc820d4643e1701542a88ac",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a9144b31f712b03837b1303cddcb1ae9abd98da44f1088ac"}}},
			want:    "76a9144b31f712b03837b1303cddcb1ae9abd98da44f1088ac",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a9140d85a1d3f77383eb3dacfd83c46e2c7915aba91d88ac"}}},
			want:    "76a9140d85a1d3f77383eb3dacfd83c46e2c7915aba91d88ac",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decredParser.GetAddrDescFromVout(&tt.args.vout)
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
			args:    args{script: "76a914ad06dd6ddee55cbca9a9e3713bd7587509a3056488ac"},
			want:    []string{"Dsgjncbv1fYMywusjnrSBrzvAde8APEPP1f"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "standard p2pk with compressed pubkey (0x02)",
			args:    args{script: "2102192d74d0cb94344c9569c2e77901573d8d7903c3ebec3a957724895dca52c6b4ac"},
			want:    []string{"DsknZYjE1z9sTNPC9gUDTGPSEzibv7cqLQv"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed",
			args:    args{script: "410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"},
			want:    []string{"DsXoXicfH6CBTwtMzFTFJjgsRheGhCEpp2f"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a91463bcc565f9e68ee0189dd5cc67f1b0e5f02f45cb87"},
			want:    []string{"DcgYx6SzsWsaTFYEHwZ83wyKntCMiJYrJ3M"},
			want2:   true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := decredParser.GetAddressesFromAddrDesc(b)
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

func TestDeriveAddressDescriptors(t *testing.T) {
	// The public extended keys for test vectors in [BIP32].
	testVec1MasterPubKey := "dpubZF8BRmciAzYoTjXZ3bbRWLVCwUKtTquact3Tr6ye77Rgmw76VyqMb9TB9KpfrvUYEM5d1Au4fQzE2BbtxRjwzGsqnWHmtQP9UV1kxZaqvb6"
	testVec2MasterPubKey := "dpubZF4LSCdF9YKZfNzTVYhz4RBxsjYXqms8AQnMBHXZ8GUKoRSigG7kQnKiJt5pzk93Q8FxcdVBEkQZruSXduGtWnkwXzGnjbSovQ97dCxqaXc"

	type args struct {
		xpub    string
		change  uint32
		indexes []uint32
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "m/44'/42'/0'",
			args: args{
				xpub:    testVec1MasterPubKey,
				change:  0,
				indexes: []uint32{0},
			},
			want: []string{"Dsk2wpSUikQLpuN2TpkZdboNP4dNAYZESNH"},
		},
		{
			name: "m/44'/42'/0'/1234",
			args: args{
				xpub:    testVec2MasterPubKey,
				change:  0,
				indexes: []uint32{0, 1234},
			},
			want: []string{"DsnVjX18XzJdCDy2oa1r619ArrP9nFAyTGn", "DsjsgzQRHAQE489E74NGHccDF5i4pn4JJc7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decredParser.DeriveAddressDescriptors(tt.args.xpub, tt.args.change, tt.args.indexes)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveAddressDescriptors() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotAddresses := make([]string, len(got))
			for i, ad := range got {
				aa, _, err := decredParser.GetAddressesFromAddrDesc(ad)
				if err != nil || len(aa) != 1 {
					t.Errorf("DeriveAddressDescriptorsFromAddrDesc() got incorrect address descriptor %v, error %v", ad, err)
					return
				}
				gotAddresses[i] = aa[0]
			}
			if !reflect.DeepEqual(gotAddresses, tt.want) {
				t.Errorf("DeriveAddressDescriptors() = %v, want %v", gotAddresses, tt.want)
			}
		})
	}
}

func TestDeriveBasePath(t *testing.T) {
	type args struct {
		xpub string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "m/44'/42'/0'",
			args: args{
				xpub: "dpubZF8BRmciAzYoTjXZ3bbRWLVCwUKtTquact3Tr6ye77Rgmw76VyqMb9TB9KpfrvUYEM5d1Au4fQzE2BbtxRjwzGsqnWHmtQP9UV1kxZaqvb6",
			},
			want: "m/44'/42'/0'",
		},
		{
			name: "m/44'/42'/0'",
			args: args{
				xpub: "dpubZFf1tYMxcku9nvDRxnYdE4yrEESrkuQFRq5RwA4KoYQKpDSRszN2emePTwLgfQpd4mZHGrHbQkKPZdjH1BcopomXRnr5Gt43rjpNEfeuJLN",
			},
			want: "m/44'/42'/0'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decredParser.DerivationBasePath(tt.args.xpub)
			if (err != nil) != tt.wantErr {
				t.Errorf("BitcoinParser.DerivationBasePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BitcoinParser.DerivationBasePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
