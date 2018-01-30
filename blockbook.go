package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"blockbook/bitcoin"
	"blockbook/db"
	"blockbook/server"

	"github.com/pkg/profile"
)

type Blockchain interface {
	GetBestBlockHash() (string, error)
	GetBestBlockHeight() (uint32, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*bitcoin.BlockHeader, error)
	GetBlock(hash string) (*bitcoin.Block, error)
}

type Index interface {
	GetBestBlock() (uint32, string, error)
	GetBlockHash(height uint32) (string, error)
	GetTransactions(outputScript []byte, lower uint32, higher uint32, fn func(txid string) error) error
	ConnectBlock(block *bitcoin.Block) error
	DisconnectBlock(block *bitcoin.Block) error
	DisconnectBlocks(lower uint32, higher uint32) error
}

var (
	rpcURL     = flag.String("rpcurl", "http://localhost:8332", "url of bitcoin RPC service")
	rpcUser    = flag.String("rpcuser", "rpc", "rpc username")
	rpcPass    = flag.String("rpcpass", "rpc", "rpc password")
	rpcTimeout = flag.Uint("rpctimeout", 25, "rpc timeout in seconds")

	dbPath = flag.String("path", "./data", "path to address index directory")

	blockHeight    = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil     = flag.Int("blockuntil", -1, "height of the final block")
	rollbackHeight = flag.Int("rollback", -1, "rollback to the given height and quit")

	queryAddress = flag.String("address", "", "query contents of this address")

	resync = flag.Bool("resync", false, "resync until tip")
	repair = flag.Bool("repair", false, "repair the database")
	prof   = flag.Bool("prof", false, "profile program execution")

	syncChunk   = flag.Int("chunk", 100, "block chunk size for processing")
	syncWorkers = flag.Int("workers", 8, "number of workers to process blocks")
	dryRun      = flag.Bool("dryrun", false, "do not index blocks, only download")
	parse       = flag.Bool("parse", false, "use in-process block parsing")

	httpServerBinding = flag.String("httpserver", "", "http server binding [address]:port, if missing no http server")

	zeroMQBinding = flag.String("zeromq", "", "binding to zeromq, if missing no zeromq connection")
)

func main() {
	flag.Parse()

	if *prof {
		defer profile.Start().Stop()
	}

	if *repair {
		if err := db.RepairRocksDB(*dbPath); err != nil {
			log.Fatalf("RepairRocksDB %s: %v", *dbPath, err)
		}
		return
	}

	rpc := bitcoin.NewBitcoinRPC(
		*rpcURL,
		*rpcUser,
		*rpcPass,
		time.Duration(*rpcTimeout)*time.Second)

	if *parse {
		rpc.Parser = &bitcoin.BitcoinBlockParser{
			Params: bitcoin.GetChainParams()[0],
		}
	}

	db, err := db.NewRocksDB(*dbPath)
	if err != nil {
		log.Fatalf("NewRocksDB %v", err)
	}
	defer db.Close()

	if *rollbackHeight >= 0 {
		bestHeight, _, err := db.GetBestBlock()
		if err != nil {
			log.Fatalf("rollbackHeight: %v", err)
		}
		if uint32(*rollbackHeight) > bestHeight {
			log.Printf("nothing to rollback, rollbackHeight %d, bestHeight: %d", *rollbackHeight, bestHeight)
		} else {
			err = db.DisconnectBlocks(uint32(*rollbackHeight), bestHeight)
			if err != nil {
				log.Fatalf("rollbackHeight: %v", err)
			}
		}
		return
	}

	if *resync {
		if err := resyncIndex(rpc, db); err != nil {
			log.Fatalf("resyncIndex %v", err)
		}
	}

	var httpServer *server.HttpServer
	if *httpServerBinding != "" {
		httpServer, err = server.New(*httpServerBinding, db)
		if err != nil {
			log.Fatalf("https: %v", err)
		}
		go func() {
			err = httpServer.Run()
			if err != nil {
				log.Fatalf("https: %v", err)
			}
		}()
	}

	var mq *bitcoin.MQ
	if *zeroMQBinding != "" {
		mq, err = bitcoin.New(*zeroMQBinding, mqHandler)
		if err != nil {
			log.Fatalf("mq: %v", err)
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
			script, err := bitcoin.AddressToOutputScript(address)
			if err != nil {
				log.Fatalf("GetTransactions %v", err)
			}
			if err = db.GetTransactions(script, height, until, printResult); err != nil {
				log.Fatalf("GetTransactions %v", err)
			}
		} else if !*resync {
			if err = connectBlocksParallel(
				rpc,
				db,
				height,
				until,
				*syncChunk,
				*syncWorkers,
			); err != nil {
				log.Fatalf("connectBlocksParallel %v", err)
			}
		}
	}

	if httpServer != nil {
		waitForSignalAndShutdown(httpServer, mq, 5*time.Second)
	}
}

func mqHandler(m *bitcoin.MQMessage) {
	body := hex.EncodeToString(m.Body)
	log.Printf("MQ: %s-%d  %s", m.Topic, m.Sequence, body)
}

func waitForSignalAndShutdown(s *server.HttpServer, mq *bitcoin.MQ, timeout time.Duration) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	sig := <-stop

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Shutdown: %v", sig)

	if mq != nil {
		if err := mq.Shutdown(); err != nil {
			log.Printf("MQ.Shutdown error: %v", err)
		}
	}

	if s != nil {
		if err := s.Shutdown(ctx); err != nil {
			log.Printf("HttpServer.Shutdown error: %v", err)
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
	localBestHeight, local, err := index.GetBestBlock()
	if err != nil {
		local = ""
	}

	// If the locally indexed block is the same as the best block on the
	// network, we're done.
	if local == remote {
		log.Printf("resync: synced on %d %s", localBestHeight, local)
		return nil
	}

	var header *bitcoin.BlockHeader
	if local != "" {
		// Is local tip on the best chain?
		header, err = chain.GetBlockHeader(local)
		forked := false
		if err != nil {
			if e, ok := err.(*bitcoin.RPCError); ok && e.Message == "Block not found" {
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
			// find and disconnect forked blocks and then synchronize again
			log.Printf("resync: local is forked")
			var height uint32
			for height = localBestHeight - 1; height >= 0; height-- {
				local, err = index.GetBlockHash(height)
				if err != nil {
					return err
				}
				remote, err = chain.GetBlockHash(height)
				if err != nil {
					return err
				}
				if local == remote {
					break
				}
			}
			err = index.DisconnectBlocks(height+1, localBestHeight)
			if err != nil {
				return err
			}
			return resyncIndex(chain, index)
		}
	}

	startHeight := uint32(0)
	var hash string
	if header != nil {
		log.Printf("resync: local is behind")
		hash = header.Next
		startHeight = localBestHeight
	} else {
		// If the local block is missing, we're indexing from the genesis block
		// or from the start block specified by flags
		if *blockHeight > 0 {
			startHeight = uint32(*blockHeight)
		}
		log.Printf("resync: genesis from block %d", startHeight)
		hash, err = chain.GetBlockHash(startHeight)
		if err != nil {
			return err
		}
	}

	// if parallel operation is enabled and the number of blocks to be connected is large,
	// use parallel routine to load majority of blocks
	if *syncWorkers > 1 {
		chainBestHeight, err := chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if chainBestHeight-startHeight > uint32(*syncChunk) {
			log.Printf("resync: parallel sync of blocks %d-%d", startHeight, chainBestHeight)
			err = connectBlocksParallel(
				chain,
				index,
				startHeight,
				chainBestHeight,
				*syncChunk,
				*syncWorkers,
			)
			if err != nil {
				return err
			}
			// after parallel load finish the sync using standard way,
			// new blocks may have been created in the meantime
			return resyncIndex(chain, index)
		}
	}

	return connectBlocks(chain, index, hash)
}

func connectBlocks(
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
				if e, ok := err.(*bitcoin.RPCError); ok && (e.Message == "Block height out of range" || e.Message == "Block not found") {
					break
				}
				log.Fatalf("connectBlocksParallel %d-%d %v", low, high, err)
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
		// if higher is over the best block, continue with lower block, otherwise return error
		if e, ok := err.(*bitcoin.RPCError); !ok || e.Message != "Block height out of range" {
			return err
		}
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
	block *bitcoin.Block
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
