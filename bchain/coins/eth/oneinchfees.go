package eth

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"

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

type oneInchFeeProvider struct {
	*alternativeFeeProvider
	params feeProviderParams
	apiKey string
}

// NewOneInchFeesProvider initializes https://api.1inch.dev provider
func NewOneInchFeesProvider(chain bchain.BlockChain, params string, metrics *common.Metrics) (alternativeFeeProviderInterface, error) {
	p := &oneInchFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{metrics: metrics, name: "1inch"}}
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds <= 0 {
		return nil, errors.New("NewOneInchFeesProvider: missing config parameters 'url' or 'periodSeconds'.")
	}
	p.apiKey = os.Getenv("ONE_INCH_API_KEY")
	if p.apiKey == "" {
		return nil, errors.New("NewOneInchFeesProvider: missing ONE_INCH_API_KEY env variable.")
	}
	p.chain = chain
	// Fees are fetched on demand: cached for periodSeconds, kept through
	// throttling bursts for the configured stale window (defaults to 10 minutes).
	p.ttl = time.Duration(p.params.PeriodSeconds) * time.Second
	p.staleSyncDuration = feeStaleDuration(p.params.PeriodSeconds, p.params.StaleSeconds)
	p.fetch = p.fetchFees
	go p.warmUp()
	return p, nil
}

func (p *oneInchFeeProvider) fetchFees() (*bchain.Eip1559Fees, error) {
	var data oneInchFeeFeesResult
	if err := p.getData(&data); err != nil {
		return nil, err
	}
	return oneInchFeesFromData(&data), nil
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

func oneInchFeesFromData(data *oneInchFeeFeesResult) *bchain.Eip1559Fees {
	fees := bchain.Eip1559Fees{}
	fees.BaseFeePerGas = bigIntFromString(data.BaseFee)
	// 1inch's tiers are tighter than Infura's. Remap so the suite's low/medium/high map
	// to comparable aggressiveness regardless of provider:
	//   1inch medium → suite low
	//   1inch high   → suite medium
	//   1inch instant → suite high
	// 1inch's low tier is discarded (too conservative).
	fees.Low = oneInchFeesFromResult(&data.Medium)
	fees.Medium = oneInchFeesFromResult(&data.High)
	fees.High = oneInchFeesFromResult(&data.Instant)
	return &fees
}

func (p *oneInchFeeProvider) getData(res interface{}) error {
	var httpData []byte
	httpReq, err := http.NewRequest("GET", p.params.URL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", " Bearer "+p.apiKey)
	httpRes, err := feeHTTPClient.Do(httpReq)
	if httpRes != nil {
		defer httpRes.Body.Close()
	}
	if err != nil {
		p.observeRequest("network_error")
		return err
	}
	if httpRes.StatusCode != http.StatusOK {
		p.observeRequest("http_" + strconv.Itoa(httpRes.StatusCode))
		return errors.New(p.params.URL + " returned status " + strconv.Itoa(httpRes.StatusCode))
	}
	if err := common.SafeDecodeResponseFromReader(httpRes.Body, res); err != nil {
		p.observeRequest("decode_error")
		return err
	}
	p.observeRequest("ok")
	return nil
}
