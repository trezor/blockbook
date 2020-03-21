package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/trezor/blockbook/build/tools"
)

const (
	configsDir  = "configs"
	templateDir = "build/templates"
	outputDir   = "build/pkg-defs"
)

func main() {
	if len(os.Args) < 2 {
		var coins []string
		filepath.Walk(filepath.Join(configsDir, "coins"), func(path string, info os.FileInfo, err error) error {
			n := strings.TrimSuffix(info.Name(), ".json")
			if n != info.Name() {
				coins = append(coins, n)
			}
			return nil
		})
		fmt.Fprintf(os.Stderr, "Usage: %s coin\nCoin is one of:\n%v\n", filepath.Base(os.Args[0]), coins)
		os.Exit(1)
	}

	coin := os.Args[1]
	config, err := build.LoadConfig(configsDir, coin)
	if err == nil {
		err = build.GeneratePackageDefinitions(config, templateDir, outputDir)
	}
	if err != nil {
		panic(err)
	}
	fmt.Printf("Package files for %v generated to %v\n", coin, outputDir)
}
