package btc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
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

type whatTheFeeFee struct {
	blocks    int
	feesPerKB []int
}

type whatTheFeeData struct {
	params        whatTheFeeParams
	probabilities []string
	fees          []whatTheFeeFee
	lastSync      time.Time
	chain         bchain.BlockChain
	mux           sync.Mutex
}

var whatTheFee whatTheFeeData

// InitWhatTheFee initializes https://whatthefee.io handler
func InitWhatTheFee(chain bchain.BlockChain, params string) error {
	err := json.Unmarshal([]byte(params), &whatTheFee.params)
	if err != nil {
		return err
	}
	if whatTheFee.params.URL == "" || whatTheFee.params.PeriodSeconds == 0 {
		return errors.New("Missing parameters")
	}
	whatTheFee.chain = chain
	go whatTheFeeDownloader()
	return nil
}

func whatTheFeeDownloader() {
	period := time.Duration(whatTheFee.params.PeriodSeconds) * time.Second
	timer := time.NewTimer(period)
	counter := 0
	for {
		var data whatTheFeeServiceResult
		err := whatTheFeeGetData(&data)
		if err != nil {
			glog.Error("whatTheFeeGetData ", err)
		} else {
			if whatTheFeeProcessData(&data) {
				if counter%60 == 0 {
					whatTheFeeCompareToDefault()
				}
				counter++
			}
		}
		<-timer.C
		timer.Reset(period)
	}
}

func whatTheFeeProcessData(data *whatTheFeeServiceResult) bool {
	if len(data.Index) == 0 || len(data.Index) != len(data.Data) || len(data.Columns) == 0 {
		glog.Errorf("invalid data %+v", data)
		return false
	}
	whatTheFee.mux.Lock()
	defer whatTheFee.mux.Unlock()
	whatTheFee.probabilities = data.Columns
	whatTheFee.fees = make([]whatTheFeeFee, len(data.Index))
	for i, blocks := range data.Index {
		if len(data.Columns) != len(data.Data[i]) {
			glog.Errorf("invalid data %+v", data)
			return false
		}
		fees := make([]int, len(data.Columns))
		for j, l := range data.Data[i] {
			fees[j] = int(1000 * math.Exp(float64(l)/100))
		}
		whatTheFee.fees[i] = whatTheFeeFee{
			blocks:    blocks,
			feesPerKB: fees,
		}
	}
	whatTheFee.lastSync = time.Now()
	glog.Infof("%+v", whatTheFee.fees)
	return true
}

func whatTheFeeGetData(res interface{}) error {
	var httpData []byte
	httpReq, err := http.NewRequest("GET", whatTheFee.params.URL, bytes.NewBuffer(httpData))
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

func whatTheFeeCompareToDefault() {
	output := ""
	for _, fee := range whatTheFee.fees {
		output += fmt.Sprint(fee.blocks, ",")
		for _, wtf := range fee.feesPerKB {
			output += fmt.Sprint(wtf, ",")
		}
		conservative, err := whatTheFee.chain.EstimateSmartFee(fee.blocks, true)
		if err != nil {
			glog.Error(err)
			return
		}
		economical, err := whatTheFee.chain.EstimateSmartFee(fee.blocks, false)
		if err != nil {
			glog.Error(err)
			return
		}
		output += fmt.Sprint(conservative.String(), ",", economical.String(), "\n")
	}
	glog.Info("whatTheFeeCompareToDefault\n", output)
}
