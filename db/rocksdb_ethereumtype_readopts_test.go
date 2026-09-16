//go:build unittest

package db

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/trezor/blockbook/tests/dbtestdata"
)

// stripAddrContracts derives the expected partial decode from a fully decoded row
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
	contract0d := addressToAddrDesc(dbtestdata.EthAddrContract0d, parser)
	contract47 := addressToAddrDesc(dbtestdata.EthAddrContract47, parser)
	contract4a := addressToAddrDesc(dbtestdata.EthAddrContract4a, parser)
	notAContract := addressToAddrDesc(dbtestdata.EthAddr7b, parser)
	rows := []struct {
		name string
		data AddrContracts
	}{
		{"mixed", AddrContracts{TotalTxs: 30, NonContractTxs: 20, InternalTxs: 10, Contracts: generateAddrContracts(2, 2, 3, 2, 3)}},
		{"fungibleOnly", AddrContracts{TotalTxs: 3, NonContractTxs: 3, Contracts: generateAddrContracts(3, 0, 0, 0, 0)}},
		{"holdingsOnly", AddrContracts{TotalTxs: 5, Contracts: generateAddrContracts(0, 1, 4, 1, 4)}},
	}
	optSets := []struct {
		name string
		opts AddrContractsReadOptions
	}{
		{"full", fullAddrContractsRead},
		{"none", AddrContractsReadOptions{}},
		{"valuesOnly", AddrContractsReadOptions{Values: true}},
		{"holdingsOnly", AddrContractsReadOptions{Holdings: true}},
		{"holdingsOf47", AddrContractsReadOptions{Holdings: true, Contract: contract47}},
		{"holdingsOf4a", AddrContractsReadOptions{Values: true, Holdings: true, Contract: contract4a}},
		{"holdingsOfFungible0d", AddrContractsReadOptions{Holdings: true, Contract: contract0d}},
		{"holdingsOfAbsentContract", AddrContractsReadOptions{Holdings: true, Contract: notAContract}},
	}
	for _, row := range rows {
		packed := packAddrContracts(&row.data)
		full, err := unpackAddrContracts(packed, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(full, &row.data) {
			t.Fatalf("%s: full decode differs from source", row.name)
		}
		for _, os := range optSets {
			t.Run(row.name+"/"+os.name, func(t *testing.T) {
				got, err := unpackAddrContractsOpt(packed, nil, os.opts)
				if err != nil {
					t.Fatal(err)
				}
				want := stripAddrContracts(full, os.opts)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("unpackAddrContractsOpt() = %+v, want %+v", got, want)
				}
			})
		}
	}
}

func Test_GetAddrDescContractsOpt_UnknownAddress(t *testing.T) {
	parser := ethereumTestnetParser()
	d := setupRocksDB(t, parser)
	defer closeAndDestroyRocksDB(t, d)
	got, err := d.GetAddrDescContractsOpt(addressToAddrDesc(dbtestdata.EthAddr7b, parser), AddrContractsReadOptions{})
	if err != nil || got != nil {
		t.Errorf("GetAddrDescContractsOpt() = %v, %v, want nil, nil", got, err)
	}
}

// the API reads rows with reduced options; the skip path must not allocate for what it skips
func Benchmark_unpackAddrContractsOpt_Mixed(b *testing.B) {
	contract47 := addressToAddrDesc(dbtestdata.EthAddrContract47, ethereumTestnetParser())
	for _, bc := range []struct {
		name string
		opts AddrContractsReadOptions
	}{
		{"full", fullAddrContractsRead},
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
