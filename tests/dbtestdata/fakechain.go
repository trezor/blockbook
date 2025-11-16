package dbtestdata

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/trezor/blockbook/bchain"
)

type fakeBlockChain struct {
	*bchain.BaseChain
}

// NewFakeBlockChain returns mocked blockchain RPC interface used for tests
func NewFakeBlockChain(parser bchain.BlockChainParser) (bchain.BlockChain, error) {
	return &fakeBlockChain{&bchain.BaseChain{Parser: parser}}, nil
}

func (c *fakeBlockChain) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	return bchain.NewMempoolBitcoinType(chain, 1, 1, 0, "", false), nil
}

func (c *fakeBlockChain) Initialize() error {
	return nil
}

func (c *fakeBlockChain) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
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
		Bestblockhash: GetTestBitcoinTypeBlock2(c.Parser).BlockHeader.Hash,
		Version:       "001001",
		Subversion:    c.GetSubversion(),
	}, nil
}

func (c *fakeBlockChain) GetBestBlockHash() (v string, err error) {
	return GetTestBitcoinTypeBlock2(c.Parser).BlockHeader.Hash, nil
}

func (c *fakeBlockChain) GetBestBlockHeight() (v uint32, err error) {
	return GetTestBitcoinTypeBlock2(c.Parser).BlockHeader.Height, nil
}

func (c *fakeBlockChain) GetBlockHash(height uint32) (v string, err error) {
	b1 := GetTestBitcoinTypeBlock1(c.Parser)
	if height == b1.BlockHeader.Height {
		return b1.BlockHeader.Hash, nil
	}
	b2 := GetTestBitcoinTypeBlock2(c.Parser)
	if height == b2.BlockHeader.Height {
		return b2.BlockHeader.Hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

func (c *fakeBlockChain) GetBlockHeader(hash string) (v *bchain.BlockHeader, err error) {
	b1 := GetTestBitcoinTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash {
		return &b1.BlockHeader, nil
	}
	b2 := GetTestBitcoinTypeBlock2(c.Parser)
	if hash == b2.BlockHeader.Hash {
		return &b2.BlockHeader, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChain) GetBlock(hash string, height uint32) (v *bchain.Block, err error) {
	b1 := GetTestBitcoinTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash || height == b1.BlockHeader.Height {
		return b1, nil
	}
	b2 := GetTestBitcoinTypeBlock2(c.Parser)
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
	b1 := GetTestBitcoinTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash {
		return getBlockInfo(b1), nil
	}
	b2 := GetTestBitcoinTypeBlock2(c.Parser)
	if hash == b2.BlockHeader.Hash {
		return getBlockInfo(b2), nil
	}
	return nil, bchain.ErrBlockNotFound
}

func getTxInBlock(b *bchain.Block, txid string) *bchain.Tx {
	for _, tx := range b.Txs {
		if tx.Txid == txid {
			return &tx
		}
	}
	return nil
}

func (c *fakeBlockChain) GetBlockRaw(hash string) (string, error) {
	return "00e0ff3fd42677a86f1515bafcf9802c1765e02226655a9b97fd44132602000000000000", nil
}

func (c *fakeBlockChain) GetTransaction(txid string) (v *bchain.Tx, err error) {
	v = getTxInBlock(GetTestBitcoinTypeBlock1(c.Parser), txid)
	if v == nil {
		v = getTxInBlock(GetTestBitcoinTypeBlock2(c.Parser), txid)
	}
	if v != nil {
		return v, nil
	}
	return nil, bchain.ErrTxNotFound
}

func (c *fakeBlockChain) GetTransactionSpecific(tx *bchain.Tx) (v json.RawMessage, err error) {
	// txSpecific extends Tx with an additional Size and Vsize info
	type txSpecific struct {
		*bchain.Tx
		Vsize int `json:"vsize,omitempty"`
		Size  int `json:"size,omitempty"`
	}

	tx, err = c.GetTransaction(tx.Txid)
	if err != nil {
		return nil, err
	}
	txS := txSpecific{Tx: tx}

	if tx.Txid == "7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" {
		txS.Vsize = 206
		txS.Size = 376
	} else if tx.Txid == "fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" {
		txS.Size = 300
	} else if tx.Txid == "05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07" {
		txS.Hex = "010000000001012720b597ef06045c935960342b0bbc45aab5fd5642017282f5110216caaa2364010000002322002069dae530beb09a05d46d0b2aee98645b15bb5d1e808a386b5ef0c48aed5531cbffffffff021ec403000000000017a914203c9dbd3ffbd1a790fc1609fb430efa5cbe516d87061523000000000017a91465dfc5c16e80b86b589df3f85dacd43f5c5b4a8f8704004730440220783e9349fc48f22aa0064acf32bc255eafa761eb9fa8f90a504986713c52dc3702206fc6a1a42f74ea0b416b35671770c0d26fc453668e6107edc271f11e629cda1001483045022100b82ef510c7eec61f39bee3e73a19df451fb8cca842b66bc94696d6a095dd8e96022071767bf8e4859de06cd5caf75e833e284328570ea1caa88bc93478a8d0fa9ac90147522103958c08660082c9ce90399ded0da7c3b39ed20a7767160f12428191e005aa42572102b1e6d8187f54d83d1ffd70508e24c5bd3603bccb2346d8c6677434169de8bc2652ae00000000"
	} else if tx.Txid == "3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" {
		txS.Vsize = 400
	}

	rm, err := json.Marshal(txS)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(rm), nil
}

func (c *fakeBlockChain) GetTransactionForMempool(txid string) (v *bchain.Tx, err error) {
	return nil, errors.New("Not implemented")
}

func (c *fakeBlockChain) EstimateSmartFee(blocks int, conservative bool) (v big.Int, err error) {
	if !conservative {
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

func (c *fakeBlockChain) SendRawTransaction(tx string, disableAlternativeRPC bool) (v string, err error) {
	if tx == "123456" {
		return "9876", nil
	}
	return "", errors.New("Invalid data")
}

// GetChainParser returns parser for the blockchain
func (c *fakeBlockChain) GetChainParser() bchain.BlockChainParser {
	return c.Parser
}

// GetMempoolTransactions returns transactions in mempool
func (c *fakeBlockChain) GetMempoolTransactions() ([]string, error) {
	return nil, errors.New("Not implemented")
}
