package btc

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// https://mempool.space/api/v1/fees/precise returns the recommended fees with sub-sat/vB precision.
// Example response:
// {"fastestFee":2.605,"halfHourFee":1.603,"hourFee":0.843,"economyFee":0.2,"minimumFee":0.1}

type mempoolSpacePreciseFeeResult struct {
	FastestFee  float64 `json:"fastestFee"`
	HalfHourFee float64 `json:"halfHourFee"`
	HourFee     float64 `json:"hourFee"`
	EconomyFee  float64 `json:"economyFee"`
	MinimumFee  float64 `json:"minimumFee"`
}

type mempoolSpacePreciseFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
}

type mempoolSpacePreciseFeeProvider struct {
	*alternativeFeeProvider
	params mempoolSpacePreciseFeeParams
}

// NewMempoolSpacePreciseFee initializes the provider completely.
func NewMempoolSpacePreciseFee(chain bchain.BlockChain, params string, metrics *common.Metrics) (alternativeFeeProviderInterface, error) {
	var paramsParsed mempoolSpacePreciseFeeParams
	err := json.Unmarshal([]byte(params), &paramsParsed)
	if err != nil {
		return nil, err
	}

	p, err := NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain(paramsParsed)
	if err != nil {
		return nil, err
	}

	p.chain = chain
	p.metrics = metrics
	p.name = "mempoolspaceprecise"
	go p.downloader()
	return p, nil
}

// NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain initializes the provider from already parsed parameters and without chain.
// Refactored like this for better testability.
func NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain(params mempoolSpacePreciseFeeParams) (*mempoolSpacePreciseFeeProvider, error) {
	if params.URL == "" {
		return nil, errors.New("NewMempoolSpacePreciseFee: Missing url")
	}
	// Negative period would make the timer fire immediately, busy-looping requests
	if params.PeriodSeconds <= 0 {
		return nil, errors.New("NewMempoolSpacePreciseFee: Missing periodSeconds")
	}

	return &mempoolSpacePreciseFeeProvider{
		alternativeFeeProvider: &alternativeFeeProvider{},
		params:                 params,
	}, nil
}

func (p *mempoolSpacePreciseFeeProvider) downloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data mempoolSpacePreciseFeeResult
		err := p.getData(&data)
		if err != nil {
			glog.Error("getData ", err)
		} else {
			if p.processData(&data) {
				if counter%60 == 0 {
					p.compareToDefault()
				}
				counter++
			}
		}
		<-timer.C
		timer.Reset(period)
	}
}

func (p *mempoolSpacePreciseFeeProvider) processData(data *mempoolSpacePreciseFeeResult) bool {
	// Block targets follow the mempool.space UI semantics, so Suite's targets 1/3/6/36
	// map to fastest/halfHour/hour/economy and users see the same numbers as on mempool.space
	fees := []alternativeFeeProviderFee{
		{blocks: 1, feePerKB: feePerVBToFeePerKB(data.FastestFee)},    // ~10 minutes
		{blocks: 3, feePerKB: feePerVBToFeePerKB(data.HalfHourFee)},   // ~30 minutes
		{blocks: 6, feePerKB: feePerVBToFeePerKB(data.HourFee)},       // ~1 hour
		{blocks: 500, feePerKB: feePerVBToFeePerKB(data.EconomyFee)},  // no priority
		{blocks: 1008, feePerKB: feePerVBToFeePerKB(data.MinimumFee)}, // purge floor
	}
	// Validate after conversion and keep the previous table on a malformed poll,
	// so the node fallback kicks in once the last good data goes stale
	for _, fee := range fees {
		if fee.feePerKB <= 0 {
			glog.Errorf("processData: invalid data %+v", data)
			return false
		}
	}
	p.mux.Lock()
	defer p.mux.Unlock()
	p.fees = fees
	p.lastSync = time.Now()
	return true
}

// feePerVBToFeePerKB converts exactly, without rounding to significant digits,
// so that the served values are identical to the ones on mempool.space.
// Returns 0 for values that are not sane positive fees (negative, NaN, Inf or huge),
// as their float to int conversion is implementation-dependent.
func feePerVBToFeePerKB(fee float64) int {
	feePerKB := math.Round(fee * 1000)
	if !(feePerKB >= 1 && feePerKB <= 1e9) {
		return 0
	}
	return int(feePerKB)
}

func (p *mempoolSpacePreciseFeeProvider) getData(res interface{}) error {
	httpReq, err := http.NewRequest("GET", p.params.URL, nil)
	if err != nil {
		return err
	}
	httpRes, err := http.DefaultClient.Do(httpReq)
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
