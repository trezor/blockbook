//go:build unittest

package db

// ResetContractInfoCache drops the package-level ContractInfo cache. A blockbook process
// holds one RocksDB, but tests - including those in other packages - build several, and every
// instance reads through this one cache.
func ResetContractInfoCache() {
	cachedContracts.reset()
}
