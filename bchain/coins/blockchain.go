package coins

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/zec"
	"blockbook/common"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/juju/errors"
)

type blockChainFactory func(config json.RawMessage, pushHandler func(*bchain.MQMessage), metrics *common.Metrics) (bchain.BlockChain, error)

var blockChainFactories = make(map[string]blockChainFactory)

func init() {
	blockChainFactories["btc"] = btc.NewBitcoinRPC
	blockChainFactories["btc-testnet"] = btc.NewBitcoinRPC
	blockChainFactories["zec"] = zec.NewZCashRPC
}

// NewBlockChain creates bchain.BlockChain of type defined by parameter coin
func NewBlockChain(coin string, configfile string, pushHandler func(*bchain.MQMessage), metrics *common.Metrics) (bchain.BlockChain, error) {
	bcf, ok := blockChainFactories[coin]
	if !ok {
		return nil, errors.New(fmt.Sprint("Unsupported coin ", coin, ". Must be one of ", reflect.ValueOf(blockChainFactories).MapKeys()))
	}
	data, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, errors.Annotatef(err, "Error reading file %v", configfile)
	}
	var config json.RawMessage
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, errors.Annotatef(err, "Error parsing file %v", configfile)
	}
	bc, err := bcf(config, pushHandler, metrics)
	if err != nil {
		return nil, err
	}
	bc.Initialize(bchain.NewMempool(bc, metrics))
	return bc, nil
}
