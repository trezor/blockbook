package db

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/flier/gorocksdb"
	"github.com/golang/glog"
	"github.com/juju/errors"
)

// FiatRatesTimeFormat is a format string for storing FiatRates timestamps in rocksdb
const FiatRatesTimeFormat = "20060102150405" // YYYYMMDDhhmmss

var tickersMux sync.Mutex
var lastTickerInDB *CurrencyRatesTicker
var currentTicker *CurrencyRatesTicker

// CurrencyRatesTicker contains coin ticker data fetched from API
type CurrencyRatesTicker struct {
	Timestamp  time.Time          // return as unix timestamp in API
	Rates      map[string]float32 // rates of the base currency against a list of vs currencies
	TokenRates map[string]float32 // rates of the tokens (identified by the address of the contract) against the base currency
}

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

func packCurrencyRatesTicker(ticker *CurrencyRatesTicker) []byte {
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

func unpackCurrencyRatesTicker(buf []byte) (*CurrencyRatesTicker, error) {
	var (
		ticker CurrencyRatesTicker
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
func (d *RocksDB) FiatRatesStoreTicker(wb *gorocksdb.WriteBatch, ticker *CurrencyRatesTicker) error {
	if len(ticker.Rates) == 0 {
		return errors.New("Error storing ticker: empty rates")
	}
	wb.PutCF(d.cfh[cfFiatRates], packTimestamp(&ticker.Timestamp), packCurrencyRatesTicker(ticker))
	return nil
}

func getTickerFromIterator(it *gorocksdb.Iterator, vsCurrency string, token string) (*CurrencyRatesTicker, error) {
	timeObj, err := time.Parse(FiatRatesTimeFormat, string(it.Key().Data()))
	if err != nil {
		return nil, err
	}
	ticker, err := unpackCurrencyRatesTicker(it.Value().Data())
	if err != nil {
		return nil, err
	}
	if vsCurrency != "" {
		if ticker.Rates == nil {
			return nil, nil
		}
		if _, found := ticker.Rates[vsCurrency]; !found {
			return nil, nil
		}
	}
	if token != "" {
		if ticker.TokenRates == nil {
			return nil, nil
		}
		if _, found := ticker.TokenRates[token]; !found {
			return nil, nil
		}
	}
	ticker.Timestamp = timeObj.UTC()
	return ticker, nil
}

// FiatRatesGetTicker gets FiatRates ticker at the specified timestamp if it exist
func (d *RocksDB) FiatRatesGetTicker(tickerTime *time.Time) (*CurrencyRatesTicker, error) {
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
func (d *RocksDB) FiatRatesFindTicker(tickerTime *time.Time, vsCurrency string, token string) (*CurrencyRatesTicker, error) {
	tickersMux.Lock()
	if currentTicker != nil && lastTickerInDB != nil {
		if tickerTime.After(lastTickerInDB.Timestamp) {
			f := true
			if token != "" && currentTicker.TokenRates != nil {
				_, f = currentTicker.TokenRates[token]
			}
			if f {
				tickersMux.Unlock()
				return currentTicker, nil
			}
		}
	}
	tickersMux.Unlock()

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
func (d *RocksDB) FiatRatesFindLastTicker(vsCurrency string, token string) (*CurrencyRatesTicker, error) {
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
				tickersMux.Lock()
				lastTickerInDB = ticker
				tickersMux.Unlock()
			}
			return ticker, nil
		}
	}
	return nil, nil
}

// FiatRatesGetCurrentTicker return current ticker
func (d *RocksDB) FiatRatesGetCurrentTicker(tickerTime *time.Time, token string) (*CurrencyRatesTicker, error) {
	tickersMux.Lock()
	defer tickersMux.Unlock()
	return currentTicker, nil
}

// FiatRatesCurrentTicker return current ticker
func (d *RocksDB) FiatRatesSetCurrentTicker(t *CurrencyRatesTicker) {
	tickersMux.Lock()
	defer tickersMux.Unlock()
	currentTicker = t
}