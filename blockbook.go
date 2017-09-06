package main

import (
	"errors"
	"flag"
	"log"
	"time"
)

type BlockParser interface {
	ParseBlock(b []byte) (*Block, error)
}

var (
	ErrTxNotFound = errors.New("transaction not found")
)

type Blocks interface {
	GetBestBlockHash() (string, error)
	GetBlockHash(height uint32) (string, error)
	GetBlock(hash string) (*Block, error)
}

type Outpoints interface {
	GetAddresses(txid string, vout uint32) ([]string, error)
}

type Addresses interface {
	GetTransactions(address string, lower uint32, higher uint32, fn func(txids []string) error) error
}

type Indexer interface {
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
			if err = db.GetTransactions(address, height, until, printResult); err != nil {
				log.Fatal(err)
			}
		} else {
			if err = indexBlocks(rpc, db, db, height, until); err != nil {
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

func (b *Block) GetAllAddresses(outpoints Outpoints) (map[string][]string, error) {
	addrs := make(map[string][]string, 0)

	for _, tx := range b.Txs {
		ta, err := b.GetTxAddresses(outpoints, tx)
		if err != nil {
			return nil, err
		}
		for _, addr := range ta {
			addrs[addr] = append(addrs[addr], tx.Txid)
		}
	}

	return addrs, nil
}

func (b *Block) GetTxAddresses(outpoints Outpoints, tx *Tx) ([]string, error) {
	seen := make(map[string]struct{}) // Only unique values.

	// Process outputs.
	for _, o := range tx.Vout {
		for _, a := range o.ScriptPubKey.Addresses {
			seen[a] = struct{}{}
		}
	}

	// Process inputs.  For each input, we need to take a look to the
	// outpoint index.
	for _, i := range tx.Vin {
		if i.Coinbase != "" {
			continue
		}

		// Lookup output in in the outpoint index.  In case it's not
		// found, take a look in this block.
		va, err := outpoints.GetAddresses(i.Txid, i.Vout)
		if err == ErrTxNotFound {
			va, err = b.GetAddresses(i.Txid, i.Vout)
		}
		if err != nil {
			return nil, err
		}

		for _, a := range va {
			seen[a] = struct{}{}
		}
	}

	// Convert the result set into a slice.
	addrs := make([]string, len(seen))
	i := 0
	for a := range seen {
		addrs[i] = a
		i++
	}
	return addrs, nil
}

func (b *Block) GetAddresses(txid string, vout uint32) ([]string, error) {
	// TODO: Lookup transaction in constant time.
	for _, tx := range b.Txs {
		if tx.Txid == txid {
			return tx.Vout[vout].ScriptPubKey.Addresses, nil
		}
	}
	return nil, ErrTxNotFound
}

func indexBlocks(
	blocks Blocks,
	outpoints Outpoints,
	index Indexer,
	lower uint32,
	higher uint32,
) error {
	bch := make(chan blockResult, 3)

	go getBlocks(lower, higher, blocks, bch)

	for res := range bch {
		if res.err != nil {
			return res.err
		}
		addrs, err := res.block.GetAllAddresses(outpoints)
		if err != nil {
			return err
		}
		if err := index.ConnectBlock(res.block, addrs); err != nil {
			return err
		}
	}
	return nil
}

type blockResult struct {
	block *Block
	err   error
}

func getBlocks(lower uint32, higher uint32, blocks Blocks, results chan<- blockResult) {
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
