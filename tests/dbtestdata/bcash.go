package dbtestdata

import (
	_ "embed"
	"encoding/json"

	"github.com/trezor/blockbook/bchain"
)

type BcashOutputFixture struct {
	Descriptor string             `json:"descriptor"`
	Address    string             `json:"address"`
	Value      string             `json:"value"`
	Token      *bchain.BcashToken `json:"tokenData"`
}

type BcashTxFixture struct {
	Name    string               `json:"name"`
	Chain   string               `json:"chain"`
	Txid    string               `json:"txid"`
	Hex     string               `json:"hex"`
	Inputs  []BcashOutputFixture `json:"inputs,omitempty"`
	Outputs []BcashOutputFixture `json:"outputs"`
}

//go:embed bcash.json
var bcashFixtures []byte

func GetBcashFixtures() ([]BcashTxFixture, error) {
	var fixtures []BcashTxFixture
	err := json.Unmarshal(bcashFixtures, &fixtures)
	return fixtures, err
}
