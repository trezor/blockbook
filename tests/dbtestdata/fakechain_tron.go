package dbtestdata

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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

func normalizeHexID(id string) string {
	id = strings.ToLower(id)
	id = strings.TrimPrefix(id, "0x")
	return id
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
	if normalizeHexID(hash) == normalizeHexID(b.BlockHeader.Hash) {
		return &b.BlockHeader, nil
	}
	return nil, bchain.ErrBlockNotFound
}

// GetBlock
func (c *fakeBlockChainTronType) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	b1 := GetTestTronBlock0(c.Parser)
	if normalizeHexID(hash) == normalizeHexID(b1.BlockHeader.Hash) || height == b1.BlockHeader.Height {
		return b1, nil
	}
	b2 := GetTestTronBlock1(c.Parser)
	if normalizeHexID(hash) == normalizeHexID(b2.BlockHeader.Hash) || height == b2.BlockHeader.Height {
		return b2, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func (c *fakeBlockChainTronType) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	b := GetTestTronBlock1(c.Parser)
	if normalizeHexID(hash) == normalizeHexID(b.BlockHeader.Hash) {
		return getBlockInfo(b), nil
	}
	return nil, bchain.ErrBlockNotFound
}

// GetTransaction
func (c *fakeBlockChainTronType) GetTransaction(txid string) (*bchain.Tx, error) {
	blk := GetTestTronBlock1(c.Parser)
	normTxid := normalizeHexID(txid)
	var t *bchain.Tx
	for i := range blk.Txs {
		if normalizeHexID(blk.Txs[i].Txid) == normTxid {
			t = &blk.Txs[i]
			break
		}
	}
	if t == nil {
		return nil, bchain.ErrTxNotFound
	}
	return t, nil
}

func (c *fakeBlockChainTronType) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	txS, err := c.GetTransaction(tx.Txid)
	if err != nil {
		return nil, err
	}
	csd, ok := txS.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return nil, bchain.ErrTxNotFound
	}
	csdCopy := csd
	if csd.Tx != nil {
		txCopy := *csd.Tx
		txCopy.Hash = normalizeHexID(txCopy.Hash)
		txCopy.BlockHash = normalizeHexID(txCopy.BlockHash)
		csdCopy.Tx = &txCopy
	}
	rm, err := json.Marshal(&csdCopy)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(rm), nil
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

func (c *fakeBlockChainTronType) GetAddressChainExtraData(addrDesc bchain.AddressDescriptor) (json.RawMessage, error) {
	seed := int64(0)
	if len(addrDesc) > 0 {
		seed = int64(addrDesc[0])
	}

	payload, err := json.Marshal(&bchain.TronAccountExtraData{
		AvailableBandwidth: seed,
		TotalBandwidth:     seed + 1000,
		AvailableEnergy:    seed * 100,
		TotalEnergy:        seed*100 + 10000,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

// EthereumTypeRpcCall validates address parameters similarly to Tron RPC and accepts both Base58 and hex.
func (c *fakeBlockChainTronType) EthereumTypeRpcCall(data, to, from string) (string, error) {
	type tronAddressNormalizer interface {
		FromTronAddressToHex(addr string) (string, error)
	}
	parser, ok := c.Parser.(tronAddressNormalizer)
	if !ok {
		return "", errors.New("tron parser does not support address normalization")
	}
	if to != "" {
		if _, err := parser.FromTronAddressToHex(to); err != nil {
			return "", err
		}
	}
	if from != "" {
		if _, err := parser.FromTronAddressToHex(from); err != nil {
			return "", err
		}
	}
	return data + "abcd", nil
}
