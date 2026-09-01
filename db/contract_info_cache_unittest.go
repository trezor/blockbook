//go:build unittest

package db

// ResetContractInfoCache drops the package-level ContractInfo cache. A blockbook process
// holds one RocksDB, but tests - including those in other packages - build several, and every
// instance reads through this one cache.
// TODO: move the cache onto the RocksDB struct so multi-DB tests need no reset.
func ResetContractInfoCache() {
	cachedContracts.reset()
}
