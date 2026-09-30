//go:build unittest

package bch

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestBcashTransactionVectors(t *testing.T) {
	fixtures, err := dbtestdata.GetBcashFixtures()
	require.NoError(t, err)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			parser, err := NewBCashParser(GetChainParams(fixture.Chain), &btc.Configuration{AddressFormat: "cashaddr"})
			require.NoError(t, err)
			raw, err := hex.DecodeString(fixture.Hex)
			require.NoError(t, err)
			tx, err := parser.ParseTx(raw)
			require.NoError(t, err)
			require.Equal(t, fixture.Txid, tx.Txid)
			require.Len(t, tx.Vout, len(fixture.Outputs))
			for i, want := range fixture.Outputs {
				descriptor, addresses, _, token, err := GetAddressesAndTokenFromVout(parser, &tx.Vout[i])
				require.NoError(t, err)
				require.Equal(t, want.Descriptor, hex.EncodeToString(descriptor))
				require.Equal(t, []string{want.Address}, addresses)
				require.Equal(t, want.Value, tx.Vout[i].ValueSat.String())
				wantToken, err := json.Marshal(want.Token)
				require.NoError(t, err)
				gotToken, err := json.Marshal(token)
				require.NoError(t, err)
				require.JSONEq(t, string(wantToken), string(gotToken))
				roundTrip, err := parser.GetAddrDescFromAddress(want.Address)
				require.NoError(t, err)
				require.Equal(t, hex.EncodeToString(descriptor), hex.EncodeToString(roundTrip))
			}
		})
	}
}

func TestBcashCommitmentLengthOverflow(t *testing.T) {
	prefix := append([]byte{0xef}, make([]byte, 32)...)
	prefix = append(prefix, 0x60, 0xff)
	for i := 0; i < 8; i++ {
		prefix = append(prefix, math.MaxUint8)
	}
	require.NotPanics(t, func() {
		_, _, err := UnpackTokenData(prefix)
		require.Error(t, err)
	})
}

func TestBcashChipnet(t *testing.T) {
	parser, err := NewBCashParser(GetChainParams("chip"), &btc.Configuration{AddressFormat: "cashaddr"})
	require.NoError(t, err)
	addresses, _, err := parser.GetAddressesFromAddrDesc([]byte{0x76, 0xa9, 0x14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x88, 0xac})
	require.NoError(t, err)
	require.Equal(t, []string{"bchtest:qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdpn3jdgd"}, addresses)
}
