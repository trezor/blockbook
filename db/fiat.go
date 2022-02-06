package db

import (
	"encoding/json"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// FiatRatesTimeFormat is a format string for storing FiatRates timestamps in rocksdb
const FiatRatesTimeFormat = "20060102150405" // YYYYMMDDhhmmss

// CurrencyRatesTicker contains coin ticker data fetched from API
type CurrencyRatesTicker struct {
	Timestamp *time.Time // return as unix timestamp in API
	Rates     map[string]float64
}

// ResultTickerAsString contains formatted CurrencyRatesTicker data
type ResultTickerAsString struct {
	Timestamp int64              `json:"ts,omitempty"`
	Rates     map[string]float64 `json:"rates"`
	Error     string             `json:"error,omitempty"`
}

// ResultTickersAsString contains a formatted CurrencyRatesTicker list
type ResultTickersAsString struct {
	Tickers []ResultTickerAsString `json:"tickers"`
}

// ResultTickerListAsString contains formatted data about available currency tickers
type ResultTickerListAsString struct {
	Timestamp int64    `json:"ts,omitempty"`
	Tickers   []string `json:"available_currencies"`
	Error     string   `json:"error,omitempty"`
}

// FiatRatesConvertDate checks if the date is in correct format and returns the Time object.
// Possible formats are: YYYYMMDDhhmmss, YYYYMMDDhhmm, YYYYMMDDhh, YYYYMMDD
func FiatRatesConvertDate(date string) (*time.Time, error) {
	for format := FiatRatesTimeFormat; len(format) >= 8; format = format[:len(format)-2] {
		convertedDate, err := time.Parse(format, date)
		if err == nil {
			return &convertedDate, nil
		}
	}
	msg := "Date \"" + date + "\" does not match any of available formats. "
	msg += "Possible formats are: YYYYMMDDhhmmss, YYYYMMDDhhmm, YYYYMMDDhh, YYYYMMDD"
	return nil, errors.New(msg)
}

// FiatRatesStoreTicker stores ticker data at the specified time
func (d *RocksDB) FiatRatesStoreTicker(ticker *CurrencyRatesTicker) error {
	if len(ticker.Rates) == 0 {
		return errors.New("Error storing ticker: empty rates")
	} else if ticker.Timestamp == nil {
		return errors.New("Error storing ticker: empty timestamp")
	}
	ratesMarshalled, err := json.Marshal(ticker.Rates)
	if err != nil {
		glog.Error("Error marshalling ticker rates: ", err)
		return err
	}
	timeFormatted := ticker.Timestamp.UTC().Format(FiatRatesTimeFormat)
	err = d.db.PutCF(d.wo, d.cfh[cfFiatRates], []byte(timeFormatted), ratesMarshalled)
	if err != nil {
		glog.Error("Error storing ticker: ", err)
		return err
	}
	return nil
}

// FiatRatesFindTicker gets FiatRates data closest to the specified timestamp
func (d *RocksDB) FiatRatesFindTicker(tickerTime *time.Time) (*CurrencyRatesTicker, error) {
	ticker := &CurrencyRatesTicker{}
	tickerTimeFormatted := tickerTime.UTC().Format(FiatRatesTimeFormat)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.Seek([]byte(tickerTimeFormatted)); it.Valid(); it.Next() {
		timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
		if err != nil {
			glog.Error("FiatRatesFindTicker time parse error: ", err)
			return nil, err
		}
		timeObj = timeObj.UTC()
		ticker.Timestamp = &timeObj
		err = json.Unmarshal(it.Value().Data(), &ticker.Rates)
		if err != nil {
			glog.Error("FiatRatesFindTicker error unpacking rates: ", err)
			return nil, err
		}
		break
	}
	if err := it.Err(); err != nil {
		glog.Error("FiatRatesFindTicker Iterator error: ", err)
		return nil, err
	}
	if !it.Valid() {
		return nil, nil // ticker not found
	}
	return ticker, nil
}

// FiatRatesFindLastTicker gets the last FiatRates record
func (d *RocksDB) FiatRatesFindLastTicker() (*CurrencyRatesTicker, error) {
	ticker := &CurrencyRatesTicker{}
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.SeekToLast(); it.Valid(); it.Next() {
		timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
		if err != nil {
			glog.Error("FiatRatesFindTicker time parse error: ", err)
			return nil, err
		}
		timeObj = timeObj.UTC()
		ticker.Timestamp = &timeObj
		err = json.Unmarshal(it.Value().Data(), &ticker.Rates)
		if err != nil {
			glog.Error("FiatRatesFindTicker error unpacking rates: ", err)
			return nil, err
		}
		break
	}
	if err := it.Err(); err != nil {
		glog.Error("FiatRatesFindLastTicker Iterator error: ", err)
		return ticker, err
	}
	if !it.Valid() {
		return nil, nil // ticker not found
	}
	return ticker, nil
}
