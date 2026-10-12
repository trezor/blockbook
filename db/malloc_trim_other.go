//go:build !(linux && cgo)

package db

// trimCHeap is a no-op where glibc's malloc_trim is not available.
func trimCHeap() bool {
	return false
}
