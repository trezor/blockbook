// +build integration

package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

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
	path = filepath.Join(path, filename)
	return ioutil.ReadFile(path)
}

var testConfigRegistry map[string]*TestConfig

func LoadTestConfig(coin string) (*TestConfig, error) {
	if testConfigRegistry == nil {
		b, err := readDataFile(".", "bchain/tests/rpc", "config.json")
		if err != nil {
			return nil, err
		}
		var v map[string]*TestConfig
		err = json.Unmarshal(b, &v)
		if err != nil {
			return nil, err
		}
		testConfigRegistry = v
	}
	c, found := testConfigRegistry[coin]
	if !found {
		return nil, errors.New("Test config not found")
	}
	return c, nil
}

func LoadRPCConfig(coin string) (json.RawMessage, error) {
	t := `{
	"coin_name": "%s",
	"rpcURL": "%s",
	"rpcUser": "%s",
	"rpcPass": "%s",
	"rpcTimeout": 25,
	"parse": true
	}`

	c, err := LoadTestConfig(coin)
	if err != nil {
		return json.RawMessage{}, err
	}

	return json.RawMessage(fmt.Sprintf(t, coin, c.URL, c.User, c.Pass)), nil
}

func LoadTestData(coin string) (*TestData, error) {
	b, err := readDataFile(".", "bchain/tests/rpc/testdata", coin+".json")
	if err != nil {
		return nil, err
	}
	var v TestData
	err = json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
