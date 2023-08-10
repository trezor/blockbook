package db

import (
	"encoding/hex"

	"github.com/golang/glog"
	"github.com/martinboehm/btcutil/gcs"
	"github.com/trezor/blockbook/bchain"
)

func computeBlockFilter(allAddrDesc [][]byte, blockHash string, taprootOnly bool) string {
	// TODO: take these from config - how to access it? From BitcoinRPC?
	// TODO: these things should probably be an argument to this function,
	// so it is better testable
	golombFilterP := uint8(20)
	golombFilterM := uint64(1 << golombFilterP)

	// TODO: code below is almost a copy-paste from computeGolombFilter,
	// it might be possible to refactor it into a common function, e.g.
	// computeGolomb(allAddrDescriptors, P, M, taprootOnly, hashIdentifier) -> filterData
	// but where to put it?

	uniqueScripts := make(map[string]struct{})
	filterData := make([][]byte, 0)

	handleAddrDesc := func(ad bchain.AddressDescriptor) {
		if taprootOnly && !ad.IsTaproot() {
			return
		}
		if len(ad) == 0 {
			return
		}
		s := string(ad)
		if _, found := uniqueScripts[s]; !found {
			filterData = append(filterData, ad)
			uniqueScripts[s] = struct{}{}
		}
	}

	for _, ad := range allAddrDesc {
		handleAddrDesc(ad)
	}

	if len(filterData) == 0 {
		return ""
	}

	b, _ := hex.DecodeString(blockHash)
	if len(b) < gcs.KeySize {
		return ""
	}

	filter, err := gcs.BuildGCSFilter(golombFilterP, golombFilterM, *(*[gcs.KeySize]byte)(b[:gcs.KeySize]), filterData)
	if err != nil {
		glog.Error("Cannot create golomb filter for ", blockHash, ", ", err)
		return ""
	}

	fb, err := filter.NBytes()
	if err != nil {
		glog.Error("Error getting NBytes from golomb filter for ", blockHash, ", ", err)
		return ""
	}

	// TODO: maybe not returning string but []byte, when we are saving it
	// as []byte anyway?
	return hex.EncodeToString(fb)
}
