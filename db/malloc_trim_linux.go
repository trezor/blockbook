//go:build linux && cgo

package db

// #include <malloc.h>
import "C"

// trimCHeap returns freed glibc heap pages to the kernel. RocksDB allocates record values and memtable
// blocks through glibc, and after a cache write-back they are free but still resident in arena heaps.
// It reports whether anything was released.
func trimCHeap() bool {
	return C.malloc_trim(0) != 0
}
