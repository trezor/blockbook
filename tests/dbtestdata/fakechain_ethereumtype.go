package dbtestdata

import (
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/trezor/blockbook/bchain"
)

type fakeBlockChainEthereumType struct {
	*fakeBlockChain
}

// NewFakeBlockChainEthereumType returns mocked blockchain RPC interface used for tests
func NewFakeBlockChainEthereumType(parser bchain.BlockChainParser) (bchain.BlockChain, error) {
	return &fakeBlockChainEthereumType{&fakeBlockChain{&bchain.BaseChain{Parser: parser}}}, nil
}

func (c *fakeBlockChainEthereumType) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	return bchain.NewMempoolEthereumType(chain, 1, false), nil
}

func (c *fakeBlockChainEthereumType) GetChainInfo() (v *bchain.ChainInfo, err error) {
	return &bchain.ChainInfo{
		Chain:         c.GetNetworkName(),
		Blocks:        2,
		Headers:       2,
		Bestblockhash: GetTestEthereumTypeBlock2(c.Parser).BlockHeader.Hash,
		Version:       "001001",
		Subversion:    c.GetSubversion(),
	}, nil
}

func (c *fakeBlockChainEthereumType) GetBestBlockHash() (v string, err error) {
	return GetTestEthereumTypeBlock2(c.Parser).BlockHeader.Hash, nil
}

func (c *fakeBlockChainEthereumType) GetBestBlockHeight() (v uint32, err error) {
	return GetTestEthereumTypeBlock2(c.Parser).BlockHeader.Height, nil
}

func (c *fakeBlockChainEthereumType) GetBlockHash(height uint32) (v string, err error) {
	b1 := GetTestEthereumTypeBlock1(c.Parser)
	if height == b1.BlockHeader.Height {
		return b1.BlockHeader.Hash, nil
	}
	b2 := GetTestEthereumTypeBlock2(c.Parser)
	if height == b2.BlockHeader.Height {
		return b2.BlockHeader.Hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

func (c *fakeBlockChainEthereumType) GetBlockHeader(hash string) (v *bchain.BlockHeader, err error) {
	b1 := GetTestEthereumTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash {
		return &b1.BlockHeader, nil
	}
	b2 := GetTestEthereumTypeBlock2(c.Parser)
	if hash == b2.BlockHeader.Hash {
		return &b2.BlockHeader, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChainEthereumType) GetBlock(hash string, height uint32) (v *bchain.Block, err error) {
	b1 := GetTestEthereumTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash || height == b1.BlockHeader.Height {
		return b1, nil
	}
	b2 := GetTestEthereumTypeBlock2(c.Parser)
	if hash == b2.BlockHeader.Hash || height == b2.BlockHeader.Height {
		return b2, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChainEthereumType) GetBlockInfo(hash string) (v *bchain.BlockInfo, err error) {
	b1 := GetTestEthereumTypeBlock1(c.Parser)
	if hash == b1.BlockHeader.Hash {
		return getBlockInfo(b1), nil
	}
	b2 := GetTestEthereumTypeBlock2(c.Parser)
	if hash == b2.BlockHeader.Hash {
		return getBlockInfo(b2), nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChainEthereumType) GetTransaction(txid string) (v *bchain.Tx, err error) {
	v = getTxInBlock(GetTestEthereumTypeBlock1(c.Parser), txid)
	if v == nil {
		v = getTxInBlock(GetTestEthereumTypeBlock2(c.Parser), txid)
	}
	if v != nil {
		return v, nil
	}
	return nil, bchain.ErrTxNotFound
}

func (c *fakeBlockChainEthereumType) GetTransactionSpecific(tx *bchain.Tx) (v json.RawMessage, err error) {
	txS, _ := tx.CoinSpecificData.(bchain.EthereumSpecificData)

	rm, err := json.Marshal(txS)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(rm), nil
}

func (c *fakeBlockChainEthereumType) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	return big.NewInt(123450000 + int64(addrDesc[0])), nil
}

func (c *fakeBlockChainEthereumType) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	return uint64(addrDesc[0]), nil
}

func (c *fakeBlockChainEthereumType) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	addresses, _, _ := c.Parser.GetAddressesFromAddrDesc(contractDesc)
	return &bchain.ContractInfo{
		Standard:       bchain.ERC20TokenStandard,
		Contract:       addresses[0],
		Name:           "Contract " + strconv.Itoa(int(contractDesc[0])),
		Symbol:         "S" + strconv.Itoa(int(contractDesc[0])),
		Decimals:       18,
		CreatedInBlock: 12345,
	}, nil
}

// EthereumTypeGetErc20ContractBalance returns simulated balance
func (c *fakeBlockChainEthereumType) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	return big.NewInt(1000000000 + int64(addrDesc[0])*1000 + int64(contractDesc[0])), nil
}

// EthereumTypeRpcCall calls eth_call with given data and to address
func (c *fakeBlockChainEthereumType) EthereumTypeRpcCall(data, to, from string) (string, error) {
	return data + "abcd", nil
}

// EthereumTypeGetRawTransaction returns simulated transaction hex data
func (c *fakeBlockChainEthereumType) EthereumTypeGetRawTransaction(txid string) (string, error) {
	return txid + "abcd", nil
}

// GetTokenURI returns URI derived from the input contractDesc
func (c *fakeBlockChainEthereumType) GetTokenURI(contractDesc bchain.AddressDescriptor, tokenID *big.Int) (string, error) {
	return "https://ipfs.io/ipfs/" + contractDesc.String()[3:] + ".json", nil
}
