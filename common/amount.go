// Same as api.Amount, made available to all sub-packages without circular dependencies
// TODO: make use of common.Amount in api package and remove duplication

package common

import (
	"fmt"
	"math/big"
	"strings"
)

// Amount is a datatype holding amounts
type Amount big.Int

// IsZeroBigInt checks if big int has zero value
func IsZeroBigInt(b *big.Int) bool {
	return len(b.Bits()) == 0
}

// Compare returns an integer comparing two Amounts. The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
// Nil Amount is always less then non-nil amount, two nil Amounts are equal
func (a *Amount) Compare(b *Amount) int {
	if b == nil {
		if a == nil {
			return 0
		}
		return 1
	}
	if a == nil {
		return -1
	}
	return (*big.Int)(a).Cmp((*big.Int)(b))
}

// MarshalJSON Amount serialization
func (a *Amount) MarshalJSON() (out []byte, err error) {
	if a == nil {
		return []byte(`"0"`), nil
	}
	return []byte(`"` + (*big.Int)(a).String() + `"`), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	if len(s) > 0 {
		bigValue, parsed := new(big.Int).SetString(s, 10)
		if !parsed {
			return fmt.Errorf("couldn't parse number: %s", s)
		}
		*a = Amount(*bigValue)
	} else {
		// assuming empty string means zero
		*a = Amount{}
	}
	return nil
}

func (a *Amount) String() string {
	if a == nil {
		return ""
	}
	return (*big.Int)(a).String()
}

// AsBigInt returns big.Int type for the Amount (empty if Amount is nil)
func (a *Amount) AsBigInt() big.Int {
	if a == nil {
		return *new(big.Int)
	}
	return big.Int(*a)
}

// AsInt64 returns Amount as int64 (0 if Amount is nil).
// It is used only for legacy interfaces (socket.io)
// and generally not recommended to use for possible loss of precision.
func (a *Amount) AsInt64() int64 {
	if a == nil {
		return 0
	}
	return (*big.Int)(a).Int64()
}

// AsUint64 returns Amount as uint64 (0 if Amount is nil).
func (a *Amount) AsUint64() uint64 {
	if a == nil {
		return 0
	}
	return (*big.Int)(a).Uint64()
}
