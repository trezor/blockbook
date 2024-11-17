package eth

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// https://api.1inch.dev/gas-price/v1.5/1 returns
// {
// 	"baseFee": "12456587953",
// 	"low": {
// 	  "maxPriorityFeePerGas": "1000000",
// 	  "maxFeePerGas": "14948905543"
// 	},
// 	"medium": {
// 	  "maxPriorityFeePerGas": "2000000",
// 	  "maxFeePerGas": "14949905543"
// 	},
// 	"high": {
// 	  "maxPriorityFeePerGas": "5000000",
// 	  "maxFeePerGas": "14952905543"
// 	},
// 	"instant": {
// 	  "maxPriorityFeePerGas": "10000000",
// 	  "maxFeePerGas": "29905811086"
// 	}
// }

type oneInchFeeFeeResult struct {
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
}

type oneInchFeeFeesResult struct {
	BaseFee string              `json:"baseFee"`
	Low     oneInchFeeFeeResult `json:"low"`
	Medium  oneInchFeeFeeResult `json:"medium"`
	High    oneInchFeeFeeResult `json:"high"`
	Instant oneInchFeeFeeResult `json:"instant"`
}

type oneInchFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
}

type oneInchFeeProvider struct {
	*alternativeFeeProvider
	params oneInchFeeParams
	apiKey string
}

// NewOneInchFeesProvider initializes https://api.1inch.dev provider
func NewOneInchFeesProvider(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	p := &oneInchFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds == 0 {
		return nil, errors.New("NewOneInchFeesProvider: missing config parameters 'url' or 'periodSeconds'.")
	}
	p.apiKey = os.Getenv("ONE_INCH_API_KEY")
	if p.apiKey == "" {
		return nil, errors.New("NewOneInchFeesProvider: missing ONE_INCH_API_KEY env variable.")
	}
	p.chain = chain
	go p.FeeDownloader()
	return p, nil
}

func (p *oneInchFeeProvider) FeeDownloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	for {
		var data oneInchFeeFeesResult
		err := p.getData(&data)
		if err != nil {
			glog.Error("oneInchFeeProvider.FeeDownloader", err)
		} else {
			p.processData(&data)
		}
		<-timer.C
		timer.Reset(period)
	}
}

func bigIntFromString(s string) *big.Int {
	b := big.NewInt(0)
	b, _ = b.SetString(s, 10)
	return b
}

func oneInchFeesFromResult(result *oneInchFeeFeeResult) *bchain.Eip1559Fee {
	fee := bchain.Eip1559Fee{}
	fee.MaxFeePerGas = bigIntFromString(result.MaxFeePerGas)
	fee.MaxPriorityFeePerGas = bigIntFromString(result.MaxPriorityFeePerGas)
	return &fee
}

func (p *oneInchFeeProvider) processData(data *oneInchFeeFeesResult) bool {
	fees := bchain.Eip1559Fees{}
	fees.BaseFeePerGas = bigIntFromString(data.BaseFee)
	fees.Instant = oneInchFeesFromResult(&data.Instant)
	fees.High = oneInchFeesFromResult(&data.High)
	fees.Medium = oneInchFeesFromResult(&data.Medium)
	fees.Low = oneInchFeesFromResult(&data.Low)
	p.mux.Lock()
	defer p.mux.Unlock()
	p.lastSync = time.Now()
	p.eip1559Fees = &fees
	return true
}

func (p *oneInchFeeProvider) getData(res interface{}) error {
	var httpData []byte
	httpReq, err := http.NewRequest("GET", p.params.URL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", " Bearer "+p.apiKey)
	httpRes, err := http.DefaultClient.Do(httpReq)
	if httpRes != nil {
		defer httpRes.Body.Close()
	}
	if err != nil {
		return err
	}
	if httpRes.StatusCode != http.StatusOK {
		return errors.New(p.params.URL + " returned status " + strconv.Itoa(httpRes.StatusCode))
	}
	return common.SafeDecodeResponseFromReader(httpRes.Body, &res)
}
