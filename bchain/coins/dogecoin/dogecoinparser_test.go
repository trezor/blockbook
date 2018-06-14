package dogecoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestAddressToOutputScript_Mainnet(t *testing.T) {
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
			got, err := parser.AddressToOutputScript(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressToOutputScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("AddressToOutputScript() = %v, want %v", h, tt.want)
			}
		})
	}
}

var (
	testTx1       bchain.Tx
	testTxPacked1 = "00030e6d8ba8d7aa2001000000016b3c0c53267964120acf7f7e72217e3f463e52ce622f89659f6a6bb8e69a4d91000000006c493046022100a96454237e3a020994534583e28c04757881374bceac89f933ea9ff00b4db259022100fbb757ff7ea4f02c4e42556b2834c61eba1f1af605db089d836a0614d90a3b46012103cebdde6d1046e285df4f48497bc50dc20a4a258ca5b7308cb0a929c9fdadcd9dffffffff0217e823ca7f0200001976a914eef21768a546590993e313c7f3dfadf6a6efa1e888acaddf4cba010000001976a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac00000000"

	testTx2       bchain.Tx
	testTxPacked2 = "0001193a8ba8d7835601000000016d0211b5656f1b8c2ac002445638e247082090ffc5d5fa7c38b445b84a2c2054000000006b4830450221008856f2f620df278c0fc6a5d5e2d50451c0a65a75aaf7a4a9cbfcac3918b5536802203dc685a784d49e2a95eb72763ad62f02094af78507c57b0a3c3f1d8a60f74db6012102db814cd43df584804fde1949365a6309714e342aef0794dc58385d7e413444cdffffffff0237daa2ee0a4715001976a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac8008389e7e8d03001976a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac00000000"
)

func init() {
	var (
		addr1, addr2, addr3, addr4 bchain.Address
		err                        error
	)
	addr1, err = bchain.NewBaseAddress("DSvXNiqvG42wdteLqh3i6inxgDTs8Y9w2i")
	if err == nil {
		addr2, err = bchain.NewBaseAddress("DRemF3ZcqJ1PFeM7e7sXzzwQJKR8GNUtwK")
	}
	if err == nil {
		addr3, err = bchain.NewBaseAddress("DJa8bWDrZKu4HgsYRYWuJrvxt6iTYuvXJ6")
	}
	if err == nil {
		addr4, err = bchain.NewBaseAddress("DDTtqnuZ5kfRT5qh2c7sNtqrJmV3iXYdGG")
	}
	if err != nil {
		panic(err)
	}

	testTx1 = bchain.Tx{
		Hex:       "01000000016b3c0c53267964120acf7f7e72217e3f463e52ce622f89659f6a6bb8e69a4d91000000006c493046022100a96454237e3a020994534583e28c04757881374bceac89f933ea9ff00b4db259022100fbb757ff7ea4f02c4e42556b2834c61eba1f1af605db089d836a0614d90a3b46012103cebdde6d1046e285df4f48497bc50dc20a4a258ca5b7308cb0a929c9fdadcd9dffffffff0217e823ca7f0200001976a914eef21768a546590993e313c7f3dfadf6a6efa1e888acaddf4cba010000001976a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac00000000",
		Blocktime: 1519053456,
		Txid:      "097ea09ba284f3f2a9e880e11f837edf7e5cea81c8da2238f5bc7c2c4c407943",
		LockTime:  0,
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
				Value: 27478.75452951,
				N:     0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914eef21768a546590993e313c7f3dfadf6a6efa1e888ac",
					Addresses: []string{
						"DSvXNiqvG42wdteLqh3i6inxgDTs8Y9w2i",
					},
				},
				Address: addr1,
			},
			{
				Value: 74.20567469,
				N:     1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914e0fee2ea29dd9c6c759d8341bd0da4c4f738cced88ac",
					Addresses: []string{
						"DRemF3ZcqJ1PFeM7e7sXzzwQJKR8GNUtwK",
					},
				},
				Address: addr2,
			},
		},
	}

	testTx2 = bchain.Tx{
		Hex:       "01000000016d0211b5656f1b8c2ac002445638e247082090ffc5d5fa7c38b445b84a2c2054000000006b4830450221008856f2f620df278c0fc6a5d5e2d50451c0a65a75aaf7a4a9cbfcac3918b5536802203dc685a784d49e2a95eb72763ad62f02094af78507c57b0a3c3f1d8a60f74db6012102db814cd43df584804fde1949365a6309714e342aef0794dc58385d7e413444cdffffffff0237daa2ee0a4715001976a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac8008389e7e8d03001976a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac00000000",
		Blocktime: 1519050987,
		Txid:      "b276545af246e3ed5a4e3e5b60d359942a1808579effc53ff4f343e4f6cfc5a0",
		LockTime:  0,
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
				Value: 59890867.89818935,
				N:     0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9149355c01ed20057eac9fe0bbf8b07d87e62fe712d88ac",
					Addresses: []string{
						"DJa8bWDrZKu4HgsYRYWuJrvxt6iTYuvXJ6",
					},
				},
				Address: addr3,
			},
			{
				Value: 9999998.90000000,
				N:     1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a9145b4f2511c94e4fcaa8f8835b2458f8cb6542ca7688ac",
					Addresses: []string{
						"DDTtqnuZ5kfRT5qh2c7sNtqrJmV3iXYdGG",
					},
				},
				Address: addr4,
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
