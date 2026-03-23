//go:build integration

package bchain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	buildcfg "github.com/trezor/blockbook/build/tools"
)

const (
	testBuildEnvVar = "BB_BUILD_ENV"
	testBuildEnvDev = "dev"
)

var testEnvMu sync.Mutex

// BlockchainCfg contains fields read from blockbook's blockchaincfg.json after being rendered from templates.
type BlockchainCfg struct {
	// more fields can be added later as needed
	RpcUrl     string `json:"rpc_url"`
	RpcUrlWs   string `json:"rpc_url_ws"`
	RpcUser    string `json:"rpc_user"`
	RpcPass    string `json:"rpc_pass"`
	RpcTimeout int    `json:"rpc_timeout"`
	Parse      bool   `json:"parse"`
}

// LoadBlockchainCfg returns the resolved blockchaincfg.json (env overrides are honored in tests)
func LoadBlockchainCfg(t *testing.T, coinAlias string) BlockchainCfg {
	t.Helper()

	rawCfg, err := loadBlockchainCfgBytes(coinAlias)
	if err != nil {
		t.Fatalf("%v", err)
	}

	var blockchainCfg BlockchainCfg
	if err := json.Unmarshal(rawCfg, &blockchainCfg); err != nil {
		t.Fatalf("unmarshal blockchain config for %s: %v", coinAlias, err)
	}
	if blockchainCfg.RpcUrl == "" {
		t.Fatalf("empty rpc_url for %s", coinAlias)
	}
	return blockchainCfg
}

// LoadBlockchainCfgRaw returns the rendered blockchaincfg.json payload for integration tests.
func LoadBlockchainCfgRaw(coinAlias string) (json.RawMessage, error) {
	rawCfg, err := loadBlockchainCfgBytes(coinAlias)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(rawCfg), nil
}

func loadBlockchainCfgBytes(coinAlias string) ([]byte, error) {
	configsDir, err := repoConfigsDir()
	if err != nil {
		return nil, fmt.Errorf("integration config path error: %w", err)
	}
	templatesDir, err := repoTemplatesDir(configsDir)
	if err != nil {
		return nil, fmt.Errorf("integration templates path error: %w", err)
	}

	var config *buildcfg.Config
	err = withDefaultTestBuildEnv(func() error {
		var loadErr error
		config, loadErr = buildcfg.LoadConfig(configsDir, coinAlias)
		return loadErr
	})
	if err != nil {
		return nil, fmt.Errorf("load config for %s: %w", coinAlias, err)
	}

	outputDir, err := os.MkdirTemp("", "integration_blockchaincfg")
	if err != nil {
		return nil, fmt.Errorf("integration temp dir error: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(outputDir)
	}()

	// Render templates so tests read the same generated blockchaincfg.json as packaging.
	if err := buildcfg.GeneratePackageDefinitions(config, templatesDir, outputDir); err != nil {
		return nil, fmt.Errorf("generate package definitions for %s: %w", coinAlias, err)
	}

	blockchainCfgPath := filepath.Join(outputDir, "blockbook", "blockchaincfg.json")
	rawCfg, err := os.ReadFile(blockchainCfgPath)
	if err != nil {
		return nil, fmt.Errorf("read blockchain config for %s: %w", coinAlias, err)
	}
	return rawCfg, nil
}

func withDefaultTestBuildEnv(fn func() error) error {
	testEnvMu.Lock()
	defer testEnvMu.Unlock()

	if _, ok := os.LookupEnv(testBuildEnvVar); ok {
		return fn()
	}
	if err := os.Setenv(testBuildEnvVar, testBuildEnvDev); err != nil {
		return err
	}
	defer func() {
		_ = os.Unsetenv(testBuildEnvVar)
	}()
	return fn()
}

// repoTemplatesDir locates build/templates relative to the repo root.
func repoTemplatesDir(configsDir string) (string, error) {
	repoRoot := filepath.Dir(configsDir)
	templatesDir := filepath.Join(repoRoot, "build", "templates")
	if _, err := os.Stat(templatesDir); err == nil {
		return templatesDir, nil
	} else if os.IsNotExist(err) {
		return "", fmt.Errorf("build/templates not found near %s", configsDir)
	} else {
		return "", err
	}
}

// repoConfigsDir finds configs/coins from the caller path so tests can run from any subdir.
func repoConfigsDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("unable to resolve caller path")
	}
	dir := filepath.Dir(file)
	// Walk up so tests can run from any subdir while still locating configs.
	for i := 0; i < 3; i++ {
		configsDir := filepath.Join(dir, "configs")
		if _, err := os.Stat(filepath.Join(configsDir, "coins")); err == nil {
			return configsDir, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("configs/coins not found from caller path")
}
