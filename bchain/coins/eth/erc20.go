package eth

import (
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/juju/errors"
)

var erc20abi = `[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x06fdde03"},
{"constant":true,"inputs":[],"name":"symbol","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x95d89b41"},
{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"payable":false,"type":"function","signature":"0x313ce567"},
{"constant":true,"inputs":[],"name":"totalSupply","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function","signature":"0x18160ddd"},
{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"payable":false,"type":"function","signature":"0x70a08231"},
{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0xa9059cbb"},
{"constant":false,"inputs":[{"name":"_from","type":"address"},{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0x23b872dd"},
{"constant":false,"inputs":[{"name":"_spender","type":"address"},{"name":"_value","type":"uint256"}],"name":"approve","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0x095ea7b3"},
{"constant":true,"inputs":[{"name":"_owner","type":"address"},{"name":"_spender","type":"address"}],"name":"allowance","outputs":[{"name":"remaining","type":"uint256"}],"payable":false,"type":"function","signature":"0xdd62ed3e"},
{"anonymous":false,"inputs":[{"indexed":true,"name":"_from","type":"address"},{"indexed":true,"name":"_to","type":"address"},{"indexed":false,"name":"_value","type":"uint256"}],"name":"Transfer","type":"event","signature":"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"},
{"anonymous":false,"inputs":[{"indexed":true,"name":"_owner","type":"address"},{"indexed":true,"name":"_spender","type":"address"},{"indexed":false,"name":"_value","type":"uint256"}],"name":"Approval","type":"event","signature":"0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"},
{"inputs":[{"name":"_initialAmount","type":"uint256"},{"name":"_tokenName","type":"string"},{"name":"_decimalUnits","type":"uint8"},{"name":"_tokenSymbol","type":"string"}],"payable":false,"type":"constructor"},
{"constant":false,"inputs":[{"name":"_spender","type":"address"},{"name":"_value","type":"uint256"},{"name":"_extraData","type":"bytes"}],"name":"approveAndCall","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0xcae9ca51"},
{"constant":true,"inputs":[],"name":"version","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x54fd4d50"}]`

// doing the parsing/processing without using go-ethereum/accounts/abi library, it is simple to get data from Transfer event
const erc20EventTransferSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

type erc20Transfer struct {
	Contract ethcommon.Address
	From     ethcommon.Address
	To       ethcommon.Address
	Tokens   big.Int
}

func addressFromPaddedHex(s string) (*ethcommon.Address, error) {
	var t big.Int
	_, ok := t.SetString(s, 0)
	if !ok {
		return nil, errors.New("Data is not a number")
	}
	a := ethcommon.BigToAddress(&t)
	return &a, nil
}

func erc20GetTransfersFromLog(logs []*rpcLog) ([]erc20Transfer, error) {
	var r []erc20Transfer
	for _, l := range logs {
		if len(l.Topics) == 3 && l.Topics[0] == erc20EventTransferSignature {
			var t big.Int
			_, ok := t.SetString(l.Data, 0)
			if !ok {
				return nil, errors.New("Data is not a number")
			}
			from, err := addressFromPaddedHex(l.Topics[1])
			if err != nil {
				return nil, err
			}
			to, err := addressFromPaddedHex(l.Topics[2])
			if err != nil {
				return nil, err
			}
			r = append(r, erc20Transfer{
				Contract: l.Address,
				From:     *from,
				To:       *to,
				Tokens:   t,
			})
		}
	}
	return r, nil
}
