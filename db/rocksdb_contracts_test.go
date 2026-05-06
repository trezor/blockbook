//go:build unittest

package db

import (
	"reflect"
	"testing"

	"github.com/bsm/go-vlq"
	"github.com/trezor/blockbook/bchain"
)

func packCoreContractInfo(contractInfo *bchain.ContractInfo) []byte {
	buf := packString(contractInfo.Name)
	buf = append(buf, packString(contractInfo.Symbol)...)
	buf = append(buf, packString(string(contractInfo.Standard))...)
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(contractInfo.Decimals), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(contractInfo.CreatedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(contractInfo.DestructedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func Test_packUnpackContractInfo(t *testing.T) {
	tests := []struct {
		name         string
		contractInfo bchain.ContractInfo
	}{
		{
			name:         "empty",
			contractInfo: bchain.ContractInfo{},
		},
		{
			name: "unknown",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.UnknownTokenStandard,
				Standard:          bchain.UnknownTokenStandard,
				Name:              "Test contract",
				Symbol:            "TCT",
				Decimals:          18,
				CreatedInBlock:    1234567,
				DestructedInBlock: 234567890,
			},
		},
		{
			name: "ERC20",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.ERC20TokenStandard,
				Standard:          bchain.ERC20TokenStandard,
				Name:              "GreenContract🟢",
				Symbol:            "🟢",
				Decimals:          0,
				CreatedInBlock:    1,
				DestructedInBlock: 2,
			},
		},
		{
			name: "ERC20-ERC4626",
			contractInfo: bchain.ContractInfo{
				Type:              bchain.ERC20TokenStandard,
				Standard:          bchain.ERC20TokenStandard,
				Name:              "Vault Share",
				Symbol:            "vSHARE",
				Decimals:          18,
				CreatedInBlock:    100,
				DestructedInBlock: 0,
				IsErc4626:         true,
			},
		},
		{
			name: "ERC20-ERC4626-with-asset",
			contractInfo: bchain.ContractInfo{
				Type:                 bchain.ERC20TokenStandard,
				Standard:             bchain.ERC20TokenStandard,
				Name:                 "Vault Share",
				Symbol:               "vSHARE",
				Decimals:             18,
				CreatedInBlock:       200,
				DestructedInBlock:    0,
				IsErc4626:            true,
				Erc4626AssetContract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := packContractInfo(&tt.contractInfo)
			if got, err := unpackContractInfo(buf); !reflect.DeepEqual(*got, tt.contractInfo) || err != nil {
				t.Errorf("packUnpackContractInfo() = %v, want %v, error %v", *got, tt.contractInfo, err)
			}
		})
	}
}

func Test_packContractInfo_OmitsProtocolSectionWithoutProtocols(t *testing.T) {
	core := bchain.ContractInfo{
		Type:              bchain.ERC20TokenStandard,
		Standard:          bchain.ERC20TokenStandard,
		Name:              "Core Only",
		Symbol:            "CORE",
		Decimals:          6,
		CreatedInBlock:    77,
		DestructedInBlock: 0,
	}

	buf := packContractInfo(&core)
	expected := packCoreContractInfo(&core)
	if !reflect.DeepEqual(buf, expected) {
		t.Fatalf("expected no protocol section, got %x want %x", buf, expected)
	}
}

func Test_packContractInfo_WritesErc4626ProtocolContainer(t *testing.T) {
	contractInfo := bchain.ContractInfo{
		Type:                 bchain.ERC20TokenStandard,
		Standard:             bchain.ERC20TokenStandard,
		Name:                 "Vault Share",
		Symbol:               "vSHARE",
		Decimals:             18,
		CreatedInBlock:       200,
		DestructedInBlock:    0,
		IsErc4626:            true,
		Erc4626AssetContract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	}

	buf := packContractInfo(&contractInfo)
	coreLen := len(packCoreContractInfo(&contractInfo))
	tail := buf[coreLen:]
	if len(tail) == 0 {
		t.Fatal("expected protocol section to be present")
	}

	header, l, ok := unpackVaruintSafe(tail)
	if !ok {
		t.Fatal("expected extension header")
	}
	if header != contractInfoExtensionsVersion1 {
		t.Fatalf("unexpected extension header: %d", header)
	}
	tail = tail[l:]

	count, l, ok := unpackVaruintSafe(tail)
	if !ok {
		t.Fatal("expected extension count")
	}
	if count != 1 {
		t.Fatalf("unexpected extension count: %d", count)
	}
	tail = tail[l:]

	protocolID, l, ok := unpackVaruintSafe(tail)
	if !ok {
		t.Fatal("expected protocol id")
	}
	if protocolID != contractInfoProtocolErc4626 {
		t.Fatalf("unexpected protocol id: %d", protocolID)
	}
	tail = tail[l:]

	payloadLen, l, ok := unpackVaruintSafe(tail)
	if !ok {
		t.Fatal("expected payload length")
	}
	tail = tail[l:]
	if int(payloadLen) != len(tail) {
		t.Fatalf("unexpected payload length: %d actual %d", payloadLen, len(tail))
	}

	flags, l, ok := unpackVaruintSafe(tail)
	if !ok {
		t.Fatal("expected payload flags")
	}
	if flags != contractInfoFlagErc4626 {
		t.Fatalf("unexpected payload flags: %d", flags)
	}
	asset, consumed, ok := unpackStringSafe(tail[l:])
	if !ok {
		t.Fatal("expected asset contract string")
	}
	if asset != contractInfo.Erc4626AssetContract {
		t.Fatalf("unexpected asset contract: %q", asset)
	}
	if l+consumed != len(tail) {
		t.Fatalf("unexpected trailing payload bytes: consumed %d total %d", l+consumed, len(tail))
	}
}

func Test_packContractInfo_AssetOnlyStillWritesProtocolContainer(t *testing.T) {
	contractInfo := bchain.ContractInfo{
		Type:                 bchain.ERC20TokenStandard,
		Standard:             bchain.ERC20TokenStandard,
		Name:                 "Vault Share",
		Symbol:               "vSHARE",
		Decimals:             18,
		CreatedInBlock:       200,
		DestructedInBlock:    0,
		Erc4626AssetContract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	}

	buf := packContractInfo(&contractInfo)
	got, err := unpackContractInfo(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsErc4626 {
		t.Fatal("expected IsErc4626 to stay false")
	}
	if got.Erc4626AssetContract != contractInfo.Erc4626AssetContract {
		t.Fatalf("unexpected asset contract: %q", got.Erc4626AssetContract)
	}
}

func Test_unpackContractInfo_IgnoresNonExtensionTail(t *testing.T) {
	core := bchain.ContractInfo{
		Type:              bchain.ERC20TokenStandard,
		Standard:          bchain.ERC20TokenStandard,
		Name:              "Core Only",
		Symbol:            "CORE",
		Decimals:          18,
		CreatedInBlock:    321,
		DestructedInBlock: 0,
	}

	buf := packString(core.Name)
	buf = append(buf, packString(core.Symbol)...)
	buf = append(buf, packString(string(core.Standard))...)
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(core.Decimals), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(core.CreatedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(core.DestructedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(1, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, packString("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")...)

	got, err := unpackContractInfo(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != core.Name || got.Symbol != core.Symbol || got.Standard != core.Standard || got.Decimals != core.Decimals || got.CreatedInBlock != core.CreatedInBlock || got.DestructedInBlock != core.DestructedInBlock {
		t.Fatalf("core fields mismatch: %+v", got)
	}
	if got.IsErc4626 || got.Erc4626AssetContract != "" {
		t.Fatalf("unexpected protocol data from non-extension tail: %+v", got)
	}
}

func Test_unpackContractInfo_DecodesKnownExtensionAmongUnknownOnes(t *testing.T) {
	core := bchain.ContractInfo{
		Type:              bchain.ERC20TokenStandard,
		Standard:          bchain.ERC20TokenStandard,
		Name:              "Core Only",
		Symbol:            "CORE",
		Decimals:          6,
		CreatedInBlock:    77,
		DestructedInBlock: 0,
	}
	buf := packCoreContractInfo(&core)
	var varBuf [vlq.MaxLen64]byte

	l := packVaruint(contractInfoExtensionsVersion1, varBuf[:])
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(2, varBuf[:])
	buf = append(buf, varBuf[:l]...)

	l = packVaruint(999, varBuf[:])
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(3, varBuf[:])
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, 0xaa, 0xbb, 0xcc)

	payload := packContractInfoErc4626Payload(&bchain.ContractInfo{
		IsErc4626:            true,
		Erc4626AssetContract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
	})
	l = packVaruint(contractInfoProtocolErc4626, varBuf[:])
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(len(payload)), varBuf[:])
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, payload...)

	got, err := unpackContractInfo(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != core.Name || got.Symbol != core.Symbol || got.Standard != core.Standard || got.Decimals != core.Decimals || got.CreatedInBlock != core.CreatedInBlock || got.DestructedInBlock != core.DestructedInBlock {
		t.Fatalf("core fields mismatch: %+v", got)
	}
	if !got.IsErc4626 || got.Erc4626AssetContract != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
		t.Fatalf("expected known extension to decode: %+v", got)
	}
}

func Test_unpackContractInfo_IgnoresUnknownOrMalformedExtensions(t *testing.T) {
	core := bchain.ContractInfo{
		Type:              bchain.ERC20TokenStandard,
		Standard:          bchain.ERC20TokenStandard,
		Name:              "Core Only",
		Symbol:            "CORE",
		Decimals:          6,
		CreatedInBlock:    77,
		DestructedInBlock: 0,
	}
	base := packString(core.Name)
	base = append(base, packString(core.Symbol)...)
	base = append(base, packString(string(core.Standard))...)
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(core.Decimals), varBuf)
	base = append(base, varBuf[:l]...)
	l = packVaruint(uint(core.CreatedInBlock), varBuf)
	base = append(base, varBuf[:l]...)
	l = packVaruint(uint(core.DestructedInBlock), varBuf)
	base = append(base, varBuf[:l]...)

	tests := []struct {
		name string
		tail []byte
	}{
		{
			name: "unknown-version",
			tail: func() []byte {
				var buf []byte
				l := packVaruint(contractInfoExtensionsMarker|99, varBuf)
				buf = append(buf, varBuf[:l]...)
				return buf
			}(),
		},
		{
			name: "malformed-extension-payload",
			tail: func() []byte {
				var buf []byte
				l := packVaruint(contractInfoExtensionsVersion1, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packVaruint(1, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packVaruint(contractInfoProtocolErc4626, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packVaruint(5, varBuf)
				buf = append(buf, varBuf[:l]...)
				buf = append(buf, 0x01, 0x02)
				return buf
			}(),
		},
		{
			name: "count-exceeds-actual-extensions",
			tail: func() []byte {
				var buf []byte
				l := packVaruint(contractInfoExtensionsVersion1, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packVaruint(2, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packVaruint(contractInfoProtocolErc4626, varBuf)
				buf = append(buf, varBuf[:l]...)
				payload := packContractInfoErc4626Payload(&bchain.ContractInfo{
					IsErc4626:            true,
					Erc4626AssetContract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
				})
				l = packVaruint(uint(len(payload)), varBuf)
				buf = append(buf, varBuf[:l]...)
				buf = append(buf, payload...)
				return buf
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unpackContractInfo(append(append([]byte{}, base...), tt.tail...))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != core.Name || got.Symbol != core.Symbol || got.Standard != core.Standard || got.Decimals != core.Decimals || got.CreatedInBlock != core.CreatedInBlock || got.DestructedInBlock != core.DestructedInBlock {
				t.Fatalf("core fields mismatch: %+v", got)
			}
			if tt.name == "count-exceeds-actual-extensions" {
				if !got.IsErc4626 || got.Erc4626AssetContract != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
					t.Fatalf("expected first valid extension to survive malformed tail: %+v", got)
				}
				return
			}
			if got.IsErc4626 || got.Erc4626AssetContract != "" {
				t.Fatalf("unexpected protocol data from %s tail: %+v", tt.name, got)
			}
		})
	}
}
