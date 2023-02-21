package db

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
)

// FiatRatesTimeFormat is a format string for storing FiatRates timestamps in rocksdb
const FiatRatesTimeFormat = "20060102150405" // YYYYMMDDhhmmss

var lastTickerInDB *common.CurrencyRatesTicker
var lastTickerInDBMux sync.Mutex

func packTimestamp(t *time.Time) []byte {
	return []byte(t.UTC().Format(FiatRatesTimeFormat))
}

func packFloat32(buf []byte, n float32) int {
	binary.BigEndian.PutUint32(buf, math.Float32bits(n))
	return 4
}

func unpackFloat32(buf []byte) (float32, int) {
	return math.Float32frombits(binary.BigEndian.Uint32(buf)), 4
}

func packCurrencyRatesTicker(ticker *common.CurrencyRatesTicker) []byte {
	buf := make([]byte, 0, 32)
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(len(ticker.Rates)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for c, v := range ticker.Rates {
		buf = append(buf, packString(c)...)
		l = packFloat32(varBuf, v)
		buf = append(buf, varBuf[:l]...)
	}
	l = packVaruint(uint(len(ticker.TokenRates)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for c, v := range ticker.TokenRates {
		buf = append(buf, packString(c)...)
		l = packFloat32(varBuf, v)
		buf = append(buf, varBuf[:l]...)
	}
	return buf
}

func unpackCurrencyRatesTicker(buf []byte) (*common.CurrencyRatesTicker, error) {
	var (
		ticker common.CurrencyRatesTicker
		s      string
		l      int
		len    uint
		v      float32
	)
	len, l = unpackVaruint(buf)
	buf = buf[l:]
	if len > 0 {
		ticker.Rates = make(map[string]float32, len)
		for i := 0; i < int(len); i++ {
			s, l = unpackString(buf)
			buf = buf[l:]
			v, l = unpackFloat32(buf)
			buf = buf[l:]
			ticker.Rates[s] = v
		}
	}
	len, l = unpackVaruint(buf)
	buf = buf[l:]
	if len > 0 {
		ticker.TokenRates = make(map[string]float32, len)
		for i := 0; i < int(len); i++ {
			s, l = unpackString(buf)
			buf = buf[l:]
			v, l = unpackFloat32(buf)
			buf = buf[l:]
			ticker.TokenRates[s] = v
		}
	}
	return &ticker, nil
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
func (d *RocksDB) FiatRatesStoreTicker(wb *grocksdb.WriteBatch, ticker *common.CurrencyRatesTicker) error {
	if len(ticker.Rates) == 0 {
		return errors.New("Error storing ticker: empty rates")
	}
	wb.PutCF(d.cfh[cfFiatRates], packTimestamp(&ticker.Timestamp), packCurrencyRatesTicker(ticker))
	return nil
}

func getTickerFromIterator(it *grocksdb.Iterator, vsCurrency string, token string) (*common.CurrencyRatesTicker, error) {
	timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
	if err != nil {
		return nil, err
	}
	ticker, err := unpackCurrencyRatesTicker(it.Value().Data())
	if err != nil {
		return nil, err
	}
	if !common.IsSuitableTicker(ticker, vsCurrency, token) {
		return nil, nil
	}
	ticker.Timestamp = timeObj.UTC()
	return ticker, nil
}

// FiatRatesGetTicker gets FiatRates ticker at the specified timestamp if it exist
func (d *RocksDB) FiatRatesGetTicker(tickerTime *time.Time) (*common.CurrencyRatesTicker, error) {
	tickerTimeFormatted := tickerTime.UTC().Format(FiatRatesTimeFormat)
	val, err := d.db.GetCF(d.ro, d.cfh[cfFiatRates], []byte(tickerTimeFormatted))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	if len(data) == 0 {
		return nil, nil
	}
	ticker, err := unpackCurrencyRatesTicker(data)
	if err != nil {
		return nil, err
	}
	ticker.Timestamp = tickerTime.UTC()
	return ticker, nil
}

// FiatRatesFindTicker gets FiatRates data closest to the specified timestamp, of the base currency, vsCurrency or the token if specified
func (d *RocksDB) FiatRatesFindTicker(tickerTime *time.Time, vsCurrency string, token string) (*common.CurrencyRatesTicker, error) {
	currentTicker := d.is.GetCurrentTicker("", "")
	lastTickerInDBMux.Lock()
	dbTicker := lastTickerInDB
	lastTickerInDBMux.Unlock()
	if currentTicker != nil {
		if !tickerTime.Before(currentTicker.Timestamp) || (dbTicker != nil && tickerTime.After(dbTicker.Timestamp)) {
			f := true
			if token != "" && currentTicker.TokenRates != nil {
				_, f = currentTicker.TokenRates[token]
			}
			if f {
				return currentTicker, nil
			}
		}
	}

	tickerTimeFormatted := tickerTime.UTC().Format(FiatRatesTimeFormat)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.Seek([]byte(tickerTimeFormatted)); it.Valid(); it.Next() {
		ticker, err := getTickerFromIterator(it, vsCurrency, token)
		if err != nil {
			glog.Error("FiatRatesFindTicker error: ", err)
			return nil, err
		}
		if ticker != nil {
			return ticker, nil
		}
	}
	return nil, nil
}

// FiatRatesFindLastTicker gets the last FiatRates record, of the base currency, vsCurrency or the token if specified
func (d *RocksDB) FiatRatesFindLastTicker(vsCurrency string, token string) (*common.CurrencyRatesTicker, error) {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.SeekToLast(); it.Valid(); it.Prev() {
		ticker, err := getTickerFromIterator(it, vsCurrency, token)
		if err != nil {
			glog.Error("FiatRatesFindLastTicker error: ", err)
			return nil, err
		}
		if ticker != nil {
			// if without filter, store the ticker for later use
			if vsCurrency == "" && token == "" {
				lastTickerInDBMux.Lock()
				lastTickerInDB = ticker
				lastTickerInDBMux.Unlock()
			}
			return ticker, nil
		}
	}
	return nil, nil
}
