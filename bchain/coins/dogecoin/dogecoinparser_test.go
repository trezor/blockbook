//go:build unittest

package dogecoin

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
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
			name:    "P2PKH1",
			args:    args{address: "DHZYinsaM9nW5piCMN639ELRKbZomThPnZ"},
			want:    "76a9148841590909747c0f97af158f22fadacb1652522088ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "DSzaAYEYyy9ngjoJ294r7jzFM3xhD6bKHK"},
			want:    "76a914efb6158f75743c611858fdfd0f4aaec6cc6196bc88ac",
			wantErr: false,
		},
		{
			name:    "P2PKH3",
			args:    args{address: "DHobAps6DjZ5n4xMV75n7kJv299Zi85FCG"},
			want:    "76a9148ae937291e72f7368421dbaa966c44950eb14db788ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "9tg1kVUk339Tk58ewu5T8QT82Z6cE4UvSU"},
			want:    "a9141889a089400ea25d28694fd98aa7702b21eeeab187",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "9sLa1AKzjWuNTe1CkLh5GDYyRP9enb1Spp"},
			want:    "a91409e41aff9f97412ab3d4a07cf0667fdba84caf4487",
			wantErr: false,
		},
	}
	parser := NewDogecoinParser(GetChainParams("main"), &btc.Configuration{})

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

func Test_GetAddrDescFromAddress_Testnet(t *testing.T) {
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
			args:    args{address: "neYNjrnowpYrbS4QanPngXBi6MQcX6FsWV"},
			want:    "76a9147183fdc10d664551d151922f95e58ab548d8ad2688ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "nYrWFiv3zz5MewmN6ZJUpEfLQwz63Ptf1i"},
			want:    "76a9143320ff38bc2d44e404cc3f0b36202f3a4897e05088ac",
			wantErr: false,
		},
		{
			name:    "P2PKH3",
			args:    args{address: "nbn2EWCDp2xcb7jTxhcXytLKZuctY8xXiB"},
			want:    "76a914533051d74f660325166fd342250f99fd366214ec88ac",
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{address: "2MyChfh5WfqzDTyFibZq2uSF3WcYFE1G5te"},
			want:    "a91441569cc9dbdc08a99d20079bfd12071a2bdbf8e987",
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{address: "2NCnuCgdAAQHQvSQVw9eJA8UfbffupFLaYm"},
			want:    "a914d66804cbba3b9035f2447b5454699f657dd3275087",
			wantErr: false,
		},
		{
			name:    "P2SH3",
			args:    args{address: "2N2ju8ukjDQbJRB4ptNtekDzYNiqSQHARvd"},
			want:    "a9146825756d503c3a81659409636d6e6c40755fcdcf87",
			wantErr: false,
		},
	}
	parser := NewDogecoinParser(GetChainParams("test"), &btc.Configuration{})

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

func Test_GetAddressesFromAddrDesc_Mainnet(t *testing.T) {
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
			name:    "P2PKH1",
			args:    args{script: "76a9148841590909747c0f97af158f22fadacb1652522088ac"},
			want:    []string{"DHZYinsaM9nW5piCMN639ELRKbZomThPnZ"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{script: "76a914efb6158f75743c611858fdfd0f4aaec6cc6196bc88ac"},
			want:    []string{"DSzaAYEYyy9ngjoJ294r7jzFM3xhD6bKHK"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH3",
			args:    args{script: "76a91450e86eeac599ad023b8981296d01b50bdabcdd9788ac"},
			want:    []string{"DCWu3MLz9xBGFuuLyNDf6QjuGp49f5tfc9"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{script: "a9141889a089400ea25d28694fd98aa7702b21eeeab187"},
			want:    []string{"9tg1kVUk339Tk58ewu5T8QT82Z6cE4UvSU"},
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
			name:    "OP_RETURN hex",
			args:    args{script: "6a072020f1686f6a20"},
			want:    []string{"OP_RETURN 2020f1686f6a20"},
			want2:   false,
			wantErr: false,
		},
	}

	parser := NewDogecoinParser(GetChainParams("main"), &btc.Configuration{})

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

func Test_GetAddressesFromAddrDesc_Testnet(t *testing.T) {
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
			name:    "P2PKH1",
			args:    args{script: "76a9147183fdc10d664551d151922f95e58ab548d8ad2688ac"},
			want:    []string{"neYNjrnowpYrbS4QanPngXBi6MQcX6FsWV"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{script: "76a9143320ff38bc2d44e404cc3f0b36202f3a4897e05088ac"},
			want:    []string{"nYrWFiv3zz5MewmN6ZJUpEfLQwz63Ptf1i"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH3",
			args:    args{script: "76a914533051d74f660325166fd342250f99fd366214ec88ac"},
			want:    []string{"nbn2EWCDp2xcb7jTxhcXytLKZuctY8xXiB"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH1",
			args:    args{script: "a91441569cc9dbdc08a99d20079bfd12071a2bdbf8e987"},
			want:    []string{"2MyChfh5WfqzDTyFibZq2uSF3WcYFE1G5te"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH2",
			args:    args{script: "a914d66804cbba3b9035f2447b5454699f657dd3275087"},
			want:    []string{"2NCnuCgdAAQHQvSQVw9eJA8UfbffupFLaYm"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2SH3",
			args:    args{script: "a9146825756d503c3a81659409636d6e6c40755fcdcf87"},
			want:    []string{"2N2ju8ukjDQbJRB4ptNtekDzYNiqSQHARvd"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "OP_RETURN ascii",
			args:    args{script: "6a0c48656c6c6f20746865726521"},
			want:    []string{"OP_RETURN (Hello there!)"},
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
	}

	parser := NewDogecoinParser(GetChainParams("test"), &btc.Configuration{})

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
	testTx1       bchain.Tx
	testTxPacked1 = "00030e6d8ba8d7aa2001000000016b3c0c53267964120acf7f7e72217e3f463e52ce622f89659f6a6bb8e69a4d91000000006c493046022100a96454237e3a020994534583e28c04757881374bceac89f933ea9ff00b4db259022100fbb757ff7ea4f02c4e42556b2834c61eba1f1af605db089d836a0614d90a3b46012103cebdde6d1046e285df4f48497bc50dc20a4a258ca5b7308cb0a929c9fdadcd9dffffffff0217e823ca7f0200001976a914eef21768a546590993e313c7f3dfadf6a6efa1e888acaddf4cba010000001976a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac00000000"

	testTx2       bchain.Tx
	testTxPacked2 = "0001193a8ba8d7835601000000016d0211b5656f1b8c2ac002445638e247082090ffc5d5fa7c38b445b84a2c2054000000006b4830450221008856f2f620df278c0fc6a5d5e2d50451c0a65a75aaf7a4a9cbfcac3918b5536802203dc685a784d49e2a95eb72763ad62f02094af78507c57b0a3c3f1d8a60f74db6012102db814cd43df584804fde1949365a6309714e342aef0794dc58385d7e413444cdffffffff0237daa2ee0a4715001976a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac8008389e7e8d03001976a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac00000000"

	testTx1_Testnet       bchain.Tx
	testTxPacked1_Testnet = "0030d40b8c8bc4d552010000000104207e905afaf6facfafe7f63cac0afaab07c36b8aab34d5a6b6402859cfbb3b010000006b483045022100daf5631310f5f8ffa4b6a5e9151e3eefbc797ab67ee52e78621526d32694705b02206c038bd527397d3d6f01cd72011a61b58f5d8b74c2d067274d09a8f70f436d330121031d716677bfa840265c32f02b1ff519e59b2d76fcc1753c59516d6891058e6826ffffffff0256b54009000000001976a914e56f55dbb600c10e1f4f6d5da38eea5e664504cf88ac220d5871030000001976a914e8600a08d053bcd41bc316be574efbba6e126d3088ac00000000"

	testTx2_Testnet       bchain.Tx
	testTxPacked2_Testnet = "0030d4068c8bc4c770010000000342dd48439e8fe54b161be919bd9a34e887fa88ca38b1200fc26f4edffc3e69950000000069463043021f48a06f76e53665563090919d01dab845039d7ca4050ea00d65dd6542f2e219022024ae5e8806d737c80e83aa969dd549afbf675a94582889b61a630c0a328969870121036a0a9600ef07072a55fdd770fe4c6e4a138ce3c409eb4328f37fb3066d12e598feffffff9883d80ffa7af541808640940b2a7fb69c8ad817ef51049cdf9ee09731460a12010000006a47304402201d6a14b9a2f275a64edfa447e13347e53023bc192de577a0f7fa5309f26d987b0220386ba58a20032395c8c773cc9be405251b9f5d47182f3be5055dbe01ce0894650121038548fe799340b63c4e05e1edee6a36c5a2b82abec4efecc60bef0205969fb9befeffffffd7d88db648983248f58adb62a428baad726e9d6f40a9dfbf8181c7cd013d2dad000000006b483045022100880a8e9c9fdf43ef8f8c2e6817327d6cee89f12e47d77eb6e470a1b2d99b695d0220755911c3ec6d6704c34d8fd447875c9dbaf37c83a2079352be827c558e1b3f3b012103ddd9ebd38c8891b2ee56b17a155367edcc251fc7aca30130523c834d94fd04d2feffffff021ed9092c000000001976a91433bc558a159810fe4353b09b38b270a74a02621f88ac29ebe9a50000000017a914d0e87f2ab081c1c00e3dca9b653d654889ddb7148705d43000"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "01000000016b3c0c53267964120acf7f7e72217e3f463e52ce622f89659f6a6bb8e69a4d91000000006c493046022100a96454237e3a020994534583e28c04757881374bceac89f933ea9ff00b4db259022100fbb757ff7ea4f02c4e42556b2834c61eba1f1af605db089d836a0614d90a3b46012103cebdde6d1046e285df4f48497bc50dc20a4a258ca5b7308cb0a929c9fdadcd9dffffffff0217e823ca7f0200001976a914eef21768a546590993e313c7f3dfadf6a6efa1e888acaddf4cba010000001976a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac00000000",
		Blocktime: 1519053456,
		Txid:      "097ea09ba284f3f2a9e880e11f837edf7e5cea81c8da2238f5bc7c2c4c407943",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "493046022100a96454237e3a020994534583e28c04757881374bceac89f933ea9ff00b4db259022100fbb757ff7ea4f02c4e42556b2834c61eba1f1af605db089d836a0614d90a3b46012103cebdde6d1046e285df4f48497bc50dc20a4a258ca5b7308cb0a929c9fdadcd9d",
				},
				Txid:     "914d9ae6b86b6a9f65892f62ce523e463f7e21727e7fcf0a12647926530c3c6b",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(2747875452951),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914eef21768a546590993e313c7f3dfadf6a6efa1e888ac",
					Addresses: []string{
						"DSvXNiqvG42wdteLqh3i6inxgDTs8Y9w2i",
					},
				},
			},
			{
				ValueSat: *big.NewInt(7420567469),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac",
					Addresses: []string{
						"DRemF3ZcqJ1PFeM7e7sXzzwQJKR8GNUtwK",
					},
				},
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "01000000016d0211b5656f1b8c2ac002445638e247082090ffc5d5fa7c38b445b84a2c2054000000006b4830450221008856f2f620df278c0fc6a5d5e2d50451c0a65a75aaf7a4a9cbfcac3918b5536802203dc685a784d49e2a95eb72763ad62f02094af78507c57b0a3c3f1d8a60f74db6012102db814cd43df584804fde1949365a6309714e342aef0794dc58385d7e413444cdffffffff0237daa2ee0a4715001976a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac8008389e7e8d03001976a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac00000000",
		Blocktime: 1519050987,
		Txid:      "b276545af246e3ed5a4e3e5b60d359942a1808579effc53ff4f343e4f6cfc5a0",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "4830450221008856f2f620df278c0fc6a5d5e2d50451c0a65a75aaf7a4a9cbfcac3918b5536802203dc685a784d49e2a95eb72763ad62f02094af78507c57b0a3c3f1d8a60f74db6012102db814cd43df584804fde1949365a6309714e342aef0794dc58385d7e413444cd",
				},
				Txid:     "54202c4ab845b4387cfad5c5ff90200847e238564402c02a8c1b6f65b511026d",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(5989086789818935),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac",
					Addresses: []string{
						"DJa8bWDrZKu4HgsYRYWuJrvxt6iTYuvXJ6",
					},
				},
			},
			{
				ValueSat: *big.NewInt(999999890000000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac",
					Addresses: []string{
						"DDTtqnuZ5kfRT5qh2c7sNtqrJmV3iXYdGG",
					},
				},
			},
		},
	}

	testTx1_Testnet = bchain.Tx{
		Hex:       "010000000104207e905afaf6facfafe7f63cac0afaab07c36b8aab34d5a6b6402859cfbb3b010000006b483045022100daf5631310f5f8ffa4b6a5e9151e3eefbc797ab67ee52e78621526d32694705b02206c038bd527397d3d6f01cd72011a61b58f5d8b74c2d067274d09a8f70f436d330121031d716677bfa840265c32f02b1ff519e59b2d76fcc1753c59516d6891058e6826ffffffff0256b54009000000001976a914e56f55dbb600c10e1f4f6d5da38eea5e664504cf88ac220d5871030000001976a914e8600a08d053bcd41bc316be574efbba6e126d3088ac00000000",
		Blocktime: 1622709609,
		Txid:      "43cfbc6db77a8e9aad25913c2298da81421e513e216420b8af2562e744a030c9",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100daf5631310f5f8ffa4b6a5e9151e3eefbc797ab67ee52e78621526d32694705b02206c038bd527397d3d6f01cd72011a61b58f5d8b74c2d067274d09a8f70f436d330121031d716677bfa840265c32f02b1ff519e59b2d76fcc1753c59516d6891058e6826",
				},
				Txid:     "3bbbcf592840b6a6d534ab8a6bc307abfa0aac3cf6e7afcffaf6fa5a907e2004",
				Vout:     1,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(155235670),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e56f55dbb600c10e1f4f6d5da38eea5e664504cf88ac",
					Addresses: []string{
						"nq7JNvDtsGxB7wRjqc6DQ7JRzzXtHT2ETD",
					},
				},
			},
			{
				ValueSat: *big.NewInt(14786497826),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e8600a08d053bcd41bc316be574efbba6e126d3088ac",
					Addresses: []string{
						"nqNr5hLREhQTjx4DA4TfJfeFR9YjMQXrxn",
					},
				},
			},
		},
	}

	testTx2_Testnet = bchain.Tx{
		Hex:       "010000000342dd48439e8fe54b161be919bd9a34e887fa88ca38b1200fc26f4edffc3e69950000000069463043021f48a06f76e53665563090919d01dab845039d7ca4050ea00d65dd6542f2e219022024ae5e8806d737c80e83aa969dd549afbf675a94582889b61a630c0a328969870121036a0a9600ef07072a55fdd770fe4c6e4a138ce3c409eb4328f37fb3066d12e598feffffff9883d80ffa7af541808640940b2a7fb69c8ad817ef51049cdf9ee09731460a12010000006a47304402201d6a14b9a2f275a64edfa447e13347e53023bc192de577a0f7fa5309f26d987b0220386ba58a20032395c8c773cc9be405251b9f5d47182f3be5055dbe01ce0894650121038548fe799340b63c4e05e1edee6a36c5a2b82abec4efecc60bef0205969fb9befeffffffd7d88db648983248f58adb62a428baad726e9d6f40a9dfbf8181c7cd013d2dad000000006b483045022100880a8e9c9fdf43ef8f8c2e6817327d6cee89f12e47d77eb6e470a1b2d99b695d0220755911c3ec6d6704c34d8fd447875c9dbaf37c83a2079352be827c558e1b3f3b012103ddd9ebd38c8891b2ee56b17a155367edcc251fc7aca30130523c834d94fd04d2feffffff021ed9092c000000001976a91433bc558a159810fe4353b09b38b270a74a02621f88ac29ebe9a50000000017a914d0e87f2ab081c1c00e3dca9b653d654889ddb7148705d43000",
		Blocktime: 1622708728,
		Txid:      "91e2f3a9dde1e2da53f29c73033084b3d1a3b0c0ba6737d6418cfa9cad62be3c",
		LockTime:  3200005,
		Version:   1,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "463043021f48a06f76e53665563090919d01dab845039d7ca4050ea00d65dd6542f2e219022024ae5e8806d737c80e83aa969dd549afbf675a94582889b61a630c0a328969870121036a0a9600ef07072a55fdd770fe4c6e4a138ce3c409eb4328f37fb3066d12e598",
				},
				Txid:     "95693efcdf4e6fc20f20b138ca88fa87e8349abd19e91b164be58f9e4348dd42",
				Vout:     0,
				Sequence: 4294967294, // Locktime is enabled for this transaction; see BIP-125
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "47304402201d6a14b9a2f275a64edfa447e13347e53023bc192de577a0f7fa5309f26d987b0220386ba58a20032395c8c773cc9be405251b9f5d47182f3be5055dbe01ce0894650121038548fe799340b63c4e05e1edee6a36c5a2b82abec4efecc60bef0205969fb9be",
				},
				Txid:     "120a463197e09edf9c0451ef17d88a9cb67f2a0b9440868041f57afa0fd88398",
				Vout:     1,
				Sequence: 4294967294,
			},
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "483045022100880a8e9c9fdf43ef8f8c2e6817327d6cee89f12e47d77eb6e470a1b2d99b695d0220755911c3ec6d6704c34d8fd447875c9dbaf37c83a2079352be827c558e1b3f3b012103ddd9ebd38c8891b2ee56b17a155367edcc251fc7aca30130523c834d94fd04d2",
				},
				Txid:     "ad2d3d01cdc78181bfdfa9406f9d6e72adba28a462db8af548329848b68dd8d7",
				Vout:     0,
				Sequence: 4294967294,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(738842910),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a91433bc558a159810fe4353b09b38b270a74a02621f88ac",
					Addresses: []string{
						"nYuiLjsDCVCBwZ3XhNv3qiiDiwmtQRLNhY",
					},
				},
			},
			{
				ValueSat: *big.NewInt(2783570729),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "a914d0e87f2ab081c1c00e3dca9b653d654889ddb71487",
					Addresses: []string{
						"2NCHq4LmqvjB9mqrHMWNUjfDs2FdsmJah6b",
					},
				},
			},
		},
	}
}

func Test_PackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *DogecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "dogecoin-1",
			args: args{
				tx:        testTx1,
				height:    200301,
				blockTime: 1519053456,
				parser:    NewDogecoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "dogecoin-2",
			args: args{
				tx:        testTx2,
				height:    71994,
				blockTime: 1519050987,
				parser:    NewDogecoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked2,
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

func Test_PackTx_Testnet(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *DogecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "dogecoin-testnet-1",
			args: args{
				tx:        testTx1_Testnet,
				height:    3200011,
				blockTime: 1622709609,
				parser:    NewDogecoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked1_Testnet,
			wantErr: false,
		},
		{
			name: "dogecoin-testnet-2",
			args: args{
				tx:        testTx2_Testnet,
				height:    3200006,
				blockTime: 1622708728,
				parser:    NewDogecoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    testTxPacked2_Testnet,
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

func Test_UnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *DogecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "dogecoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewDogecoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   200301,
			wantErr: false,
		},
		{
			name: "dogecoin-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewDogecoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   71994,
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

func Test_UnpackTx_Testnet(t *testing.T) {
	type args struct {
		packedTx string
		parser   *DogecoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "dogecoin-testnet-1",
			args: args{
				packedTx: testTxPacked1_Testnet,
				parser:   NewDogecoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx1_Testnet,
			want1:   3200011,
			wantErr: false,
		},
		{
			name: "dogecoin-testnet-2",
			args: args{
				packedTx: testTxPacked2_Testnet,
				parser:   NewDogecoinParser(GetChainParams("test"), &btc.Configuration{}),
			},
			want:    &testTx2_Testnet,
			want1:   3200006,
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

type testBlock struct {
	size int
	time int64
	txs  []string
}

var testParseBlockTxs = map[int]testBlock{
	// block without auxpow
	12345: {
		size: 8582,
		time: 1387104223,
		txs: []string{
			"9d1662dcc1443af9999c4fd1d6921b91027b5e2d0d3ebfaa41d84163cb99cad5",
			"8284292cedeb0c9c509f9baa235802d52a546e1e9990040d35d018b97ad11cfa",
			"3299d93aae5c3d37c795c07150ceaf008aefa5aad3205ea2519f94a35adbbe10",
			"3f03016f32b63db48fdc0b17443c2d917ba5e307dcc2fc803feeb21c7219ee1b",
			"a889449e9bc618c131c01f564cd309d2217ba1c5731480314795e44f1e02609b",
			"29f79d91c10bc311ff5b69fe7ba57101969f68b6391cf0ca67d5f37ca1f0601b",
			"b794ebc7c0176c35b125cd8b84a980257cf3dd9cefe2ed47da4ed1d73ee568f3",
			"0ec479ba3c954dd422d75c4c5488a6edc3c588deb10ebdbfa8bd8edb7afcfea0",
			"f357b6e667dfa456e7988bfa474377df25d0e0bfe07e5f97fc97ea3a0155f031",
			"4ff189766f0455721a93d6be27a91eafa750383c800cb053fad2f86c434122d2",
			"446d164e2ec4c9f2ac6c499c110735606d949a3625fb849274ac627c033eddbc",
			"c489edebd8a2e17fd08f2801f528b95663aaafe15c897d56686423dd430e2d1f",
			"3f42a7f1a356897da324d41eed94169c79438212bb9874eea58e9cbaf07481df",
			"62c88fdd0fb111676844fcbaebc9e2211a0c990aa7e7529539cb25947a307a1b",
			"522c47e315bc1949826339c535d419eb206aec4a332f91dfbd25c206f3c9527b",
			"18ea78346e7e34cbdf2d2b6ba1630f8b15f9ef9a940114a3e6ee92d26f96691e",
			"43dc0fbd1b9b87bcfc9a51c89457a7b3274855c01d429193aff1181791225f3c",
			"d78cdfaadbe5b6b591529cb5c6869866a4cabe46ef82aa835fd2432056b4a383",
			"d181759c7a3900ccaf4958f1f25a44949163ceefc306006502efc7a1de6f579e",
			"8610b9230188854c7871258163cd1c2db353443d631c5512bff17224a24e95bf",
			"e82f40a6bea32122f1d568d427c92708dcb684bdb3035ff3905617230e5ae5b8",
			"c50ae6c127f8c346c60e7438fbd10c44c3629f3fe426646db77a2250fb2939f9",
			"585202c03894ecaf25188ba4e5447dadd413f2010c2dc2a65c37598dbc6ad907",
			"8bd766fde8c65e2f724dad581944dde4e23e4dbb4f7f7faf55bc348923f4d5ee",
			"2d2fa25691088181569e508dd8f683b21f2b80ceefb5ccbd6714ebe2a697139f",
			"5954622ffc602bec177d61da6c26a68990c42c1886627b218c3ab0e9e3491f4a",
			"01b634bc53334df1cd9f04522729a34d811c418c2535144c3ed156cbc319e43e",
			"c429a6c8265482b2d824af03afe1c090b233a856f243791485cb4269f2729649",
			"dbe79231b916b6fb47a91ef874f35150270eb571af60c2d640ded92b41749940",
			"1c396493a8dfd59557052b6e8643123405894b64f48b2eb6eb7a003159034077",
			"2e2816ffb7bf1378f11acf5ba30d498efc8fd219d4b67a725e8254ce61b1b7ee",
		},
	},
	// 1st block with auxpow
	371337: {
		size: 1704,
		time: 1410464577,
		txs: []string{
			"4547b14bc16db4184fa9f141d645627430dd3dfa662d0e6f418fba497091da75",
			"a965dba2ed06827ed9a24f0568ec05b73c431bc7f0fb6913b144e62db7faa519",
			"5e3ab18cb7ba3abc44e62fb3a43d4c8168d00cf0a2e0f8dbeb2636bb9a212d12",
			"f022935ac7c4c734bd2c9c6a780f8e7280352de8bd358d760d0645b7fe734a93",
			"ec063cc8025f9f30a6ed40fc8b1fe63b0cbd2ea2c62664eb26b365e6243828ca",
			"02c16e3389320da3e77686d39773dda65a1ecdf98a2ef9cfb938c9f4b58f7a40",
		},
	},
	// block with auxpow
	567890: {
		size: 3833,
		time: 1422855443,
		txs: []string{
			"db20feea53be1f60848a66604d5bca63df62de4f6c66220f9c84436d788625a8",
			"cf7e9e27c0f56f0b100eaf5c776ce106025e3412bd5927c6e1ce575500e24eaa",
			"af84e010c1cf0bd927740d08e5e8163db45397b70f00df07aea5339c14d5f3aa",
			"7362e25e8131255d101e5d874e6b6bb2faa7a821356cb041f1843d0901dffdbd",
			"3b875344302e8893f6d5c9e7269d806ed27217ec67944940ae9048fc619bdae9",
			"e3b95e269b7c251d87e8e241ea2a08a66ec14d12a1012762be368b3db55471e3",
			"6ba3f95a37bcab5d0cb5b8bd2fe48040db0a6ae390f320d6dcc8162cc096ff8f",
			"3211ccc66d05b10959fa6e56d1955c12368ea52b40303558b254d7dc22570382",
			"54c1b279e78b924dfa15857c80131c3ddf835ab02f513dc03aa514f87b680493",
		},
	},
	// recent block
	2264125: {
		size: 8531,
		time: 1529099968,
		txs: []string{
			"76f0126562c99e020b5fba41b68dd8141a4f21eef62012b76a1e0635092045e9",
			"7bb6688bec16de94014574e3e1d3f6f5fb956530d6b179b28db367f1fd8ae099",
			"d7e2ee30c3d179ac896651fc09c1396333f41d952d008af8d5d6665cbea377bf",
			"8e4783878df782003c43d014fcbb9c57d2034dfd1d9fcd7319bb1a9f501dbbb7",
			"8d2a4ae226b6f23eea545957be5d71c68cd08674d96a3502d4ca21ffadacb5a9",
			"a0da2b49de881133655c54b1b5c23af443a71c2b937e2d9bbdf3f498247e6b7b",
			"c780a19b9cf46ed70b53c5d5722e8d33951211a4051cb165b25fb0c22a4ae1ff",
			"ce29c2644d642bb4fedd09d0840ed98c9945bf292967fede8fcc6b26054b4058",
			"a360b0566f68c329e2757918f67ee6421d3d76f70f1b452cdd32266805986119",
			"17e85bd33cc5fb5035e489c5188979f45e75e92d14221eca937e14f5f7d7b074",
			"3973eb930fd2d0726abbd81912eae645384268cd3500b9ec84d806fdd65a426a",
			"b91cc1c98e5c77e80eec9bf93e86af27f810b00dfbce3ee2646758797a28d5f2",
			"1a8c7bd3389dcbbc1133ee600898ed9e082f7a9c75f9eb52f33940ed7c2247ef",
			"9b1782449bbd3fc3014c363167777f7bdf41f5ef6db192fbda784b29603911b0",
			"afab4bcdc1a32891d638579c3029ae49ee72be3303425c6d62e1f8eaebe0ce18",
			"5f839f9cd5293c02ff4f7cf5589c53dec52adb42a077599dc7a2c5842a156ca9",
			"756d2dfd1d2872ba2531fae3b8984008506871bec41d19cb299f5e0f216cfb9b",
			"6aa82514ab7a9cc624fabf3d06ccbd46ecb4009b3c784768e6243d7840d4bf93",
			"d1430b3f7ecf147534796c39ba631ea22ac03530e25b9428367c0dc381b10863",
			"2aeb69b1eb9eef8039da6b97d7851e46f57325851e6998ef5a84fc9a826c2c74",
			"fc61d13eef806af8da693cfa621fe92110694f1514567b186a35c54e7ef4a188",
			"a02dd44e60ba62fa00c83a67116f8079bf71062939b207bee0808cb98b30cf22",
			"279f97cfc606fe62777b44614ff28675ce661687904e068e3ec79f619c4fdae7",
			"d515d271849717b091a9c46bf11c47efb9d975e72b668c137786a208cf0a9739",
			"a800da44e6eed944043561fe22ee0a6e11341e6bc1a8ec2789b83930cc9b170e",
		},
	},
}

func helperLoadBlock(t *testing.T, height int) []byte {
	name := fmt.Sprintf("block_dump.%d", height)
	path := filepath.Join("testdata", name)

	d, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d = bytes.TrimSpace(d)

	b := make([]byte, hex.DecodedLen(len(d)))
	_, err = hex.Decode(b, d)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func TestParseBlock(t *testing.T) {
	p := NewDogecoinParser(GetChainParams("main"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs {
		b := helperLoadBlock(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Fatal(err)
		}

		if blk.Size != tb.size {
			t.Errorf("ParseBlock() block size: got %d, want %d", blk.Size, tb.size)
		}

		if blk.Time != tb.time {
			t.Errorf("ParseBlock() block time: got %d, want %d", blk.Time, tb.time)
		}

		if len(blk.Txs) != len(tb.txs) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.txs))
		}

		for ti, tx := range tb.txs {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}

var testParseBlockTxs_Testnet = map[int]testBlock{
	// block without auxpow
	99999: {
		size: 789,
		time: 1401525639,
		txs: []string{
			"a4006895a14f5eb8796c784e6845d6aaea57b87325ccd066e900dc53aa8bf6a4",
			"63c8d9dff87bf56804748e33c6c69f66d4930c4403b01cfd9a9d520fb91e4e17",
			"3fa993ca67bcd2b2dec15c61b7fc2947e268ebe71bcf77dc00236c99c15eaa72",
		},
	},
	// 1st block with auxpow – see https://github.com/dogecoin/dogecoin/releases/tag/v1.8.0-beta-1
	158100: {
		size: 227,
		time: 1407217902,
		txs: []string{
			"f09d8c02cd8a3da1af2a9722860fc58c593ffb3b6c51ffe09f978c89744561a7",
		},
	},
	// random block with auxpow
	1234580: {
		size: 850,
		time: 1521801579,
		txs: []string{
			"b16bd42a4718fa8db2f88fd7f5d268582af1b9cfd378600ba4e615708d7550cf",
			"2786caddf5c090be0554fe39511299b5daf5644024e80643c70c6573465252f1",
			"423f4c0efa75a521a29c21100b90d4f6bdbafeb0778657154cedb638b9c3b439",
		},
	},
	// recent block
	3274642: {
		size: 847,
		time: 1627931191,
		txs: []string{
			"aefc849cd9522e6d93a0ef4c2647dec96386756f02238bb05507407a589fb9a9",
			"877a2b2bf1f5b52e4893866aff3d573c49e22666662a510b0ff313bdf10e76b3",
			"8c9130974b68a81c652f3b43c6d352b892d920fe498d1f2fb566c1155169f443",
			"7fee19b12f19f426bc3e90b36d0149917695bcc4c57a07f0efebbdd30820a079",
		},
	},
}

func helperLoadBlock_Testnet(t *testing.T, height int) []byte {
	name := fmt.Sprintf("block_dump_testnet.%d", height)
	path := filepath.Join("testdata", name)

	d, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	d = bytes.TrimSpace(d)

	b := make([]byte, hex.DecodedLen(len(d)))
	_, err = hex.Decode(b, d)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func TestParseBlock_Testnet(t *testing.T) {
	p := NewDogecoinParser(GetChainParams("test"), &btc.Configuration{})

	for height, tb := range testParseBlockTxs_Testnet {
		b := helperLoadBlock_Testnet(t, height)

		blk, err := p.ParseBlock(b)
		if err != nil {
			t.Fatal(err)
		}

		if blk.Size != tb.size {
			t.Errorf("ParseBlock() block size: got %d, want %d", blk.Size, tb.size)
		}

		if blk.Time != tb.time {
			t.Errorf("ParseBlock() block time: got %d, want %d", blk.Time, tb.time)
		}

		if len(blk.Txs) != len(tb.txs) {
			t.Errorf("ParseBlock() number of transactions: got %d, want %d", len(blk.Txs), len(tb.txs))
		}

		for ti, tx := range tb.txs {
			if blk.Txs[ti].Txid != tx {
				t.Errorf("ParseBlock() transaction %d: got %s, want %s", ti, blk.Txs[ti].Txid, tx)
			}
		}
	}
}
