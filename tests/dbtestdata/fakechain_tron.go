package dbtestdata

import (
	"strconv"

	"github.com/trezor/blockbook/bchain"
)

// fakeBlockChainTronType
type fakeBlockChainTronType struct {
	*fakeBlockChainEthereumType
}

// redefine token-standards to avoid circular dependency when importing "tron" package
const (
	TRC20TokenType   bchain.TokenStandardName = "TRC20"
	TRC721TokenType  bchain.TokenStandardName = "TRC721"
	TRC1155TokenType bchain.TokenStandardName = "TRC1155"
)

// NewFakeBlockChainTronType
func NewFakeBlockChainTronType(parser bchain.BlockChainParser) (bchain.BlockChain, error) {
	bchain.EthereumTokenStandardMap = []bchain.TokenStandardName{TRC20TokenType, TRC721TokenType, TRC1155TokenType}

	return &fakeBlockChainTronType{
		fakeBlockChainEthereumType: &fakeBlockChainEthereumType{&fakeBlockChain{&bchain.BaseChain{Parser: parser}}},
	}, nil
}

// GetChainInfo
func (c *fakeBlockChainTronType) GetChainInfo() (*bchain.ChainInfo, error) {
	return &bchain.ChainInfo{
		Chain:         c.GetNetworkName(),
		Blocks:        2,
		Headers:       2,
		Bestblockhash: GetTestTronBlock1(c.Parser).BlockHeader.Hash,
		Version:       "tron_test_1.0",
		Subversion:    "MockTron",
	}, nil
}

// GetBestBlockHash
func (c *fakeBlockChainTronType) GetBestBlockHash() (string, error) {
	return GetTestTronBlock1(c.Parser).BlockHeader.Hash, nil
}

// GetBestBlockHeight
func (c *fakeBlockChainTronType) GetBestBlockHeight() (uint32, error) {
	return GetTestTronBlock1(c.Parser).BlockHeader.Height, nil
}

// GetBlockHash
func (c *fakeBlockChainTronType) GetBlockHash(height uint32) (string, error) {
	b := GetTestTronBlock1(c.Parser)
	if height == b.BlockHeader.Height {
		return b.BlockHeader.Hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

// GetBlockHeader
func (c *fakeBlockChainTronType) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	b := GetTestTronBlock1(c.Parser)
	if hash == b.BlockHeader.Hash {
		return &b.BlockHeader, nil
	}
	return nil, bchain.ErrBlockNotFound
}

// GetBlock
func (c *fakeBlockChainTronType) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	b1 := GetTestTronBlock0(c.Parser)
	if hash == b1.BlockHeader.Hash || height == b1.BlockHeader.Height {
		return b1, nil
	}
	b2 := GetTestTronBlock1(c.Parser)
	if hash == b2.BlockHeader.Hash || height == b2.BlockHeader.Height {
		return b2, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChainTronType) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	b := GetTestTronBlock1(c.Parser)
	if hash == b.BlockHeader.Hash {
		return getBlockInfo(b), nil
	}
	return nil, bchain.ErrBlockNotFound
}

// GetTransaction
func (c *fakeBlockChainTronType) GetTransaction(txid string) (*bchain.Tx, error) {
	blk := GetTestTronBlock1(c.Parser)
	t := getTxInBlock(blk, txid)
	if t == nil {
		return nil, bchain.ErrTxNotFound
	}
	return t, nil
}

func (c *fakeBlockChainTronType) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	addresses, _, _ := c.Parser.GetAddressesFromAddrDesc(contractDesc)
	return &bchain.ContractInfo{
		Standard:       TRC20TokenType,
		Contract:       addresses[0],
		Name:           "TronTestContract" + strconv.Itoa(int(contractDesc[0])),
		Symbol:         "TRC" + strconv.Itoa(int(contractDesc[0])),
		Decimals:       6,
		CreatedInBlock: 1000,
	}, nil
}
