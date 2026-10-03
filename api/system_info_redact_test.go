//go:build unittest

package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/fiat"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// chainInfoFailingChain answers GetChainInfo with the *url.Error shape the http client produces
// on a dial failure, which carries the whole backend url.
type chainInfoFailingChain struct {
	bchain.BlockChain
}

func (c *chainInfoFailingChain) GetChainInfo() (*bchain.ChainInfo, error) {
	return nil, errors.New(`Post "https://mainnet.example.io/v3/SECRET_PROVIDER_KEY": dial tcp: connection refused`)
}

func TestGetSystemInfo_BackendErrorRedactsBackendURL(t *testing.T) {
	fake, err := dbtestdata.NewFakeBlockChainEthereumType(eth.NewEthereumParser(1, true))
	require.NoError(t, err)
	w, _, _ := setupContractProbeWorker(t, &chainInfoFailingChain{BlockChain: fake})
	w.fiatRates = &fiat.FiatRates{}

	si, err := w.GetSystemInfo(false)
	require.NoError(t, err)
	require.False(t, si.Blockbook.InSync)

	body, err := json.Marshal(si)
	require.NoError(t, err)
	require.NotContains(t, string(body), "SECRET_PROVIDER_KEY")
	require.Equal(t, `GetChainInfo: Post "https://mainnet.example.io": dial tcp: connection refused`, si.Backend.BackendError)
	require.True(t, strings.Contains(string(body), `mainnet.example.io`), "host is kept for the operator")
}
