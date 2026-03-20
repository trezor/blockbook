package dbtestdata

import (
	"encoding/json"
	"errors"
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

// ResolveENS resolves an ENS name to an Ethereum address
func (c *fakeBlockChainEthereumType) ResolveENS(name string) (*bchain.ENSResolution, error) {
	switch name {
	case "vitalik.eth":
		return &bchain.ENSResolution{
			Name:    name,
			Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		}, nil
	case "expired.eth":
		return nil, errors.New("ENS name expired")
	case "nonexistent.eth":
		return nil, errors.New("ENS name not found")
	case "address7b.eth":
		return &bchain.ENSResolution{
			Name:    name,
			Address: "0x7B62EB7fe80350DC7EC945C0B73242cb9877FB1b",
		}, nil
	case "address20.eth":
		return &bchain.ENSResolution{
			Name:    name,
			Address: "0x20cD153de35D469BA46127A0C8F18626b59a256A",
		}, nil
	default:
		if !isValidENSName(name) {
			return nil, errors.New("invalid ENS name")
		}
		// For any other valid ENS name, return a mock address
		return &bchain.ENSResolution{
			Name:    name,
			Address: "0x" + name + "abcd1234567890abcdef1234567890abcdef12",
		}, nil
	}
}

// ReverseResolveENS resolves an Ethereum address to an ENS name (reverse lookup)
func (c *fakeBlockChainEthereumType) ReverseResolveENS(address string) (*bchain.ENSResolution, error) {
	// Normalize address to checksummed format for comparison
	normalizedAddr := normalizeAddress(address)

	switch normalizedAddr {
	case "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045":
		return &bchain.ENSResolution{
			Name:    "vitalik.eth",
			Address: normalizedAddr,
		}, nil
	case "0x7B62EB7fe80350DC7EC945C0B73242cb9877FB1b":
		return &bchain.ENSResolution{
			Name:    "address7b.eth",
			Address: normalizedAddr,
		}, nil
	case "0x20cD153de35D469BA46127A0C8F18626b59a256A":
		return &bchain.ENSResolution{
			Name:    "address20.eth",
			Address: normalizedAddr,
		}, nil
	default:
		// No ENS name found for this address
		return nil, errors.New("no ENS name found for address")
	}
}

func (c *fakeBlockChainEthereumType) CheckENSExpiration(name string) (bool, error) {
	switch name {
	case "vitalik.eth":
		return false, nil // Not expired
	case "expired.eth":
		return true, nil // Expired
	case "nonexistent.eth":
		return false, nil // Not expired (doesn't exist)
	case "address7b.eth":
		return false, nil // Not expired
	case "address20.eth":
		return false, nil // Not expired
	default:
		if !isValidENSName(name) {
			return false, errors.New("invalid ENS name")
		}
		return false, nil // Not expired by default
	}
}

func isValidENSName(name string) bool {
	if name == "" {
		return false
	}

	return len(name) > 4 && name[len(name)-4:] == ".eth"
}

// normalizeAddress converts an Ethereum address to a consistent format
// This is a simple implementation that converts to lowercase and ensures 0x prefix
func normalizeAddress(address string) string {
	// Remove 0x prefix if present
	if len(address) > 2 && address[:2] == "0x" {
		address = address[2:]
	}

	// Convert to lowercase
	address = toLower(address)

	// Add 0x prefix back
	return "0x" + address
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + ('a' - 'A')
		} else {
			result[i] = c
		}
	}
	return string(result)
}
