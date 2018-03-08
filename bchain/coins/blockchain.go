package coins

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/zec"
	"fmt"
	"reflect"
	"time"

	"github.com/juju/errors"
)

type blockChainFactory func(url string, user string, password string, timeout time.Duration, parse bool) (bchain.BlockChain, error)

var blockChainFactories = make(map[string]blockChainFactory)

func init() {
	blockChainFactories["btc"] = btc.NewBitcoinRPC
	blockChainFactories["zec"] = zec.NewZCashRPC
}

// NewBlockChain creates bchain.BlockChain of type defined by parameter coin
func NewBlockChain(coin string, url string, user string, password string, timeout time.Duration, parse bool) (bchain.BlockChain, error) {
	bcf, ok := blockChainFactories[coin]
	if !ok {
		return nil, errors.New(fmt.Sprint("Unsupported coin ", coin, ". Must be one of ", reflect.ValueOf(blockChainFactories).MapKeys()))
	}
	return bcf(url, user, password, timeout, parse)
}
