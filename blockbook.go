package main

import (
	"flag"
	"log"
	"time"
)

type BlockParser interface {
	ParseBlock(b []byte) (*Block, error)
}

type BlockOracle interface {
	GetBestBlockHash() (string, error)
	GetBlockHash(height uint32) (string, error)
	GetBlock(hash string) (*Block, error)
}

type OutpointAddressOracle interface {
	GetOutpointAddresses(txid string, vout uint32) ([]string, error)
}

type AddressTransactionOracle interface {
	GetAddressTransactions(address string, lower uint32, higher uint32, fn func(txids []string) error) error
}

type BlockIndex interface {
	ConnectBlock(block *Block, txids map[string][]string) error
	DisconnectBlock(block *Block, txids map[string][]string) error
}

var (
	chain = flag.String("chain", "mainnet", "none | mainnet | regtest | testnet3 | simnet")

	rpcURL     = flag.String("rpcurl", "http://localhost:8332", "url of bitcoin RPC service")
	rpcUser    = flag.String("rpcuser", "rpc", "rpc username")
	rpcPass    = flag.String("rpcpass", "rpc", "rpc password")
	rpcTimeout = flag.Uint("rpctimeout", 25, "rpc timeout in seconds")
	rpcCache   = flag.Int("rpccache", 50000, "number to tx replies to cache")

	dbPath = flag.String("path", "./data", "path to address index directory")

	blockHeight  = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil   = flag.Int("blockuntil", -1, "height of the final block")
	queryAddress = flag.String("address", "", "query contents of this address")
)

func main() {
	flag.Parse()

	timeout := time.Duration(*rpcTimeout) * time.Second
	rpc := NewBitcoinRPC(*rpcURL, *rpcUser, *rpcPass, timeout)
	if *rpcCache > 0 {
		rpc.EnableCache(*rpcCache)
	}

	if *chain != "none" {
		for _, p := range GetChainParams() {
			if p.Name == *chain {
				rpc.Parser = &BitcoinBlockParser{Params: p}
			}
		}
		if rpc.Parser == nil {
			log.Fatal("unknown chain")
		}
	}

	db, err := NewRocksDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if *blockHeight >= 0 {
		if *blockUntil < 0 {
			*blockUntil = *blockHeight
		}
		height := uint32(*blockHeight)
		until := uint32(*blockUntil)
		address := *queryAddress

		if address != "" {
			if err = db.GetAddressTransactions(address, height, until, printResult); err != nil {
				log.Fatal(err)
			}
		} else {
			if err = indexBlocks(rpc, rpc, db, height, until); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func printResult(txids []string) error {
	for i, txid := range txids {
		log.Printf("%d: %s", i, txid)
	}
	return nil
}

func (b *Block) CollectBlockAddresses(o OutpointAddressOracle) (map[string][]string, error) {
	addrs := make(map[string][]string, 0)

	for _, tx := range b.Txs {
		voutAddrs, err := tx.CollectAddresses(o)
		if err != nil {
			return nil, err
		}
		for _, addr := range voutAddrs {
			addrs[addr] = append(addrs[addr], tx.Txid)
		}
	}

	return addrs, nil
}

func (tx *Tx) CollectAddresses(o OutpointAddressOracle) ([]string, error) {
	addrs := make([]string, 0)
	seen := make(map[string]struct{})

	for _, vout := range tx.Vout {
		for _, addr := range vout.ScriptPubKey.Addresses {
			if _, found := seen[addr]; !found {
				addrs = append(addrs, addr)
				seen[addr] = struct{}{}
			}
		}
	}

	for _, vin := range tx.Vin {
		if vin.Coinbase != "" {
			continue
		}
		vinAddrs, err := o.GetOutpointAddresses(vin.Txid, vin.Vout)
		if err != nil {
			return nil, err
		}
		for _, addr := range vinAddrs {
			if _, found := seen[addr]; !found {
				addrs = append(addrs, addr)
				seen[addr] = struct{}{}
			}
		}
	}

	return addrs, nil
}

func indexBlocks(
	blocks BlockOracle,
	outpoints OutpointAddressOracle,
	index BlockIndex,
	lower uint32,
	higher uint32,
) error {
	bch := make(chan blockResult, 3)

	go getBlocks(lower, higher, blocks, bch)

	for res := range bch {
		if res.err != nil {
			return res.err
		}
		addrs, err := res.block.CollectBlockAddresses(outpoints)
		if err != nil {
			return err
		}
		if err := index.IndexBlock(res.block, addrs); err != nil {
			return err
		}
	}
	return nil
}

type blockResult struct {
	block *Block
	err   error
}

func getBlocks(lower uint32, higher uint32, blocks BlockOracle, results chan<- blockResult) {
	defer close(results)

	height := lower
		hash, err := blocks.GetBlockHash(height)
		if err != nil {
			results <- blockResult{err: err}
			return
		}

	for height <= higher {
		block, err := blocks.GetBlock(hash)
		if err != nil {
			results <- blockResult{err: err}
			return
		}
		hash = block.Next
		height = block.Height + 1
		results <- blockResult{block: block}
	}
}
