package common

import "github.com/schancel/cashaddr-converter/address"

type Output struct {
	CashAddr string `json:"cashaddr,omitempty"`
	Legacy   string `json:"legacy,omitempty"`
	Copay    string `json:"copay,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

func GetAllFormats(addr *address.Address) (*Output, error) {
	cashaddr, err := addr.CashAddress()
	if err != nil {
		return nil, err
	}
	cashaddrstr, err := cashaddr.Encode()
	if err != nil {
		return nil, err
	}

	old, err := addr.Legacy()
	if err != nil {
		return nil, err
	}
	oldstr, err := old.Encode()
	if err != nil {
		return nil, err
	}

	copay, err := addr.Copay()
	if err != nil {
		return nil, err
	}
	copaystr, err := copay.Encode()
	if err != nil {
		return nil, err
	}

	return &Output{
		CashAddr: cashaddrstr,
		Legacy:   oldstr,
		Copay:    copaystr,
		Hash:     addr.Hex(),
	}, nil
}
