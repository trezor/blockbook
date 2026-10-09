// Command typescriptify regenerates blockbook-api.ts from the Go API structs.
//
//	make typescriptify                  # inside the build image, no local RocksDB needed
//	go run ./build/tools/typescriptify  # from the repository root
//
// On top of the library's ts_type/ts_doc tags, two conventions keep the output reproducible:
//   - Structs implementing bchain.ChainExtraPayloadWrapper are emitted as discriminated unions
//     built from bchain.ChainExtraPayloads, so `payloadType === 'tron'` narrows the payload in
//     TypeScript (the library can neither emit named aliases nor unions itself).
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

var wrapperIface = reflect.TypeOf((*bchain.ChainExtraPayloadWrapper)(nil)).Elem()

// apiTypes are the roots of the generated file; nested structs are discovered by the library.
// The payload structs behind the unions are added from bchain.ChainExtraPayloads.
var apiTypes = []interface{}{
	// API - REST and Websocket
	api.APIError{},
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
	out, err := generate(apiTypes, bchain.ChainExtraPayloads)
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

// generate returns the complete file contents for the given root types and payload registry.
func generate(roots []interface{}, payloads []bchain.ChainExtraPayload) (string, error) {
	t := newConverter()
	nullable, wrappers, err := scanFields(roots)
	if err != nil {
		return "", err
	}
	aliases, err := renderUnions(t, wrappers, payloads)
	if err != nil {
		return "", err
	}
	for _, p := range payloads {
		t.Add(p.Tx)
		t.Add(p.Account)
	}
	for _, root := range roots {
		t.Add(root)
	}
	body, err := t.Convert(nil)
	if err != nil {
		return "", err
	}
	body, err = applyNullable(body, nullable, t.Indent)
	if err != nil {
		return "", err
	}
	for _, w := range wrappers {
		if strings.Contains(body, "export interface "+w.Name()+" {") {
			return "", fmt.Errorf("union %s was also emitted as an interface; the alias would be shadowed", w.Name())
		}
	}
	// A root type already emitted as a nested type leaves a stray blank line behind.
	body = regexp.MustCompile("\n{2,}").ReplaceAllString(body, "\n")
	return header + aliases + body + "\n", nil
}

// renderUnions emits one closed union per wrapper struct, a member per registered payload, and
// tells the library to type every field of the wrapper type with the alias name. Wrappers are
// registered by value because the library dereferences pointer fields before matching.
func renderUnions(t *typescriptify.TypeScriptify, wrappers []reflect.Type, payloads []bchain.ChainExtraPayload) (string, error) {
	if len(wrappers) > 0 && len(payloads) == 0 {
		return "", fmt.Errorf("no chainExtraData payloads registered")
	}
	var b strings.Builder
	for _, w := range wrappers {
		name := w.Name()
		pick := reflect.Zero(w).Interface().(bchain.ChainExtraPayloadWrapper)
		members := make([]string, 0, len(payloads))
		for _, p := range payloads {
			payload := pick.ChainExtraPayload(p)
			if p.Type == bchain.ChainExtraPayloadTypeUnknown || payload == nil || reflect.TypeOf(payload).Kind() != reflect.Struct {
				return "", fmt.Errorf("%s: payload type %q must map to a struct", name, p.Type)
			}
			members = append(members, fmt.Sprintf("{ payloadType: '%s'; payload?: %s }", p.Type, reflect.TypeOf(payload).Name()))
		}
		t.ManageType(reflect.Zero(w).Interface(), typescriptify.TypeOptions{TSType: name})
		fmt.Fprintf(&b, "export type %s = %s;\n", name, strings.Join(members, " | "))
	}
	return b.String(), nil
}

// nullableField identifies one `ts_nullable` field by the interface and JSON name the library emits.
type nullableField struct {
	iface string
	json  string
}

// scanFields walks the struct graph the same way the library does and gathers the ts_nullable
// fields and, in first-seen order, the wrapper structs that must be emitted as unions.
func scanFields(roots []interface{}) ([]nullableField, []reflect.Type, error) {
	seen := map[reflect.Type]bool{}
	var nullable []nullableField
	var wrappers []reflect.Type
	var walk func(typ reflect.Type) error
	walk = func(typ reflect.Type) error {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return nil
		}
		seen[typ] = true
		if typ.Implements(wrapperIface) {
			// The wrapper is replaced by its union, so its own fields are never emitted.
			wrappers = append(wrappers, typ)
			return nil
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			jsonTag := f.Tag.Get("json")
			if f.Tag.Get("ts_nullable") == "true" {
				if f.Type.Kind() != reflect.Ptr || strings.Contains(jsonTag, ",omitempty") {
					return fmt.Errorf("%s.%s: ts_nullable requires a pointer field without omitempty", typ.Name(), f.Name)
				}
				nullable = append(nullable, nullableField{iface: typ.Name(), json: strings.Split(jsonTag, ",")[0]})
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
			return nil, nil, err
		}
	}
	return nullable, wrappers, nil
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
