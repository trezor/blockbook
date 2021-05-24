// +build unittest

package btc

import (
	"encoding/hex"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
)

func TestMain(m *testing.M) {
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
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
			args:    args{address: "1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want:    "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac",
			wantErr: false,
		},
		{
			name:    "P2PKH from P2PK",
			args:    args{address: "1HY6bKYhFH7HF3F48ikvziPHLrEWPGwXcE"},
			want:    "76a914b563933904dceba5c234e978bea0e9eb8b7e721b88ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{address: "321x69Cb9HZLWwAWGiUBT1U81r1zPLnEjL"},
			want:    "a9140394b3cf9a44782c10105b93962daa8dba304d7f87",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{address: "bc1qrsf2l34jvqnq0lduyz0j5pfu2nkd93nnq0qggn"},
			want:    "00141c12afc6b2602607fdbc209f2a053c54ecd2c673",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{address: "bc1qqwtn5s8vjnqdzrm0du885c46ypzt05vakmljhasx28shlv5a355sw5exgr"},
			want:    "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29",
			wantErr: false,
		},
	}
	parser := NewBitcoinParser(GetChainParams("main"), &Configuration{})

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
			name:    "P2PKH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac"}}},
			want:    "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac",
			wantErr: false,
		},
		{
			name:    "P2PK compressed 1P3rU1Nk1pmc2BiWC8dEy9bZa1ZbMp5jfg",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "21020e46e79a2a8d12b9b5d12c7a91adb4e454edfae43c0a0cb805427d2ac7613fd9ac"}}},
			want:    "76a914f1dce4182fce875748c4986b240ff7d7bc3fffb088ac",
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed 1HY6bKYhFH7HF3F48ikvziPHLrEWPGwXcE",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "41041057356b91bfd3efeff5fc0fa8b865faafafb67bd653c5da2cd16ce15c7b86db0e622c8e1e135f68918a23601eb49208c1ac72c7b64a4ee99c396cf788da16ccac"}}},
			want:    "76a914b563933904dceba5c234e978bea0e9eb8b7e721b88ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "a9140394b3cf9a44782c10105b93962daa8dba304d7f87"}}},
			want:    "a9140394b3cf9a44782c10105b93962daa8dba304d7f87",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "00141c12afc6b2602607fdbc209f2a053c54ecd2c673"}}},
			want:    "00141c12afc6b2602607fdbc209f2a053c54ecd2c673",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29"}}},
			want:    "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29",
			wantErr: false,
		},
	}
	parser := NewBitcoinParser(GetChainParams("main"), &Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromVout(&tt.args.vout)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromVout() error = %v, wantErr %v", err, tt.wantErr)
				return
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
			args:    args{script: "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac"},
			want:    []string{"1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PK compressed",
			args:    args{script: "21020e46e79a2a8d12b9b5d12c7a91adb4e454edfae43c0a0cb805427d2ac7613fd9ac"},
			want:    []string{"1P3rU1Nk1pmc2BiWC8dEy9bZa1ZbMp5jfg"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2PK uncompressed",
			args:    args{script: "41041057356b91bfd3efeff5fc0fa8b865faafafb67bd653c5da2cd16ce15c7b86db0e622c8e1e135f68918a23601eb49208c1ac72c7b64a4ee99c396cf788da16ccac"},
			want:    []string{"1HY6bKYhFH7HF3F48ikvziPHLrEWPGwXcE"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9140394b3cf9a44782c10105b93962daa8dba304d7f87"},
			want:    []string{"321x69Cb9HZLWwAWGiUBT1U81r1zPLnEjL"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00141c12afc6b2602607fdbc209f2a053c54ecd2c673"},
			want:    []string{"bc1qrsf2l34jvqnq0lduyz0j5pfu2nkd93nnq0qggn"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29"},
			want:    []string{"bc1qqwtn5s8vjnqdzrm0du885c46ypzt05vakmljhasx28shlv5a355sw5exgr"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "OP_RETURN ascii",
			args:    args{script: "6a0461686f6a"},
			want:    []string{"OP_RETURN (ahoj)"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN OP_PUSHDATA1 ascii",
			args:    args{script: "6a4c0b446c6f7568792074657874"},
			want:    []string{"OP_RETURN (Dlouhy text)"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN OP_PUSHDATA1 utf8",
			args:    args{script: "6a31e5bfabe981a9e381ab4254434658e58f96e5bc95e3818ce381a7e3818de3828b5043e3818ce6acb2e38197e38184e38082"},
			want:    []string{"OP_RETURN (快適にBTCFX取引ができるPCが欲しい。)"},
			want2:   false,
			wantErr: false,
		},
		{
			name: "OP_RETURN OP_PUSHDATA2 ascii",
			args: args{script: "6a4dd7035765277265206e6f20737472616e6765727320746f206c6f76650a596f75206b6e6f77207468652072756c657320616e6420736f20646f20490a412066756c6c20636f6d6d69746d656e74277320776861742049276d207468696e6b696e67206f660a596f7520776f756c646e27742067657420746869732066726f6d20616e79206f74686572206775790a49206a7573742077616e6e612074656c6c20796f7520686f772049276d206665656c696e670a476f747461206d616b6520796f7520756e6465727374616e640a0a43484f5255530a4e6576657220676f6e6e61206769766520796f752075702c0a4e6576657220676f6e6e61206c657420796f7520646f776e0a4e6576657220676f6e6e612072756e2061726f756e6420616e642064657365727420796f750a4e6576657220676f6e6e61206d616b6520796f75206372792c0a4e6576657220676f6e6e612073617920676f6f646279650a4e6576657220676f6e6e612074656c6c2061206c696520616e64206875727420796f750a0a5765277665206b6e6f776e2065616368206f7468657220666f7220736f206c6f6e670a596f75722068656172742773206265656e20616368696e672062757420796f7527726520746f6f2073687920746f207361792069740a496e7369646520776520626f7468206b6e6f7720776861742773206265656e20676f696e67206f6e0a5765206b6e6f77207468652067616d6520616e6420776527726520676f6e6e6120706c61792069740a416e6420696620796f752061736b206d6520686f772049276d206665656c696e670a446f6e27742074656c6c206d6520796f7527726520746f6f20626c696e6420746f20736565202843484f525553290a0a43484f52555343484f5255530a284f6f68206769766520796f75207570290a284f6f68206769766520796f75207570290a284f6f6829206e6576657220676f6e6e6120676976652c206e6576657220676f6e6e6120676976650a286769766520796f75207570290a284f6f6829206e6576657220676f6e6e6120676976652c206e6576657220676f6e6e6120676976650a286769766520796f75207570290a0a5765277665206b6e6f776e2065616368206f7468657220666f7220736f206c6f6e670a596f75722068656172742773206265656e20616368696e672062757420796f7527726520746f6f2073687920746f207361792069740a496e7369646520776520626f7468206b6e6f7720776861742773206265656e20676f696e67206f6e0a5765206b6e6f77207468652067616d6520616e6420776527726520676f6e6e6120706c61792069742028544f2046524f4e54290a0a"},
			want: []string{`OP_RETURN (We're no strangers to love
You know the rules and so do I
A full commitment's what I'm thinking of
You wouldn't get this from any other guy
I just wanna tell you how I'm feeling
Gotta make you understand

CHORUS
Never gonna give you up,
Never gonna let you down
Never gonna run around and desert you
Never gonna make you cry,
Never gonna say goodbye
Never gonna tell a lie and hurt you

We've known each other for so long
Your heart's been aching but you're too shy to say it
Inside we both know what's been going on
We know the game and we're gonna play it
And if you ask me how I'm feeling
Don't tell me you're too blind to see (CHORUS)

CHORUSCHORUS
(Ooh give you up)
(Ooh give you up)
(Ooh) never gonna give, never gonna give
(give you up)
(Ooh) never gonna give, never gonna give
(give you up)

We've known each other for so long
Your heart's been aching but you're too shy to say it
Inside we both know what's been going on
We know the game and we're gonna play it (TO FRONT)

)`},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN hex",
			args:    args{script: "6a072020f1686f6a20"},
			want:    []string{"OP_RETURN 2020f1686f6a20"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN omni simple send tether",
			args:    args{script: "6a146f6d6e69000000000000001f00000709bb647351"},
			want:    []string{"OMNI Simple Send: 77383.80022609 TetherUS (#31)"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN omni simple send not supported coin",
			args:    args{script: "6a146f6d6e69000000000000000300000709bb647351"},
			want:    []string{"OP_RETURN 6f6d6e69000000000000000300000709bb647351"},
			want2:   false,
			wantErr: false,
		},
		{
			name:    "OP_RETURN omni not supported version",
			args:    args{script: "6a146f6d6e69010000000000000300000709bb647351"},
			want:    []string{"OP_RETURN 6f6d6e69010000000000000300000709bb647351"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewBitcoinParser(GetChainParams("main"), &Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
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

var (
	testTx1, testTx2, testTx3 bchain.Tx

	testTxPacked1 = "0001e2408ba8d7af5401000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700"
	testTxPacked2 = "0007c91a899ab7da6a010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000"
	testTxPacked3 = "00003d818bfda9aa3e02000000000102deb1999a857ab0a13d6b12fbd95ea75b409edde5f2ff747507ce42d9986a8b9d0000000000fdffffff9fd2d3361e203b2375eba6438efbef5b3075531e7e583c7cc76b7294fe7f22980000000000fdffffff02a0860100000000001600148091746745464e7555c31e9a5afceac14a02978ae7fc1c0000000000160014565ea9ff4589d3e05ba149ae6e257752bfdc2a1e0247304402207d67d320a8e813f986b35e9791935fcb736754812b7038686f5de6cfdcda99cd02201c3bb2c178e0056016437ecfe365a7eef84aa9d293ebdc566177af82e22fcdd3012103abb30c1bbe878b07b58dc169b1d061d48c60be8107f632a59778b38bf7ceea5a02473044022044f54a478cfe086e870cb026c9dcd4e14e63778bef569a4d55a6332725cd9a9802202f0e94c04e6f328fc64ad9efe552888c299750d1b8d033324825a3ff29920e030121036fcd433428aa7dc65c4f5408fa31f208c54fe4b4c6c1ae9c39a825ed4f1ac039813d0000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000017f9a22c9cbf54bd902400df746f138f37bcf5b4d93eb755820e974ba43ed5f42040000006a4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80feffffff019c9700000000000017a9146144d57c8aff48492c9dfb914e120b20bad72d6f8773d00700",
		Blocktime: 1519053802,
		Txid:      "056e3d82e5ffd0e915fb9b62797d76263508c34fe3e5dbed30dd3e943930f204",
		LockTime:  512115,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4730440220037f4ed5427cde81d55b9b6a2fd08c8a25090c2c2fff3a75c1a57625ca8a7118022076c702fe55969fa08137f71afd4851c48e31082dd3c40c919c92cdbc826758d30121029f6da5623c9f9b68a9baf9c1bc7511df88fa34c6c2f71f7c62f2f03ff48dca80",
				},
				Txid:     "425fed43ba74e9205875eb934d5bcf7bf338f146f70d4002d94bf5cbc9229a7f",
				Vout:     4,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(38812),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a9146144d57c8aff48492c9dfb914e120b20bad72d6f87",
					Addresses: []string{
						"3AZKvpKhSh1o8t1QrX3UeXG9d2BhCRnbcK",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "010000000001019d64f0c72a0d206001decbffaa722eb1044534c74eee7a5df8318e42a4323ec10000000017160014550da1f5d25a9dae2eafd6902b4194c4c6500af6ffffffff02809698000000000017a914cd668d781ece600efa4b2404dc91fd26b8b8aed8870553d7360000000017a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a8702473044022076aba4ad559616905fa51d4ddd357fc1fdb428d40cb388e042cdd1da4a1b7357022011916f90c712ead9a66d5f058252efd280439ad8956a967e95d437d246710bc9012102a80a5964c5612bb769ef73147b2cf3c149bc0fd4ecb02f8097629c94ab013ffd00000000",
		Blocktime: 1235678901,
		Txid:      "474e6795760ebe81cb4023dc227e5a0efe340e1771c89a0035276361ed733de7",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "160014550da1f5d25a9dae2eafd6902b4194c4c6500af6",
				},
				Txid:     "c13e32a4428e31f85d7aee4ec7344504b12e72aaffcbde0160200d2ac7f0649d",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(10000000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914cd668d781ece600efa4b2404dc91fd26b8b8aed887",
					Addresses: []string{
						"2NByHN6A8QYkBATzxf4pRGbCSHD5CEN2TRu",
					},
				},
			},
			{
				ValueSat: *big.NewInt(920081157),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914246655bdbd54c7e477d0ea2375e86e0db2b8f80a87",
					Addresses: []string{
						"2MvZguYaGjM7JihBgNqgLF2Ca2Enb76Hj9D",
					},
				},
			},
		},
	}

	testTx3 = bchain.Tx{
		Hex:       "02000000000102deb1999a857ab0a13d6b12fbd95ea75b409edde5f2ff747507ce42d9986a8b9d0000000000fdffffff9fd2d3361e203b2375eba6438efbef5b3075531e7e583c7cc76b7294fe7f22980000000000fdffffff02a0860100000000001600148091746745464e7555c31e9a5afceac14a02978ae7fc1c0000000000160014565ea9ff4589d3e05ba149ae6e257752bfdc2a1e0247304402207d67d320a8e813f986b35e9791935fcb736754812b7038686f5de6cfdcda99cd02201c3bb2c178e0056016437ecfe365a7eef84aa9d293ebdc566177af82e22fcdd3012103abb30c1bbe878b07b58dc169b1d061d48c60be8107f632a59778b38bf7ceea5a02473044022044f54a478cfe086e870cb026c9dcd4e14e63778bef569a4d55a6332725cd9a9802202f0e94c04e6f328fc64ad9efe552888c299750d1b8d033324825a3ff29920e030121036fcd433428aa7dc65c4f5408fa31f208c54fe4b4c6c1ae9c39a825ed4f1ac039813d0000",
		Blocktime: 1607805599,
		Txid:      "24551a58a1d1fb89d7052e2bbac7cb69a7825ee1e39439befbec8c32148cf735",
		LockTime:  15745,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "9d8b6a98d942ce077574fff2e5dd9e405ba75ed9fb126b3da1b07a859a99b1de",
				Vout:     0,
				Sequence: 4294967293,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "",
				},
				Txid:     "98227ffe94726bc77c3c587e1e5375305beffb8e43a6eb75233b201e36d3d29f",
				Vout:     0,
				Sequence: 4294967293,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(100000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "00148091746745464e7555c31e9a5afceac14a02978a",
					Addresses: []string{
						"tb1qszghge69ge8824wrr6d94l82c99q99u2ccgv5w",
					},
				},
			},
			{
				ValueSat: *big.NewInt(1899751),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "0014565ea9ff4589d3e05ba149ae6e257752bfdc2a1e",
					Addresses: []string{
						"tb1q2e02nl6938f7qkapfxhxufth22lac2s792vsxp",
					},
				},
			},
		},
	}
}

func TestPackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *BitcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "btc-1",
			args: args{
				tx:        testTx1,
				height:    123456,
				blockTime: 1519053802,
				parser:    NewBitcoinParser(GetChainParams("main"), &Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				tx:        testTx2,
				height:    510234,
				blockTime: 1235678901,
				parser:    NewBitcoinParser(GetChainParams("test"), &Configuration{}),
			},
			want:    testTxPacked2,
			wantErr: false,
		},
		{
			name: "signet-1",
			args: args{
				tx:        testTx3,
				height:    15745,
				blockTime: 1607805599,
				parser:    NewBitcoinParser(GetChainParams("signet"), &Configuration{}),
			},
			want:    testTxPacked3,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.parser.PackTx(&tt.args.tx, tt.args.height, tt.args.blockTime)
			if (err != nil) != tt.wantErr {
				t.Errorf("packTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("packTx() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestUnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *BitcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "btc-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewBitcoinParser(GetChainParams("main"), &Configuration{}),
			},
			want:    &testTx1,
			want1:   123456,
			wantErr: false,
		},
		{
			name: "testnet-1",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewBitcoinParser(GetChainParams("test"), &Configuration{}),
			},
			want:    &testTx2,
			want1:   510234,
			wantErr: false,
		},
		{
			name: "signet-1",
			args: args{
				packedTx: testTxPacked3,
				parser:   NewBitcoinParser(GetChainParams("signet"), &Configuration{}),
			},
			want:    &testTx3,
			want1:   15745,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.packedTx)
			got, got1, err := tt.args.parser.UnpackTx(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("unpackTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unpackTx() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("unpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestDeriveAddressDescriptors(t *testing.T) {
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518})
	type args struct {
		xpub    string
		change  uint32
		indexes []uint32
		parser  *BitcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "m/44'/0'/0'",
			args: args{
				xpub:    "xpub6BosfCnifzxcFwrSzQiqu2DBVTshkCXacvNsWGYJVVhhawA7d4R5WSWGFNbi8Aw6ZRc1brxMyWMzG3DSSSSoekkudhUd9yLb6qx39T9nMdj",
				change:  0,
				indexes: []uint32{0, 1234},
				parser:  btcMainParser,
			},
			want: []string{"1LqBGSKuX5yYUonjxT5qGfpUsXKYYWeabA", "1P9w11dXAmG3QBjKLAvCsek8izs1iR2iFi"},
		},
		{
			name: "m/49'/0'/0'",
			args: args{
				xpub:    "ypub6Ww3ibxVfGzLrAH1PNcjyAWenMTbbAosGNB6VvmSEgytSER9azLDWCxoJwW7Ke7icmizBMXrzBx9979FfaHxHcrArf3zbeJJJUZPf663zsP",
				change:  0,
				indexes: []uint32{0, 1234},
				parser:  btcMainParser,
			},
			want: []string{"37VucYSaXLCAsxYyAPfbSi9eh4iEcbShgf", "367meFzJ9KqDLm9PX6U8Z8RdmkSNBuxX8T"},
		},
		{
			name: "m/84'/0'/0'",
			args: args{
				xpub:    "zpub6rFR7y4Q2AijBEqTUquhVz398htDFrtymD9xYYfG1m4wAcvPhXNfE3EfH1r1ADqtfSdVCToUG868RvUUkgDKf31mGDtKsAYz2oz2AGutZYs",
				change:  0,
				indexes: []uint32{0, 1234},
				parser:  btcMainParser,
			},
			want: []string{"bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu", "bc1q4nm6g46ujzyjaeusralaz2nfv2rf04jjfyamkw"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.parser.DeriveAddressDescriptors(tt.args.xpub, tt.args.change, tt.args.indexes)
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
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518})
	btcTestnetsParser := NewBitcoinParser(GetChainParams("test"), &Configuration{XPubMagic: 70617039, XPubMagicSegwitP2sh: 71979618, XPubMagicSegwitNative: 73342198})
	type args struct {
		xpub      string
		change    uint32
		fromIndex uint32
		toIndex   uint32
		parser    *BitcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "m/44'/0'/0'",
			args: args{
				xpub:      "xpub6BosfCnifzxcFwrSzQiqu2DBVTshkCXacvNsWGYJVVhhawA7d4R5WSWGFNbi8Aw6ZRc1brxMyWMzG3DSSSSoekkudhUd9yLb6qx39T9nMdj",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    btcMainParser,
			},
			want: []string{"1LqBGSKuX5yYUonjxT5qGfpUsXKYYWeabA"},
		},
		{
			name: "m/49'/0'/0'",
			args: args{
				xpub:      "ypub6Ww3ibxVfGzLrAH1PNcjyAWenMTbbAosGNB6VvmSEgytSER9azLDWCxoJwW7Ke7icmizBMXrzBx9979FfaHxHcrArf3zbeJJJUZPf663zsP",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    btcMainParser,
			},
			want: []string{"37VucYSaXLCAsxYyAPfbSi9eh4iEcbShgf"},
		},
		{
			name: "m/84'/0'/0'",
			args: args{
				xpub:      "zpub6rFR7y4Q2AijBEqTUquhVz398htDFrtymD9xYYfG1m4wAcvPhXNfE3EfH1r1ADqtfSdVCToUG868RvUUkgDKf31mGDtKsAYz2oz2AGutZYs",
				change:    0,
				fromIndex: 0,
				toIndex:   1,
				parser:    btcMainParser,
			},
			want: []string{"bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu"},
		},
		{
			name: "m/49'/1'/0'",
			args: args{
				xpub:      "upub5DR1Mg5nykixzYjFXWW5GghAU7dDqoPVJ2jrqFbL8sJ7Hs7jn69MP7KBnnmxn88GeZtnH8PRKV9w5MMSFX8AdEAoXY8Qd8BJPoXtpMeHMxJ",
				change:    0,
				fromIndex: 0,
				toIndex:   10,
				parser:    btcTestnetsParser,
			},
			want: []string{"2N4Q5FhU2497BryFfUgbqkAJE87aKHUhXMp", "2Mt7P2BAfE922zmfXrdcYTLyR7GUvbwSEns", "2N6aUMgQk8y1zvoq6FeWFyotyj75WY9BGsu", "2NA7tbZWM9BcRwBuebKSQe2xbhhF1paJwBM", "2N8RZMzvrUUnpLmvACX9ysmJ2MX3GK5jcQM", "2MvUUSiQZDSqyeSdofKX9KrSCio1nANPDTe", "2NBXaWu1HazjoUVgrXgcKNoBLhtkkD9Gmet", "2N791Ttf89tMVw2maj86E1Y3VgxD9Mc7PU7", "2NCJmwEq8GJm8t8GWWyBXAfpw7F2qZEVP5Y", "2NEgW71hWKer2XCSA8ZCC2VnWpB77L6bk68"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.parser.DeriveAddressDescriptorsFromTo(tt.args.xpub, tt.args.change, tt.args.fromIndex, tt.args.toIndex)
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

func BenchmarkDeriveAddressDescriptorsFromToXpub(b *testing.B) {
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518})
	for i := 0; i < b.N; i++ {
		btcMainParser.DeriveAddressDescriptorsFromTo("xpub6BosfCnifzxcFwrSzQiqu2DBVTshkCXacvNsWGYJVVhhawA7d4R5WSWGFNbi8Aw6ZRc1brxMyWMzG3DSSSSoekkudhUd9yLb6qx39T9nMdj", 1, 0, 100)
	}
}

func BenchmarkDeriveAddressDescriptorsFromToYpub(b *testing.B) {
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518})
	for i := 0; i < b.N; i++ {
		btcMainParser.DeriveAddressDescriptorsFromTo("ypub6Ww3ibxVfGzLrAH1PNcjyAWenMTbbAosGNB6VvmSEgytSER9azLDWCxoJwW7Ke7icmizBMXrzBx9979FfaHxHcrArf3zbeJJJUZPf663zsP", 1, 0, 100)
	}
}

func BenchmarkDeriveAddressDescriptorsFromToZpub(b *testing.B) {
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518})
	for i := 0; i < b.N; i++ {
		btcMainParser.DeriveAddressDescriptorsFromTo("zpub6rFR7y4Q2AijBEqTUquhVz398htDFrtymD9xYYfG1m4wAcvPhXNfE3EfH1r1ADqtfSdVCToUG868RvUUkgDKf31mGDtKsAYz2oz2AGutZYs", 1, 0, 100)
	}
}

func TestBitcoinParser_DerivationBasePath(t *testing.T) {
	btcMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, XPubMagicSegwitP2sh: 77429938, XPubMagicSegwitNative: 78792518, Slip44: 0})
	btcTestnetsParser := NewBitcoinParser(GetChainParams("test"), &Configuration{XPubMagic: 70617039, XPubMagicSegwitP2sh: 71979618, XPubMagicSegwitNative: 73342198, Slip44: 1})
	zecMainParser := NewBitcoinParser(GetChainParams("main"), &Configuration{XPubMagic: 76067358, Slip44: 133})
	type args struct {
		xpub   string
		parser *BitcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "m/84'/0'/0'",
			args: args{
				xpub:   "zpub6rFR7y4Q2AijBEqTUquhVz398htDFrtymD9xYYfG1m4wAcvPhXNfE3EfH1r1ADqtfSdVCToUG868RvUUkgDKf31mGDtKsAYz2oz2AGutZYs",
				parser: btcMainParser,
			},
			want: "m/84'/0'/0'",
		},
		{
			name: "m/49'/0'/55 - not hardened account",
			args: args{
				xpub:   "ypub6XKbB5DJRAbW4TRJLp4uXQXG3ob5BtByXsNZFBjq9qcbzrczjVXfCz5cEo1SFDexmeWRnbCMDaRgaW4m9d2nBaa8FvUQCu3n9G1UBR8WhbT",
				parser: btcMainParser,
			},
			want: "m/49'/0'/55",
		},
		{
			name: "m/49'/0' - incomplete path, without account",
			args: args{
				xpub:   "ypub6UzM8PUqxcSoqC9gumfoiFhE8Qt84HbGpCD4eVJfJAojXTVtBxeddvTWJGJhGoaVBNJLmEgMdLXHgaLVJa4xEvk2tcokkdZhFdkxMLUE9sB",
				parser: btcMainParser,
			},
			want: "unknown/0'",
		},
		{
			name: "m/49'/1'/0'",
			args: args{
				xpub:   "upub5DR1Mg5nykixzYjFXWW5GghAU7dDqoPVJ2jrqFbL8sJ7Hs7jn69MP7KBnnmxn88GeZtnH8PRKV9w5MMSFX8AdEAoXY8Qd8BJPoXtpMeHMxJ",
				parser: btcTestnetsParser,
			},
			want: "m/49'/1'/0'",
		},
		{
			name: "m/44'/133'/12'",
			args: args{
				xpub:   "xpub6CQdEahwhKRTLYpP6cyb7ZaGb3r4tVdyPX6dC1PfrNuByrCkWDgUkmpD28UdV9QccKgY1ZiAbGv1Fakcg2LxdFVSTNKHcjdRjqhjPK8Trkb",
				parser: zecMainParser,
			},
			want: "m/44'/133'/12'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.parser.DerivationBasePath(tt.args.xpub)
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
