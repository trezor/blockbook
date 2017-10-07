package main

import (
	"flag"
	"log"
	"sync"
	"time"

	"github.com/pkg/profile"
)

type BlockParser interface {
	ParseBlock(b []byte) (*Block, error)
}

type Blockchain interface {
	GetBestBlockHash() (string, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string) (*Block, error)
}

type Index interface {
	GetBestBlockHash() (string, error)
	GetBlockHash(height uint32) (string, error)
	GetTransactions(address string, lower uint32, higher uint32, fn func(txid string) error) error
	ConnectBlock(block *Block) error
	DisconnectBlock(block *Block) error
}

var (
	rpcURL     = flag.String("rpcurl", "http://localhost:8332", "url of bitcoin RPC service")
	rpcUser    = flag.String("rpcuser", "rpc", "rpc username")
	rpcPass    = flag.String("rpcpass", "rpc", "rpc password")
	rpcTimeout = flag.Uint("rpctimeout", 25, "rpc timeout in seconds")

	dbPath = flag.String("path", "./data", "path to address index directory")

	blockHeight = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil  = flag.Int("blockuntil", -1, "height of the final block")

	queryAddress = flag.String("address", "", "query contents of this address")

	resync = flag.Bool("resync", false, "resync until tip")
	repair = flag.Bool("repair", false, "repair the database")
	prof   = flag.Bool("prof", false, "profile program execution")

	syncChunk   = flag.Int("chunk", 100, "block chunk size for processing")
	syncWorkers = flag.Int("workers", 8, "number of workers to process blocks")
	dryRun      = flag.Bool("dryrun", false, "do not index blocks, only download")
	parse       = flag.Bool("parse", false, "use in-process block parsing")
)

func main() {
	flag.Parse()

	if *repair {
		if err := RepairRocksDB(*dbPath); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *prof {
		defer profile.Start().Stop()
	}

	rpc := NewBitcoinRPC(
		*rpcURL,
		*rpcUser,
		*rpcPass,
		time.Duration(*rpcTimeout)*time.Second)

	if *parse {
		rpc.Parser = &BitcoinBlockParser{
			Params: GetChainParams()[0],
		}
	}

	db, err := NewRocksDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if *resync {
		if err := resyncIndex(rpc, db); err != nil {
			log.Fatal(err)
		}
	}

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
			if err = connectBlocksParallel(
				rpc,
				db,
				height,
				until,
				*syncChunk,
				*syncWorkers,
			); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func printResult(txid string) error {
	log.Printf("%s", txid)
	return nil
}

func resyncIndex(chain Blockchain, index Index) error {
	remote, err := chain.GetBestBlockHash()
	if err != nil {
		return err
	}
	local, err := index.GetBestBlockHash()
	if err != nil {
		local = ""
	}

	// If the local block is missing, we're indexing from the genesis block.
	if local == "" {
		log.Printf("resync: genesis")

		hash, err := chain.GetBlockHash(0)
		if err != nil {
			return err
		}
		return connectBlock(chain, index, hash)
	}

	// If the locally indexed block is the same as the best block on the
	// network, we're done.
	if local == remote {
		log.Printf("resync: synced on %s", local)
		return nil
	}

	// Is local tip on the best chain?
	header, err := chain.GetBlockHeader(local)
	forked := false
	if err != nil {
		if e, ok := err.(*RPCError); ok && e.Message == "Block not found" {
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
		return disconnectBlock(chain, index, header.Hash)
	} else {
		log.Printf("resync: local is behind")
		return connectBlock(chain, index, header.Next)
	}
}

func connectBlock(
	chain Blockchain,
	index Index,
	hash string,
) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go getBlockChain(hash, chain, bch, done)

	for res := range bch {
		if res.err != nil {
			return res.err
		}
		err := index.ConnectBlock(res.block)
		if err != nil {
			return err
		}
	}

	return nil
}

func disconnectBlock(
	chain Blockchain,
	index Index,
	hash string,
) error {
	return nil
}

func connectBlocksParallel(
	chain Blockchain,
	index Index,
	lower uint32,
	higher uint32,
	chunkSize int,
	numWorkers int,
) error {
	var wg sync.WaitGroup

	work := func(i int) {
		defer wg.Done()

		offset := uint32(chunkSize * i)
		stride := uint32(chunkSize * numWorkers)

		for low := lower + offset; low <= higher; low += stride {
			high := low + uint32(chunkSize-1)
			if high > higher {
				high = higher
			}
			err := connectBlockChunk(chain, index, low, high)
			if err != nil {
				log.Fatal(err) // TODO
			}
		}
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go work(i)
	}
	wg.Wait()

	return nil
}

func connectBlockChunk(
	chain Blockchain,
	index Index,
	lower uint32,
	higher uint32,
) error {
	connected, err := isBlockConnected(chain, index, higher)
	if err != nil || connected {
		return err
	}

	height := lower
	hash, err := chain.GetBlockHash(lower)
	if err != nil {
		return err
	}

	for height <= higher {
		block, err := chain.GetBlock(hash)
		if err != nil {
			return err
		}
		hash = block.Next
		height = block.Height + 1
		if *dryRun {
			continue
		}
		err = index.ConnectBlock(block)
		if err != nil {
			return err
		}
	}

	return nil
}

func isBlockConnected(
	chain Blockchain,
	index Index,
	height uint32,
) (bool, error) {
	local, err := index.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	remote, err := chain.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	if local != remote {
		return false, nil
	}
	return true, nil
}

type blockResult struct {
	block *Block
	err   error
}

func getBlockChain(
	hash string,
	chain Blockchain,
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
		block, err := chain.GetBlock(hash)
		if err != nil {
			out <- blockResult{err: err}
			return
		}
		hash = block.Next
		out <- blockResult{block: block}
	}
}
