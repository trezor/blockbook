package main

import (
	"encoding/hex"
	"log"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

// BitcoinRPC is an interface to JSON-RPC bitcoind service.
type BitcoinRPC struct {
	client  JSONRPC
	Parser  BlockParser
	txCache *lru.Cache
}

// NewBitcoinRPC returns new BitcoinRPC instance.
func NewBitcoinRPC(url string, user string, password string, timeout time.Duration) *BitcoinRPC {
	return &BitcoinRPC{
		client: JSONRPC{
			Client:   http.Client{Timeout: timeout},
			URL:      url,
			User:     user,
			Password: password,
		},
	}
}

// EnableCache turns on LRU caching for transaction lookups.
func (b *BitcoinRPC) EnableCache(size int) error {
	c, err := lru.New(size)
	if err != nil {
		return err
	}
	b.txCache = c
	return nil
}

// ClearCache purges the cache used for transaction results.
func (b *BitcoinRPC) ClearCache() {
	if b.txCache != nil {
		b.txCache.Purge()
	}
}

// GetBlock returns information about the block with the given hash.
func (b *BitcoinRPC) GetBlock(hash string) (block *Block, err error) {
	if b.Parser != nil {
		return b.GetBlockAndParse(hash)
	} else {
		return b.GetParsedBlock(hash)
	}
}

// GetBlockAndParse returns information about the block with the given hash.
//
// It downloads raw block and parses it in-process.
func (b *BitcoinRPC) GetBlockAndParse(hash string) (block *Block, err error) {
	log.Printf("rpc: getblock (verbose=false) %v", hash)
	var header BlockHeader
	err = b.client.Call("getblockheader", &header, hash)
	if err != nil {
		return
	}
	var raw string
	err = b.client.Call("getblock", &raw, hash, false) // verbose=false
	if err != nil {
		return
	}
	data, err := hex.DecodeString(raw)
	if err != nil {
		return
	}
	block, err = b.Parser.ParseBlock(data)
	if err == nil {
		block.Hash = header.Hash
		block.Height = header.Height
		block.Next = header.Next
	}
	return
}

// GetParsedBlock returns information about the block with the given hash.
//
// It downloads parsed block with transaction IDs and then looks them up,
// one by one.
func (b *BitcoinRPC) GetParsedBlock(hash string) (block *Block, err error) {
	log.Printf("rpc: getblock (verbose=true) %v", hash)
	block = &Block{}
	err = b.client.Call("getblock", block, hash, true) // verbose=true
	if err != nil {
		return
	}
	for _, txid := range block.Txids {
		tx, err := b.GetTransaction(txid)
		if err != nil {
			return nil, err
		}
		block.Txs = append(block.Txs, tx)
	}
	return
}

// GetBestBlockHash returns hash of the tip of the best-block-chain.
func (b *BitcoinRPC) GetBestBlockHash() (hash string, err error) {
	log.Printf("rpc: getbestblockhash")
	err = b.client.Call("getbestblockhash", &hash)
	return
}

// GetBlockHash returns hash of block in best-block-chain at given height.
func (b *BitcoinRPC) GetBlockHash(height uint32) (hash string, err error) {
	log.Printf("rpc: getblockhash %v", height)
	err = b.client.Call("getblockhash", &hash, height)
	return
}

// GetTransaction returns the number of blocks in the longest chain.  If the
// transaction cache is turned on, returned Tx.Confirmations is stale.
func (b *BitcoinRPC) GetTransaction(txid string) (tx *Tx, err error) {
	if b.txCache != nil {
		if cachedTx, ok := b.txCache.Get(txid); ok {
			tx = cachedTx.(*Tx)
			return
		}
	}
	log.Printf("rpc: getrawtransaction %v", txid)
	tx = &Tx{}
	err = b.client.Call("getrawtransaction", tx, txid, true) // verbose = true
	if b.txCache != nil {
		b.txCache.Add(txid, tx)
	}
	return
}

// GetOutpointAddresses returns all unique addresses from given transaction output.
func (b *BitcoinRPC) GetOutpointAddresses(txid string, vout uint32) ([]string, error) {
	tx, err := b.GetTransaction(txid)
	if err != nil {
		return nil, err
	}
	return tx.Vout[vout].ScriptPubKey.Addresses, nil
}
