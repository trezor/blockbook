//go:build unittest

package satoxcoin

import (
	"encoding/hex"
	"math/big"
	"os"
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
			args:    args{address: "SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"},
			want:    "76a9141e391a5c2346da0c7d9984e6a86902f4fc65c29d88ac",
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{address: "SQ5iQMsmqZiYY96rTx5Hisd7sx5GiGUbbN"},
			want:    "76a9141e95812f9e546dbb175b9816d32f98deeaa0f9ff88ac",
			wantErr: false,
		},
		{
			name:    "InvalidAddress",
			args:    args{address: "invalid_address"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "EmptyAddress",
			args:    args{address: ""},
			want:    "",
			wantErr: true,
		},
	}
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetAddrDescFromAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddrDescFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				h := hex.EncodeToString(got)
				if !reflect.DeepEqual(h, tt.want) {
					t.Errorf("GetAddrDescFromAddress() = %v, want %v", h, tt.want)
				}
			}
		})
	}
}

func Test_GetAddressesFromAddrDesc(t *testing.T) {
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
			args:    args{script: "76a9141e391a5c2346da0c7d9984e6a86902f4fc65c29d88ac"},
			want:    []string{"SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"},
			want2:   true,
			wantErr: false,
		},
		{
			name:    "P2PKH2",
			args:    args{script: "76a9141e95812f9e546dbb175b9816d32f98deeaa0f9ff88ac"},
			want:    []string{"SQ5iQMsmqZiYY96rTx5Hisd7sx5GiGUbbN"},
			want2:   true,
			wantErr: false,
		},
	}
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, _ := hex.DecodeString(tt.args.script)
			got, got2, err := parser.GetAddressesFromAddrDesc(script)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAddressesFromAddrDesc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetAddressesFromAddrDesc() = %v, want %v", got, tt.want)
			}
			if got2 != tt.want2 {
				t.Errorf("GetAddressesFromAddrDesc() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

// Test transaction data for Satoxcoin
var (
	testTx1       bchain.Tx
	testTxPacked1 = "0a20d4d3a093586eae0c3668fd288d9e24955928a894c20b551b38dd18c99b123a7c12e1010200000001c171348ffc8976074fa064e48598a816fce3798afc635fb67d99580e50b8e614000000006a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46affffffff02f8137114000000001976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ace0c48110000000001976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac0000000018c7e1b3e50528849128329401122014e6b8500e58997db65f63fc8a79e3fc16a89885e464a04f077689fc8f3471c1226a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46a28ffffffff0f3a450a04147113f81a1976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ac222253484d31746d64766b6b3776446f69477877554a414d4e4e6d447179775a3574456e3a470a041081c4e010011a1976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac2222535631463939623955424272434d38614e4b7567737173444d3869716f4371374d744002"
)

func init() {
	// Create a simple Satoxcoin transaction for testing
	testTx1 = bchain.Tx{
		Hex:       "0200000001c171348ffc8976074fa064e48598a816fce3798afc635fb67d99580e50b8e614000000006a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46affffffff02f8137114000000001976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ace0c48110000000001976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac00000000",
		Blocktime: 1554837703,
		Time:      1554837703,
		Txid:      "d4d3a093586eae0c3668fd288d9e24955928a894c20b551b38dd18c99b123a7c",
		LockTime:  0,
		Version:   2,
		Vin: []bchain.Vin{
			{
				ScriptSig: bchain.ScriptSig{
					Hex: "473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46a",
				},
				Txid:     "14e6b8500e58997db65f63fc8a79e3fc16a89885e464a04f077689fc8f3471c1",
				Vout:     0,
				Sequence: 4294967295,
			},
		},
		Vout: []bchain.Vout{
			{
				ValueSat: *big.NewInt(342955000),
				N:        0,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ac",
					Addresses: []string{
						"SHM1tmdvkk7vDoiGxwUJAMNNmDqywZ5tEn",
					},
				},
			},
			{
				ValueSat: *big.NewInt(276940000),
				N:        1,
				ScriptPubKey: bchain.ScriptPubKey{
					Hex: "76a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac",
					Addresses: []string{
						"SV1F99b9UBBrCM8aNKugsqsDM8iqoCq7Mt",
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
		parser    *SatoxcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "satoxcoin-1",
			args: args{
				tx:        testTx1,
				height:    657540,
				blockTime: 1554837703,
				parser:    NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    testTxPacked1,
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
		parser   *SatoxcoinParser
	}
	tests := []struct {
		name    string
		args    args
		want    *bchain.Tx
		want1   uint32
		wantErr bool
	}{
		{
			name: "satoxcoin-1",
			args: args{
				packedTx: testTxPacked1,
				parser:   NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{}),
			},
			want:    &testTx1,
			want1:   657540,
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

func Test_GetChainParams(t *testing.T) {
	tests := []struct {
		name string
		want *chaincfg.Params
	}{
		{
			name: "main",
			want: &MainNetParams,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetChainParams(tt.name)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetChainParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_SatoxcoinNetworkParams(t *testing.T) {
	params := GetChainParams("main")

	// Test Satoxcoin-specific network parameters
	if params.Net != MainnetMagic {
		t.Errorf("Expected magic number %x, got %x", MainnetMagic, params.Net)
	}

	if params.PubKeyHashAddrID[0] != 63 { // 'S' prefix
		t.Errorf("Expected P2PKH address prefix %d, got %d", 63, params.PubKeyHashAddrID[0])
	}

	if params.ScriptHashAddrID[0] != 122 { // 's' prefix
		t.Errorf("Expected P2SH address prefix %d, got %d", 122, params.ScriptHashAddrID[0])
	}

	if params.HDCoinType != 9007 { // SLIP44 for Satoxcoin
		t.Errorf("Expected HD coin type %d, got %d", 9007, params.HDCoinType)
	}
}

// Additional 10 comprehensive tests for maximum reliability

func Test_ParserInitialization(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})
	if parser == nil {
		t.Error("Parser should not be nil")
	}
	if parser.BitcoinLikeParser == nil {
		t.Error("BitcoinLikeParser should not be nil")
	}
	if parser.baseparser == nil {
		t.Error("baseparser should not be nil")
	}
}

func Test_AddressValidation(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test valid Satoxcoin addresses
	validAddresses := []string{
		"SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa",
		"SQ5iQMsmqZiYY96rTx5Hisd7sx5GiGUbbN",
	}

	for _, addr := range validAddresses {
		_, err := parser.GetAddrDescFromAddress(addr)
		if err != nil {
			t.Errorf("Valid address %s should not return error: %v", addr, err)
		}
	}

	// Test invalid addresses
	invalidAddresses := []string{
		"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", // Bitcoin address
		"RAoGkGhKwzxLnstApumYPD2eTrAJ849cga", // Ravencoin address
		"invalid_address",
		"",
	}

	for _, addr := range invalidAddresses {
		_, err := parser.GetAddrDescFromAddress(addr)
		if err == nil {
			t.Errorf("Invalid address %s should return error", addr)
		}
	}
}

func Test_ScriptValidation(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test valid scripts
	validScripts := []string{
		"76a9141e391a5c2346da0c7d9984e6a86902f4fc65c29d88ac",
		"76a9141e95812f9e546dbb175b9816d32f98deeaa0f9ff88ac",
	}

	for _, script := range validScripts {
		scriptBytes, _ := hex.DecodeString(script)
		_, _, err := parser.GetAddressesFromAddrDesc(scriptBytes)
		if err != nil {
			t.Errorf("Valid script %s should not return error: %v", script, err)
		}
	}
}

func Test_TransactionValidation(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test transaction with valid structure
	tx := bchain.Tx{
		Hex:       "0200000001c171348ffc8976074fa064e48598a816fce3798afc635fb67d99580e50b8e614000000006a473044022009e07574fa543ad259bd3334eb365c655c96d310c578b64c24d7f77fa7dc591c0220427d8ae6eacd1ca2d1994e9ec49cb322aacdde98e4bdb065e0fce81162fb3aa9012102d46827546548b9b47ae1e9e84fc4e53513e0987eeb1dd41220ba39f67d3bf46affffffff02f8137114000000001976a914587a2afa560ccaeaeb67cb72a0db7e2573a179e488ace0c48110000000001976a914d85e6ab66ab0b2c4cfd40ca3b0a779529da5799288ac00000000",
		Blocktime: 1554837703,
		Time:      1554837703,
		Txid:      "d4d3a093586eae0c3668fd288d9e24955928a894c20b551b38dd18c99b123a7c",
		LockTime:  0,
		Version:   2,
	}

	_, err := parser.PackTx(&tx, 657540, 1554837703)
	if err != nil {
		t.Errorf("Valid transaction should not return error: %v", err)
	}
}

func Test_NetworkConsistency(t *testing.T) {
	// Test that network parameters are consistent across multiple calls
	params1 := GetChainParams("main")
	params2 := GetChainParams("main")

	if !reflect.DeepEqual(params1, params2) {
		t.Error("Network parameters should be consistent across calls")
	}

	// Test that magic number is unique
	if params1.Net == chaincfg.MainNetParams.Net {
		t.Error("Satoxcoin magic number should be different from Bitcoin")
	}
}

func Test_AddressFormatConsistency(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test that address format is consistent
	addr := "SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"
	desc1, err1 := parser.GetAddrDescFromAddress(addr)
	desc2, err2 := parser.GetAddrDescFromAddress(addr)

	if err1 != nil || err2 != nil {
		t.Error("Address parsing should not fail")
	}

	if !reflect.DeepEqual(desc1, desc2) {
		t.Error("Address parsing should be consistent")
	}
}

func Test_ScriptFormatConsistency(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test that script format is consistent
	script := "76a9141e391a5c2346da0c7d9984e6a86902f4fc65c29d88ac"
	scriptBytes, _ := hex.DecodeString(script)

	addr1, _, err1 := parser.GetAddressesFromAddrDesc(scriptBytes)
	addr2, _, err2 := parser.GetAddressesFromAddrDesc(scriptBytes)

	if err1 != nil || err2 != nil {
		t.Error("Script parsing should not fail")
	}

	if !reflect.DeepEqual(addr1, addr2) {
		t.Error("Script parsing should be consistent")
	}
}

func Test_TransactionRoundTrip(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test pack -> unpack round trip
	originalTx := testTx1
	packed, err := parser.PackTx(&originalTx, 657540, 1554837703)
	if err != nil {
		t.Errorf("PackTx failed: %v", err)
		return
	}

	unpacked, height, err := parser.UnpackTx(packed)
	if err != nil {
		t.Errorf("UnpackTx failed: %v", err)
		return
	}

	if height != 657540 {
		t.Errorf("Expected height %d, got %d", 657540, height)
	}

	// Compare key fields
	if unpacked.Txid != originalTx.Txid {
		t.Errorf("Txid mismatch: expected %s, got %s", originalTx.Txid, unpacked.Txid)
	}

	if unpacked.Version != originalTx.Version {
		t.Errorf("Version mismatch: expected %d, got %d", originalTx.Version, unpacked.Version)
	}
}

func Test_ParserConfiguration(t *testing.T) {
	config := &btc.Configuration{}
	parser := NewSatoxcoinParser(GetChainParams("main"), config)

	// Test that parser is properly configured
	if parser == nil {
		t.Error("Parser should be created successfully")
	}

	// Test that parser can handle basic operations
	addr := "SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"
	_, err := parser.GetAddrDescFromAddress(addr)
	if err != nil {
		t.Errorf("Parser should handle basic address parsing: %v", err)
	}
}

func Test_EdgeCases(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test address with special characters
	specialAddr := "SQ3oiBR@wUU7qprNBHPPr1MkgsPRcpRsaa"
	_, err := parser.GetAddrDescFromAddress(specialAddr)
	if err == nil {
		t.Error("Address with special characters should fail")
	}

	// Test address with wrong prefix
	wrongPrefixAddr := "1Q3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"
	_, err = parser.GetAddrDescFromAddress(wrongPrefixAddr)
	if err == nil {
		t.Error("Address with wrong prefix should fail")
	}
}

func Test_MultipleParserInstances(t *testing.T) {
	// Test that multiple parser instances work correctly
	parser1 := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})
	parser2 := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	if parser1 == nil || parser2 == nil {
		t.Error("Multiple parser instances should be created successfully")
	}

	// Test that both parsers work identically
	addr := "SQ3oiBRawUU7qprNBHPPr1MkgsPRcpRsaa"
	desc1, err1 := parser1.GetAddrDescFromAddress(addr)
	desc2, err2 := parser2.GetAddrDescFromAddress(addr)

	if err1 != nil || err2 != nil {
		t.Error("Both parsers should handle address parsing")
	}

	if !reflect.DeepEqual(desc1, desc2) {
		t.Error("Multiple parser instances should produce identical results")
	}
}

func Test_ChainParamsRegistration(t *testing.T) {
	// Test that chain parameters are properly registered
	params := GetChainParams("main")

	// Test that parameters are not empty
	if params.Net == 0 {
		t.Error("Network magic number should not be zero")
	}

	if len(params.PubKeyHashAddrID) == 0 {
		t.Error("P2PKH address prefix should not be empty")
	}

	if len(params.ScriptHashAddrID) == 0 {
		t.Error("P2SH address prefix should not be empty")
	}

	if params.HDCoinType == 0 {
		t.Error("HD coin type should not be zero")
	}
}

func Test_BurnAddress(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test the official Satoxcoin burn address
	burnAddress := "SQBurnSatoXAddressXXXXXXXXXXUqEipi"

	// Test that the burn address can be parsed
	desc, err := parser.GetAddrDescFromAddress(burnAddress)
	if err != nil {
		t.Errorf("Burn address should be parseable: %v", err)
	}

	// Test that the burn address can be converted back
	addresses, _, err := parser.GetAddressesFromAddrDesc(desc)
	if err != nil {
		t.Errorf("Burn address script should be convertible back: %v", err)
	}

	if len(addresses) == 0 {
		t.Error("Burn address should return at least one address")
	}

	if addresses[0] != burnAddress {
		t.Errorf("Expected burn address %s, got %s", burnAddress, addresses[0])
	}

	// Test that the burn address has the correct format
	if len(burnAddress) != 34 {
		t.Errorf("Burn address should be 34 characters long, got %d", len(burnAddress))
	}

	if burnAddress[:2] != "SQ" {
		t.Errorf("Burn address should start with 'SQ', got %s", burnAddress[:2])
	}
}

func Test_BurnAddressValidation(t *testing.T) {
	parser := NewSatoxcoinParser(GetChainParams("main"), &btc.Configuration{})

	// Test the official Satoxcoin burn address
	burnAddress := "SQBurnSatoXAddressXXXXXXXXXXUqEipi"

	// Test that the official burn address is valid
	_, err := parser.GetAddrDescFromAddress(burnAddress)
	if err != nil {
		t.Errorf("Official burn address should be valid: %v", err)
	}

	// Test invalid burn addresses
	invalidBurnAddresses := []string{
		"SQBurnSatoXAddressXXXXXXXXXX",            // Too short
		"SQBurnSatoXAddressXXXXXXXXXXUqEipiExtra", // Too long
		"1BurnSatoXAddressXXXXXXXXXXUqEipi",       // Wrong prefix
		"SQBurnAddressXXXXXXXXXXXXXXXXXX",         // Invalid checksum
		"SQBurnXXXXXXXXXXXXXXXXXXXXXXXX",          // Invalid checksum
	}

	for _, addr := range invalidBurnAddresses {
		_, err := parser.GetAddrDescFromAddress(addr)
		if err == nil {
			t.Errorf("Invalid burn address %s should fail", addr)
		}
	}
}

func init() {
	chaincfg.ResetParams()
}
