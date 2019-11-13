//build unittest

package fiat

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/db"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/martinboehm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	// set the current directory to blockbook root so that ./static/ works
	if err := os.Chdir(".."); err != nil {
		glog.Fatal("Chdir error:", err)
	}
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func setupRocksDB(t *testing.T, parser bchain.BlockChainParser) (*db.RocksDB, *common.InternalState, string) {
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("fakecoin")
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	return d, is, tmp
}

func closeAndDestroyRocksDB(t *testing.T, db *db.RocksDB, dbpath string) {
	// destroy db
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dbpath)
}

type testBitcoinParser struct {
	*btc.BitcoinParser
}

func bitcoinTestnetParser() *btc.BitcoinParser {
	return btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})
}

func TestCoinGecko(t *testing.T) {
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d, tmp)

	configJSON := `{
	"fiat_rates": "coingecko",
    "fiat_rates_params": "{\"url\": \"https://api.coingecko.com/api/v3\", \"coin\": \"bitcoin\", \"periodSeconds\": 60}"
	}`

	type coinGeckoParams struct {
		FiatRates       string `json:"fiat_rates"`
		FiatRatesParams string `json:"fiat_rates_params"`
	}

	var config coinGeckoParams
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		t.Errorf("Error parsing config: %v", err)
	}

	if config.FiatRates == "" || config.FiatRatesParams == "" {
		t.Errorf("Error parsing FiatRates config - empty parameter")
		return
	}
	var coingecko *CoingeckoDownloader
	coingecko, err = NewCoingeckoDownloader(d, config.FiatRatesParams, true)
	if err != nil {
		t.Errorf("Coingecko init error: %v\n", err)
	}
	if config.FiatRates == "coingecko" {
		err = coingecko.Run()
		if err != nil {
			t.Errorf("Error running CoinGeckoDownloader: %v", err)
		}
	}
}
