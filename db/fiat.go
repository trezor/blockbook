package db

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
)

// FiatRatesTimeFormat is a format string for storing FiatRates timestamps in rocksdb
const FiatRatesTimeFormat = "20060102150405" // YYYYMMDDhhmmss

const historicalFiatBootstrapStateKey = "HistoricalFiatBootstrapComplete"
const historicalFiatBootstrapAttemptsKey = "HistoricalFiatBootstrapAttempts"

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

// FiatRatesFindTickers gets FiatRates data closest to each specified timestamp.
// The method is optimized for timestamps sorted in ascending order.
func (d *RocksDB) FiatRatesFindTickers(timestamps []int64, vsCurrency string, token string) ([]*common.CurrencyRatesTicker, error) {
	tickers := make([]*common.CurrencyRatesTicker, len(timestamps))
	if len(timestamps) == 0 {
		return tickers, nil
	}
	if len(timestamps) == 1 {
		ts := time.Unix(timestamps[0], 0).UTC()
		ticker, err := d.FiatRatesFindTicker(&ts, vsCurrency, token)
		if err != nil {
			return nil, err
		}
		tickers[0] = ticker
		return tickers, nil
	}

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	first := true
	// Cache decoding result for the current iterator key. For sparse token rates,
	// multiple timestamps often resolve to the same key; avoid re-decoding it.
	var decodedKey []byte
	var decodedTicker *common.CurrencyRatesTicker
	hasDecodedKey := false
	for i, ts := range timestamps {
		seekKey := []byte(time.Unix(ts, 0).UTC().Format(FiatRatesTimeFormat))
		if first {
			it.Seek(seekKey)
			first = false
		} else if it.Valid() && bytes.Compare(it.Key().Data(), seekKey) < 0 {
			it.Seek(seekKey)
		}

		for ; it.Valid(); it.Next() {
			keyData := it.Key().Data()
			if hasDecodedKey && bytes.Equal(keyData, decodedKey) {
				if decodedTicker != nil {
					tickers[i] = decodedTicker
					break
				}
				continue
			}

			ticker, err := getTickerFromIterator(it, vsCurrency, token)
			if err != nil {
				glog.Error("FiatRatesFindTickers error: ", err)
				return nil, err
			}
			decodedKey = append(decodedKey[:0], keyData...)
			decodedTicker = ticker
			hasDecodedKey = true
			if ticker != nil {
				tickers[i] = ticker
				break
			}
		}
	}
	return tickers, nil
}

// FiatRatesGetAllTickers gets FiatRates data closest to the specified timestamp, of the base currency, vsCurrency or the token if specified
func (d *RocksDB) FiatRatesGetAllTickers(fn func(ticker *common.CurrencyRatesTicker) error) error {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfFiatRates])
	defer it.Close()

	for it.SeekToFirst(); it.Valid(); it.Next() {
		ticker, err := getTickerFromIterator(it, "", "")
		if err != nil {
			return err
		}
		if ticker == nil {
			return errors.New("FiatRatesGetAllTickers got nil ticker")
		}
		if err = fn(ticker); err != nil {
			return err
		}
	}
	return nil
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
			return ticker, nil
		}
	}
	return nil, nil
}

func (d *RocksDB) FiatRatesGetSpecialTickers(key string) (*[]common.CurrencyRatesTicker, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(key))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	if data == nil {
		return nil, nil
	}
	var tickers []common.CurrencyRatesTicker
	if err := json.Unmarshal(data, &tickers); err != nil {
		return nil, err
	}
	return &tickers, nil
}

func (d *RocksDB) FiatRatesStoreSpecialTickers(key string, tickers *[]common.CurrencyRatesTicker) error {
	data, err := json.Marshal(tickers)
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(key), data)
}

// FiatRatesGetHistoricalBootstrapComplete gets persisted historical bootstrap completion state.
// found=false means no state was stored yet (legacy deployments or pre-bootstrap).
func (d *RocksDB) FiatRatesGetHistoricalBootstrapComplete() (complete bool, found bool, err error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(historicalFiatBootstrapStateKey))
	if err != nil {
		return false, false, err
	}
	defer val.Free()
	data := val.Data()
	if data == nil {
		return false, false, nil
	}
	if err := json.Unmarshal(data, &complete); err != nil {
		return false, false, err
	}
	return complete, true, nil
}

// FiatRatesSetHistoricalBootstrapComplete stores historical bootstrap completion state.
func (d *RocksDB) FiatRatesSetHistoricalBootstrapComplete(complete bool) error {
	data, err := json.Marshal(complete)
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(historicalFiatBootstrapStateKey), data)
}

// FiatRatesGetHistoricalBootstrapAttempts gets persisted number of failed bootstrap attempts.
// found=false means no attempt counter was stored yet.
func (d *RocksDB) FiatRatesGetHistoricalBootstrapAttempts() (attempts int, found bool, err error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(historicalFiatBootstrapAttemptsKey))
	if err != nil {
		return 0, false, err
	}
	defer val.Free()
	data := val.Data()
	if data == nil {
		return 0, false, nil
	}
	if err := json.Unmarshal(data, &attempts); err != nil {
		return 0, false, err
	}
	return attempts, true, nil
}

// FiatRatesSetHistoricalBootstrapAttempts stores number of failed bootstrap attempts.
func (d *RocksDB) FiatRatesSetHistoricalBootstrapAttempts(attempts int) error {
	data, err := json.Marshal(attempts)
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(historicalFiatBootstrapAttemptsKey), data)
}
