package address

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/schancel/cashaddr-converter/cashaddress"
	"github.com/schancel/cashaddr-converter/legacy"
)

type AddressType byte

const (
	P2KH AddressType = 0
	P2SH AddressType = 1
)

type NetworkType string

const (
	RegTest NetworkType = "bchreg"
	TestNet NetworkType = "bchtest"
	MainNet NetworkType = "bitcoincash"
)

// NewFromString takes a address string in Legacy or CashAddress format and
// returns an `*Address` or an error.
func NewFromString(addr string) (*Address, error) {
	legaddr, err := legacy.Decode(addr)
	if err == nil {
		addr, err := NewFromLegacy(legaddr)
		if err != nil {
			return nil, err
		}
		return addr, nil
	}

	cashaddr, err := cashaddress.Decode(addr, cashaddress.MainNet)
	if err == nil {
		addr, err := NewFromCashAddress(cashaddr)
		if err != nil {
			return nil, err
		}
		return addr, nil
	}

	return nil, errors.New("unable to decode address")
}

// NewFromCashAddress takes a cashaddress.Address and returns a generic
// `Address` struct.
func NewFromCashAddress(addr *cashaddress.Address) (*Address, error) {
	var network = MainNet
	var addrtype = P2SH

	switch addr.Prefix {
	case cashaddress.MainNet:
		network = MainNet
	case cashaddress.TestNet:
		network = TestNet
	case cashaddress.RegTest:
		network = RegTest
	default:
		return nil, errors.New("invalid address network")
	}

	switch addr.Version {
	case cashaddress.P2KH:
		addrtype = P2KH
	case cashaddress.P2SH:
		addrtype = P2SH
	default:
		return nil, errors.New("invalid address type")
	}

	return &Address{
		Network: network,
		Version: addrtype,
		Payload: addr.Payload,
	}, nil
}

// NewFromLegacy takes a cashaddress.Address and returns a generic
// `Address` struct.
func NewFromLegacy(addr *legacy.Address) (*Address, error) {
	var addrtype AddressType
	var network NetworkType = MainNet

	switch addr.Version {
	case legacy.P2KH:
		addrtype = P2KH
	case legacy.P2SH:
		addrtype = P2SH
	case legacy.P2KHCopay:
		addrtype = P2KH
	case legacy.P2SHCopay:
		addrtype = P2SH
	case legacy.P2KHTestnet:
		addrtype = P2KH
		network = TestNet
	case legacy.P2SHTestnet:
		addrtype = P2SH
		network = TestNet
	default:
		return nil, fmt.Errorf("unknown address type: %d", addr.Version)
	}

	return &Address{
		Network: network,
		Version: addrtype,
		Payload: addr.Payload,
	}, nil
}

// Address is a generic representation of a Bitcoin Cash address for
// converting to various string representations.
type Address struct {
	Network NetworkType
	Version AddressType
	Payload []byte
}

// Verify checks to make sure the network and address type are valid.
func (r *Address) Verify() error {
	switch r.Version {
	case P2KH:
	case P2SH:
	default:
		return errors.New("invalid address type")
	}

	switch r.Network {
	case RegTest:
	case TestNet:
	case MainNet:
	default:
		return errors.New("invalid network")
	}

	return nil
}

// Hex returns a string representation of the Address payload in hexadecimal.
func (addr *Address) Hex() string {
	var format = "0x%s"
	hexStr := strings.ToUpper(big.NewInt(0).SetBytes(addr.Payload).Text(16))

	// Prepend a zero
	if len(hexStr)%2 == 1 {
		format = "0x0%s"
	}

	return fmt.Sprintf(format, hexStr)
}

// CashAddress converts various address fields to create a
// `*cashaddress.Address`
func (addr *Address) CashAddress() (*cashaddress.Address, error) {
	var network = cashaddress.MainNet
	var addrtype = cashaddress.P2SH

	switch addr.Network {
	case MainNet:
		network = cashaddress.MainNet
	case TestNet:
		network = cashaddress.TestNet
	case RegTest:
		network = cashaddress.RegTest
	default:
		return nil, errors.New("invalid address network")
	}

	switch addr.Version {
	case P2KH:
		addrtype = cashaddress.P2KH
	case P2SH:
		addrtype = cashaddress.P2SH
	default:
		return nil, errors.New("invalid address type")
	}
	return &cashaddress.Address{
		Version: addrtype,
		Prefix:  network,
		Payload: addr.Payload,
	}, nil
}

// Legacy returns a legacy address struct with the appropriate fields set to
// encode to the correct legacy address version
func (addr *Address) Legacy() (*legacy.Address, error) {
	var versionByte uint8 = 0
	switch {
	case addr.Version == P2KH:
		versionByte = legacy.P2KH
		if addr.Network == TestNet {
			versionByte = legacy.P2KHTestnet
		}
	case addr.Version == P2SH:
		versionByte = legacy.P2SH
		if addr.Network == TestNet {
			versionByte = legacy.P2SHTestnet
		}
	default:
		return nil, errors.New("invalid address type")
	}

	return &legacy.Address{
		Version: versionByte,
		Payload: addr.Payload,
	}, nil
}

// Copay returns a legacy address struct with the appropriate fields set to
// encode to the correct copay address version
func (addr *Address) Copay() (*legacy.Address, error) {
	var versionByte uint8 = 0
	switch {
	case addr.Version == P2KH:
		versionByte = legacy.P2KHCopay
		if addr.Network == TestNet {
			versionByte = legacy.P2KHTestnet
		}
	case addr.Version == P2SH:
		versionByte = legacy.P2SHCopay
		if addr.Network == TestNet {
			versionByte = legacy.P2SHTestnet
		}
	default:
		return nil, errors.New("invalid address type")
	}

	return &legacy.Address{
		Version: versionByte,
		Payload: addr.Payload,
	}, nil
}
