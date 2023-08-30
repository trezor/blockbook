package bchain

import (
	"bytes"
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
	FilterScriptsTaprootNoOrdinals
)

// GolombFilter is computing golomb filter of address descriptors
type GolombFilter struct {
	Enabled           bool
	UseZeroedKey      bool
	p                 uint8
	key               string
	filterScripts     string
	filterScriptsType FilterScriptsType
	filterData        [][]byte
	uniqueData        map[string]struct{}
	// All the unique txids that contain ordinal data
	ordinalTxIds map[string]struct{}
	// Mapping of txid to address descriptors - only used in case of taproot-noordinals
	allAddressDescriptors map[string][]AddressDescriptor
}

// NewGolombFilter initializes the GolombFilter handler
func NewGolombFilter(p uint8, filterScripts string, key string, useZeroedKey bool) (*GolombFilter, error) {
	if p == 0 {
		return &GolombFilter{Enabled: false}, nil
	}
	gf := GolombFilter{
		Enabled:           true,
		UseZeroedKey:      useZeroedKey,
		p:                 p,
		key:               key,
		filterScripts:     filterScripts,
		filterScriptsType: filterScriptsToScriptsType(filterScripts),
		filterData:        make([][]byte, 0),
		uniqueData:        make(map[string]struct{}),
	}
	// reject invalid filterScripts
	if gf.filterScriptsType == FilterScriptsInvalid {
		return nil, errors.Errorf("Invalid/unsupported filterScripts parameter %s", filterScripts)
	}
	// set ordinal-related fields if needed
	if gf.ignoreOrdinals() {
		gf.ordinalTxIds = make(map[string]struct{})
		gf.allAddressDescriptors = make(map[string][]AddressDescriptor)
	}
	return &gf, nil
}

// Gets the M parameter that we are using for the filter
// Currently it relies on P parameter, but that can change
func GetGolombParamM(p uint8) uint64 {
	return uint64(1 << uint64(p))
}

// Checks whether this input contains ordinal data
func isInputOrdinal(vin Vin) bool {
	byte_pattern := []byte{
		0x00, // OP_0, OP_FALSE
		0x63, // OP_IF
		0x03, // OP_PUSHBYTES_3
		0x6f, // "o"
		0x72, // "r"
		0x64, // "d"
		0x01, // OP_PUSHBYTES_1
	}
	// Witness needs to have at least 3 items and the second one needs to contain certain pattern
	return len(vin.Witness) > 2 && bytes.Contains(vin.Witness[1], byte_pattern)
}

// Whether a transaction contains any ordinal data
func txContainsOrdinal(tx *Tx) bool {
	for _, vin := range tx.Vin {
		if isInputOrdinal(vin) {
			return true
		}
	}
	return false
}

// Saving all the ordinal-related txIds so we can later ignore their address descriptors
func (f *GolombFilter) markTxAndParentsAsOrdinals(tx *Tx) {
	f.ordinalTxIds[tx.Txid] = struct{}{}
	for _, vin := range tx.Vin {
		f.ordinalTxIds[vin.Txid] = struct{}{}
	}
}

// Adding a new address descriptor mapped to a txid
func (f *GolombFilter) addTxIdMapping(ad AddressDescriptor, tx *Tx) {
	f.allAddressDescriptors[tx.Txid] = append(f.allAddressDescriptors[tx.Txid], ad)
}

// AddAddrDesc adds taproot address descriptor to the data for the filter
func (f *GolombFilter) AddAddrDesc(ad AddressDescriptor, tx *Tx) {
	if f.ignoreNonTaproot() && !ad.IsTaproot() {
		return
	}
	if f.ignoreOrdinals() && tx != nil && txContainsOrdinal(tx) {
		f.markTxAndParentsAsOrdinals(tx)
		return
	}
	if len(ad) == 0 {
		return
	}
	// When ignoring ordinals, we need to save all the address descriptors before
	// filtering out the "invalid" ones.
	if f.ignoreOrdinals() && tx != nil {
		f.addTxIdMapping(ad, tx)
		return
	}
	f.includeAddrDesc(ad)
}

// Private function to be called with descriptors that were already validated
func (f *GolombFilter) includeAddrDesc(ad AddressDescriptor) {
	s := string(ad)
	if _, found := f.uniqueData[s]; !found {
		f.filterData = append(f.filterData, ad)
		f.uniqueData[s] = struct{}{}
	}
}

// Including all the address descriptors from non-ordinal transactions
func (f *GolombFilter) includeAllAddressDescriptorsOrdinals() {
	for txid, ads := range f.allAddressDescriptors {
		// Ignoring the txids that contain ordinal data
		if _, found := f.ordinalTxIds[txid]; found {
			continue
		}
		for _, ad := range ads {
			f.includeAddrDesc(ad)
		}
	}
}

// Compute computes golomb filter from the data
func (f *GolombFilter) Compute() []byte {
	m := GetGolombParamM(f.p)

	// In case of ignoring the ordinals, we still need to assemble the filter data
	if f.ignoreOrdinals() {
		f.includeAllAddressDescriptorsOrdinals()
	}

	if len(f.filterData) == 0 {
		return nil
	}

	// Used key is possibly just zeroes, otherwise get it from the supplied key
	var key [gcs.KeySize]byte
	if f.UseZeroedKey {
		key = [gcs.KeySize]byte{}
	} else {
		b, _ := hex.DecodeString(f.key)
		if len(b) < gcs.KeySize {
			return nil
		}
		copy(key[:], b[:gcs.KeySize])
	}

	filter, err := gcs.BuildGCSFilter(f.p, m, key, f.filterData)
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

func (f *GolombFilter) ignoreNonTaproot() bool {
	switch f.filterScriptsType {
	case FilterScriptsTaproot, FilterScriptsTaprootNoOrdinals:
		return true
	}
	return false
}

func (f *GolombFilter) ignoreOrdinals() bool {
	switch f.filterScriptsType {
	case FilterScriptsTaprootNoOrdinals:
		return true
	}
	return false
}

func filterScriptsToScriptsType(filterScripts string) FilterScriptsType {
	switch filterScripts {
	case "":
		return FilterScriptsAll
	case "taproot":
		return FilterScriptsTaproot
	case "taproot-noordinals":
		return FilterScriptsTaprootNoOrdinals
	}
	return FilterScriptsInvalid
}
