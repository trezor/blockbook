// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package bchain

import (
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// TokenhubABI is the input ABI used to generate the binding from.
const TokenhubABI = "[{\"inputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"string\",\"name\":\"key\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"value\",\"type\":\"bytes\"}],\"name\":\"paramChange\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"receiveDeposit\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"bep20Addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"refundAddr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"status\",\"type\":\"uint32\"}],\"name\":\"refundFailure\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"bep20Addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"refundAddr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"status\",\"type\":\"uint32\"}],\"name\":\"refundSuccess\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"rewardTo\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"bep20Addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"refundAddr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"transferInSuccess\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"bep20Addr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"senderAddr\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"relayFee\",\"type\":\"uint256\"}],\"name\":\"transferOutSuccess\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint8\",\"name\":\"channelId\",\"type\":\"uint8\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"msgBytes\",\"type\":\"bytes\"}],\"name\":\"unexpectedPackage\",\"type\":\"event\"},{\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"fallback\"},{\"constant\":true,\"inputs\":[],\"name\":\"BEP2_TOKEN_DECIMALS\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"BEP2_TOKEN_SYMBOL_FOR_BNB\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"BIND_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"CODE_OK\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"CROSS_CHAIN_CONTRACT_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"ERROR_FAIL_DECODE\",\"outputs\":[{\"internalType\":\"uint32\",\"name\":\"\",\"type\":\"uint32\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"GOV_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"GOV_HUB_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"INCENTIVIZE_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"INIT_MINIMUM_RELAY_FEE\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"LIGHT_CLIENT_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"MAXIMUM_BEP20_SYMBOL_LEN\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"MAX_BEP2_TOTAL_SUPPLY\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"MAX_GAS_FOR_CALLING_BEP20\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"MAX_GAS_FOR_TRANSFER_BNB\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"MINIMUM_BEP20_SYMBOL_LEN\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"RELAYERHUB_CONTRACT_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"REWARD_UPPER_LIMIT\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"SLASH_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"SLASH_CONTRACT_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"STAKING_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"SYSTEM_REWARD_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TEN_DECIMALS\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TOKEN_HUB_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TOKEN_MANAGER_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_FAILURE_INSUFFICIENT_BALANCE\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_FAILURE_NON_PAYABLE_RECIPIENT\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_FAILURE_TIMEOUT\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_FAILURE_UNBOUND_TOKEN\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_FAILURE_UNKNOWN\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_IN_SUCCESS\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"TRANSFER_OUT_CHANNELID\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"VALIDATOR_CONTRACT_ADDR\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"alreadyInit\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"bep20ContractDecimals\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"bscChainID\",\"outputs\":[{\"internalType\":\"uint16\",\"name\":\"\",\"type\":\"uint16\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"relayFee\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"init\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"addresspayable\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"claimRewards\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getMiniRelayFee\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"channelId\",\"type\":\"uint8\"},{\"internalType\":\"bytes\",\"name\":\"msgBytes\",\"type\":\"bytes\"}],\"name\":\"handleSynPackage\",\"outputs\":[{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"channelId\",\"type\":\"uint8\"},{\"internalType\":\"bytes\",\"name\":\"msgBytes\",\"type\":\"bytes\"}],\"name\":\"handleAckPackage\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"channelId\",\"type\":\"uint8\"},{\"internalType\":\"bytes\",\"name\":\"msgBytes\",\"type\":\"bytes\"}],\"name\":\"handleFailAckPackage\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"address\",\"name\":\"contractAddr\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"expireTime\",\"type\":\"uint64\"}],\"name\":\"transferOut\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"address[]\",\"name\":\"recipientAddrs\",\"type\":\"address[]\"},{\"internalType\":\"uint256[]\",\"name\":\"amounts\",\"type\":\"uint256[]\"},{\"internalType\":\"address[]\",\"name\":\"refundAddrs\",\"type\":\"address[]\"},{\"internalType\":\"uint64\",\"name\":\"expireTime\",\"type\":\"uint64\"}],\"name\":\"batchTransferOutBNB\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"string\",\"name\":\"key\",\"type\":\"string\"},{\"internalType\":\"bytes\",\"name\":\"value\",\"type\":\"bytes\"}],\"name\":\"updateParam\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"bep2Symbol\",\"type\":\"bytes32\"}],\"name\":\"getContractAddrByBEP2Symbol\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"internalType\":\"address\",\"name\":\"contractAddr\",\"type\":\"address\"}],\"name\":\"getBep2SymbolByContractAddr\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"bep2Symbol\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"contractAddr\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"decimals\",\"type\":\"uint256\"}],\"name\":\"bindToken\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"bep2Symbol\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"contractAddr\",\"type\":\"address\"}],\"name\":\"unbindToken\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"internalType\":\"string\",\"name\":\"bep2Symbol\",\"type\":\"string\"}],\"name\":\"getBoundContract\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"internalType\":\"address\",\"name\":\"contractAddr\",\"type\":\"address\"}],\"name\":\"getBoundBep2Symbol\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"}]"

// Tokenhub is an auto generated Go binding around an Ethereum contract.
type Tokenhub struct {
	TokenhubCaller     // Read-only binding to the contract
	TokenhubTransactor // Write-only binding to the contract
	TokenhubFilterer   // Log filterer for contract events
}

// TokenhubCaller is an auto generated read-only Go binding around an Ethereum contract.
type TokenhubCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TokenhubTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TokenhubTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TokenhubFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TokenhubFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TokenhubSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TokenhubSession struct {
	Contract     *Tokenhub         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TokenhubCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TokenhubCallerSession struct {
	Contract *TokenhubCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// TokenhubTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TokenhubTransactorSession struct {
	Contract     *TokenhubTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// TokenhubRaw is an auto generated low-level Go binding around an Ethereum contract.
type TokenhubRaw struct {
	Contract *Tokenhub // Generic contract binding to access the raw methods on
}

// TokenhubCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TokenhubCallerRaw struct {
	Contract *TokenhubCaller // Generic read-only contract binding to access the raw methods on
}

// TokenhubTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TokenhubTransactorRaw struct {
	Contract *TokenhubTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTokenhub creates a new instance of Tokenhub, bound to a specific deployed contract.
func NewTokenhub(address common.Address, backend bind.ContractBackend) (*Tokenhub, error) {
	contract, err := bindTokenhub(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Tokenhub{TokenhubCaller: TokenhubCaller{contract: contract}, TokenhubTransactor: TokenhubTransactor{contract: contract}, TokenhubFilterer: TokenhubFilterer{contract: contract}}, nil
}

// NewTokenhubCaller creates a new read-only instance of Tokenhub, bound to a specific deployed contract.
func NewTokenhubCaller(address common.Address, caller bind.ContractCaller) (*TokenhubCaller, error) {
	contract, err := bindTokenhub(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TokenhubCaller{contract: contract}, nil
}

// NewTokenhubTransactor creates a new write-only instance of Tokenhub, bound to a specific deployed contract.
func NewTokenhubTransactor(address common.Address, transactor bind.ContractTransactor) (*TokenhubTransactor, error) {
	contract, err := bindTokenhub(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TokenhubTransactor{contract: contract}, nil
}

// NewTokenhubFilterer creates a new log filterer instance of Tokenhub, bound to a specific deployed contract.
func NewTokenhubFilterer(address common.Address, filterer bind.ContractFilterer) (*TokenhubFilterer, error) {
	contract, err := bindTokenhub(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TokenhubFilterer{contract: contract}, nil
}

// bindTokenhub binds a generic wrapper to an already deployed contract.
func bindTokenhub(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(TokenhubABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Tokenhub *TokenhubRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _Tokenhub.Contract.TokenhubCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Tokenhub *TokenhubRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Tokenhub.Contract.TokenhubTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Tokenhub *TokenhubRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Tokenhub.Contract.TokenhubTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Tokenhub *TokenhubCallerRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _Tokenhub.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Tokenhub *TokenhubTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Tokenhub.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Tokenhub *TokenhubTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Tokenhub.Contract.contract.Transact(opts, method, params...)
}

// BEP2TOKENDECIMALS is a free data retrieval call binding the contract method 0x61368475.
//
// Solidity: function BEP2_TOKEN_DECIMALS() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) BEP2TOKENDECIMALS(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "BEP2_TOKEN_DECIMALS")
	return *ret0, err
}

// BEP2TOKENDECIMALS is a free data retrieval call binding the contract method 0x61368475.
//
// Solidity: function BEP2_TOKEN_DECIMALS() constant returns(uint8)
func (_Tokenhub *TokenhubSession) BEP2TOKENDECIMALS() (uint8, error) {
	return _Tokenhub.Contract.BEP2TOKENDECIMALS(&_Tokenhub.CallOpts)
}

// BEP2TOKENDECIMALS is a free data retrieval call binding the contract method 0x61368475.
//
// Solidity: function BEP2_TOKEN_DECIMALS() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) BEP2TOKENDECIMALS() (uint8, error) {
	return _Tokenhub.Contract.BEP2TOKENDECIMALS(&_Tokenhub.CallOpts)
}

// BEP2TOKENSYMBOLFORBNB is a free data retrieval call binding the contract method 0xb9fd21e3.
//
// Solidity: function BEP2_TOKEN_SYMBOL_FOR_BNB() constant returns(bytes32)
func (_Tokenhub *TokenhubCaller) BEP2TOKENSYMBOLFORBNB(opts *bind.CallOpts) ([32]byte, error) {
	var (
		ret0 = new([32]byte)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "BEP2_TOKEN_SYMBOL_FOR_BNB")
	return *ret0, err
}

// BEP2TOKENSYMBOLFORBNB is a free data retrieval call binding the contract method 0xb9fd21e3.
//
// Solidity: function BEP2_TOKEN_SYMBOL_FOR_BNB() constant returns(bytes32)
func (_Tokenhub *TokenhubSession) BEP2TOKENSYMBOLFORBNB() ([32]byte, error) {
	return _Tokenhub.Contract.BEP2TOKENSYMBOLFORBNB(&_Tokenhub.CallOpts)
}

// BEP2TOKENSYMBOLFORBNB is a free data retrieval call binding the contract method 0xb9fd21e3.
//
// Solidity: function BEP2_TOKEN_SYMBOL_FOR_BNB() constant returns(bytes32)
func (_Tokenhub *TokenhubCallerSession) BEP2TOKENSYMBOLFORBNB() ([32]byte, error) {
	return _Tokenhub.Contract.BEP2TOKENSYMBOLFORBNB(&_Tokenhub.CallOpts)
}

// BINDCHANNELID is a free data retrieval call binding the contract method 0x3dffc387.
//
// Solidity: function BIND_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) BINDCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "BIND_CHANNELID")
	return *ret0, err
}

// BINDCHANNELID is a free data retrieval call binding the contract method 0x3dffc387.
//
// Solidity: function BIND_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) BINDCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.BINDCHANNELID(&_Tokenhub.CallOpts)
}

// BINDCHANNELID is a free data retrieval call binding the contract method 0x3dffc387.
//
// Solidity: function BIND_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) BINDCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.BINDCHANNELID(&_Tokenhub.CallOpts)
}

// CODEOK is a free data retrieval call binding the contract method 0xab51bb96.
//
// Solidity: function CODE_OK() constant returns(uint32)
func (_Tokenhub *TokenhubCaller) CODEOK(opts *bind.CallOpts) (uint32, error) {
	var (
		ret0 = new(uint32)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "CODE_OK")
	return *ret0, err
}

// CODEOK is a free data retrieval call binding the contract method 0xab51bb96.
//
// Solidity: function CODE_OK() constant returns(uint32)
func (_Tokenhub *TokenhubSession) CODEOK() (uint32, error) {
	return _Tokenhub.Contract.CODEOK(&_Tokenhub.CallOpts)
}

// CODEOK is a free data retrieval call binding the contract method 0xab51bb96.
//
// Solidity: function CODE_OK() constant returns(uint32)
func (_Tokenhub *TokenhubCallerSession) CODEOK() (uint32, error) {
	return _Tokenhub.Contract.CODEOK(&_Tokenhub.CallOpts)
}

// CROSSCHAINCONTRACTADDR is a free data retrieval call binding the contract method 0x51e80672.
//
// Solidity: function CROSS_CHAIN_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) CROSSCHAINCONTRACTADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "CROSS_CHAIN_CONTRACT_ADDR")
	return *ret0, err
}

// CROSSCHAINCONTRACTADDR is a free data retrieval call binding the contract method 0x51e80672.
//
// Solidity: function CROSS_CHAIN_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) CROSSCHAINCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.CROSSCHAINCONTRACTADDR(&_Tokenhub.CallOpts)
}

// CROSSCHAINCONTRACTADDR is a free data retrieval call binding the contract method 0x51e80672.
//
// Solidity: function CROSS_CHAIN_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) CROSSCHAINCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.CROSSCHAINCONTRACTADDR(&_Tokenhub.CallOpts)
}

// ERRORFAILDECODE is a free data retrieval call binding the contract method 0x0bee7a67.
//
// Solidity: function ERROR_FAIL_DECODE() constant returns(uint32)
func (_Tokenhub *TokenhubCaller) ERRORFAILDECODE(opts *bind.CallOpts) (uint32, error) {
	var (
		ret0 = new(uint32)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "ERROR_FAIL_DECODE")
	return *ret0, err
}

// ERRORFAILDECODE is a free data retrieval call binding the contract method 0x0bee7a67.
//
// Solidity: function ERROR_FAIL_DECODE() constant returns(uint32)
func (_Tokenhub *TokenhubSession) ERRORFAILDECODE() (uint32, error) {
	return _Tokenhub.Contract.ERRORFAILDECODE(&_Tokenhub.CallOpts)
}

// ERRORFAILDECODE is a free data retrieval call binding the contract method 0x0bee7a67.
//
// Solidity: function ERROR_FAIL_DECODE() constant returns(uint32)
func (_Tokenhub *TokenhubCallerSession) ERRORFAILDECODE() (uint32, error) {
	return _Tokenhub.Contract.ERRORFAILDECODE(&_Tokenhub.CallOpts)
}

// GOVCHANNELID is a free data retrieval call binding the contract method 0x96713da9.
//
// Solidity: function GOV_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) GOVCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "GOV_CHANNELID")
	return *ret0, err
}

// GOVCHANNELID is a free data retrieval call binding the contract method 0x96713da9.
//
// Solidity: function GOV_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) GOVCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.GOVCHANNELID(&_Tokenhub.CallOpts)
}

// GOVCHANNELID is a free data retrieval call binding the contract method 0x96713da9.
//
// Solidity: function GOV_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) GOVCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.GOVCHANNELID(&_Tokenhub.CallOpts)
}

// GOVHUBADDR is a free data retrieval call binding the contract method 0x9dc09262.
//
// Solidity: function GOV_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) GOVHUBADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "GOV_HUB_ADDR")
	return *ret0, err
}

// GOVHUBADDR is a free data retrieval call binding the contract method 0x9dc09262.
//
// Solidity: function GOV_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) GOVHUBADDR() (common.Address, error) {
	return _Tokenhub.Contract.GOVHUBADDR(&_Tokenhub.CallOpts)
}

// GOVHUBADDR is a free data retrieval call binding the contract method 0x9dc09262.
//
// Solidity: function GOV_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) GOVHUBADDR() (common.Address, error) {
	return _Tokenhub.Contract.GOVHUBADDR(&_Tokenhub.CallOpts)
}

// INCENTIVIZEADDR is a free data retrieval call binding the contract method 0x6e47b482.
//
// Solidity: function INCENTIVIZE_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) INCENTIVIZEADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "INCENTIVIZE_ADDR")
	return *ret0, err
}

// INCENTIVIZEADDR is a free data retrieval call binding the contract method 0x6e47b482.
//
// Solidity: function INCENTIVIZE_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) INCENTIVIZEADDR() (common.Address, error) {
	return _Tokenhub.Contract.INCENTIVIZEADDR(&_Tokenhub.CallOpts)
}

// INCENTIVIZEADDR is a free data retrieval call binding the contract method 0x6e47b482.
//
// Solidity: function INCENTIVIZE_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) INCENTIVIZEADDR() (common.Address, error) {
	return _Tokenhub.Contract.INCENTIVIZEADDR(&_Tokenhub.CallOpts)
}

// INITMINIMUMRELAYFEE is a free data retrieval call binding the contract method 0x50432d32.
//
// Solidity: function INIT_MINIMUM_RELAY_FEE() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) INITMINIMUMRELAYFEE(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "INIT_MINIMUM_RELAY_FEE")
	return *ret0, err
}

// INITMINIMUMRELAYFEE is a free data retrieval call binding the contract method 0x50432d32.
//
// Solidity: function INIT_MINIMUM_RELAY_FEE() constant returns(uint256)
func (_Tokenhub *TokenhubSession) INITMINIMUMRELAYFEE() (*big.Int, error) {
	return _Tokenhub.Contract.INITMINIMUMRELAYFEE(&_Tokenhub.CallOpts)
}

// INITMINIMUMRELAYFEE is a free data retrieval call binding the contract method 0x50432d32.
//
// Solidity: function INIT_MINIMUM_RELAY_FEE() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) INITMINIMUMRELAYFEE() (*big.Int, error) {
	return _Tokenhub.Contract.INITMINIMUMRELAYFEE(&_Tokenhub.CallOpts)
}

// LIGHTCLIENTADDR is a free data retrieval call binding the contract method 0xdc927faf.
//
// Solidity: function LIGHT_CLIENT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) LIGHTCLIENTADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "LIGHT_CLIENT_ADDR")
	return *ret0, err
}

// LIGHTCLIENTADDR is a free data retrieval call binding the contract method 0xdc927faf.
//
// Solidity: function LIGHT_CLIENT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) LIGHTCLIENTADDR() (common.Address, error) {
	return _Tokenhub.Contract.LIGHTCLIENTADDR(&_Tokenhub.CallOpts)
}

// LIGHTCLIENTADDR is a free data retrieval call binding the contract method 0xdc927faf.
//
// Solidity: function LIGHT_CLIENT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) LIGHTCLIENTADDR() (common.Address, error) {
	return _Tokenhub.Contract.LIGHTCLIENTADDR(&_Tokenhub.CallOpts)
}

// MAXIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0xd9e6dae9.
//
// Solidity: function MAXIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) MAXIMUMBEP20SYMBOLLEN(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "MAXIMUM_BEP20_SYMBOL_LEN")
	return *ret0, err
}

// MAXIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0xd9e6dae9.
//
// Solidity: function MAXIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubSession) MAXIMUMBEP20SYMBOLLEN() (uint8, error) {
	return _Tokenhub.Contract.MAXIMUMBEP20SYMBOLLEN(&_Tokenhub.CallOpts)
}

// MAXIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0xd9e6dae9.
//
// Solidity: function MAXIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) MAXIMUMBEP20SYMBOLLEN() (uint8, error) {
	return _Tokenhub.Contract.MAXIMUMBEP20SYMBOLLEN(&_Tokenhub.CallOpts)
}

// MAXBEP2TOTALSUPPLY is a free data retrieval call binding the contract method 0x9a854bbd.
//
// Solidity: function MAX_BEP2_TOTAL_SUPPLY() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) MAXBEP2TOTALSUPPLY(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "MAX_BEP2_TOTAL_SUPPLY")
	return *ret0, err
}

// MAXBEP2TOTALSUPPLY is a free data retrieval call binding the contract method 0x9a854bbd.
//
// Solidity: function MAX_BEP2_TOTAL_SUPPLY() constant returns(uint256)
func (_Tokenhub *TokenhubSession) MAXBEP2TOTALSUPPLY() (*big.Int, error) {
	return _Tokenhub.Contract.MAXBEP2TOTALSUPPLY(&_Tokenhub.CallOpts)
}

// MAXBEP2TOTALSUPPLY is a free data retrieval call binding the contract method 0x9a854bbd.
//
// Solidity: function MAX_BEP2_TOTAL_SUPPLY() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) MAXBEP2TOTALSUPPLY() (*big.Int, error) {
	return _Tokenhub.Contract.MAXBEP2TOTALSUPPLY(&_Tokenhub.CallOpts)
}

// MAXGASFORCALLINGBEP20 is a free data retrieval call binding the contract method 0xba35ead6.
//
// Solidity: function MAX_GAS_FOR_CALLING_BEP20() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) MAXGASFORCALLINGBEP20(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "MAX_GAS_FOR_CALLING_BEP20")
	return *ret0, err
}

// MAXGASFORCALLINGBEP20 is a free data retrieval call binding the contract method 0xba35ead6.
//
// Solidity: function MAX_GAS_FOR_CALLING_BEP20() constant returns(uint256)
func (_Tokenhub *TokenhubSession) MAXGASFORCALLINGBEP20() (*big.Int, error) {
	return _Tokenhub.Contract.MAXGASFORCALLINGBEP20(&_Tokenhub.CallOpts)
}

// MAXGASFORCALLINGBEP20 is a free data retrieval call binding the contract method 0xba35ead6.
//
// Solidity: function MAX_GAS_FOR_CALLING_BEP20() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) MAXGASFORCALLINGBEP20() (*big.Int, error) {
	return _Tokenhub.Contract.MAXGASFORCALLINGBEP20(&_Tokenhub.CallOpts)
}

// MAXGASFORTRANSFERBNB is a free data retrieval call binding the contract method 0xfa9e9159.
//
// Solidity: function MAX_GAS_FOR_TRANSFER_BNB() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) MAXGASFORTRANSFERBNB(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "MAX_GAS_FOR_TRANSFER_BNB")
	return *ret0, err
}

// MAXGASFORTRANSFERBNB is a free data retrieval call binding the contract method 0xfa9e9159.
//
// Solidity: function MAX_GAS_FOR_TRANSFER_BNB() constant returns(uint256)
func (_Tokenhub *TokenhubSession) MAXGASFORTRANSFERBNB() (*big.Int, error) {
	return _Tokenhub.Contract.MAXGASFORTRANSFERBNB(&_Tokenhub.CallOpts)
}

// MAXGASFORTRANSFERBNB is a free data retrieval call binding the contract method 0xfa9e9159.
//
// Solidity: function MAX_GAS_FOR_TRANSFER_BNB() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) MAXGASFORTRANSFERBNB() (*big.Int, error) {
	return _Tokenhub.Contract.MAXGASFORTRANSFERBNB(&_Tokenhub.CallOpts)
}

// MINIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0x66dea52a.
//
// Solidity: function MINIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) MINIMUMBEP20SYMBOLLEN(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "MINIMUM_BEP20_SYMBOL_LEN")
	return *ret0, err
}

// MINIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0x66dea52a.
//
// Solidity: function MINIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubSession) MINIMUMBEP20SYMBOLLEN() (uint8, error) {
	return _Tokenhub.Contract.MINIMUMBEP20SYMBOLLEN(&_Tokenhub.CallOpts)
}

// MINIMUMBEP20SYMBOLLEN is a free data retrieval call binding the contract method 0x66dea52a.
//
// Solidity: function MINIMUM_BEP20_SYMBOL_LEN() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) MINIMUMBEP20SYMBOLLEN() (uint8, error) {
	return _Tokenhub.Contract.MINIMUMBEP20SYMBOLLEN(&_Tokenhub.CallOpts)
}

// RELAYERHUBCONTRACTADDR is a free data retrieval call binding the contract method 0xa1a11bf5.
//
// Solidity: function RELAYERHUB_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) RELAYERHUBCONTRACTADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "RELAYERHUB_CONTRACT_ADDR")
	return *ret0, err
}

// RELAYERHUBCONTRACTADDR is a free data retrieval call binding the contract method 0xa1a11bf5.
//
// Solidity: function RELAYERHUB_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) RELAYERHUBCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.RELAYERHUBCONTRACTADDR(&_Tokenhub.CallOpts)
}

// RELAYERHUBCONTRACTADDR is a free data retrieval call binding the contract method 0xa1a11bf5.
//
// Solidity: function RELAYERHUB_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) RELAYERHUBCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.RELAYERHUBCONTRACTADDR(&_Tokenhub.CallOpts)
}

// REWARDUPPERLIMIT is a free data retrieval call binding the contract method 0x43a368b9.
//
// Solidity: function REWARD_UPPER_LIMIT() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) REWARDUPPERLIMIT(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "REWARD_UPPER_LIMIT")
	return *ret0, err
}

// REWARDUPPERLIMIT is a free data retrieval call binding the contract method 0x43a368b9.
//
// Solidity: function REWARD_UPPER_LIMIT() constant returns(uint256)
func (_Tokenhub *TokenhubSession) REWARDUPPERLIMIT() (*big.Int, error) {
	return _Tokenhub.Contract.REWARDUPPERLIMIT(&_Tokenhub.CallOpts)
}

// REWARDUPPERLIMIT is a free data retrieval call binding the contract method 0x43a368b9.
//
// Solidity: function REWARD_UPPER_LIMIT() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) REWARDUPPERLIMIT() (*big.Int, error) {
	return _Tokenhub.Contract.REWARDUPPERLIMIT(&_Tokenhub.CallOpts)
}

// SLASHCHANNELID is a free data retrieval call binding the contract method 0x7942fd05.
//
// Solidity: function SLASH_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) SLASHCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "SLASH_CHANNELID")
	return *ret0, err
}

// SLASHCHANNELID is a free data retrieval call binding the contract method 0x7942fd05.
//
// Solidity: function SLASH_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) SLASHCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.SLASHCHANNELID(&_Tokenhub.CallOpts)
}

// SLASHCHANNELID is a free data retrieval call binding the contract method 0x7942fd05.
//
// Solidity: function SLASH_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) SLASHCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.SLASHCHANNELID(&_Tokenhub.CallOpts)
}

// SLASHCONTRACTADDR is a free data retrieval call binding the contract method 0x43756e5c.
//
// Solidity: function SLASH_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) SLASHCONTRACTADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "SLASH_CONTRACT_ADDR")
	return *ret0, err
}

// SLASHCONTRACTADDR is a free data retrieval call binding the contract method 0x43756e5c.
//
// Solidity: function SLASH_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) SLASHCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.SLASHCONTRACTADDR(&_Tokenhub.CallOpts)
}

// SLASHCONTRACTADDR is a free data retrieval call binding the contract method 0x43756e5c.
//
// Solidity: function SLASH_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) SLASHCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.SLASHCONTRACTADDR(&_Tokenhub.CallOpts)
}

// STAKINGCHANNELID is a free data retrieval call binding the contract method 0x4bf6c882.
//
// Solidity: function STAKING_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) STAKINGCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "STAKING_CHANNELID")
	return *ret0, err
}

// STAKINGCHANNELID is a free data retrieval call binding the contract method 0x4bf6c882.
//
// Solidity: function STAKING_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) STAKINGCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.STAKINGCHANNELID(&_Tokenhub.CallOpts)
}

// STAKINGCHANNELID is a free data retrieval call binding the contract method 0x4bf6c882.
//
// Solidity: function STAKING_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) STAKINGCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.STAKINGCHANNELID(&_Tokenhub.CallOpts)
}

// SYSTEMREWARDADDR is a free data retrieval call binding the contract method 0xc81b1662.
//
// Solidity: function SYSTEM_REWARD_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) SYSTEMREWARDADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "SYSTEM_REWARD_ADDR")
	return *ret0, err
}

// SYSTEMREWARDADDR is a free data retrieval call binding the contract method 0xc81b1662.
//
// Solidity: function SYSTEM_REWARD_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) SYSTEMREWARDADDR() (common.Address, error) {
	return _Tokenhub.Contract.SYSTEMREWARDADDR(&_Tokenhub.CallOpts)
}

// SYSTEMREWARDADDR is a free data retrieval call binding the contract method 0xc81b1662.
//
// Solidity: function SYSTEM_REWARD_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) SYSTEMREWARDADDR() (common.Address, error) {
	return _Tokenhub.Contract.SYSTEMREWARDADDR(&_Tokenhub.CallOpts)
}

// TENDECIMALS is a free data retrieval call binding the contract method 0x5d499b1b.
//
// Solidity: function TEN_DECIMALS() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) TENDECIMALS(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TEN_DECIMALS")
	return *ret0, err
}

// TENDECIMALS is a free data retrieval call binding the contract method 0x5d499b1b.
//
// Solidity: function TEN_DECIMALS() constant returns(uint256)
func (_Tokenhub *TokenhubSession) TENDECIMALS() (*big.Int, error) {
	return _Tokenhub.Contract.TENDECIMALS(&_Tokenhub.CallOpts)
}

// TENDECIMALS is a free data retrieval call binding the contract method 0x5d499b1b.
//
// Solidity: function TEN_DECIMALS() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) TENDECIMALS() (*big.Int, error) {
	return _Tokenhub.Contract.TENDECIMALS(&_Tokenhub.CallOpts)
}

// TOKENHUBADDR is a free data retrieval call binding the contract method 0xfd6a6879.
//
// Solidity: function TOKEN_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) TOKENHUBADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TOKEN_HUB_ADDR")
	return *ret0, err
}

// TOKENHUBADDR is a free data retrieval call binding the contract method 0xfd6a6879.
//
// Solidity: function TOKEN_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) TOKENHUBADDR() (common.Address, error) {
	return _Tokenhub.Contract.TOKENHUBADDR(&_Tokenhub.CallOpts)
}

// TOKENHUBADDR is a free data retrieval call binding the contract method 0xfd6a6879.
//
// Solidity: function TOKEN_HUB_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) TOKENHUBADDR() (common.Address, error) {
	return _Tokenhub.Contract.TOKENHUBADDR(&_Tokenhub.CallOpts)
}

// TOKENMANAGERADDR is a free data retrieval call binding the contract method 0x75d47a0a.
//
// Solidity: function TOKEN_MANAGER_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) TOKENMANAGERADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TOKEN_MANAGER_ADDR")
	return *ret0, err
}

// TOKENMANAGERADDR is a free data retrieval call binding the contract method 0x75d47a0a.
//
// Solidity: function TOKEN_MANAGER_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) TOKENMANAGERADDR() (common.Address, error) {
	return _Tokenhub.Contract.TOKENMANAGERADDR(&_Tokenhub.CallOpts)
}

// TOKENMANAGERADDR is a free data retrieval call binding the contract method 0x75d47a0a.
//
// Solidity: function TOKEN_MANAGER_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) TOKENMANAGERADDR() (common.Address, error) {
	return _Tokenhub.Contract.TOKENMANAGERADDR(&_Tokenhub.CallOpts)
}

// TRANSFERINCHANNELID is a free data retrieval call binding the contract method 0x70fd5bad.
//
// Solidity: function TRANSFER_IN_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_CHANNELID")
	return *ret0, err
}

// TRANSFERINCHANNELID is a free data retrieval call binding the contract method 0x70fd5bad.
//
// Solidity: function TRANSFER_IN_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINCHANNELID(&_Tokenhub.CallOpts)
}

// TRANSFERINCHANNELID is a free data retrieval call binding the contract method 0x70fd5bad.
//
// Solidity: function TRANSFER_IN_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINCHANNELID(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREINSUFFICIENTBALANCE is a free data retrieval call binding the contract method 0xa7c9f02d.
//
// Solidity: function TRANSFER_IN_FAILURE_INSUFFICIENT_BALANCE() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINFAILUREINSUFFICIENTBALANCE(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_FAILURE_INSUFFICIENT_BALANCE")
	return *ret0, err
}

// TRANSFERINFAILUREINSUFFICIENTBALANCE is a free data retrieval call binding the contract method 0xa7c9f02d.
//
// Solidity: function TRANSFER_IN_FAILURE_INSUFFICIENT_BALANCE() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINFAILUREINSUFFICIENTBALANCE() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREINSUFFICIENTBALANCE(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREINSUFFICIENTBALANCE is a free data retrieval call binding the contract method 0xa7c9f02d.
//
// Solidity: function TRANSFER_IN_FAILURE_INSUFFICIENT_BALANCE() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINFAILUREINSUFFICIENTBALANCE() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREINSUFFICIENTBALANCE(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILURENONPAYABLERECIPIENT is a free data retrieval call binding the contract method 0xebf71d53.
//
// Solidity: function TRANSFER_IN_FAILURE_NON_PAYABLE_RECIPIENT() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINFAILURENONPAYABLERECIPIENT(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_FAILURE_NON_PAYABLE_RECIPIENT")
	return *ret0, err
}

// TRANSFERINFAILURENONPAYABLERECIPIENT is a free data retrieval call binding the contract method 0xebf71d53.
//
// Solidity: function TRANSFER_IN_FAILURE_NON_PAYABLE_RECIPIENT() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINFAILURENONPAYABLERECIPIENT() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILURENONPAYABLERECIPIENT(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILURENONPAYABLERECIPIENT is a free data retrieval call binding the contract method 0xebf71d53.
//
// Solidity: function TRANSFER_IN_FAILURE_NON_PAYABLE_RECIPIENT() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINFAILURENONPAYABLERECIPIENT() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILURENONPAYABLERECIPIENT(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILURETIMEOUT is a free data retrieval call binding the contract method 0x8b87b21f.
//
// Solidity: function TRANSFER_IN_FAILURE_TIMEOUT() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINFAILURETIMEOUT(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_FAILURE_TIMEOUT")
	return *ret0, err
}

// TRANSFERINFAILURETIMEOUT is a free data retrieval call binding the contract method 0x8b87b21f.
//
// Solidity: function TRANSFER_IN_FAILURE_TIMEOUT() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINFAILURETIMEOUT() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILURETIMEOUT(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILURETIMEOUT is a free data retrieval call binding the contract method 0x8b87b21f.
//
// Solidity: function TRANSFER_IN_FAILURE_TIMEOUT() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINFAILURETIMEOUT() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILURETIMEOUT(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREUNBOUNDTOKEN is a free data retrieval call binding the contract method 0xff9c0027.
//
// Solidity: function TRANSFER_IN_FAILURE_UNBOUND_TOKEN() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINFAILUREUNBOUNDTOKEN(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_FAILURE_UNBOUND_TOKEN")
	return *ret0, err
}

// TRANSFERINFAILUREUNBOUNDTOKEN is a free data retrieval call binding the contract method 0xff9c0027.
//
// Solidity: function TRANSFER_IN_FAILURE_UNBOUND_TOKEN() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINFAILUREUNBOUNDTOKEN() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREUNBOUNDTOKEN(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREUNBOUNDTOKEN is a free data retrieval call binding the contract method 0xff9c0027.
//
// Solidity: function TRANSFER_IN_FAILURE_UNBOUND_TOKEN() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINFAILUREUNBOUNDTOKEN() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREUNBOUNDTOKEN(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREUNKNOWN is a free data retrieval call binding the contract method 0xf0148472.
//
// Solidity: function TRANSFER_IN_FAILURE_UNKNOWN() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINFAILUREUNKNOWN(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_FAILURE_UNKNOWN")
	return *ret0, err
}

// TRANSFERINFAILUREUNKNOWN is a free data retrieval call binding the contract method 0xf0148472.
//
// Solidity: function TRANSFER_IN_FAILURE_UNKNOWN() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINFAILUREUNKNOWN() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREUNKNOWN(&_Tokenhub.CallOpts)
}

// TRANSFERINFAILUREUNKNOWN is a free data retrieval call binding the contract method 0xf0148472.
//
// Solidity: function TRANSFER_IN_FAILURE_UNKNOWN() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINFAILUREUNKNOWN() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINFAILUREUNKNOWN(&_Tokenhub.CallOpts)
}

// TRANSFERINSUCCESS is a free data retrieval call binding the contract method 0xa496fba2.
//
// Solidity: function TRANSFER_IN_SUCCESS() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFERINSUCCESS(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_IN_SUCCESS")
	return *ret0, err
}

// TRANSFERINSUCCESS is a free data retrieval call binding the contract method 0xa496fba2.
//
// Solidity: function TRANSFER_IN_SUCCESS() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFERINSUCCESS() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINSUCCESS(&_Tokenhub.CallOpts)
}

// TRANSFERINSUCCESS is a free data retrieval call binding the contract method 0xa496fba2.
//
// Solidity: function TRANSFER_IN_SUCCESS() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFERINSUCCESS() (uint8, error) {
	return _Tokenhub.Contract.TRANSFERINSUCCESS(&_Tokenhub.CallOpts)
}

// TRANSFEROUTCHANNELID is a free data retrieval call binding the contract method 0xfc3e5908.
//
// Solidity: function TRANSFER_OUT_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCaller) TRANSFEROUTCHANNELID(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "TRANSFER_OUT_CHANNELID")
	return *ret0, err
}

// TRANSFEROUTCHANNELID is a free data retrieval call binding the contract method 0xfc3e5908.
//
// Solidity: function TRANSFER_OUT_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubSession) TRANSFEROUTCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.TRANSFEROUTCHANNELID(&_Tokenhub.CallOpts)
}

// TRANSFEROUTCHANNELID is a free data retrieval call binding the contract method 0xfc3e5908.
//
// Solidity: function TRANSFER_OUT_CHANNELID() constant returns(uint8)
func (_Tokenhub *TokenhubCallerSession) TRANSFEROUTCHANNELID() (uint8, error) {
	return _Tokenhub.Contract.TRANSFEROUTCHANNELID(&_Tokenhub.CallOpts)
}

// VALIDATORCONTRACTADDR is a free data retrieval call binding the contract method 0xf9a2bbc7.
//
// Solidity: function VALIDATOR_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCaller) VALIDATORCONTRACTADDR(opts *bind.CallOpts) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "VALIDATOR_CONTRACT_ADDR")
	return *ret0, err
}

// VALIDATORCONTRACTADDR is a free data retrieval call binding the contract method 0xf9a2bbc7.
//
// Solidity: function VALIDATOR_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubSession) VALIDATORCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.VALIDATORCONTRACTADDR(&_Tokenhub.CallOpts)
}

// VALIDATORCONTRACTADDR is a free data retrieval call binding the contract method 0xf9a2bbc7.
//
// Solidity: function VALIDATOR_CONTRACT_ADDR() constant returns(address)
func (_Tokenhub *TokenhubCallerSession) VALIDATORCONTRACTADDR() (common.Address, error) {
	return _Tokenhub.Contract.VALIDATORCONTRACTADDR(&_Tokenhub.CallOpts)
}

// AlreadyInit is a free data retrieval call binding the contract method 0xa78abc16.
//
// Solidity: function alreadyInit() constant returns(bool)
func (_Tokenhub *TokenhubCaller) AlreadyInit(opts *bind.CallOpts) (bool, error) {
	var (
		ret0 = new(bool)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "alreadyInit")
	return *ret0, err
}

// AlreadyInit is a free data retrieval call binding the contract method 0xa78abc16.
//
// Solidity: function alreadyInit() constant returns(bool)
func (_Tokenhub *TokenhubSession) AlreadyInit() (bool, error) {
	return _Tokenhub.Contract.AlreadyInit(&_Tokenhub.CallOpts)
}

// AlreadyInit is a free data retrieval call binding the contract method 0xa78abc16.
//
// Solidity: function alreadyInit() constant returns(bool)
func (_Tokenhub *TokenhubCallerSession) AlreadyInit() (bool, error) {
	return _Tokenhub.Contract.AlreadyInit(&_Tokenhub.CallOpts)
}

// Bep20ContractDecimals is a free data retrieval call binding the contract method 0xbbface1f.
//
// Solidity: function bep20ContractDecimals(address ) constant returns(uint256)
func (_Tokenhub *TokenhubCaller) Bep20ContractDecimals(opts *bind.CallOpts, arg0 common.Address) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "bep20ContractDecimals", arg0)
	return *ret0, err
}

// Bep20ContractDecimals is a free data retrieval call binding the contract method 0xbbface1f.
//
// Solidity: function bep20ContractDecimals(address ) constant returns(uint256)
func (_Tokenhub *TokenhubSession) Bep20ContractDecimals(arg0 common.Address) (*big.Int, error) {
	return _Tokenhub.Contract.Bep20ContractDecimals(&_Tokenhub.CallOpts, arg0)
}

// Bep20ContractDecimals is a free data retrieval call binding the contract method 0xbbface1f.
//
// Solidity: function bep20ContractDecimals(address ) constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) Bep20ContractDecimals(arg0 common.Address) (*big.Int, error) {
	return _Tokenhub.Contract.Bep20ContractDecimals(&_Tokenhub.CallOpts, arg0)
}

// BscChainID is a free data retrieval call binding the contract method 0x493279b1.
//
// Solidity: function bscChainID() constant returns(uint16)
func (_Tokenhub *TokenhubCaller) BscChainID(opts *bind.CallOpts) (uint16, error) {
	var (
		ret0 = new(uint16)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "bscChainID")
	return *ret0, err
}

// BscChainID is a free data retrieval call binding the contract method 0x493279b1.
//
// Solidity: function bscChainID() constant returns(uint16)
func (_Tokenhub *TokenhubSession) BscChainID() (uint16, error) {
	return _Tokenhub.Contract.BscChainID(&_Tokenhub.CallOpts)
}

// BscChainID is a free data retrieval call binding the contract method 0x493279b1.
//
// Solidity: function bscChainID() constant returns(uint16)
func (_Tokenhub *TokenhubCallerSession) BscChainID() (uint16, error) {
	return _Tokenhub.Contract.BscChainID(&_Tokenhub.CallOpts)
}

// GetBep2SymbolByContractAddr is a free data retrieval call binding the contract method 0xbd466461.
//
// Solidity: function getBep2SymbolByContractAddr(address contractAddr) constant returns(bytes32)
func (_Tokenhub *TokenhubCaller) GetBep2SymbolByContractAddr(opts *bind.CallOpts, contractAddr common.Address) ([32]byte, error) {
	var (
		ret0 = new([32]byte)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "getBep2SymbolByContractAddr", contractAddr)
	return *ret0, err
}

// GetBep2SymbolByContractAddr is a free data retrieval call binding the contract method 0xbd466461.
//
// Solidity: function getBep2SymbolByContractAddr(address contractAddr) constant returns(bytes32)
func (_Tokenhub *TokenhubSession) GetBep2SymbolByContractAddr(contractAddr common.Address) ([32]byte, error) {
	return _Tokenhub.Contract.GetBep2SymbolByContractAddr(&_Tokenhub.CallOpts, contractAddr)
}

// GetBep2SymbolByContractAddr is a free data retrieval call binding the contract method 0xbd466461.
//
// Solidity: function getBep2SymbolByContractAddr(address contractAddr) constant returns(bytes32)
func (_Tokenhub *TokenhubCallerSession) GetBep2SymbolByContractAddr(contractAddr common.Address) ([32]byte, error) {
	return _Tokenhub.Contract.GetBep2SymbolByContractAddr(&_Tokenhub.CallOpts, contractAddr)
}

// GetBoundBep2Symbol is a free data retrieval call binding the contract method 0xfc1a598f.
//
// Solidity: function getBoundBep2Symbol(address contractAddr) constant returns(string)
func (_Tokenhub *TokenhubCaller) GetBoundBep2Symbol(opts *bind.CallOpts, contractAddr common.Address) (string, error) {
	var (
		ret0 = new(string)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "getBoundBep2Symbol", contractAddr)
	return *ret0, err
}

// GetBoundBep2Symbol is a free data retrieval call binding the contract method 0xfc1a598f.
//
// Solidity: function getBoundBep2Symbol(address contractAddr) constant returns(string)
func (_Tokenhub *TokenhubSession) GetBoundBep2Symbol(contractAddr common.Address) (string, error) {
	return _Tokenhub.Contract.GetBoundBep2Symbol(&_Tokenhub.CallOpts, contractAddr)
}

// GetBoundBep2Symbol is a free data retrieval call binding the contract method 0xfc1a598f.
//
// Solidity: function getBoundBep2Symbol(address contractAddr) constant returns(string)
func (_Tokenhub *TokenhubCallerSession) GetBoundBep2Symbol(contractAddr common.Address) (string, error) {
	return _Tokenhub.Contract.GetBoundBep2Symbol(&_Tokenhub.CallOpts, contractAddr)
}

// GetBoundContract is a free data retrieval call binding the contract method 0x3d713223.
//
// Solidity: function getBoundContract(string bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubCaller) GetBoundContract(opts *bind.CallOpts, bep2Symbol string) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "getBoundContract", bep2Symbol)
	return *ret0, err
}

// GetBoundContract is a free data retrieval call binding the contract method 0x3d713223.
//
// Solidity: function getBoundContract(string bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubSession) GetBoundContract(bep2Symbol string) (common.Address, error) {
	return _Tokenhub.Contract.GetBoundContract(&_Tokenhub.CallOpts, bep2Symbol)
}

// GetBoundContract is a free data retrieval call binding the contract method 0x3d713223.
//
// Solidity: function getBoundContract(string bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubCallerSession) GetBoundContract(bep2Symbol string) (common.Address, error) {
	return _Tokenhub.Contract.GetBoundContract(&_Tokenhub.CallOpts, bep2Symbol)
}

// GetContractAddrByBEP2Symbol is a free data retrieval call binding the contract method 0x59b92789.
//
// Solidity: function getContractAddrByBEP2Symbol(bytes32 bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubCaller) GetContractAddrByBEP2Symbol(opts *bind.CallOpts, bep2Symbol [32]byte) (common.Address, error) {
	var (
		ret0 = new(common.Address)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "getContractAddrByBEP2Symbol", bep2Symbol)
	return *ret0, err
}

// GetContractAddrByBEP2Symbol is a free data retrieval call binding the contract method 0x59b92789.
//
// Solidity: function getContractAddrByBEP2Symbol(bytes32 bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubSession) GetContractAddrByBEP2Symbol(bep2Symbol [32]byte) (common.Address, error) {
	return _Tokenhub.Contract.GetContractAddrByBEP2Symbol(&_Tokenhub.CallOpts, bep2Symbol)
}

// GetContractAddrByBEP2Symbol is a free data retrieval call binding the contract method 0x59b92789.
//
// Solidity: function getContractAddrByBEP2Symbol(bytes32 bep2Symbol) constant returns(address)
func (_Tokenhub *TokenhubCallerSession) GetContractAddrByBEP2Symbol(bep2Symbol [32]byte) (common.Address, error) {
	return _Tokenhub.Contract.GetContractAddrByBEP2Symbol(&_Tokenhub.CallOpts, bep2Symbol)
}

// GetMiniRelayFee is a free data retrieval call binding the contract method 0x149d14d9.
//
// Solidity: function getMiniRelayFee() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) GetMiniRelayFee(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "getMiniRelayFee")
	return *ret0, err
}

// GetMiniRelayFee is a free data retrieval call binding the contract method 0x149d14d9.
//
// Solidity: function getMiniRelayFee() constant returns(uint256)
func (_Tokenhub *TokenhubSession) GetMiniRelayFee() (*big.Int, error) {
	return _Tokenhub.Contract.GetMiniRelayFee(&_Tokenhub.CallOpts)
}

// GetMiniRelayFee is a free data retrieval call binding the contract method 0x149d14d9.
//
// Solidity: function getMiniRelayFee() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) GetMiniRelayFee() (*big.Int, error) {
	return _Tokenhub.Contract.GetMiniRelayFee(&_Tokenhub.CallOpts)
}

// RelayFee is a free data retrieval call binding the contract method 0x71d30863.
//
// Solidity: function relayFee() constant returns(uint256)
func (_Tokenhub *TokenhubCaller) RelayFee(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _Tokenhub.contract.Call(opts, out, "relayFee")
	return *ret0, err
}

// RelayFee is a free data retrieval call binding the contract method 0x71d30863.
//
// Solidity: function relayFee() constant returns(uint256)
func (_Tokenhub *TokenhubSession) RelayFee() (*big.Int, error) {
	return _Tokenhub.Contract.RelayFee(&_Tokenhub.CallOpts)
}

// RelayFee is a free data retrieval call binding the contract method 0x71d30863.
//
// Solidity: function relayFee() constant returns(uint256)
func (_Tokenhub *TokenhubCallerSession) RelayFee() (*big.Int, error) {
	return _Tokenhub.Contract.RelayFee(&_Tokenhub.CallOpts)
}

// BatchTransferOutBNB is a paid mutator transaction binding the contract method 0x6e056520.
//
// Solidity: function batchTransferOutBNB(address[] recipientAddrs, uint256[] amounts, address[] refundAddrs, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubTransactor) BatchTransferOutBNB(opts *bind.TransactOpts, recipientAddrs []common.Address, amounts []*big.Int, refundAddrs []common.Address, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "batchTransferOutBNB", recipientAddrs, amounts, refundAddrs, expireTime)
}

// BatchTransferOutBNB is a paid mutator transaction binding the contract method 0x6e056520.
//
// Solidity: function batchTransferOutBNB(address[] recipientAddrs, uint256[] amounts, address[] refundAddrs, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubSession) BatchTransferOutBNB(recipientAddrs []common.Address, amounts []*big.Int, refundAddrs []common.Address, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.Contract.BatchTransferOutBNB(&_Tokenhub.TransactOpts, recipientAddrs, amounts, refundAddrs, expireTime)
}

// BatchTransferOutBNB is a paid mutator transaction binding the contract method 0x6e056520.
//
// Solidity: function batchTransferOutBNB(address[] recipientAddrs, uint256[] amounts, address[] refundAddrs, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubTransactorSession) BatchTransferOutBNB(recipientAddrs []common.Address, amounts []*big.Int, refundAddrs []common.Address, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.Contract.BatchTransferOutBNB(&_Tokenhub.TransactOpts, recipientAddrs, amounts, refundAddrs, expireTime)
}

// BindToken is a paid mutator transaction binding the contract method 0x8eff336c.
//
// Solidity: function bindToken(bytes32 bep2Symbol, address contractAddr, uint256 decimals) returns()
func (_Tokenhub *TokenhubTransactor) BindToken(opts *bind.TransactOpts, bep2Symbol [32]byte, contractAddr common.Address, decimals *big.Int) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "bindToken", bep2Symbol, contractAddr, decimals)
}

// BindToken is a paid mutator transaction binding the contract method 0x8eff336c.
//
// Solidity: function bindToken(bytes32 bep2Symbol, address contractAddr, uint256 decimals) returns()
func (_Tokenhub *TokenhubSession) BindToken(bep2Symbol [32]byte, contractAddr common.Address, decimals *big.Int) (*types.Transaction, error) {
	return _Tokenhub.Contract.BindToken(&_Tokenhub.TransactOpts, bep2Symbol, contractAddr, decimals)
}

// BindToken is a paid mutator transaction binding the contract method 0x8eff336c.
//
// Solidity: function bindToken(bytes32 bep2Symbol, address contractAddr, uint256 decimals) returns()
func (_Tokenhub *TokenhubTransactorSession) BindToken(bep2Symbol [32]byte, contractAddr common.Address, decimals *big.Int) (*types.Transaction, error) {
	return _Tokenhub.Contract.BindToken(&_Tokenhub.TransactOpts, bep2Symbol, contractAddr, decimals)
}

// ClaimRewards is a paid mutator transaction binding the contract method 0x9a99b4f0.
//
// Solidity: function claimRewards(address to, uint256 amount) returns(uint256)
func (_Tokenhub *TokenhubTransactor) ClaimRewards(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "claimRewards", to, amount)
}

// ClaimRewards is a paid mutator transaction binding the contract method 0x9a99b4f0.
//
// Solidity: function claimRewards(address to, uint256 amount) returns(uint256)
func (_Tokenhub *TokenhubSession) ClaimRewards(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Tokenhub.Contract.ClaimRewards(&_Tokenhub.TransactOpts, to, amount)
}

// ClaimRewards is a paid mutator transaction binding the contract method 0x9a99b4f0.
//
// Solidity: function claimRewards(address to, uint256 amount) returns(uint256)
func (_Tokenhub *TokenhubTransactorSession) ClaimRewards(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Tokenhub.Contract.ClaimRewards(&_Tokenhub.TransactOpts, to, amount)
}

// HandleAckPackage is a paid mutator transaction binding the contract method 0x831d65d1.
//
// Solidity: function handleAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubTransactor) HandleAckPackage(opts *bind.TransactOpts, channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "handleAckPackage", channelId, msgBytes)
}

// HandleAckPackage is a paid mutator transaction binding the contract method 0x831d65d1.
//
// Solidity: function handleAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubSession) HandleAckPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleAckPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// HandleAckPackage is a paid mutator transaction binding the contract method 0x831d65d1.
//
// Solidity: function handleAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubTransactorSession) HandleAckPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleAckPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// HandleFailAckPackage is a paid mutator transaction binding the contract method 0xc8509d81.
//
// Solidity: function handleFailAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubTransactor) HandleFailAckPackage(opts *bind.TransactOpts, channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "handleFailAckPackage", channelId, msgBytes)
}

// HandleFailAckPackage is a paid mutator transaction binding the contract method 0xc8509d81.
//
// Solidity: function handleFailAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubSession) HandleFailAckPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleFailAckPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// HandleFailAckPackage is a paid mutator transaction binding the contract method 0xc8509d81.
//
// Solidity: function handleFailAckPackage(uint8 channelId, bytes msgBytes) returns()
func (_Tokenhub *TokenhubTransactorSession) HandleFailAckPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleFailAckPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// HandleSynPackage is a paid mutator transaction binding the contract method 0x1182b875.
//
// Solidity: function handleSynPackage(uint8 channelId, bytes msgBytes) returns(bytes)
func (_Tokenhub *TokenhubTransactor) HandleSynPackage(opts *bind.TransactOpts, channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "handleSynPackage", channelId, msgBytes)
}

// HandleSynPackage is a paid mutator transaction binding the contract method 0x1182b875.
//
// Solidity: function handleSynPackage(uint8 channelId, bytes msgBytes) returns(bytes)
func (_Tokenhub *TokenhubSession) HandleSynPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleSynPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// HandleSynPackage is a paid mutator transaction binding the contract method 0x1182b875.
//
// Solidity: function handleSynPackage(uint8 channelId, bytes msgBytes) returns(bytes)
func (_Tokenhub *TokenhubTransactorSession) HandleSynPackage(channelId uint8, msgBytes []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.HandleSynPackage(&_Tokenhub.TransactOpts, channelId, msgBytes)
}

// Init is a paid mutator transaction binding the contract method 0xe1c7392a.
//
// Solidity: function init() returns()
func (_Tokenhub *TokenhubTransactor) Init(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "init")
}

// Init is a paid mutator transaction binding the contract method 0xe1c7392a.
//
// Solidity: function init() returns()
func (_Tokenhub *TokenhubSession) Init() (*types.Transaction, error) {
	return _Tokenhub.Contract.Init(&_Tokenhub.TransactOpts)
}

// Init is a paid mutator transaction binding the contract method 0xe1c7392a.
//
// Solidity: function init() returns()
func (_Tokenhub *TokenhubTransactorSession) Init() (*types.Transaction, error) {
	return _Tokenhub.Contract.Init(&_Tokenhub.TransactOpts)
}

// TransferOut is a paid mutator transaction binding the contract method 0xaa7415f5.
//
// Solidity: function transferOut(address contractAddr, address recipient, uint256 amount, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubTransactor) TransferOut(opts *bind.TransactOpts, contractAddr common.Address, recipient common.Address, amount *big.Int, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "transferOut", contractAddr, recipient, amount, expireTime)
}

// TransferOut is a paid mutator transaction binding the contract method 0xaa7415f5.
//
// Solidity: function transferOut(address contractAddr, address recipient, uint256 amount, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubSession) TransferOut(contractAddr common.Address, recipient common.Address, amount *big.Int, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.Contract.TransferOut(&_Tokenhub.TransactOpts, contractAddr, recipient, amount, expireTime)
}

// TransferOut is a paid mutator transaction binding the contract method 0xaa7415f5.
//
// Solidity: function transferOut(address contractAddr, address recipient, uint256 amount, uint64 expireTime) returns(bool)
func (_Tokenhub *TokenhubTransactorSession) TransferOut(contractAddr common.Address, recipient common.Address, amount *big.Int, expireTime uint64) (*types.Transaction, error) {
	return _Tokenhub.Contract.TransferOut(&_Tokenhub.TransactOpts, contractAddr, recipient, amount, expireTime)
}

// UnbindToken is a paid mutator transaction binding the contract method 0xb99328c5.
//
// Solidity: function unbindToken(bytes32 bep2Symbol, address contractAddr) returns()
func (_Tokenhub *TokenhubTransactor) UnbindToken(opts *bind.TransactOpts, bep2Symbol [32]byte, contractAddr common.Address) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "unbindToken", bep2Symbol, contractAddr)
}

// UnbindToken is a paid mutator transaction binding the contract method 0xb99328c5.
//
// Solidity: function unbindToken(bytes32 bep2Symbol, address contractAddr) returns()
func (_Tokenhub *TokenhubSession) UnbindToken(bep2Symbol [32]byte, contractAddr common.Address) (*types.Transaction, error) {
	return _Tokenhub.Contract.UnbindToken(&_Tokenhub.TransactOpts, bep2Symbol, contractAddr)
}

// UnbindToken is a paid mutator transaction binding the contract method 0xb99328c5.
//
// Solidity: function unbindToken(bytes32 bep2Symbol, address contractAddr) returns()
func (_Tokenhub *TokenhubTransactorSession) UnbindToken(bep2Symbol [32]byte, contractAddr common.Address) (*types.Transaction, error) {
	return _Tokenhub.Contract.UnbindToken(&_Tokenhub.TransactOpts, bep2Symbol, contractAddr)
}

// UpdateParam is a paid mutator transaction binding the contract method 0xac431751.
//
// Solidity: function updateParam(string key, bytes value) returns()
func (_Tokenhub *TokenhubTransactor) UpdateParam(opts *bind.TransactOpts, key string, value []byte) (*types.Transaction, error) {
	return _Tokenhub.contract.Transact(opts, "updateParam", key, value)
}

// UpdateParam is a paid mutator transaction binding the contract method 0xac431751.
//
// Solidity: function updateParam(string key, bytes value) returns()
func (_Tokenhub *TokenhubSession) UpdateParam(key string, value []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.UpdateParam(&_Tokenhub.TransactOpts, key, value)
}

// UpdateParam is a paid mutator transaction binding the contract method 0xac431751.
//
// Solidity: function updateParam(string key, bytes value) returns()
func (_Tokenhub *TokenhubTransactorSession) UpdateParam(key string, value []byte) (*types.Transaction, error) {
	return _Tokenhub.Contract.UpdateParam(&_Tokenhub.TransactOpts, key, value)
}

// TokenhubParamChangeIterator is returned from FilterParamChange and is used to iterate over the raw logs and unpacked data for ParamChange events raised by the Tokenhub contract.
type TokenhubParamChangeIterator struct {
	Event *TokenhubParamChange // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubParamChangeIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubParamChange)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubParamChange)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubParamChangeIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubParamChangeIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubParamChange represents a ParamChange event raised by the Tokenhub contract.
type TokenhubParamChange struct {
	Key   string
	Value []byte
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterParamChange is a free log retrieval operation binding the contract event 0x6cdb0ac70ab7f2e2d035cca5be60d89906f2dede7648ddbd7402189c1eeed17a.
//
// Solidity: event paramChange(string key, bytes value)
func (_Tokenhub *TokenhubFilterer) FilterParamChange(opts *bind.FilterOpts) (*TokenhubParamChangeIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "paramChange")
	if err != nil {
		return nil, err
	}
	return &TokenhubParamChangeIterator{contract: _Tokenhub.contract, event: "paramChange", logs: logs, sub: sub}, nil
}

// WatchParamChange is a free log subscription operation binding the contract event 0x6cdb0ac70ab7f2e2d035cca5be60d89906f2dede7648ddbd7402189c1eeed17a.
//
// Solidity: event paramChange(string key, bytes value)
func (_Tokenhub *TokenhubFilterer) WatchParamChange(opts *bind.WatchOpts, sink chan<- *TokenhubParamChange) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "paramChange")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubParamChange)
				if err := _Tokenhub.contract.UnpackLog(event, "paramChange", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseParamChange is a log parse operation binding the contract event 0x6cdb0ac70ab7f2e2d035cca5be60d89906f2dede7648ddbd7402189c1eeed17a.
//
// Solidity: event paramChange(string key, bytes value)
func (_Tokenhub *TokenhubFilterer) ParseParamChange(log types.Log) (*TokenhubParamChange, error) {
	event := new(TokenhubParamChange)
	if err := _Tokenhub.contract.UnpackLog(event, "paramChange", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubReceiveDepositIterator is returned from FilterReceiveDeposit and is used to iterate over the raw logs and unpacked data for ReceiveDeposit events raised by the Tokenhub contract.
type TokenhubReceiveDepositIterator struct {
	Event *TokenhubReceiveDeposit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubReceiveDepositIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubReceiveDeposit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubReceiveDeposit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubReceiveDepositIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubReceiveDepositIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubReceiveDeposit represents a ReceiveDeposit event raised by the Tokenhub contract.
type TokenhubReceiveDeposit struct {
	From   common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterReceiveDeposit is a free log retrieval operation binding the contract event 0x6c98249d85d88c3753a04a22230f595e4dc8d3dc86c34af35deeeedc861b89db.
//
// Solidity: event receiveDeposit(address from, uint256 amount)
func (_Tokenhub *TokenhubFilterer) FilterReceiveDeposit(opts *bind.FilterOpts) (*TokenhubReceiveDepositIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "receiveDeposit")
	if err != nil {
		return nil, err
	}
	return &TokenhubReceiveDepositIterator{contract: _Tokenhub.contract, event: "receiveDeposit", logs: logs, sub: sub}, nil
}

// WatchReceiveDeposit is a free log subscription operation binding the contract event 0x6c98249d85d88c3753a04a22230f595e4dc8d3dc86c34af35deeeedc861b89db.
//
// Solidity: event receiveDeposit(address from, uint256 amount)
func (_Tokenhub *TokenhubFilterer) WatchReceiveDeposit(opts *bind.WatchOpts, sink chan<- *TokenhubReceiveDeposit) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "receiveDeposit")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubReceiveDeposit)
				if err := _Tokenhub.contract.UnpackLog(event, "receiveDeposit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseReceiveDeposit is a log parse operation binding the contract event 0x6c98249d85d88c3753a04a22230f595e4dc8d3dc86c34af35deeeedc861b89db.
//
// Solidity: event receiveDeposit(address from, uint256 amount)
func (_Tokenhub *TokenhubFilterer) ParseReceiveDeposit(log types.Log) (*TokenhubReceiveDeposit, error) {
	event := new(TokenhubReceiveDeposit)
	if err := _Tokenhub.contract.UnpackLog(event, "receiveDeposit", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubRefundFailureIterator is returned from FilterRefundFailure and is used to iterate over the raw logs and unpacked data for RefundFailure events raised by the Tokenhub contract.
type TokenhubRefundFailureIterator struct {
	Event *TokenhubRefundFailure // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubRefundFailureIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubRefundFailure)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubRefundFailure)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubRefundFailureIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubRefundFailureIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubRefundFailure represents a RefundFailure event raised by the Tokenhub contract.
type TokenhubRefundFailure struct {
	Bep20Addr  common.Address
	RefundAddr common.Address
	Amount     *big.Int
	Status     uint32
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterRefundFailure is a free log retrieval operation binding the contract event 0x203f9f67a785f4f81be4d48b109aa0c498d1bc8097ecc2627063f480cc5fe73e.
//
// Solidity: event refundFailure(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) FilterRefundFailure(opts *bind.FilterOpts) (*TokenhubRefundFailureIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "refundFailure")
	if err != nil {
		return nil, err
	}
	return &TokenhubRefundFailureIterator{contract: _Tokenhub.contract, event: "refundFailure", logs: logs, sub: sub}, nil
}

// WatchRefundFailure is a free log subscription operation binding the contract event 0x203f9f67a785f4f81be4d48b109aa0c498d1bc8097ecc2627063f480cc5fe73e.
//
// Solidity: event refundFailure(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) WatchRefundFailure(opts *bind.WatchOpts, sink chan<- *TokenhubRefundFailure) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "refundFailure")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubRefundFailure)
				if err := _Tokenhub.contract.UnpackLog(event, "refundFailure", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRefundFailure is a log parse operation binding the contract event 0x203f9f67a785f4f81be4d48b109aa0c498d1bc8097ecc2627063f480cc5fe73e.
//
// Solidity: event refundFailure(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) ParseRefundFailure(log types.Log) (*TokenhubRefundFailure, error) {
	event := new(TokenhubRefundFailure)
	if err := _Tokenhub.contract.UnpackLog(event, "refundFailure", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubRefundSuccessIterator is returned from FilterRefundSuccess and is used to iterate over the raw logs and unpacked data for RefundSuccess events raised by the Tokenhub contract.
type TokenhubRefundSuccessIterator struct {
	Event *TokenhubRefundSuccess // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubRefundSuccessIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubRefundSuccess)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubRefundSuccess)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubRefundSuccessIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubRefundSuccessIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubRefundSuccess represents a RefundSuccess event raised by the Tokenhub contract.
type TokenhubRefundSuccess struct {
	Bep20Addr  common.Address
	RefundAddr common.Address
	Amount     *big.Int
	Status     uint32
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterRefundSuccess is a free log retrieval operation binding the contract event 0xd468d4fa5e8fb4adc119b29a983fd0785e04af5cb8b7a3a69a47270c54b6901a.
//
// Solidity: event refundSuccess(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) FilterRefundSuccess(opts *bind.FilterOpts) (*TokenhubRefundSuccessIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "refundSuccess")
	if err != nil {
		return nil, err
	}
	return &TokenhubRefundSuccessIterator{contract: _Tokenhub.contract, event: "refundSuccess", logs: logs, sub: sub}, nil
}

// WatchRefundSuccess is a free log subscription operation binding the contract event 0xd468d4fa5e8fb4adc119b29a983fd0785e04af5cb8b7a3a69a47270c54b6901a.
//
// Solidity: event refundSuccess(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) WatchRefundSuccess(opts *bind.WatchOpts, sink chan<- *TokenhubRefundSuccess) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "refundSuccess")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubRefundSuccess)
				if err := _Tokenhub.contract.UnpackLog(event, "refundSuccess", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRefundSuccess is a log parse operation binding the contract event 0xd468d4fa5e8fb4adc119b29a983fd0785e04af5cb8b7a3a69a47270c54b6901a.
//
// Solidity: event refundSuccess(address bep20Addr, address refundAddr, uint256 amount, uint32 status)
func (_Tokenhub *TokenhubFilterer) ParseRefundSuccess(log types.Log) (*TokenhubRefundSuccess, error) {
	event := new(TokenhubRefundSuccess)
	if err := _Tokenhub.contract.UnpackLog(event, "refundSuccess", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubRewardToIterator is returned from FilterRewardTo and is used to iterate over the raw logs and unpacked data for RewardTo events raised by the Tokenhub contract.
type TokenhubRewardToIterator struct {
	Event *TokenhubRewardTo // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubRewardToIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubRewardTo)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubRewardTo)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubRewardToIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubRewardToIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubRewardTo represents a RewardTo event raised by the Tokenhub contract.
type TokenhubRewardTo struct {
	To     common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRewardTo is a free log retrieval operation binding the contract event 0xf8b71c64315fc33b2ead2adfa487955065152a8ac33d9d5193aafd7f45dc15a0.
//
// Solidity: event rewardTo(address to, uint256 amount)
func (_Tokenhub *TokenhubFilterer) FilterRewardTo(opts *bind.FilterOpts) (*TokenhubRewardToIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "rewardTo")
	if err != nil {
		return nil, err
	}
	return &TokenhubRewardToIterator{contract: _Tokenhub.contract, event: "rewardTo", logs: logs, sub: sub}, nil
}

// WatchRewardTo is a free log subscription operation binding the contract event 0xf8b71c64315fc33b2ead2adfa487955065152a8ac33d9d5193aafd7f45dc15a0.
//
// Solidity: event rewardTo(address to, uint256 amount)
func (_Tokenhub *TokenhubFilterer) WatchRewardTo(opts *bind.WatchOpts, sink chan<- *TokenhubRewardTo) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "rewardTo")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubRewardTo)
				if err := _Tokenhub.contract.UnpackLog(event, "rewardTo", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRewardTo is a log parse operation binding the contract event 0xf8b71c64315fc33b2ead2adfa487955065152a8ac33d9d5193aafd7f45dc15a0.
//
// Solidity: event rewardTo(address to, uint256 amount)
func (_Tokenhub *TokenhubFilterer) ParseRewardTo(log types.Log) (*TokenhubRewardTo, error) {
	event := new(TokenhubRewardTo)
	if err := _Tokenhub.contract.UnpackLog(event, "rewardTo", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubTransferInSuccessIterator is returned from FilterTransferInSuccess and is used to iterate over the raw logs and unpacked data for TransferInSuccess events raised by the Tokenhub contract.
type TokenhubTransferInSuccessIterator struct {
	Event *TokenhubTransferInSuccess // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubTransferInSuccessIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubTransferInSuccess)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubTransferInSuccess)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubTransferInSuccessIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubTransferInSuccessIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubTransferInSuccess represents a TransferInSuccess event raised by the Tokenhub contract.
type TokenhubTransferInSuccess struct {
	Bep20Addr  common.Address
	RefundAddr common.Address
	Amount     *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterTransferInSuccess is a free log retrieval operation binding the contract event 0x471eb9cc1ffe55ffadf15b32595415eb9d80f22e761d24bd6dffc607e1284d59.
//
// Solidity: event transferInSuccess(address bep20Addr, address refundAddr, uint256 amount)
func (_Tokenhub *TokenhubFilterer) FilterTransferInSuccess(opts *bind.FilterOpts) (*TokenhubTransferInSuccessIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "transferInSuccess")
	if err != nil {
		return nil, err
	}
	return &TokenhubTransferInSuccessIterator{contract: _Tokenhub.contract, event: "transferInSuccess", logs: logs, sub: sub}, nil
}

// WatchTransferInSuccess is a free log subscription operation binding the contract event 0x471eb9cc1ffe55ffadf15b32595415eb9d80f22e761d24bd6dffc607e1284d59.
//
// Solidity: event transferInSuccess(address bep20Addr, address refundAddr, uint256 amount)
func (_Tokenhub *TokenhubFilterer) WatchTransferInSuccess(opts *bind.WatchOpts, sink chan<- *TokenhubTransferInSuccess) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "transferInSuccess")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubTransferInSuccess)
				if err := _Tokenhub.contract.UnpackLog(event, "transferInSuccess", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseTransferInSuccess is a log parse operation binding the contract event 0x471eb9cc1ffe55ffadf15b32595415eb9d80f22e761d24bd6dffc607e1284d59.
//
// Solidity: event transferInSuccess(address bep20Addr, address refundAddr, uint256 amount)
func (_Tokenhub *TokenhubFilterer) ParseTransferInSuccess(log types.Log) (*TokenhubTransferInSuccess, error) {
	event := new(TokenhubTransferInSuccess)
	if err := _Tokenhub.contract.UnpackLog(event, "transferInSuccess", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubTransferOutSuccessIterator is returned from FilterTransferOutSuccess and is used to iterate over the raw logs and unpacked data for TransferOutSuccess events raised by the Tokenhub contract.
type TokenhubTransferOutSuccessIterator struct {
	Event *TokenhubTransferOutSuccess // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubTransferOutSuccessIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubTransferOutSuccess)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubTransferOutSuccess)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubTransferOutSuccessIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubTransferOutSuccessIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubTransferOutSuccess represents a TransferOutSuccess event raised by the Tokenhub contract.
type TokenhubTransferOutSuccess struct {
	Bep20Addr  common.Address
	SenderAddr common.Address
	Amount     *big.Int
	RelayFee   *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterTransferOutSuccess is a free log retrieval operation binding the contract event 0x74eab09b0e53aefc23f2e1b16da593f95c2dd49c6f5a23720463d10d9c330b2a.
//
// Solidity: event transferOutSuccess(address bep20Addr, address senderAddr, uint256 amount, uint256 relayFee)
func (_Tokenhub *TokenhubFilterer) FilterTransferOutSuccess(opts *bind.FilterOpts) (*TokenhubTransferOutSuccessIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "transferOutSuccess")
	if err != nil {
		return nil, err
	}
	return &TokenhubTransferOutSuccessIterator{contract: _Tokenhub.contract, event: "transferOutSuccess", logs: logs, sub: sub}, nil
}

// WatchTransferOutSuccess is a free log subscription operation binding the contract event 0x74eab09b0e53aefc23f2e1b16da593f95c2dd49c6f5a23720463d10d9c330b2a.
//
// Solidity: event transferOutSuccess(address bep20Addr, address senderAddr, uint256 amount, uint256 relayFee)
func (_Tokenhub *TokenhubFilterer) WatchTransferOutSuccess(opts *bind.WatchOpts, sink chan<- *TokenhubTransferOutSuccess) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "transferOutSuccess")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubTransferOutSuccess)
				if err := _Tokenhub.contract.UnpackLog(event, "transferOutSuccess", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseTransferOutSuccess is a log parse operation binding the contract event 0x74eab09b0e53aefc23f2e1b16da593f95c2dd49c6f5a23720463d10d9c330b2a.
//
// Solidity: event transferOutSuccess(address bep20Addr, address senderAddr, uint256 amount, uint256 relayFee)
func (_Tokenhub *TokenhubFilterer) ParseTransferOutSuccess(log types.Log) (*TokenhubTransferOutSuccess, error) {
	event := new(TokenhubTransferOutSuccess)
	if err := _Tokenhub.contract.UnpackLog(event, "transferOutSuccess", log); err != nil {
		return nil, err
	}
	return event, nil
}

// TokenhubUnexpectedPackageIterator is returned from FilterUnexpectedPackage and is used to iterate over the raw logs and unpacked data for UnexpectedPackage events raised by the Tokenhub contract.
type TokenhubUnexpectedPackageIterator struct {
	Event *TokenhubUnexpectedPackage // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *TokenhubUnexpectedPackageIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TokenhubUnexpectedPackage)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(TokenhubUnexpectedPackage)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *TokenhubUnexpectedPackageIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TokenhubUnexpectedPackageIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TokenhubUnexpectedPackage represents a UnexpectedPackage event raised by the Tokenhub contract.
type TokenhubUnexpectedPackage struct {
	ChannelId uint8
	MsgBytes  []byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterUnexpectedPackage is a free log retrieval operation binding the contract event 0x41ce201247b6ceb957dcdb217d0b8acb50b9ea0e12af9af4f5e7f38902101605.
//
// Solidity: event unexpectedPackage(uint8 channelId, bytes msgBytes)
func (_Tokenhub *TokenhubFilterer) FilterUnexpectedPackage(opts *bind.FilterOpts) (*TokenhubUnexpectedPackageIterator, error) {

	logs, sub, err := _Tokenhub.contract.FilterLogs(opts, "unexpectedPackage")
	if err != nil {
		return nil, err
	}
	return &TokenhubUnexpectedPackageIterator{contract: _Tokenhub.contract, event: "unexpectedPackage", logs: logs, sub: sub}, nil
}

// WatchUnexpectedPackage is a free log subscription operation binding the contract event 0x41ce201247b6ceb957dcdb217d0b8acb50b9ea0e12af9af4f5e7f38902101605.
//
// Solidity: event unexpectedPackage(uint8 channelId, bytes msgBytes)
func (_Tokenhub *TokenhubFilterer) WatchUnexpectedPackage(opts *bind.WatchOpts, sink chan<- *TokenhubUnexpectedPackage) (event.Subscription, error) {

	logs, sub, err := _Tokenhub.contract.WatchLogs(opts, "unexpectedPackage")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TokenhubUnexpectedPackage)
				if err := _Tokenhub.contract.UnpackLog(event, "unexpectedPackage", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUnexpectedPackage is a log parse operation binding the contract event 0x41ce201247b6ceb957dcdb217d0b8acb50b9ea0e12af9af4f5e7f38902101605.
//
// Solidity: event unexpectedPackage(uint8 channelId, bytes msgBytes)
func (_Tokenhub *TokenhubFilterer) ParseUnexpectedPackage(log types.Log) (*TokenhubUnexpectedPackage, error) {
	event := new(TokenhubUnexpectedPackage)
	if err := _Tokenhub.contract.UnpackLog(event, "unexpectedPackage", log); err != nil {
		return nil, err
	}
	return event, nil
}
