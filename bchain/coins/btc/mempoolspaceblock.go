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

// https://mempool.space/api/v1/fees/mempool-blocks returns a list of upcoming blocks and their medianFee.
// Example response:
// [
// 	{
// 	"blockSize": 1604493,
// 	"blockVSize": 997944.75,
// 	"nTx": 3350,
// 	"totalFees": 8333539,
// 	"medianFee": 3.0073509137538332,
// 	"feeRange": [
// 		2.0444444444444443,
// 		2.2135922330097086,
// 		2.608695652173913,
// 		3.016042780748663,
// 		4.0048289738430585,
// 		9.27631139325092,
// 		201.06951871657753
// 	]
// 	},
// 	...
// ]

type mempoolSpaceBlockFeeResult struct {
	BlockSize  float64 `json:"blockSize"`
	BlockVSize float64 `json:"blockVSize"`
	NTx        int     `json:"nTx"`
	TotalFees  int     `json:"totalFees"`
	MedianFee  float64 `json:"medianFee"`
	// 2nd, 10th, 25th, 50th, 75th, 90th, 98th percentiles
	FeeRange []float64 `json:"feeRange"`
}

type mempoolSpaceBlockFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
	// Either number, then take the specified index. If null or missing, take the medianFee
	FeeRangeIndex    *int `json:"feeRangeIndex,omitempty"`
	FallbackFeePerKB int  `json:"fallbackFeePerKB,omitempty"`
}

type mempoolSpaceBlockFeeProvider struct {
	*alternativeFeeProvider
	params mempoolSpaceBlockFeeParams
}

// NewMempoolSpaceBlockFee initializes the provider completely.
func NewMempoolSpaceBlockFee(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	var paramsParsed mempoolSpaceBlockFeeParams
	err := json.Unmarshal([]byte(params), &paramsParsed)
	if err != nil {
		return nil, err
	}

	p, err := NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(paramsParsed)
	if err != nil {
		return nil, err
	}

	p.chain = chain
	go p.downloader()
	return p, nil
}

// NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain initializes the provider from already parsed parameters and without chain.
// Refactored like this for better testability.
func NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(params mempoolSpaceBlockFeeParams) (*mempoolSpaceBlockFeeProvider, error) {
	// Check mandatory parameters
	if params.URL == "" {
		return nil, errors.New("NewMempoolSpaceBlockFee: Missing url")
	}
	if params.PeriodSeconds == 0 {
		return nil, errors.New("NewMempoolSpaceBlockFee: Missing periodSeconds")
	}

	// Report on what is used
	if params.FeeRangeIndex == nil {
		glog.Info("NewMempoolSpaceBlockFee: Using median fee")
	} else {
		index := *params.FeeRangeIndex
		if index < 0 || index > 6 {
			return nil, errors.New("NewMempoolSpaceBlockFee: feeRangeIndex must be between 0 and 6")
		}
		glog.Infof("NewMempoolSpaceBlockFee: Using feeRangeIndex %d", index)
	}

	p := &mempoolSpaceBlockFeeProvider{
		alternativeFeeProvider: &alternativeFeeProvider{},
		params:                 params,
	}

	if params.FallbackFeePerKB > 0 {
		p.fallbackFeePerKBIfNotAvailable = params.FallbackFeePerKB
	}

	return p, nil
}

func (p *mempoolSpaceBlockFeeProvider) downloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data []mempoolSpaceBlockFeeResult
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

func (p *mempoolSpaceBlockFeeProvider) processData(data *[]mempoolSpaceBlockFeeResult) bool {
	if len(*data) == 0 {
		glog.Error("processData: empty data")
		return false
	}

	p.mux.Lock()
	defer p.mux.Unlock()

	p.fees = make([]alternativeFeeProviderFee, 0, len(*data))

	for i, block := range *data {
		var fee float64

		if p.params.FeeRangeIndex == nil {
			fee = block.MedianFee
		} else {
			feeRange := block.FeeRange
			index := *p.params.FeeRangeIndex
			if len(feeRange) > index {
				fee = feeRange[index]
			} else {
				glog.Warningf("Block %d has too short feeRange (len=%d, required=%d). Replacing by medianFee", i, len(feeRange), index)
				fee = block.MedianFee
			}
		}

		if fee <= 0 {
			glog.Warningf("Skipping block at index %d due to invalid fee: %f", i, fee)
			continue
		}

		// TODO: it might make sense to not include _every_ block, but only e.g. first 20 and then some hardcoded ones like 50, 100, 200, etc.
		// But even storing thousands of elements in []alternativeFeeProviderFee should not make a big performance overhead
		// Depends on Suite requirements

		// We want to convert the fee to 3 significant digits
		feeRounded := common.RoundToSignificantDigits(fee, 3)
		feePerKB := int(math.Round(feeRounded * 1000))

		p.fees = append(p.fees, alternativeFeeProviderFee{
			blocks:   i + 1,
			feePerKB: feePerKB,
		})
	}

	p.lastSync = time.Now()
	return true
}

func (p *mempoolSpaceBlockFeeProvider) getData(res interface{}) error {
	httpReq, err := http.NewRequest("GET", p.params.URL, nil)
	if err != nil {
		return err
	}
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
	return common.SafeDecodeResponseFromReader(httpRes.Body, res)
}
