package main

import (
	"fmt"
	"math/big"
	"time"

	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/server"
)

func main() {
	t := typescriptify.New()
	t.CreateInterface = true
	t.Indent = "  "
	t.BackupDir = ""

	t.ManageType(api.Amount{}, typescriptify.TypeOptions{TSType: "string"})
	t.ManageType([]api.Amount{}, typescriptify.TypeOptions{TSType: "string[]"})
	t.ManageType(big.Int{}, typescriptify.TypeOptions{TSType: "number"})
	t.ManageType(time.Time{}, typescriptify.TypeOptions{TSType: "string", TSDoc: "Time in ISO 8601 YYYY-MM-DDTHH:mm:ss.sssZd"})

	// API - REST and Websocket
	t.Add(api.APIError{})
	t.Add(api.Tx{})
	t.Add(api.FeeStats{})
	t.Add(api.Address{})
	t.Add(api.Utxo{})
	t.Add(api.BalanceHistory{})
	t.Add(api.Blocks{})
	t.Add(api.Block{})
	t.Add(api.BlockRaw{})
	t.Add(api.SystemInfo{})
	t.Add(api.FiatTicker{})
	t.Add(api.FiatTickers{})
	t.Add(api.AvailableVsCurrencies{})

	// Websocket specific
	t.Add(server.WsReq{})
	t.Add(server.WsRes{})
	t.Add(server.WsAccountInfoReq{})
	t.Add(server.WsInfoRes{})
	t.Add(server.WsBlockHashReq{})
	t.Add(server.WsBlockHashRes{})
	t.Add(server.WsBlockReq{})
	t.Add(server.WsBlockFilterReq{})
	t.Add(server.WsBlockFiltersBatchReq{})
	t.Add(server.WsAccountUtxoReq{})
	t.Add(server.WsBalanceHistoryReq{})
	t.Add(server.WsTransactionReq{})
	t.Add(server.WsTransactionSpecificReq{})
	t.Add(server.WsEstimateFeeReq{})
	t.Add(server.WsEstimateFeeRes{})
	t.Add(server.WsSendTransactionReq{})
	t.Add(server.WsSubscribeAddressesReq{})
	t.Add(server.WsSubscribeFiatRatesReq{})
	t.Add(server.WsCurrentFiatRatesReq{})
	t.Add(server.WsFiatRatesForTimestampsReq{})
	t.Add(server.WsFiatRatesTickersListReq{})
	t.Add(server.WsMempoolFiltersReq{})
	t.Add(bchain.MempoolTxidFilterEntries{})

	err := t.ConvertToFile("blockbook-api.ts")
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("OK")
}
