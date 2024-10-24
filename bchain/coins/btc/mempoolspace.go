package btc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// https://mempool.space/api/v1/fees/recommended returns
// {"fastestFee":41,"halfHourFee":39,"hourFee":36,"economyFee":36,"minimumFee":20}

type mempoolSpaceFeeResult struct {
	FastestFee  int `json:"fastestFee"`
	HalfHourFee int `json:"halfHourFee"`
	HourFee     int `json:"hourFee"`
	EconomyFee  int `json:"economyFee"`
	MinimumFee  int `json:"minimumFee"`
}

type mempoolSpaceFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `json:"periodSeconds"`
}

type mempoolSpaceFeeProvider struct {
	*alternativeFeeProvider
	params mempoolSpaceFeeParams
}

// NewMempoolSpaceFee initializes https://mempool.space provider
func NewMempoolSpaceFee(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	p := &mempoolSpaceFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds == 0 {
		return nil, errors.New("NewMempoolSpaceFee: Missing parameters")
	}
	p.chain = chain
	go p.mempoolSpaceFeeDownloader()
	return p, nil
}

func (p *mempoolSpaceFeeProvider) mempoolSpaceFeeDownloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data mempoolSpaceFeeResult
		err := p.mempoolSpaceFeeGetData(&data)
		if err != nil {
			glog.Error("mempoolSpaceFeeGetData ", err)
		} else {
			if p.mempoolSpaceFeeProcessData(&data) {
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

func (p *mempoolSpaceFeeProvider) mempoolSpaceFeeProcessData(data *mempoolSpaceFeeResult) bool {
	if data.MinimumFee == 0 || data.EconomyFee == 0 || data.HourFee == 0 || data.HalfHourFee == 0 || data.FastestFee == 0 {
		glog.Errorf("mempoolSpaceFeeProcessData: invalid data %+v", data)
		return false
	}
	p.mux.Lock()
	defer p.mux.Unlock()
	p.fees = make([]alternativeFeeProviderFee, 5)
	// map mempoool.space fees to blocks

	// FastestFee is for 1 block
	p.fees[0] = alternativeFeeProviderFee{
		blocks:   1,
		feePerKB: data.FastestFee * 1000,
	}

	// HalfHourFee is for 2-6 blocks
	p.fees[1] = alternativeFeeProviderFee{
		blocks:   6,
		feePerKB: data.HalfHourFee * 1000,
	}

	// HourFee is for 7-36 blocks
	p.fees[2] = alternativeFeeProviderFee{
		blocks:   36,
		feePerKB: data.HourFee * 1000,
	}

	// EconomyFee is for 37-200 blocks
	p.fees[3] = alternativeFeeProviderFee{
		blocks:   500,
		feePerKB: data.EconomyFee * 1000,
	}

	// MinimumFee is for over 500 blocks
	p.fees[4] = alternativeFeeProviderFee{
		blocks:   1000,
		feePerKB: data.MinimumFee * 1000,
	}

	p.lastSync = time.Now()
	// glog.Infof("mempoolSpaceFees: %+v", p.fees)
	return true
}

func (p *mempoolSpaceFeeProvider) mempoolSpaceFeeGetData(res interface{}) error {
	var httpData []byte
	httpReq, err := http.NewRequest("GET", p.params.URL, bytes.NewBuffer(httpData))
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
	return common.SafeDecodeResponseFromReader(httpRes.Body, &res)
}
