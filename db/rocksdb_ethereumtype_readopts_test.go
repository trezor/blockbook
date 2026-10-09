//go:build unittest

package db

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/tests/dbtestdata"
)

// stripAddrContracts derives the expected partial decode from the source row
func stripAddrContracts(full *AddrContracts, opts AddrContractsReadOptions) *AddrContracts {
	rv := *full
	rv.Contracts = make([]AddrContract, len(full.Contracts))
	for i, c := range full.Contracts {
		s := AddrContract{Standard: c.Standard, Contract: c.Contract, Txs: c.Txs}
		if opts.Values {
			s.Value = c.Value
		}
		if opts.Holdings && (len(opts.Contract) == 0 || bytes.Equal(opts.Contract, c.Contract)) {
			s.Ids = c.Ids
			s.MultiTokenValues = c.MultiTokenValues
		}
		rv.Contracts[i] = s
	}
	return &rv
}

func Test_unpackAddrContractsOpt(t *testing.T) {
	parser := ethereumTestnetParser()
	contract47 := addressToAddrDesc(dbtestdata.EthAddrContract47, parser)
	contract4a := addressToAddrDesc(dbtestdata.EthAddrContract4a, parser)
	notAContract := addressToAddrDesc(dbtestdata.EthAddr7b, parser)
	row := AddrContracts{TotalTxs: 30, NonContractTxs: 20, InternalTxs: 10, Contracts: generateAddrContracts(2, 2, 3, 2, 3)}
	packed := packAddrContracts(&row)
	for _, tt := range []struct {
		name string
		opts AddrContractsReadOptions
	}{
		{"full", FullAddrContractsRead},
		{"none", AddrContractsReadOptions{}},
		{"valuesOnly", AddrContractsReadOptions{Values: true}},
		{"holdingsOnly", AddrContractsReadOptions{Holdings: true}},
		{"holdingsOf47", AddrContractsReadOptions{Holdings: true, Contract: contract47}},
		{"holdingsOf4a", AddrContractsReadOptions{Values: true, Holdings: true, Contract: contract4a}},
		{"holdingsOfAbsentContract", AddrContractsReadOptions{Holdings: true, Contract: notAContract}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unpackAddrContractsOpt(packed, nil, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if want := stripAddrContracts(&row, tt.opts); !reflect.DeepEqual(got, want) {
				t.Errorf("unpackAddrContractsOpt() = %+v, want %+v", got, want)
			}
		})
	}
}

// the API reads rows with reduced options; the skip path must not allocate for what it skips
func Benchmark_unpackAddrContractsOpt_Mixed(b *testing.B) {
	contract47 := addressToAddrDesc(dbtestdata.EthAddrContract47, ethereumTestnetParser())
	for _, bc := range []struct {
		name string
		opts AddrContractsReadOptions
	}{
		{"full", FullAddrContractsRead},
		{"noValues", AddrContractsReadOptions{Holdings: true}},
		{"noHoldings", AddrContractsReadOptions{Values: true}},
		{"contractsOnly", AddrContractsReadOptions{}},
		{"holdingsOf47", AddrContractsReadOptions{Holdings: true, Contract: contract47}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := unpackAddrContractsOpt(packedMixedContracts, nil, bc.opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
