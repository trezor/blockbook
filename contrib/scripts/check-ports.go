//usr/bin/go run $0 $@ ; exit
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const configDir = "configs/coins"

type Config struct {
	Coin struct {
		Name string `json:"name"`
	}
	Ports map[string]uint16 `json:"ports"`
}

func main() {
	ports := make(map[uint16][]string)
	status := 0

	files, err := ioutil.ReadDir(configDir)
	if err != nil {
		panic(err)
	}

	for _, fi := range files {
		if fi.IsDir() || fi.Name()[0] == '.' {
			continue
		}

		path := filepath.Join(configDir, fi.Name())
		f, err := os.Open(path)
		if err != nil {
			panic(fmt.Errorf("%s: %s", path, err))
		}
		defer f.Close()

		v := Config{}
		d := json.NewDecoder(f)
		err = d.Decode(&v)
		if err != nil {
			panic(fmt.Errorf("%s: json: %s", path, err))
		}

		if _, ok := v.Ports["blockbook_internal"]; !ok {
			fmt.Printf("%s: missing blockbook_internal port\n", v.Coin.Name)
			status = 1
		}
		if _, ok := v.Ports["blockbook_public"]; !ok {
			fmt.Printf("%s: missing blockbook_public port\n", v.Coin.Name)
			status = 1
		}
		if _, ok := v.Ports["backend_rpc"]; !ok {
			fmt.Printf("%s: missing backend_rpc port\n", v.Coin.Name)
			status = 1
		}

		for _, port := range v.Ports {
			if port > 0 {
				ports[port] = append(ports[port], v.Coin.Name)
			}
		}
	}

	for port, coins := range ports {
		if len(coins) > 1 {
			fmt.Printf("port %d: registered by %q\n", port, coins)
			status = 1
		}
	}

	if status == 0 {
		fmt.Println("OK")
	} else {
		fmt.Println("Got some errors")
	}

	os.Exit(status)
}
