package eth

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// https://gas.api.infura.io/v3/${api_key}/networks/1/suggestedGasFees returns
// {
//   "low": {
//     "suggestedMaxPriorityFeePerGas": "0.01128",
//     "suggestedMaxFeePerGas": "9.919888552",
//     "minWaitTimeEstimate": 15000,
//     "maxWaitTimeEstimate": 60000
//   },
//   "medium": {
//     "suggestedMaxPriorityFeePerGas": "1.148315423",
//     "suggestedMaxFeePerGas": "15.317625653",
//     "minWaitTimeEstimate": 15000,
//     "maxWaitTimeEstimate": 45000
//   },
//   "high": {
//     "suggestedMaxPriorityFeePerGas": "2",
//     "suggestedMaxFeePerGas": "24.78979967",
//     "minWaitTimeEstimate": 15000,
//     "maxWaitTimeEstimate": 30000
//   },
//   "estimatedBaseFee": "9.908608552",
//   "networkCongestion": 0.004,
//   "latestPriorityFeeRange": [
//     "0.05",
//     "4"
//   ],
//   "historicalPriorityFeeRange": [
//     "0.006381976",
//     "155.777346207"
//   ],
//   "historicalBaseFeeRange": [
//     "9.243163495",
//     "16.734915363"
//   ],
//   "priorityFeeTrend": "up",
//   "baseFeeTrend": "up",
//   "version": "0.0.1"
// }

type infuraFeeResult struct {
	MaxPriorityFeePerGas string `json:"suggestedMaxPriorityFeePerGas"`
	MaxFeePerGas         string `json:"suggestedMaxFeePerGas"`
	MinWaitTimeEstimate  int    `json:"minWaitTimeEstimate"`
	MaxWaitTimeEstimate  int    `json:"maxWaitTimeEstimate"`
}

type infuraFeesResult struct {
	BaseFee                    string          `json:"estimatedBaseFee"`
	Low                        infuraFeeResult `json:"low"`
	Medium                     infuraFeeResult `json:"medium"`
	High                       infuraFeeResult `json:"high"`
	NetworkCongestion          float64         `json:"networkCongestion"`
	LatestPriorityFeeRange     []string        `json:"latestPriorityFeeRange"`
	HistoricalPriorityFeeRange []string        `json:"historicalPriorityFeeRange"`
	HistoricalBaseFeeRange     []string        `json:"historicalBaseFeeRange"`
	PriorityFeeTrend           string          `json:"priorityFeeTrend"`
	BaseFeeTrend               string          `json:"baseFeeTrend"`
}

type infuraFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
}

type infuraFeeProvider struct {
	*alternativeFeeProvider
	params infuraFeeParams
	apiKey string
}

// NewInfuraFeesProvider initializes https://gas.api.infura.io provider
func NewInfuraFeesProvider(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	p := &infuraFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds == 0 {
		return nil, errors.New("NewInfuraFeesProvider: missing config parameters 'url' or 'periodSeconds'.")
	}
	p.apiKey = os.Getenv("INFURA_API_KEY")
	if p.apiKey == "" {
		return nil, errors.New("NewInfuraFeesProvider: missing INFURA_API_KEY env variable.")
	}
	p.params.URL = strings.Replace(p.params.URL, "${api_key}", p.apiKey, -1)
	p.chain = chain
	// if the data are not successfully downloaded 10 times, stop providing data
	p.staleSyncDuration = time.Duration(p.params.PeriodSeconds*10) * time.Second
	go p.FeeDownloader()
	return p, nil
}

func (p *infuraFeeProvider) FeeDownloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	for {
		var data infuraFeesResult
		err := p.getData(&data)
		if err != nil {
			glog.Error("infuraFeeProvider.FeeDownloader ", err)
		} else {
			p.processData(&data)
		}
		<-timer.C
		timer.Reset(period)
	}
}

func bigIntFromFloatString(s string) *big.Int {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return big.NewInt(int64(f * 1e9))
}

func infuraFeesFromResult(result *infuraFeeResult) *bchain.Eip1559Fee {
	fee := bchain.Eip1559Fee{}
	fee.MaxFeePerGas = bigIntFromFloatString(result.MaxFeePerGas)
	fee.MaxPriorityFeePerGas = bigIntFromFloatString(result.MaxPriorityFeePerGas)
	fee.MinWaitTimeEstimate = result.MinWaitTimeEstimate
	fee.MaxWaitTimeEstimate = result.MaxWaitTimeEstimate
	return &fee
}

func rangeFromString(feeRange []string) []*big.Int {
	if feeRange == nil {
		return nil
	}
	result := make([]*big.Int, len(feeRange))
	for i := range feeRange {
		result[i] = bigIntFromFloatString(feeRange[i])
	}
	return result
}

func (p *infuraFeeProvider) processData(data *infuraFeesResult) bool {
	fees := bchain.Eip1559Fees{}
	fees.BaseFeePerGas = bigIntFromFloatString(data.BaseFee)
	fees.High = infuraFeesFromResult(&data.High)
	fees.Medium = infuraFeesFromResult(&data.Medium)
	fees.Low = infuraFeesFromResult(&data.Low)
	fees.NetworkCongestion = data.NetworkCongestion
	fees.LatestPriorityFeeRange = rangeFromString(data.LatestPriorityFeeRange)
	fees.HistoricalPriorityFeeRange = rangeFromString(data.HistoricalPriorityFeeRange)
	fees.HistoricalBaseFeeRange = rangeFromString(data.HistoricalBaseFeeRange)
	fees.PriorityFeeTrend = data.PriorityFeeTrend
	fees.BaseFeeTrend = data.BaseFeeTrend
	p.mux.Lock()
	defer p.mux.Unlock()
	p.lastSync = time.Now()
	p.eip1559Fees = &fees
	return true
}

func (p *infuraFeeProvider) getData(res interface{}) error {
	var httpData []byte
	httpReq, err := http.NewRequest("GET", p.params.URL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
