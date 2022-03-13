package bchain

import (
	"errors"
	"math/big"
)

// BaseChain is base type for bchain.BlockChain
type BaseChain struct {
	Parser  BlockChainParser
	Testnet bool
	Network string
}

// TODO more bchain.BlockChain methods

// GetChainParser returns BlockChainParser
func (b *BaseChain) GetChainParser() BlockChainParser {
	return b.Parser
}

// IsTestnet returns true if the blockchain is testnet
func (b *BaseChain) IsTestnet() bool {
	return b.Testnet
}

// GetNetworkName returns network name
func (b *BaseChain) GetNetworkName() string {
	return b.Network
}

// GetBlockRaw is not supported by default
func (b *BaseChain) GetBlockRaw(hash string) (string, error) {
	return "", errors.New("GetBlockRaw: not supported")
}

// GetMempoolEntry is not supported by default
func (b *BaseChain) GetMempoolEntry(txid string) (*MempoolEntry, error) {
	return nil, errors.New("GetMempoolEntry: not supported")
}

// EthereumTypeGetBalance is not supported
func (b *BaseChain) EthereumTypeGetBalance(addrDesc AddressDescriptor) (*big.Int, error) {
	return nil, errors.New("Not supported")
}

// EthereumTypeGetNonce is not supported
func (b *BaseChain) EthereumTypeGetNonce(addrDesc AddressDescriptor) (uint64, error) {
	return 0, errors.New("Not supported")
}

// EthereumTypeEstimateGas is not supported
func (b *BaseChain) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	return 0, errors.New("Not supported")
}

// EthereumTypeGetErc20ContractInfo is not supported
func (b *BaseChain) EthereumTypeGetErc20ContractInfo(contractDesc AddressDescriptor) (*Erc20Contract, error) {
	return nil, errors.New("Not supported")
}

// EthereumTypeGetErc20ContractBalance is not supported
func (b *BaseChain) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc AddressDescriptor) (*big.Int, error) {
	return nil, errors.New("Not supported")
}
