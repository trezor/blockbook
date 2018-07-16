package vlq

import (
	"bytes"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("VLQ", func() {

	tests := []struct {
		x uint64
		b []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7F}},
		{128, []byte{0x81, 0x00}},
		{137, []byte{0x81, 0x09}},
		{8192, []byte{0xC0, 0x00}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x81, 0x80, 0x00}},
		{2097151, []byte{0xFF, 0xFF, 0x7F}},
		{2097152, []byte{0x81, 0x80, 0x80, 0x00}},
		{134217728, []byte{0xC0, 0x80, 0x80, 0x00}},
		{268435455, []byte{0xFF, 0xFF, 0xFF, 0x7F}},
		{0xFFFFFFFFFFFFFFFF, []byte{0x81, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F}},
	}

	It("should encode", func() {
		buf := make([]byte, MaxLen64)
		for _, test := range tests {
			v := buf[:PutUint(buf, test.x)]
			Expect(fmt.Sprintf("%x", v)).To(Equal(fmt.Sprintf("%x", test.b)), "for %d (%x)", test.x, test.x)
		}
	})

	It("should decode", func() {
		for _, test := range tests {
			v, n := Uint(test.b)
			Expect(v).To(Equal(test.x), "for %d", test.x)
			Expect(n).To(Equal(len(test.b)), "for %d", test.x)
		}
	})

	It("should decode from streams", func() {
		for _, test := range tests {
			v, err := ReadUint(bytes.NewReader(test.b))
			Expect(err).NotTo(HaveOccurred(), "for %d", test.x)
			Expect(v).To(Equal(test.x), "for %d", test.x)
		}
	})

	It("should not decode empty slices", func() {
		v, n := Uint([]byte{})
		Expect(v).To(Equal(uint64(0)))
		Expect(n).To(Equal(0))
	})

	It("should handle overflow", func() {
		v, n := Uint([]byte{0x81, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x80})
		Expect(v).To(Equal(uint64(0)))
		Expect(n).To(Equal(-10))
	})

})

// --------------------------------------------------------------------

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vlq")
}
