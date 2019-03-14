package nuls

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"reflect"
	"testing"
)

var (
	testTx1, testTx2 bchain.Tx

	testTxPacked1 = "0001e240daadfbe7931e000000007b22686578223a22222c2274786964223a223030323036626231323431303861356664393865383238623138316666303162393237633063366234373764343531326266656638346366353266306663326136613161222c2276657273696f6e223a312c226c6f636b74696d65223a302c2276696e223a5b7b22636f696e62617365223a22222c2274786964223a223030323035343537616230373033623164313034363264343334373034386330626233353634653430663537616531663632353136393539343364653161633831306130222c22766f7574223a302c22736372697074536967223a7b22686578223a22227d2c2273657175656e6365223a302c22616464726573736573223a5b224e736535334d77524c424a31575555365365644d485141466643507442377734225d7d5d2c22766f7574223a5b7b2256616c7565536174223a3339393939393030303030302c2276616c7565223a302c226e223a302c227363726970745075624b6579223a7b22686578223a224e7365347a705a4873557555376835796d7632387063476277486a75336a6f56222c22616464726573736573223a5b224e7365347a705a4873557555376835796d7632387063476277486a75336a6f56225d7d7d5d2c22626c6f636b74696d65223a313535323335373834343137357d"
	testTxPacked2 = "0007c91adaadfbb89946000000007b22686578223a22222c2274786964223a223030323037386139386633383163373134613036386436303565346265316565323139353438353736313165303938616262636333663530633536383066386164326535222c2276657273696f6e223a312c226c6f636b74696d65223a302c2276696e223a5b7b22636f696e62617365223a22222c2274786964223a223030323037613430646334623661633430376434396133633333616137353462623466303565343565353763323438313162313437653762663363616630363361383233222c22766f7574223a312c22736372697074536967223a7b22686578223a22227d2c2273657175656e6365223a302c22616464726573736573223a5b224e73653131397a326f53444a596b466b786d775944695974506642654e6b7169225d7d5d2c22766f7574223a5b7b2256616c7565536174223a3430303030303030303030302c2276616c7565223a302c226e223a302c227363726970745075624b6579223a7b22686578223a224e736534696b6a45383867324267734e7773737754646b53776953724b6a6a53222c22616464726573736573223a5b224e736534696b6a45383867324267734e7773737754646b53776953724b6a6a53225d7d7d2c7b2256616c7565536174223a373238363536353537303030302c2276616c7565223a302c226e223a312c227363726970745075624b6579223a7b22686578223a224e73653131397a326f53444a596b466b786d775944695974506642654e6b7169222c22616464726573736573223a5b224e73653131397a326f53444a596b466b786d775944695974506642654e6b7169225d7d7d5d2c22626c6f636b74696d65223a313535323335373435393535357d"
)

func init() {
	testTx1 = bchain.Tx{
		Hex:       "",
		Blocktime: 1552357844175,
		Txid:      "00206bb124108a5fd98e828b181ff01b927c0c6b477d4512bfef84cf52f0fc2a6a1a",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				Txid:     "00205457ab0703b1d10462d4347048c0bb3564e40f57ae1f6251695943de1ac810a0",
				Vout:     0,
				Sequence: 0,
				Addresses: []string{
					"Nse53MwRLBJ1WUU6SedMHQAFfCPtB7w4",
				},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat:  *big.NewInt(399999000000),
				N:         0,
				JsonValue: json.Number("0"),
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "Nse4zpZHsUuU7h5ymv28pcGbwHju3joV",
					Addresses: []string{
						"Nse4zpZHsUuU7h5ymv28pcGbwHju3joV",
					},
				},
			},
		},
		//CoinSpecificData: []string{},
	}

	testTx2 = bchain.Tx{
		Hex:       "",
		Blocktime: 1552357459555,
		Txid:      "002078a98f381c714a068d605e4be1ee21954857611e098abbcc3f50c5680f8ad2e5",
		LockTime:  0,
		Version:   1,
		Vin: []bchain.Vin{
			{
				Txid:     "00207a40dc4b6ac407d49a3c33aa754bb4f05e45e57c24811b147e7bf3caf063a823",
				Vout:     1,
				Sequence: 0,
				Addresses: []string{
					"Nse119z2oSDJYkFkxmwYDiYtPfBeNkqi",
				},
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat:  *big.NewInt(400000000000),
				N:         0,
				JsonValue: json.Number("0"),
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "Nse4ikjE88g2BgsNwsswTdkSwiSrKjjS",
					Addresses: []string{
						"Nse4ikjE88g2BgsNwsswTdkSwiSrKjjS",
					},
				},
			},
			{
				ValueSat:  *big.NewInt(7286565570000),
				N:         1,
				JsonValue: json.Number("0"),
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "Nse119z2oSDJYkFkxmwYDiYtPfBeNkqi",
					Addresses: []string{
						"Nse119z2oSDJYkFkxmwYDiYtPfBeNkqi",
					},
				},
			},
		},
		//CoinSpecificData: []string{},
	}
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
			args:    args{address: "Nse4j39uEMuxx5j577h3K2MDLAQ64JZN"},
			want:    "042301ac78cd3eb193287e2c59e4cbd765c5c47d432c2fa1",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "Nse2e7U7nmGT8UHsvQ7JfksLtWwoLwrd"},
			want:    "0423018a90e66a64318f6af6d673487a6560f5686fd26a2e",
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{address: "NsdvMEP57nzxmBa5z18rx9sQsLgUfNtw"},
			want:    "04230124422cfe426573e476fd45d7c2a43a75a0b6b8c478",
			wantErr: false,
		},
	}
	parser := NewNulsParser(GetChainParams("main"), &btc.Configuration{})

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
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "Nse4j39uEMuxx5j577h3K2MDLAQ64JZN"}}},
			want:    "042301ac78cd3eb193287e2c59e4cbd765c5c47d432c2fa1",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "Nse2e7U7nmGT8UHsvQ7JfksLtWwoLwrd"}}},
			want:    "0423018a90e66a64318f6af6d673487a6560f5686fd26a2e",
			wantErr: false,
		},
		{
			name:    "P2PK",
			args:    args{vout: bchain.Vout{ScriptPubKey: bchain.ScriptPubKey{Hex: "NsdvMEP57nzxmBa5z18rx9sQsLgUfNtw"}}},
			want:    "04230124422cfe426573e476fd45d7c2a43a75a0b6b8c478",
			wantErr: false,
		},
	}
	parser := NewNulsParser(GetChainParams("main"), &btc.Configuration{})

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
			args:    args{script: "042301ac78cd3eb193287e2c59e4cbd765c5c47d432c2fa1"},
			want:    []string{"Nse4j39uEMuxx5j577h3K2MDLAQ64JZN"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{script: "0423018a90e66a64318f6af6d673487a6560f5686fd26a2e"},
			want:    []string{"Nse2e7U7nmGT8UHsvQ7JfksLtWwoLwrd"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH",
			args:    args{script: "04230124422cfe426573e476fd45d7c2a43a75a0b6b8c478"},
			want:    []string{"NsdvMEP57nzxmBa5z18rx9sQsLgUfNtw"},
			want2:   true,
			wantErr: false,
		},
	}

	parser := NewNulsParser(GetChainParams("main"), &btc.Configuration{})

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

func TestPackTx(t *testing.T) {
	type args struct {
		tx        bchain.Tx
		height    uint32
		blockTime int64
		parser    *NulsParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test-1",
			args: args{
				tx:        testTx1,
				height:    123456,
				blockTime: 1552357844175,
				parser:    NewNulsParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
			wantErr: false,
		},
		{
			name: "test-2",
			args: args{
				tx:        testTx2,
				height:    510234,
				blockTime: 1552357459555,
				parser:    NewNulsParser(GetChainParams("main"), &btc.Configuration{}),
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

func TestUnpackTx(t *testing.T) {
	type args struct {
		packedTx string
		parser   *NulsParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "test-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewNulsParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   123456,
			wantErr: false,
		},
		{
			name: "test-2",
			args: args{
				packedTx: testTxPacked2,
				parser:   NewNulsParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx2,
			want1:   510234,
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
				t.Errorf("unpackTx() got = %+v, want %+v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("unpackTx() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
