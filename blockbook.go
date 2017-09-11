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
	ErrNotFound = errors.New("not found")
)

type Blocks interface {
	GetBestBlockHash() (string, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string) (*Block, error)
}

type Outpoints interface {
	// GetAddress looks up a transaction output and returns its address.
	// ErrNotFound is returned if the output is not found.  Address can be
	// empty string in case it's not intelligable.
	GetAddress(txid string, vout uint32) (string, error)
}

type Addresses interface {
	GetTransactions(address string, lower uint32, higher uint32, fn func(txids []string) error) error
}

type Indexer interface {
	ConnectBlock(block *Block, txids map[string][]string) error
	DisconnectBlock(block *Block, txids map[string][]string) error
	GetLastBlockHash() (string, error)
}

func (b *Block) GetAllAddresses(outpoints Outpoints) (map[string][]string, error) {
	addrs := make(map[string][]string, 0) // Address to a list of txids.

	for _, tx := range b.Txs {
		ta, err := b.GetTxAddresses(outpoints, tx)
		if err != nil {
			return nil, err
		}
		for a, _ := range ta {
			addrs[a] = append(addrs[a], tx.Txid)
		}
	}

	return addrs, nil
}

func (b *Block) GetTxAddresses(outpoints Outpoints, tx *Tx) (map[string]struct{}, error) {
	addrs := make(map[string]struct{}) // Only unique values.

	// Process outputs.
	for _, o := range tx.Vout {
		a := o.GetAddress()
		if a != "" {
			addrs[a] = struct{}{}
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
		a, err := outpoints.GetAddress(i.Txid, i.Vout)
		if err == ErrNotFound {
			a, err = b.GetAddress(i.Txid, i.Vout)
		}
		if err != nil {
			return nil, err
		}
		if a != "" {
			addrs[a] = struct{}{}
		}
	}

	return addrs, nil
}

func (b *Block) GetAddress(txid string, vout uint32) (string, error) {
	var t *Tx
	for _, tx := range b.Txs {
		if tx.Txid == txid {
			t = tx
			break
		}
	}
	if t == nil {
		// Transaction output was not found.
		return "", ErrNotFound
	}
	return t.GetAddress(vout)
}

func (t *Tx) GetAddress(vout uint32) (string, error) {
	if vout >= uint32(len(t.Vout)) {
		// The output doesn't exist.
		return "", ErrNotFound
	}
	return t.Vout[vout].GetAddress(), nil
}

func (o *Vout) GetAddress() string {
	if len(o.ScriptPubKey.Addresses) != 1 {
		// The output address is not intelligible.
		return ""
	}
	return o.ScriptPubKey.Addresses[0]
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

	resync = flag.Bool("resync", false, "resync until tip")
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
			if err = connectBlockRange(rpc, db, db, height, until); err != nil {
				log.Fatal(err)
			}
		}
	}

	if *resync {
		if err := resyncIndex(rpc, db, db); err != nil {
			log.Fatal(err)
		}
	}
}

func printResult(txids []string) error {
	for i, txid := range txids {
		log.Printf("%d: %s", i, txid)
	}
	return nil
}

func resyncIndex(
	blocks Blocks,
	outpoints Outpoints,
	index Indexer,
) error {
	best, err := blocks.GetBestBlockHash()
	if err != nil {
		return err
	}
	last, err := index.GetLastBlockHash()
	if err != nil {
		last = ""
	}

	// If the local block is missing, we're indexing from the genesis block.
	if last == "" {
		log.Printf("resync: genesis")

		hash, err := blocks.GetBlockHash(0)
		if err != nil {
			return err
		}
		return connectBlock(blocks, outpoints, index, hash)
	}

	// If the locally indexed block is the same as the best block on the
	// network, we're done.
	if last == best {
		log.Printf("resync: synced on %s", last)
		return nil
	}

	// Is local tip on the best chain?
	header, err := blocks.GetBlockHeader(last)
	forked := false
	if err != nil {
		if e, ok := err.(*cmdError); ok && e.Message == "Block not found" {
			forked = true
		} else {
			return err
		}
	} else {
		if header.Confirmations < 0 {
			forked = true
		}
	}

	if forked {
		log.Printf("resync: local is forked")
		// TODO: resync after disconnecting
		return disconnectBlock(blocks, outpoints, index, header.Hash)
	} else {
		log.Printf("resync: local is behind")
		return connectBlock(blocks, outpoints, index, header.Next)
	}
}

func connectBlock(
	blocks Blocks,
	outpoints Outpoints,
	index Indexer,
	hash string,
) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go getBlockChain(hash, blocks, bch, done)

	for res := range bch {
		err := res.err
		block := res.block

		if err != nil {
			return err
		}
		addrs, err := block.GetAllAddresses(outpoints)
		if err != nil {
			return err
		}
		if err := index.ConnectBlock(block, addrs); err != nil {
			return err
		}
	}

	return nil
}

func disconnectBlock(
	blocks Blocks,
	outpoints Outpoints,
	index Indexer,
	hash string,
) error {
	return nil
}

func connectBlockRange(
	blocks Blocks,
	outpoints Outpoints,
	index Indexer,
	lower uint32,
	higher uint32,
) error {
	bch := make(chan blockResult, 3)

	go getBlockRange(lower, higher, blocks, bch)

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

func getBlockRange(
	lower uint32,
	higher uint32,
	blocks Blocks,
	results chan<- blockResult,
) {
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

func getBlockChain(
	hash string,
	blocks Blocks,
	out chan blockResult,
	done chan struct{},
) {
	defer close(out)

	for hash != "" {
		select {
		case <-done:
			return
		default:
		}
		block, err := blocks.GetBlock(hash)
		if err != nil {
			out <- blockResult{err: err}
			return
		}
		hash = block.Next
		out <- blockResult{block: block}
	}
}
