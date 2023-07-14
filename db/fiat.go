package db

import (
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
