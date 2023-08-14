package bchain

import (
	"encoding/hex"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/martinboehm/btcutil/gcs"
)

type FilterScriptsType int

const (
	FilterScriptsInvalid = FilterScriptsType(iota)
	FilterScriptsAll
	FilterScriptsTaproot
)

// GolombFilter is computing golomb filter of address descriptors
type GolombFilter struct {
	Enabled           bool
	p                 uint8
	key               string
	filterScripts     string
	filterScriptsType FilterScriptsType
	filterData        [][]byte
	uniqueData        map[string]struct{}
}

// NewGolombFilter initializes the GolombFilter handler
func NewGolombFilter(p uint8, filterScripts string, key string) (*GolombFilter, error) {
	if p == 0 {
		return &GolombFilter{Enabled: false}, nil
	}
	gf := GolombFilter{
		Enabled:           true,
		p:                 p,
		key:               key,
		filterScripts:     filterScripts,
		filterScriptsType: filterScriptsToScriptsType(filterScripts),
		filterData:        make([][]byte, 0),
		uniqueData:        make(map[string]struct{}),
	}
	// only taproot and all is supported
	if gf.filterScriptsType == FilterScriptsInvalid {
		return nil, errors.Errorf("Invalid/unsupported filterScripts parameter %s", filterScripts)
	}
	return &gf, nil
}

// AddAddrDesc adds taproot address descriptor to the data for the filter
func (f *GolombFilter) AddAddrDesc(ad AddressDescriptor) {
	if f.filterScriptsType == FilterScriptsTaproot && !ad.IsTaproot() {
		return
	}
	if len(ad) == 0 {
		return
	}
	s := string(ad)
	if _, found := f.uniqueData[s]; !found {
		f.filterData = append(f.filterData, ad)
		f.uniqueData[s] = struct{}{}
	}
}

// Compute computes golomb filter from the data
func (f *GolombFilter) Compute() []byte {
	m := uint64(1 << uint64(f.p))

	if len(f.filterData) == 0 {
		return nil
	}

	b, _ := hex.DecodeString(f.key)
	if len(b) < gcs.KeySize {
		return nil
	}

	filter, err := gcs.BuildGCSFilter(f.p, m, *(*[gcs.KeySize]byte)(b[:gcs.KeySize]), f.filterData)
	if err != nil {
		glog.Error("Cannot create golomb filter for ", f.key, ", ", err)
		return nil
	}

	fb, err := filter.NBytes()
	if err != nil {
		glog.Error("Error getting NBytes from golomb filter for ", f.key, ", ", err)
		return nil
	}

	return fb
}

func filterScriptsToScriptsType(filterScripts string) FilterScriptsType {
	switch filterScripts {
	case "":
		return FilterScriptsAll
	case "taproot":
		return FilterScriptsTaproot
	}
	return FilterScriptsInvalid
}
