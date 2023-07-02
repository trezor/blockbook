// usr/bin/go run $0 $@ ; exit
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	inputDir   = "configs/coins"
	outputFile = "docs/ports.md"
)

// PortInfo contains backend and blockbook ports
type PortInfo struct {
	CoinName              string
	BlockbookInternalPort uint16
	BlockbookPublicPort   uint16
	BackendRPCPort        uint16
	BackendServicePorts   map[string]uint16
}

// PortInfoSlice is self describing
type PortInfoSlice []*PortInfo

// Config contains coin configuration
type Config struct {
	Coin struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		Alias string `json:"alias"`
	}
	Ports     map[string]uint16 `json:"ports"`
	Blockbook struct {
		PackageName string `json:"package_name"`
	}
}

func checkPorts() int {
	ports := make(map[uint16][]string)
	status := 0

	files, err := os.ReadDir(inputDir)
	if err != nil {
		panic(err)
	}

	for _, fi := range files {
		if fi.IsDir() || fi.Name()[0] == '.' {
			continue
		}

		path := filepath.Join(inputDir, fi.Name())
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
			fmt.Printf("%s (%s): missing blockbook_internal port\n", v.Coin.Name, v.Coin.Alias)
			status = 1
		}
		if _, ok := v.Ports["blockbook_public"]; !ok {
			fmt.Printf("%s (%s): missing blockbook_public port\n", v.Coin.Name, v.Coin.Alias)
			status = 1
		}
		if _, ok := v.Ports["backend_rpc"]; !ok {
			fmt.Printf("%s (%s): missing backend_rpc port\n", v.Coin.Name, v.Coin.Alias)
			status = 1
		}

		for _, port := range v.Ports {
			// ignore duplicities caused by configs that do not serve blockbook directly (consensus layers)
			if port > 0 && v.Blockbook.PackageName == "" {
				ports[port] = append(ports[port], v.Coin.Alias)
			}
		}
	}

	for port, coins := range ports {
		if len(coins) > 1 {
			fmt.Printf("port %d: registered by %q\n", port, coins)
			status = 1
		}
	}

	if status != 0 {
		fmt.Println("Got some errors")
	}
	return status
}

func main() {
	output := "stdout"
	if len(os.Args) > 1 {
		if len(os.Args) == 2 && os.Args[1] == "-w" {
			output = outputFile
		} else {
			fmt.Fprintf(os.Stderr, "Usage: %s [-w]\n", filepath.Base(os.Args[0]))
			fmt.Fprintf(os.Stderr, "    -w    write output to %s instead of stdout\n", outputFile)
			os.Exit(1)
		}
	}

	status := checkPorts()
	if status != 0 {
		os.Exit(status)
	}

	slice, err := loadPortInfo(inputDir)
	if err != nil {
		panic(err)
	}

	sortPortInfo(slice)

	err = writeMarkdown(output, slice)
	if err != nil {
		panic(err)
	}
}

func loadPortInfo(dir string) (PortInfoSlice, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	items := make(PortInfoSlice, 0, len(files))

	for _, fi := range files {
		if fi.IsDir() || fi.Name()[0] == '.' {
			continue
		}

		path := filepath.Join(dir, fi.Name())
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", path, err)
		}
		defer f.Close()

		v := Config{}
		d := json.NewDecoder(f)
		err = d.Decode(&v)
		if err != nil {
			return nil, fmt.Errorf("%s: json: %s", path, err)
		}

		// skip configs that do not have blockbook (consensus layers)
		if v.Blockbook.PackageName == "" {
			continue
		}
		name := v.Coin.Label
		// exceptions when to use Name instead of Label so that the table looks good
		if len(name) == 0 || strings.Contains(v.Coin.Name, "Ethereum") || strings.Contains(v.Coin.Name, "Archive") {
			name = v.Coin.Name
		}
		item := &PortInfo{CoinName: name, BackendServicePorts: map[string]uint16{}}
		for k, p := range v.Ports {
			if p == 0 {
				continue
			}

			switch k {
			case "blockbook_internal":
				item.BlockbookInternalPort = p
			case "blockbook_public":
				item.BlockbookPublicPort = p
			case "backend_rpc":
				item.BackendRPCPort = p
			default:
				if len(k) > 8 && k[:8] == "backend_" {
					item.BackendServicePorts[k[8:]] = p
				}
			}
		}

		items = append(items, item)
	}

	return items, nil
}

func sortPortInfo(slice PortInfoSlice) {
	// normalizes values in order to sort zero values at the bottom of the slice
	normalize := func(a, b uint16) (uint16, uint16) {
		if a == 0 {
			a = math.MaxUint16
		}
		if b == 0 {
			b = math.MaxUint16
		}
		return a, b
	}

	// sort values by BlockbookPublicPort, then by BackendRPCPort and finally by
	// CoinName; zero values are sorted at the bottom of the slice
	sort.Slice(slice, func(i, j int) bool {
		a, b := normalize(slice[i].BlockbookPublicPort, slice[j].BlockbookPublicPort)

		if a < b {
			return true
		}
		if a > b {
			return false
		}

		a, b = normalize(slice[i].BackendRPCPort, slice[j].BackendRPCPort)

		if a < b {
			return true
		}
		if a > b {
			return false
		}

		return strings.Compare(slice[i].CoinName, slice[j].CoinName) == -1
	})
}

func writeMarkdown(output string, slice PortInfoSlice) error {
	var (
		buf bytes.Buffer
		err error
	)

	fmt.Fprintf(&buf, "# Registry of ports\n\n")

	header := []string{"coin", "blockbook public", "blockbook internal", "backend rpc", "backend service ports (zmq)"}
	writeTable(&buf, header, slice)

	fmt.Fprintf(&buf, "\n> NOTE: This document is generated from coin definitions in `configs/coins` using command `go run contrib/scripts/check-and-generate-port-registry.go -w`.\n")

	out := os.Stdout
	if output != "stdout" {
		out, err = os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer out.Close()
	}

	n, err := out.Write(buf.Bytes())
	if err != nil {
		return err
	}
	if n < len(buf.Bytes()) {
		return io.ErrShortWrite
	}

	return nil
}

func writeTable(w io.Writer, header []string, slice PortInfoSlice) {
	rows := make([][]string, len(slice))
	for i, item := range slice {
		row := make([]string, len(header))
		row[0] = item.CoinName
		if item.BlockbookPublicPort > 0 {
			row[1] = fmt.Sprintf("%d", item.BlockbookPublicPort)
		}
		if item.BlockbookInternalPort > 0 {
			row[2] = fmt.Sprintf("%d", item.BlockbookInternalPort)
		}
		if item.BackendRPCPort > 0 {
			row[3] = fmt.Sprintf("%d", item.BackendRPCPort)
		}

		svcPorts := make([]string, 0, len(item.BackendServicePorts))
		for k, v := range item.BackendServicePorts {
			var s string
			if k == "message_queue" {
				s = fmt.Sprintf("%d", v)
			} else {
				s = fmt.Sprintf("%d %s", v, k)
			}
			svcPorts = append(svcPorts, s)
		}

		sort.Strings(svcPorts)
		row[4] = strings.Join(svcPorts, ", ")

		rows[i] = row
	}

	padding := make([]int, len(header))
	for column := range header {
		padding[column] = len(header[column])

		for _, row := range rows {
			padding[column] = maxInt(padding[column], len(row[column]))
		}
	}

	content := make([][]string, 0, len(rows)+2)

	content = append(content, paddedRow(header, padding))
	content = append(content, delim("-", padding))

	for _, row := range rows {
		content = append(content, paddedRow(row, padding))
	}

	for _, row := range content {
		fmt.Fprintf(w, "|%s|\n", strings.Join(row, "|"))
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func paddedRow(row []string, padding []int) []string {
	out := make([]string, len(row))
	for i := 0; i < len(row); i++ {
		format := fmt.Sprintf(" %%-%ds ", padding[i])
		out[i] = fmt.Sprintf(format, row[i])
	}
	return out
}

func delim(str string, padding []int) []string {
	out := make([]string, len(padding))
	for i := 0; i < len(padding); i++ {
		out[i] = strings.Repeat(str, padding[i]+2)
	}
	return out
}
