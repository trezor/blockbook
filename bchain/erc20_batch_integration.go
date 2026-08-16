//go:build integration

package bchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

const defaultBatchSize = 100

type ERC20BatchCase struct {
	Name            string
	RPCURL          string
	RPCURLWS        string
	Addr            common.Address
	Contracts       []common.Address
	BatchSize       int
	SkipUnavailable bool
	NewClient       ERC20BatchClientFactory
}

// RunERC20BatchBalanceTest validates batch balanceOf results against single calls.
func RunERC20BatchBalanceTest(t *testing.T, tc ERC20BatchCase) {
	t.Helper()
	if tc.BatchSize <= 0 {
		tc.BatchSize = defaultBatchSize
	}
	if tc.NewClient == nil {
		t.Fatalf("NewClient is required for ERC20 batch integration test")
	}
	rpcClient, closeFn, err := tc.NewClient(tc.RPCURL, tc.RPCURLWS, tc.BatchSize)
	if err != nil {
		handleRPCError(t, tc, fmt.Errorf("rpc dial error: %w", err))
		return
	}
	if closeFn != nil {
		t.Cleanup(closeFn)
	}
	if err := verifyBatchBalances(rpcClient, tc.Addr, tc.Contracts); err != nil {
		handleRPCError(t, tc, err)
		return
	}
	chunkedContracts := expandContracts(tc.Contracts, tc.BatchSize+1)
	if err := verifyBatchBalances(rpcClient, tc.Addr, chunkedContracts); err != nil {
		handleRPCError(t, tc, err)
		return
	}
}

func handleRPCError(t *testing.T, tc ERC20BatchCase, err error) {
	t.Helper()
	if tc.SkipUnavailable && isRPCUnavailable(err) {
		t.Skipf("WARN: %s RPC not available: %v", tc.Name, err)
		return
	}
	t.Fatalf("%v", err)
}

func expandContracts(contracts []common.Address, minLen int) []common.Address {
	if len(contracts) >= minLen {
		return contracts
	}
	out := make([]common.Address, 0, minLen)
	for len(out) < minLen {
		out = append(out, contracts...)
	}
	if len(out) > minLen {
		out = out[:minLen]
	}
	return out
}

type ERC20BatchClient interface {
	EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc AddressDescriptor, contractDescs []AddressDescriptor, blockNumber *big.Int) ([]*big.Int, error)
	EthereumTypeGetErc20ContractBalanceAtBlock(addrDesc, contractDesc AddressDescriptor, blockNumber *big.Int) (*big.Int, error)
	GetBestBlockHeight() (uint32, error)
}

type ERC20BatchClientFactory func(rpcURL, rpcURLWS string, batchSize int) (ERC20BatchClient, func(), error)

func verifyBatchBalances(rpcClient ERC20BatchClient, addr common.Address, contracts []common.Address) error {
	if len(contracts) == 0 {
		return errors.New("no contracts to query")
	}
	contractDescs := make([]AddressDescriptor, len(contracts))
	for i, c := range contracts {
		contractDescs[i] = AddressDescriptor(c.Bytes())
	}
	addrDesc := AddressDescriptor(addr.Bytes())
	height, err := rpcClient.GetBestBlockHeight()
	if err != nil {
		return fmt.Errorf("best block height error: %w", err)
	}
	blockNumber := new(big.Int).SetUint64(uint64(height))
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc, contractDescs, blockNumber)
	if err != nil {
		return fmt.Errorf("batch balances error: %w", err)
	}
	if len(balances) != len(contractDescs) {
		return fmt.Errorf("expected %d balances, got %d", len(contractDescs), len(balances))
	}
	for i, contractDesc := range contractDescs {
		single, err := rpcClient.EthereumTypeGetErc20ContractBalanceAtBlock(addrDesc, contractDesc, blockNumber)
		if err != nil {
			return fmt.Errorf("single balance error for %s: %w", contracts[i].Hex(), err)
		}
		if balances[i] == nil {
			return fmt.Errorf("batch balance missing for %s", contracts[i].Hex())
		}
		if balances[i].Cmp(single) != 0 {
			return fmt.Errorf("balance mismatch for %s: batch=%s single=%s", contracts[i].Hex(), balances[i].String(), single.String())
		}
	}
	return nil
}

func isRPCUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "timeout"):
		return true
	}
	return false
}
