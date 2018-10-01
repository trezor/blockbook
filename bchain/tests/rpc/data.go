package rpc

import (
	"blockbook/bchain"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type TestData struct {
	BlockHeight uint32                `json:"blockHeight"`
	BlockHash   string                `json:"blockHash"`
	BlockTime   int64                 `json:"blockTime"`
	BlockTxs    []string              `json:"blockTxs"`
	TxDetails   map[string]*bchain.Tx `json:"txDetails"`
}

func joinPathsWithCommonElement(p1, p2 string) (string, bool) {
	idx := strings.IndexRune(p2, filepath.Separator)
	if idx <= 0 {
		return "", false
	}
	p2root := p2[:idx]
	idx = strings.LastIndex(p1, p2root)
	if idx < 0 {
		return "", false
	}
	prefix := p1[:idx]
	return filepath.Join(prefix, p2), true
}

func readDataFile(dir, relDir, filename string) ([]byte, error) {
	var err error
	dir, err = filepath.Abs(dir)
	if err == nil {
		dir, err = filepath.EvalSymlinks(dir)
	}
	if err != nil {
		return nil, err
	}
	path, ok := joinPathsWithCommonElement(dir, relDir)
	if !ok {
		return nil, errors.New("Path not found")
	}
	filename = strings.Replace(filename, " ", "_", -1)
	path = filepath.Join(path, filename)
	return ioutil.ReadFile(path)
}

func LoadTestData(coin string, parser bchain.BlockChainParser) (*TestData, error) {
	b, err := readDataFile(".", "bchain/tests/rpc/testdata", coin+".json")
	if err != nil {
		return nil, err
	}
	var v TestData
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	// convert amounts in test json to bit.Int and clear the temporary JsonValue
	for _, tx := range v.TxDetails {
		for i := range tx.Vout {
			vout := &tx.Vout[i]
			vout.ValueSat, err = parser.AmountToBigInt(vout.JsonValue)
			if err != nil {
				return nil, err
			}
			vout.JsonValue = ""
		}
	}
	return &v, nil
}
