package db

// #include "rocksdb/c.h"
import "C"
import "github.com/linxGnu/grocksdb"

/*
	possible additional tuning, using options not accessible by grocksdb

// #include "rocksdb/c.h"
import "C"

	cNativeOpts := C.rocksdb_options_create()
	opts := &grocksdb.Options{}
	cField := reflect.Indirect(reflect.ValueOf(opts)).FieldByName("c")
	cPtr := (**C.rocksdb_options_t)(unsafe.Pointer(cField.UnsafeAddr()))
	*cPtr = cNativeOpts

	cNativeBlockOpts := C.rocksdb_block_based_options_create()
	blockOpts := &grocksdb.BlockBasedTableOptions{}
	cBlockField := reflect.Indirect(reflect.ValueOf(blockOpts)).FieldByName("c")
	cBlockPtr := (**C.rocksdb_block_based_table_options_t)(unsafe.Pointer(cBlockField.UnsafeAddr()))
	*cBlockPtr = cNativeBlockOpts

	// https://github.com/facebook/rocksdb/wiki/Partitioned-Index-Filters
	blockOpts.SetIndexType(grocksdb.KTwoLevelIndexSearchIndexType)
	C.rocksdb_block_based_options_set_partition_filters(cNativeBlockOpts, boolToChar(true))
	C.rocksdb_block_based_options_set_metadata_block_size(cNativeBlockOpts, C.uint64_t(4096))
	C.rocksdb_block_based_options_set_cache_index_and_filter_blocks_with_high_priority(cNativeBlockOpts, boolToChar(true))
	blockOpts.SetPinL0FilterAndIndexBlocksInCache(true)

// boolToChar converts a bool value to C.uchar.
func boolToChar(b bool) C.uchar {
	if b {
		return 1
	}
	return 0
}
*/

func createAndSetDBOptions(bloomBits int, c *grocksdb.Cache, maxOpenFiles int) *grocksdb.Options {
	blockOpts := grocksdb.NewDefaultBlockBasedTableOptions()
	blockOpts.SetBlockSize(32 << 10) // 32kB
	blockOpts.SetBlockCache(c)
	if bloomBits > 0 {
		blockOpts.SetFilterPolicy(grocksdb.NewBloomFilter(float64(bloomBits)))
	}
	blockOpts.SetFormatVersion(4)

	opts := grocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(blockOpts)
	opts.SetCreateIfMissing(true)
	opts.SetCreateIfMissingColumnFamilies(true)
	opts.SetMaxBackgroundCompactions(6)
	opts.SetMaxBackgroundFlushes(6)
	opts.SetBytesPerSync(8 << 20)         // 8MB
	opts.SetWriteBufferSize(1 << 27)      // 128MB
	opts.SetMaxBytesForLevelBase(1 << 27) // 128MB
	opts.SetMaxOpenFiles(maxOpenFiles)
	opts.SetCompression(grocksdb.LZ4HCCompression)
	return opts
}
