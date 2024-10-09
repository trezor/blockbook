package eth

import (
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

func (b *EthereumRPC) initStakingPools(coinShortcut string) error {
	// for now only single staking pool
	envVar := strings.ToUpper(coinShortcut) + "_STAKING_POOL_CONTRACT"
	envValue := os.Getenv(envVar)
	if envValue != "" {
		parts := strings.Split(envValue, "/")
		if len(parts) != 2 {
			glog.Errorf("Wrong format of environment variable %s=%s, expecting value '<pool name>/<pool contract>', staking pools not enabled", envVar, envValue)
			return nil
		}
		b.supportedStakingPools = []string{envValue}
		b.stakingPoolNames = []string{parts[0]}
		b.stakingPoolContracts = []string{parts[1]}
		glog.Info("Support of staking pools enabled with these pools: ", b.supportedStakingPools)
	}
	return nil
}

func (b *EthereumRPC) EthereumTypeGetSupportedStakingPools() []string {
	return b.supportedStakingPools
}

func (b *EthereumRPC) EthereumTypeGetStakingPoolsData(addrDesc bchain.AddressDescriptor) ([]bchain.StakingPoolData, error) {
	// for now only single staking pool - Everstake
	addr := hexutil.Encode(addrDesc)[2:]
	if len(b.supportedStakingPools) == 1 {
		data, err := b.everstakePoolData(addr, b.stakingPoolContracts[0], b.stakingPoolNames[0])
		if err != nil {
			return nil, err
		}
		if data != nil {
			return []bchain.StakingPoolData{*data}, nil
		}
	}
	return nil, nil
}

const everstakePendingBalanceOfMethodSignature = "0x59b8c763"          // pendingBalanceOf(address)
const everstakePendingDepositedBalanceOfMethodSignature = "0x80f14ecc" // pendingDepositedBalanceOf(address)
const everstakeDepositedBalanceOfMethodSignature = "0x68b48254"        // depositedBalanceOf(address)
const everstakeWithdrawRequestMethodSignature = "0x14cbc46a"           // withdrawRequest(address)
const everstakeRestakedRewardOfMethodSignature = "0x0c98929a"          // restakedRewardOf(address)
const everstakeAutocompoundBalanceOfMethodSignature = "0x2fec7966"     // autocompoundBalanceOf(address)

func isZeroBigInt(b *big.Int) bool {
	return len(b.Bits()) == 0
}

func (b *EthereumRPC) everstakeBalanceTypeContractCall(signature, addr, contract string) (string, error) {
	req := signature + "0000000000000000000000000000000000000000000000000000000000000000"[len(addr):] + addr
	return b.EthereumTypeRpcCall(req, contract, "")
}

func (b *EthereumRPC) everstakeContractCallSimpleNumeric(signature, addr, contract string) (*big.Int, error) {
	data, err := b.everstakeBalanceTypeContractCall(signature, addr, contract)
	if err != nil {
		return nil, err
	}
	r := parseSimpleNumericProperty(data)
	if r == nil {
		return nil, errors.New("Invalid balance")
	}
	return r, nil
}

func (b *EthereumRPC) everstakePoolData(addr, contract, name string) (*bchain.StakingPoolData, error) {
	poolData := bchain.StakingPoolData{
		Contract: contract,
		Name:     name,
	}
	allZeros := true

	value, err := b.everstakeContractCallSimpleNumeric(everstakePendingBalanceOfMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	poolData.PendingBalance = *value
	allZeros = allZeros && isZeroBigInt(value)

	value, err = b.everstakeContractCallSimpleNumeric(everstakePendingDepositedBalanceOfMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	poolData.PendingDepositedBalance = *value
	allZeros = allZeros && isZeroBigInt(value)

	value, err = b.everstakeContractCallSimpleNumeric(everstakeDepositedBalanceOfMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	poolData.DepositedBalance = *value
	allZeros = allZeros && isZeroBigInt(value)

	data, err := b.everstakeBalanceTypeContractCall(everstakeWithdrawRequestMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	value = parseSimpleNumericProperty(data)
	if value == nil {
		return nil, errors.New("Invalid balance")
	}
	poolData.WithdrawTotalAmount = *value
	allZeros = allZeros && isZeroBigInt(value)
	value = parseSimpleNumericProperty(data[64+2:])
	if value == nil {
		return nil, errors.New("Invalid balance")
	}
	poolData.ClaimableAmount = *value
	allZeros = allZeros && isZeroBigInt(value)

	value, err = b.everstakeContractCallSimpleNumeric(everstakeRestakedRewardOfMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	poolData.RestakedReward = *value
	allZeros = allZeros && isZeroBigInt(value)

	value, err = b.everstakeContractCallSimpleNumeric(everstakeAutocompoundBalanceOfMethodSignature, addr, contract)
	if err != nil {
		return nil, err
	}
	poolData.AutocompoundBalance = *value
	allZeros = allZeros && isZeroBigInt(value)

	if allZeros {
		return nil, nil
	}
	return &poolData, nil
}
