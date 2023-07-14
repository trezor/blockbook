//go:build unittest

package dcr

import (
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

var (
	testnetParser, mainnetParser *DecredParser

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
		Hex:       "0100000001193c189c71dff482b70ccb10ec9cf0ea3421a7fc51e4c7b0cf59c98a293a2f960200000000ffffffff027c87f00b0000000000001976a91418f10131a859912119c4a8510199f87f0a4cec2488ac9889495f0000000000001976a914631fb783b1e06c3f6e71777e16da6de13450465e88ac0000000000000000015ced3d6b0000000030740000000000006a47304402204e6afc21f6d065b9c082dad81a5f29136320e2b54c6cdf6b8722e4507e1a8d8902203933c5e592df3b0bbb0568f121f48ef6cbfae9cf479a57229742b5780dedc57a012103b89bb443b6ab17724458285b302291b082c59e5a022f273af0f61d47a414a537",
		Txid:      "7058766ffef2e9cee61ee4b7604a39bc91c3000cb951c4f93f3307f6e0bf4def",
		Blocktime: 1463843967,
		Time:      1463843967,
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				Txid:     "962f3a298ac959cfb0c7e451fca72134eaf09cec10cb0cb782f4df719c183c19",
				Vout:     2,
				Sequence: 4294967295,
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402204e6afc21f6d065b9c082dad81a5f29136320e2b54c6cdf6b8722e4507e1a8d8902203933c5e592df3b0bbb0568f121f48ef6cbfae9cf479a57229742b5780dedc57a012103b89bb443b6ab17724458285b302291b082c59e5a022f273af0f61d47a414a537",
				},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(200312700),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91418f10131a859912119c4a8510199f87f0a4cec2488ac",
					Addresses: []string{
						"DsTEnRLDEjQNeQ4A47fdS2pqtaFrGNzkqNa",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1598654872),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914631fb783b1e06c3f6e71777e16da6de13450465e88ac",
					Addresses: []string{
						"Dsa12P9VnCd55hTnUXpvGgFKSeGkFkzRvYb",
					},
				},
			},
		},
	}

	testTx3 = bchain.Tx{
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
	testnetParser = NewDecredParser(GetChainParams("testnet3"), &btc.Configuration{Slip44: 1})
	mainnetParser = NewDecredParser(GetChainParams("mainnet"), &btc.Configuration{Slip44: 42})
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
			got, err := testnetParser.GetAddrDescFromAddress(tt.args.address)
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
			got, err := testnetParser.GetAddrDescFromVout(&tt.args.vout)
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
			got, got2, err := testnetParser.GetAddressesFromAddrDesc(b)
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
	type args struct {
		xpub    string
		change  uint32
		indexes []uint32
		parser  *DecredParser
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
				xpub:    "dpubZFYFpu8cZxwrApmtot59LZLChk5JcdB8xCxVQ4pcsTig4fscH3EfAkhxcKKhXBQH6SGyYs2VDidoomA5qukTWMaHDkBsAtnpodAHm61ozbD",
				change:  0,
				indexes: []uint32{0, 5},
				parser:  mainnetParser,
			},
			want: []string{"DsUPx4NgAJzUQFRXnn2XZnWwEeQkQpwhqFD", "DsaT4kaGCeJU1Fef721J2DNt8UgcrmE2UsD"},
		},
		{
			name: "m/44'/42'/1'",
			args: args{
				xpub:    "dpubZFYFpu8cZxwrESo75eazNjVHtC4nWJqL5aXxExZHKnyvZxKirkpypbgeJhVzhTdfnK2986DLjich4JQqcSaSyxu5KSoZ25KJ67j4mQJ9iqx",
				change:  0,
				indexes: []uint32{0, 5},
				parser:  mainnetParser,
			},
			want: []string{"DsX5px9k9XZKFNP2Z9kyZBbfHgecm1ftNz6", "Dshjbo35CSWwNo7xMgG7UM8AWykwEjJ5DCP"},
		},
		{
			name: "m/44'/1'/0'",
			args: args{
				xpub:    "tpubVossdTiJthe9xZZ5rz47szxN6ncpLJ4XmtJS26hKciDUPtboikdwHKZPWfo4FWYuLRZ6MNkLjyPRKhxqjStBTV2BE1LCULznpqsFakkPfPr",
				change:  0,
				indexes: []uint32{0, 2},
				parser:  testnetParser,
			},
			want: []string{"TsboBwzpaH831s9J63XDcDx5GbKLcwv9ujo", "TsXrNt9nP3kBUM2Wr3rQGoPrpL7RMMSJyJH"},
		},
		{
			name: "m/44'/1'/1'",
			args: args{
				xpub:    "tpubVossdTiJtheA1fQniKn9EN1JE1Eq1kBofaq2KwywrvuNhAk1KsEM7J2r8anhMJUmmcn9Wmoh73EctpW7Vxs3gS8cbF7N3m4zVjzuyvBj3qC",
				change:  0,
				indexes: []uint32{0, 3},
				parser:  testnetParser,
			},
			want: []string{"TsndBjzcwZVjoZEuqYKwiMbCJH9QpkEekg4", "TsbrkVdFciW3Lfh1W8qjwRY9uSbdiBmY4VP"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor, err := tt.args.parser.ParseXpub(tt.args.xpub)
			if err != nil {
				t.Errorf("ParseXpub() error = %v", err)
				return
			}
			got, err := tt.args.parser.DeriveAddressDescriptors(descriptor, tt.args.change, tt.args.indexes)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveAddressDescriptorsFromTo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotAddresses := make([]string, len(got))
			for i, ad := range got {
				aa, _, err := tt.args.parser.GetAddressesFromAddrDesc(ad)
				if err != nil || len(aa) != 1 {
					t.Errorf("DeriveAddressDescriptorsFromTo() got incorrect address descriptor %v, error %v", ad, err)
					return
				}
				gotAddresses[i] = aa[0]
			}
			if !reflect.DeepEqual(gotAddresses, tt.want) {
				t.Errorf("DeriveAddressDescriptorsFromTo() = %v, want %v", gotAddresses, tt.want)
			}
		})
	}
}

func TestDeriveAddressDescriptorsFromTo(t *testing.T) {
	type args struct {
		xpub      string
		change    uint32
		fromIndex uint32
		toIndex   uint32
		parser    *DecredParser
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "m/44'/42'/2'",
			args: args{
				xpub:      "dpubZFYFpu8cZxwrGnWbdHmvsAcTaMve4W9EAUiSHzXp1c5hQvfeWgk7LxsE5LqopwfxV62CoB51fxw97YaNpdA3tdo4GHbLxtUzRmYcUtVPYUi",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    mainnetParser,
			},
			want: []string{"Dshtd1N7pKw814wgWXUq5qFVC5ENQ9oSGK7"},
		},
		{
			name: "m/44'/42'/1'",
			args: args{
				xpub:      "dpubZFYFpu8cZxwrESo75eazNjVHtC4nWJqL5aXxExZHKnyvZxKirkpypbgeJhVzhTdfnK2986DLjich4JQqcSaSyxu5KSoZ25KJ67j4mQJ9iqx",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    mainnetParser,
			},
			want: []string{"DsX5px9k9XZKFNP2Z9kyZBbfHgecm1ftNz6"},
		},
		{
			name: "m/44'/1'/2'",
			args: args{
				xpub:      "tpubVossdTiJtheA51AuNQZtqvKUbhM867Von8XBadxX3tRkDm71kyyi6U966jDPEw9RnQjNcQLwxYSnQ9kBjZxrxfmSbByRbz7D1PLjgAPmL42",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    testnetParser,
			},
			want: []string{"TsSpo87rBG21PLvvbzFk2Ust2Dbyvjfn8pQ"},
		},
		{
			name: "m/44'/1'/1'",
			args: args{
				xpub:      "tpubVossdTiJtheA1fQniKn9EN1JE1Eq1kBofaq2KwywrvuNhAk1KsEM7J2r8anhMJUmmcn9Wmoh73EctpW7Vxs3gS8cbF7N3m4zVjzuyvBj3qC",
				change:    0,
				fromIndex: 0,
				toIndex:   5,
				parser:    testnetParser,
			},
			want: []string{"TsndBjzcwZVjoZEuqYKwiMbCJH9QpkEekg4", "TshWHbnPAVCDARTcCfTEQyL9SzeHxxexX4J", "TspE6pMdC937UHHyfYJpTiKi6vPj5rVnWiG",
				"TsbrkVdFciW3Lfh1W8qjwRY9uSbdiBmY4VP", "TsagMXjC4Xj6ckPEJh8f1RKHU4cEzTtdVW6"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor, err := tt.args.parser.ParseXpub(tt.args.xpub)
			if err != nil {
				t.Errorf("ParseXpub() error = %v", err)
				return
			}
			got, err := tt.args.parser.DeriveAddressDescriptorsFromTo(descriptor, tt.args.change, tt.args.fromIndex, tt.args.toIndex)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveAddressDescriptorsFromTo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotAddresses := make([]string, len(got))
			for i, ad := range got {
				aa, _, err := tt.args.parser.GetAddressesFromAddrDesc(ad)
				if err != nil || len(aa) != 1 {
					t.Errorf("DeriveAddressDescriptorsFromTo() got incorrect address descriptor %v, error %v", ad, err)
					return
				}
				gotAddresses[i] = aa[0]
			}
			if !reflect.DeepEqual(gotAddresses, tt.want) {
				t.Errorf("DeriveAddressDescriptorsFromTo() = %v, want %v", gotAddresses, tt.want)
			}
		})
	}
}

func TestDerivationBasePath(t *testing.T) {
	tests := []struct {
		name   string
		xpub   string
		parser *DecredParser
	}{
		{
			name:   "m/44'/42'/2'",
			xpub:   "dpubZFYFpu8cZxwrGnWbdHmvsAcTaMve4W9EAUiSHzXp1c5hQvfeWgk7LxsE5LqopwfxV62CoB51fxw97YaNpdA3tdo4GHbLxtUzRmYcUtVPYUi",
			parser: mainnetParser,
		},
		{
			name:   "m/44'/42'/1'",
			xpub:   "dpubZFYFpu8cZxwrESo75eazNjVHtC4nWJqL5aXxExZHKnyvZxKirkpypbgeJhVzhTdfnK2986DLjich4JQqcSaSyxu5KSoZ25KJ67j4mQJ9iqx",
			parser: mainnetParser,
		},
		{
			name:   "m/44'/1'/2'",
			xpub:   "tpubVossdTiJtheA51AuNQZtqvKUbhM867Von8XBadxX3tRkDm71kyyi6U966jDPEw9RnQjNcQLwxYSnQ9kBjZxrxfmSbByRbz7D1PLjgAPmL42",
			parser: testnetParser,
		},
		{
			name:   "m/44'/1'/1'",
			xpub:   "tpubVossdTiJtheA1fQniKn9EN1JE1Eq1kBofaq2KwywrvuNhAk1KsEM7J2r8anhMJUmmcn9Wmoh73EctpW7Vxs3gS8cbF7N3m4zVjzuyvBj3qC",
			parser: testnetParser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor, err := tt.parser.ParseXpub(tt.xpub)
			if err != nil {
				t.Errorf("ParseXpub() error = %v", err)
				return
			}
			got, err := tt.parser.DerivationBasePath(descriptor)
			if err != nil {
				t.Errorf("DerivationBasePath() expected no error but got %v", err)
				return
			}

			if got != tt.name {
				t.Errorf("DerivationBasePath() = %v, want %v", got, tt.name)
			}
		})
	}
}

func TestPackAndUnpack(t *testing.T) {
	tests := []struct {
		name   string
		txInfo *bchain.Tx
		height uint32
		parser *DecredParser
	}{
		{
			name:   "Test_1",
			txInfo: &testTx1,
			height: 15819,
			parser: testnetParser,
		},
		{
			name:   "Test_2",
			txInfo: &testTx2,
			height: 300000,
			parser: mainnetParser,
		},
		{
			name:   "Test_3",
			txInfo: &testTx3,
			height: 15859,
			parser: testnetParser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packedTx, err := tt.parser.PackTx(tt.txInfo, tt.height, tt.txInfo.Blocktime)
			if err != nil {
				t.Errorf("PackTx() expected no error but got %v", err)
				return
			}

			unpackedtx, gotHeight, err := tt.parser.UnpackTx(packedTx)
			if err != nil {
				t.Errorf("PackTx() expected no error but got %v", err)
				return
			}

			if !reflect.DeepEqual(tt.txInfo, unpackedtx) {
				t.Errorf("TestPackAndUnpack() expected the raw tx and the unpacked tx to match but they didn't")
			}

			if gotHeight != tt.height {
				t.Errorf("TestPackAndUnpack() = got height %v, but want %v", gotHeight, tt.height)
			}
		})
	}

}
