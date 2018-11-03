package dbtestdata

import (
	"blockbook/bchain"
	"context"
	"encoding/json"
	"errors"
	"math/big"
)

type fakeBlockChain struct {
	parser bchain.BlockChainParser
}

func NewFakeBlockChain(parser bchain.BlockChainParser) (*fakeBlockChain, error) {
	return &fakeBlockChain{parser: parser}, nil
}

func (c *fakeBlockChain) Initialize() error {
	return nil
}

func (c *fakeBlockChain) Shutdown(ctx context.Context) error {
	return nil
}

func (c *fakeBlockChain) IsTestnet() bool {
	return true
}

func (c *fakeBlockChain) GetNetworkName() string {
	return "fakecoin"
}

func (c *fakeBlockChain) GetCoinName() string {
	return "Fakecoin"
}

func (c *fakeBlockChain) GetSubversion() string {
	return "/Fakecoin:0.0.1/"
}

func (c *fakeBlockChain) GetChainInfo() (v *bchain.ChainInfo, err error) {
	return &bchain.ChainInfo{
		Chain:         c.GetNetworkName(),
		Blocks:        2,
		Headers:       2,
		Bestblockhash: GetTestUTXOBlock2(c.parser).BlockHeader.Hash,
		Version:       "001001",
		Subversion:    c.GetSubversion(),
	}, nil
}

func (c *fakeBlockChain) GetBestBlockHash() (v string, err error) {
	return GetTestUTXOBlock2(c.parser).BlockHeader.Hash, nil
}

func (c *fakeBlockChain) GetBestBlockHeight() (v uint32, err error) {
	return GetTestUTXOBlock2(c.parser).BlockHeader.Height, nil
}

func (c *fakeBlockChain) GetBlockHash(height uint32) (v string, err error) {
	b1 := GetTestUTXOBlock1(c.parser)
	if height == b1.BlockHeader.Height {
		return b1.BlockHeader.Hash, nil
	}
	b2 := GetTestUTXOBlock2(c.parser)
	if height == b2.BlockHeader.Height {
		return b2.BlockHeader.Hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

func (c *fakeBlockChain) GetBlockHeader(hash string) (v *bchain.BlockHeader, err error) {
	b1 := GetTestUTXOBlock1(c.parser)
	if hash == b1.BlockHeader.Hash {
		return &b1.BlockHeader, nil
	}
	b2 := GetTestUTXOBlock2(c.parser)
	if hash == b2.BlockHeader.Hash {
		return &b2.BlockHeader, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChain) GetBlock(hash string, height uint32) (v *bchain.Block, err error) {
	b1 := GetTestUTXOBlock1(c.parser)
	if hash == b1.BlockHeader.Hash || height == b1.BlockHeader.Height {
		return b1, nil
	}
	b2 := GetTestUTXOBlock2(c.parser)
	if hash == b2.BlockHeader.Hash || height == b2.BlockHeader.Height {
		return b2, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func getBlockInfo(b *bchain.Block) *bchain.BlockInfo {
	bi := &bchain.BlockInfo{
		BlockHeader: b.BlockHeader,
	}
	for _, tx := range b.Txs {
		bi.Txids = append(bi.Txids, tx.Txid)
	}
	return bi
}

func (c *fakeBlockChain) GetBlockInfo(hash string) (v *bchain.BlockInfo, err error) {
	b1 := GetTestUTXOBlock1(c.parser)
	if hash == b1.BlockHeader.Hash {
		return getBlockInfo(b1), nil
	}
	b2 := GetTestUTXOBlock2(c.parser)
	if hash == b2.BlockHeader.Hash {
		return getBlockInfo(b2), nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChain) GetMempool() (v []string, err error) {
	return nil, errors.New("Not implemented")
}

func getTxInBlock(b *bchain.Block, txid string) *bchain.Tx {
	for _, tx := range b.Txs {
		if tx.Txid == txid {
			return &tx
		}
	}
	return nil
}

func (c *fakeBlockChain) GetTransaction(txid string) (v *bchain.Tx, err error) {
	v = getTxInBlock(GetTestUTXOBlock1(c.parser), txid)
	if v == nil {
		v = getTxInBlock(GetTestUTXOBlock2(c.parser), txid)
	}
	if v != nil {
		return v, nil
	}
	return nil, errors.New("Not found")
}

func (c *fakeBlockChain) GetTransactionSpecific(txid string) (v json.RawMessage, err error) {
	tx, err := c.GetTransaction(txid)
	if err != nil {
		return nil, err
	}
	rm, err := json.Marshal(tx)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(rm), nil
}

func (c *fakeBlockChain) GetTransactionForMempool(txid string) (v *bchain.Tx, err error) {
	return nil, errors.New("Not implemented")
}

func (c *fakeBlockChain) EstimateSmartFee(blocks int, conservative bool) (v big.Int, err error) {
	if conservative == false {
		v.SetInt64(int64(blocks)*100 - 1)
	} else {
		v.SetInt64(int64(blocks) * 100)
	}
	return
}

func (c *fakeBlockChain) EstimateFee(blocks int) (v big.Int, err error) {
	v.SetInt64(int64(blocks) * 200)
	return
}

func (c *fakeBlockChain) SendRawTransaction(tx string) (v string, err error) {
	if tx == "123456" {
		return "9876", nil
	}
	return "", errors.New("Invalid data")
}

func (c *fakeBlockChain) ResyncMempool(onNewTxAddr bchain.OnNewTxAddrFunc) (count int, err error) {
	return 0, errors.New("Not implemented")
}

func (c *fakeBlockChain) GetMempoolTransactions(address string) (v []string, err error) {
	return nil, errors.New("Not implemented")
}

func (c *fakeBlockChain) GetMempoolTransactionsForAddrDesc(addrDesc bchain.AddressDescriptor) (v []string, err error) {
	return []string{}, nil
}

func (c *fakeBlockChain) GetMempoolEntry(txid string) (v *bchain.MempoolEntry, err error) {
	return nil, errors.New("Not implemented")
}

func (c *fakeBlockChain) GetChainParser() bchain.BlockChainParser {
	return c.parser
}
