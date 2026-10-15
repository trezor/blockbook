//go:build unittest

package db

import (
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

// StoreAddrContractsForTest writes one cfAddressContracts row as given, bypassing the connect path,
// so that api tests can serve rows the indexer itself would never produce.
func (d *RocksDB) StoreAddrContractsForTest(addrDesc bchain.AddressDescriptor, acs *AddrContracts) error {
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.storeAddressContracts(wb, map[string]*AddrContracts{string(addrDesc): acs}); err != nil {
		return err
	}
	return d.WriteBatch(wb)
}
