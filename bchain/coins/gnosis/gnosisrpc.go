package gnosis

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 100
)

// GnosisRPC is an interface to JSON-RPC bsc service.
type GnosisRPC struct {
	*eth.EthereumRPC
}

// NewGnosisRPC returns new GnosisRPC instance.
func NewGnosisRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &GnosisRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	// use trace_block for internal data (debug_trace memory overhead is too expensive)
	c.(*eth.EthereumRPC).GetInternalDataForBlock = s.getInternalDataForBlock

	return s, nil
}

// Initialize GnosisRPC interface
func (b *GnosisRPC) Initialize() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &GnosisRPCClient{Client: r}
		ec := &GnosisClient{Client: ethclient.NewClient(r), GnosisRPCClient: rc}
		return rc, ec, nil
	}

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = &GnosisNewBlock{channel: make(chan *Header)}
	b.NewTx = &GnosisNewTx{channel: make(chan common.Hash)}

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch eth.Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "livenet"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

type action struct {
	Author     string `json:"author"`
	CallType   string `json:"callType"`
	From       string `json:"from"`
	Gas        string `json:"gas"`
	Input      string `json:"input"`
	RewardType string `json:"rewardType"`
	To         string `json:"to"`
	Value      string `json:"value"`
}

type traceBlockResult struct {
	Action      action `json:"action"`
	BlockHash   string `json:"blockHash"`
	BlockNumber int    `json:"blockNumber"`
	Error       string `json:"error"`
	Result      struct {
		GasUsed string `json:"gasUsed"`
		Output  string `json:"output"`
	} `json:"result"`
	Subtraces           int    `json:"subtraces"`
	TraceAddress        []int  `json:"traceAddress"`
	TransactionHash     string `json:"transactionHash"`
	TransactionPosition int    `json:"transactionPosition"`
	Type                string `json:"type"`
}

// getInternalDataForBlock extracts internal transfers and creation or destruction of contracts using the parity trace module
func (b *GnosisRPC) getInternalDataForBlock(blockHash string, blockHeight uint32, transactions []bchain.RpcTransaction) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {
	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)
	if b.EthereumRPC.ChainConfig.ProcessInternalTransactions {
		var n big.Int
		n.SetUint64(uint64(blockHeight))
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		var trace []traceBlockResult
		err := b.RPC.CallContext(ctx, &trace, "trace_block", bchain.ToBlockNumArg(&n))
		if err != nil {
			glog.Error("trace_block ", blockHash, ", error ", err)
			return data, contracts, err
		}
		for _, t := range trace {
			// initiating call does not have any trace addresses and is not an internal transfer
			if len(t.TraceAddress) == 0 {
				continue
			}
			d := &data[t.TransactionPosition]
			action := t.Action
			callType := strings.ToUpper(action.CallType)
			traceType := strings.ToUpper(t.Type)
			value, err := hexutil.DecodeBig(action.Value)
			if traceType == "CREATE" || traceType == "CREATE2" {
				d.Type = bchain.CREATE
				d.Contract = action.To
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Type:  bchain.CREATE,
					Value: *value,
					From:  action.From,
					To:    action.To, // new contract address
				})
				contracts = append(contracts, *b.GetCreationContractInfo(d.Contract, blockHeight))
			} else if t.Type == "SELFDESTRUCT" {
				d.Type = bchain.SELFDESTRUCT
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Type:  bchain.SELFDESTRUCT,
					Value: *value,
					From:  action.From, // destroyed contract address
					To:    action.To,
				})
				contracts = append(contracts, bchain.ContractInfo{Contract: action.From, DestructedInBlock: blockHeight})
			} else if callType == "DELEGATECALL" {
				// ignore DELEGATECALL (geth v1.11 the changed tracer behavior)
				// 	https://github.com/ethereum/go-ethereum/issues/26726
			} else if t.Type == "REWARD" {
				// ignore REWARD as block rewards are not associated with any specific transaction
			} else if err == nil && (value.BitLen() > 0 || b.ChainConfig.ProcessZeroInternalTransactions) {
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Value: *value,
					From:  action.From,
					To:    action.To,
				})
			}
			if t.Error != "" {
				e := eth.PackInternalTransactionError(t.Error)
				if len(e) > 1 {
					d.Error = strings.ToUpper(e[:1]) + e[1:] + ". "
				}
			}
		}
	}
	return data, contracts, nil
}
