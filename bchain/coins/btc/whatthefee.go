package btc

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

// https://whatthefee.io returns
// {"index": [3, 6, 9, 12, 18, 24, 36, 48, 72, 96, 144],
// "columns": ["0.0500", "0.2000", "0.5000", "0.8000", "0.9500"],
// "data": [[60, 180, 280, 400, 440], [20, 120, 180, 380, 440],
// [0, 120, 160, 360, 420], [0, 80, 160, 300, 380], [0, 20, 120, 220, 360],
// [0, 20, 100, 180, 300], [0, 0, 80, 140, 240], [0, 0, 60, 100, 180],
// [0, 0, 40, 60, 140], [0, 0, 20, 20, 60], [0, 0, 0, 0, 20]]}

type whatTheFeeServiceResult struct {
	Index   []int    `json:"index"`
	Columns []string `json:"columns"`
	Data    [][]int  `json:"data"`
}

type whatTheFeeParams struct {
	URL           string `json:"url"`
	PeriodSeconds int    `periodSeconds:"url"`
}

type whatTheFeeProvider struct {
	*alternativeFeeProvider
	params        whatTheFeeParams
	probabilities []string
}

// NewWhatTheFee initializes https://whatthefee.io provider
func NewWhatTheFee(chain bchain.BlockChain, params string) (alternativeFeeProviderInterface, error) {
	var p whatTheFeeProvider
	err := json.Unmarshal([]byte(params), &p.params)
	if err != nil {
		return nil, err
	}
	if p.params.URL == "" || p.params.PeriodSeconds == 0 {
		return nil, errors.New("NewWhatTheFee: Missing parameters")
	}
	p.chain = chain
	go p.whatTheFeeDownloader()
	return &p, nil
}

func (p *whatTheFeeProvider) whatTheFeeDownloader() {
	period := time.Duration(p.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data whatTheFeeServiceResult
		err := p.whatTheFeeGetData(&data)
		if err != nil {
			glog.Error("whatTheFeeGetData ", err)
		} else {
			if p.whatTheFeeProcessData(&data) {
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

func (p *whatTheFeeProvider) whatTheFeeProcessData(data *whatTheFeeServiceResult) bool {
	if len(data.Index) == 0 || len(data.Index) != len(data.Data) || len(data.Columns) == 0 {
		glog.Errorf("invalid data %+v", data)
		return false
	}
	p.mux.Lock()
	defer p.mux.Unlock()
	p.probabilities = data.Columns
	p.fees = make([]alternativeFeeProviderFee, len(data.Index))
	for i, blocks := range data.Index {
		if len(data.Columns) != len(data.Data[i]) {
			glog.Errorf("invalid data %+v", data)
			return false
		}
		fees := make([]int, len(data.Columns))
		for j, l := range data.Data[i] {
			fees[j] = int(1000 * math.Exp(float64(l)/100))
		}
		p.fees[i] = alternativeFeeProviderFee{
			blocks:   blocks,
			feePerKB: fees[len(fees)/2],
		}
	}
	p.lastSync = time.Now()
	glog.Infof("whatTheFees: %+v", p.fees)
	return true
}

func (p *whatTheFeeProvider) whatTheFeeGetData(res interface{}) error {
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
	if httpRes.StatusCode != 200 {
		return errors.New("whatthefee.io returned status " + strconv.Itoa(httpRes.StatusCode))
	}
	return safeDecodeResponse(httpRes.Body, &res)
}
