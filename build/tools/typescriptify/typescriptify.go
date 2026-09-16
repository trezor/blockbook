// Command typescriptify regenerates blockbook-api.ts from the Go API structs.
//
//	make typescriptify                  # inside the build image, no local RocksDB needed
//	go run ./build/tools/typescriptify  # from the repository root
//
// On top of the library's ts_type/ts_doc tags, two conventions keep the output reproducible:
//   - ts_type:"<Name>" where <Name> is listed in tsAliases emits a shared `export type` instead
//     of inlining the union at every use site (the library cannot emit named aliases itself).
//   - ts_nullable:"true" on a pointer field without omitempty emits `name: T | null` instead of
//     the library's `name?: T`, matching what encoding/json actually puts on the wire.
package main

import (
	"fmt"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/server"
)

const outputFile = "blockbook-api.ts"

// The first line is kept verbatim so tooling that recognises the file by it keeps working.
const header = "/* Do not change, this code is generated from Golang structs */\n" +
	"/* Regenerate with `make typescriptify` (see build/tools/typescriptify) */\n\n"

// tsAlias is a named TypeScript type referenced from Go fields via ts_type:"<name>".
type tsAlias struct {
	name       string
	definition string
}

var tsAliases = []tsAlias{
	{"TxChainExtraData", "{ payloadType: 'tron'; payload?: TronChainExtraData } | { payloadType: string; payload?: any }"},
	{"AccountChainExtraData", "{ payloadType: 'tron'; payload?: TronAccountExtraData } | { payloadType: string; payload?: any }"},
}

// apiTypes are the roots of the generated file; nested structs are discovered by the library.
var apiTypes = []interface{}{
	// API - REST and Websocket
	api.APIError{},
	bchain.TronChainExtraData{},
	bchain.TronAccountExtraData{},
	api.Tx{},
	api.FeeStats{},
	api.Address{},
	api.ContractInfoResult{},
	api.Utxo{},
	api.BalanceHistory{},
	api.Blocks{},
	api.Block{},
	api.BlockRaw{},
	api.SystemInfo{},
	api.FiatTicker{},
	api.FiatTickers{},
	api.AvailableVsCurrencies{},

	// Websocket specific
	server.WsReq{},
	server.WsRes{},
	server.WsAccountInfoReq{},
	server.WsContractInfoReq{},
	server.WsInfoRes{},
	server.WsBlockHashReq{},
	server.WsBlockHashRes{},
	server.WsBlockReq{},
	server.WsBlockFilterReq{},
	server.WsBlockFiltersBatchReq{},
	server.WsAccountUtxoReq{},
	server.WsBalanceHistoryReq{},
	server.WsTransactionReq{},
	server.WsTransactionSpecificReq{},
	server.WsEstimateFeeReq{},
	server.WsEstimateFeeRes{},
	server.WsLongTermFeeRateRes{},
	server.WsNewBlock{},
	server.WsSendTransactionReq{},
	server.WsSubscribeAddressesReq{},
	server.WsSubscribeFiatRatesReq{},
	server.WsCurrentFiatRatesReq{},
	server.WsFiatRatesForTimestampsReq{},
	server.WsFiatRatesTickersListReq{},
	server.WsMempoolFiltersReq{},
	server.WsRpcCallReq{},
	server.WsRpcCallRes{},
	bchain.MempoolTxidFilterEntries{},
}

func main() {
	out, err := generate(apiTypes, tsAliases)
	if err != nil {
		fmt.Fprintln(os.Stderr, "typescriptify:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, []byte(out), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "typescriptify:", err)
		os.Exit(1)
	}
	fmt.Println("OK")
}

func newConverter() *typescriptify.TypeScriptify {
	t := typescriptify.New()
	t.CreateInterface = true
	// The committed file has always been 4-space indented; anything else rewrites every line.
	t.Indent = "    "
	t.BackupDir = ""

	t.ManageType(api.Amount{}, typescriptify.TypeOptions{TSType: "string"})
	t.ManageType([]api.Amount{}, typescriptify.TypeOptions{TSType: "string[]"})
	t.ManageType([]*api.Amount{}, typescriptify.TypeOptions{TSType: "string[]"})
	t.ManageType(big.Int{}, typescriptify.TypeOptions{TSType: "number"})
	t.ManageType(time.Time{}, typescriptify.TypeOptions{TSType: "string", TSDoc: "Time in ISO 8601 YYYY-MM-DDTHH:mm:ss.sssZd"})
	return t
}

// generate returns the complete file contents for the given root types and aliases.
func generate(roots []interface{}, aliases []tsAlias) (string, error) {
	t := newConverter()
	for _, root := range roots {
		t.Add(root)
	}
	body, err := t.Convert(nil)
	if err != nil {
		return "", err
	}
	nullable, err := collectNullable(roots)
	if err != nil {
		return "", err
	}
	body, err = applyNullable(body, nullable, t.Indent)
	if err != nil {
		return "", err
	}
	prefix, err := renderAliases(aliases, body)
	if err != nil {
		return "", err
	}
	// A root type already emitted as a nested type leaves a stray blank line behind.
	body = regexp.MustCompile("\n{2,}").ReplaceAllString(body, "\n")
	return header + prefix + body + "\n", nil
}

// renderAliases emits the alias declarations and rejects any alias no field refers to,
// so a stale table cannot linger in the generated file unnoticed.
func renderAliases(aliases []tsAlias, body string) (string, error) {
	var b strings.Builder
	for _, a := range aliases {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(a.name) + `\b`).MatchString(body) {
			return "", fmt.Errorf("alias %s is not referenced by any ts_type tag", a.name)
		}
		fmt.Fprintf(&b, "export type %s = %s;\n", a.name, a.definition)
	}
	return b.String(), nil
}

// nullableField identifies one `ts_nullable` field by the interface and JSON name the library emits.
type nullableField struct {
	iface string
	json  string
}

// collectNullable walks the struct graph the same way the library does and gathers ts_nullable fields.
func collectNullable(roots []interface{}) ([]nullableField, error) {
	seen := map[reflect.Type]bool{}
	var out []nullableField
	var walk func(typ reflect.Type) error
	walk = func(typ reflect.Type) error {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return nil
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			jsonTag := f.Tag.Get("json")
			if f.Tag.Get("ts_nullable") == "true" {
				if f.Type.Kind() != reflect.Ptr || strings.Contains(jsonTag, ",omitempty") {
					return fmt.Errorf("%s.%s: ts_nullable requires a pointer field without omitempty", typ.Name(), f.Name)
				}
				out = append(out, nullableField{iface: typ.Name(), json: strings.Split(jsonTag, ",")[0]})
			}
			// The library does not descend into fields overridden by ts_type, so neither do we.
			if f.Tag.Get("ts_type") == "" {
				if err := walk(f.Type); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := walk(reflect.TypeOf(root)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// applyNullable rewrites `name?: T;` to `name: T | null;` inside the owning interface block.
func applyNullable(body string, fields []nullableField, indent string) (string, error) {
	lines := strings.Split(body, "\n")
	for _, nf := range fields {
		if err := markNullable(lines, nf, indent); err != nil {
			return "", err
		}
	}
	return strings.Join(lines, "\n"), nil
}

func markNullable(lines []string, nf nullableField, indent string) error {
	open := "export interface " + nf.iface + " {"
	prefix := indent + nf.json + "?: "
	inBlock := false
	for i, line := range lines {
		switch {
		case line == open:
			inBlock = true
		case inBlock && line == "}":
			return fmt.Errorf("%s.%s: no optional field line found to mark nullable", nf.iface, nf.json)
		case inBlock && strings.HasPrefix(line, prefix) && strings.HasSuffix(line, ";"):
			tsType := strings.TrimSuffix(strings.TrimPrefix(line, prefix), ";")
			lines[i] = indent + nf.json + ": " + tsType + " | null;"
			return nil
		}
	}
	return fmt.Errorf("%s: interface not found in generated output", nf.iface)
}
