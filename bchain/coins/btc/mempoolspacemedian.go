package btc

import (
	"encoding/json"
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
//   {
//     "blockSize": 1589235,
//     "blockVSize": 997914,
//     "nTx": 4224,
//     "totalFees": 6935988,
//     "medianFee": 3.622,
//     "feeRange": [ ... ]
//   },
//   ...
// ]

type mempoolSpaceMedianFeeResult struct {
	BlockSize  float64   `json:"blockSize"`
	BlockVSize float64   `json:"blockVSize"`
	NTx        int       `json:"nTx"`
	TotalFees  int       `json:"totalFees"`
	MedianFee  float64   `json:"medianFee"`
	FeeRange   []float64 `json:"feeRange"`
}

type mempoolSpaceMedianFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
}

type mempoolSpaceMedianFeeProvider struct {
	*alternativeFeeProvider
	params mempoolSpaceMedianFeeParams
}

// NewMempoolSpaceMedianFee initializes the median-fee provider using mempool.space data.
func NewMempoolSpaceMedianFee(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	p := &mempoolSpaceMedianFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds == 0 {
		return nil, errors.New("NewMempoolSpaceMedianFee: Missing parameters")
	}
	p.chain = chain
	go p.mempoolSpaceMedianFeeDownloader()
	return p, nil
}

func (p *mempoolSpaceMedianFeeProvider) mempoolSpaceMedianFeeDownloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data []mempoolSpaceMedianFeeResult
		err := p.mempoolSpaceMedianFeeGetData(&data)
		if err != nil {
			glog.Error("mempoolSpaceMedianFeeGetData ", err)
		} else {
			if p.mempoolSpaceMedianFeeProcessData(&data) {
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

func (p *mempoolSpaceMedianFeeProvider) mempoolSpaceMedianFeeProcessData(data *[]mempoolSpaceMedianFeeResult) bool {
	if len(*data) == 0 {
		glog.Error("mempoolSpaceMedianFeeProcessData: empty data")
		return false
	}

	p.mux.Lock()
	defer p.mux.Unlock()

	p.fees = make([]alternativeFeeProviderFee, 0, len(*data))

	for i, block := range *data {
		if block.MedianFee <= 0 {
			glog.Warningf("Skipping block at index %d due to invalid medianFee: %f", i, block.MedianFee)
			continue
		}

		// TODO: it might make sense to not include _every_ block, but only e.g. first 20 and then some hardcoded ones like 50, 100, 200, etc.
		// But even storing thousands of elements in []alternativeFeeProviderFee should not make a big performance overhead
		// Depends on Suite requirements

		p.fees = append(p.fees, alternativeFeeProviderFee{
			blocks:   i + 1,                       // simple mapping: index 0 -> 1 block, etc.
			feePerKB: int(block.MedianFee * 1000), // convert sat/vB to sat/KB
		})
	}

	p.lastSync = time.Now()
	return true
}

func (p *mempoolSpaceMedianFeeProvider) mempoolSpaceMedianFeeGetData(res interface{}) error {
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
