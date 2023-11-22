package common

import (
	"encoding/json"
	"os"

	"github.com/juju/errors"
)

// Config struct
type Config struct {
	CoinName                string `json:"coin_name"`
	CoinShortcut            string `json:"coin_shortcut"`
	CoinLabel               string `json:"coin_label"`
	FourByteSignatures      string `json:"fourByteSignatures"`
	FiatRates               string `json:"fiat_rates"`
	FiatRatesParams         string `json:"fiat_rates_params"`
	FiatRatesVsCurrencies   string `json:"fiat_rates_vs_currencies"`
	BlockGolombFilterP      uint8  `json:"block_golomb_filter_p"`
	BlockFilterScripts      string `json:"block_filter_scripts"`
	BlockFilterUseZeroedKey bool   `json:"block_filter_use_zeroed_key"`
}

// GetConfig loads and parses the config file and returns Config struct
func GetConfig(configFile string) (*Config, error) {
	if configFile == "" {
		return nil, errors.New("Missing blockchaincfg configuration parameter")
	}

	configFileContent, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.Errorf("Error reading file %v, %v", configFile, err)
	}

	var cn Config
	err = json.Unmarshal(configFileContent, &cn)
	if err != nil {
		return nil, errors.Annotatef(err, "Error parsing config file ")
	}
	return &cn, nil
}
