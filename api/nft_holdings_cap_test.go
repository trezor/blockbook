//go:build unittest

package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

const (
	nftSpamHolder   = "c0970000000000000000000000000000000c0970"
	nftSpamContract = "c097c097c097c097c097c097c097c097c097c097"
	transferEventID = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

// nftSpamBlock is one tx emitting n ERC721 Transfer(0x0, holder, id) logs with 256-bit ids.
func nftSpamBlock(parser *eth.EthereumParser, n int, height uint32) *bchain.Block {
	tx := dbtestdata.GetTestEthereumTypeBlock2(parser).Txs[2] // ERC721 template
	txid := fmt.Sprintf("%064x", height)
	csd := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	rtx := *csd.Tx
	rtx.Hash = "0x" + txid
	rtx.To = "0x" + nftSpamContract
	csd.Tx = &rtx
	base := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1<<40))
	logs := make([]*bchain.RpcLog, n)
	for i := range logs {
		id := new(big.Int).Add(base, big.NewInt(int64(i)))
		logs[i] = &bchain.RpcLog{
			Address: "0x" + nftSpamContract,
			Topics: []string{transferEventID,
				"0x000000000000000000000000" + dbtestdata.EthAddrZero,
				"0x000000000000000000000000" + nftSpamHolder,
				fmt.Sprintf("0x%064x", id)},
			Data: "0x",
		}
	}
	csd.Receipt = &bchain.RpcReceipt{GasUsed: "0x1", Status: "0x1", Logs: logs}
	tx.Txid = txid
	tx.CoinSpecificData = csd
	tx.Vout = []bchain.Vout{{ScriptPubKey: bchain.ScriptPubKey{Addresses: []string{"0x" + nftSpamContract}}}}
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{Height: height, Hash: fmt.Sprintf("0x%064x", height), Time: 1534860000},
		Txs:         []bchain.Tx{tx},
	}
}

func TestGetAddress_NFTHoldingsCapped(t *testing.T) {
	chain := newContractProbeChain(t)
	w, database, parser := setupContractProbeWorker(t, chain)
	cd := addrDesc(t, parser, nftSpamContract)
	chain.tokens[string(cd)] = &bchain.ContractInfo{Contract: eth.EIP55Address(cd), Name: "Spam", Symbol: "SPAM"}
	const held = 3 * maxTokenIdsInResponse
	require.NoError(t, database.ConnectBlock(nftSpamBlock(parser, held, 4321002)))

	get := func(details AccountDetails) (*Address, int, uint64) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		a, err := w.GetAddress("0x"+nftSpamHolder, 1, 25, details, &AddressFilter{Vout: AddressFilterVoutOff, OnlyConfirmed: true}, "")
		require.NoError(t, err)
		b, err := json.Marshal(a)
		require.NoError(t, err)
		runtime.ReadMemStats(&after)
		return a, len(b), after.Mallocs - before.Mallocs
	}

	a, size, _ := get(AccountDetailsTokenBalances)
	require.Len(t, a.Tokens, 1)
	require.Len(t, a.Tokens[0].Ids, maxTokenIdsInResponse)
	require.Equal(t, held, a.Tokens[0].IdsTotal)
	require.Less(t, size, 100*maxTokenIdsInResponse, "response size must not grow with stored ids")

	// basic returns no ids, so it must not decode them either
	_, _, mallocs := get(AccountDetailsBasic)
	require.Less(t, mallocs, uint64(200), "details=basic allocates per stored id")
}
