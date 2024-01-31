package rsk

import (
	"context"
	"encoding/json"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	// MainNet is production network
	MainNet eth.Network = 30
	// TestNet is test network
	TestNet eth.Network = 31
)

// RskRPC is an interface to JSON-RPC rsk service.
type RskRPC struct {
	*eth.EthereumRPC
	WSClient bchain.EVMClient
	WSRPC    bchain.EVMRPCClient
}

// NewRskRPC returns new RskRPC instance.
func NewRskRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	c, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &RskRPC{
		EthereumRPC: c.(*eth.EthereumRPC),
	}

	return s, nil
}

// Initialize rsk rpc interface
func (b *RskRPC) Initialize() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &RskRPCClient{Client: r}
		ec := &RskClient{Client: ethclient.NewClient(r), RskRPCClient: rc}
		return rc, ec, nil
	}

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}

	wsrc, wsec, wserr := b.OpenRPC(b.ChainConfig.WSURL)
	if wserr != nil {
		return wserr
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = eth.NewEthereumNewBlock()
	b.NewTx = eth.NewEthereumNewTx()
	b.WSRPC = wsrc
	b.WSClient = wsec

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
	case TestNet:
		b.Testnet = true
		b.Network = "testnet"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	return nil
}

// InitializeMempool creates subscriptions to newHeads and newPendingTransactions
func (b *RskRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Mempool not created")
	}

	// get initial mempool transactions
	txs, err := b.GetMempoolTransactions()
	if err != nil {
		return err
	}
	for _, txid := range txs {
		b.Mempool.AddTransactionToMempool(txid)
	}

	b.Mempool.OnNewTxAddr = onNewTxAddr
	b.Mempool.OnNewTx = onNewTx

	if err = b.subscribeEvents(); err != nil {
		return err
	}

	b.MempoolInitialized = true

	return nil
}

func (b *RskRPC) subscribeEvents() error {
	// new block notifications handling
	go func() {
		for {
			h, ok := b.NewBlock.Read()
			if !ok {
				break
			}
			b.UpdateBestHeader(h)
			// notify blockbook
			b.PushHandler(bchain.NotificationNewBlock)
		}
	}()

	// new block subscription
	if err := b.Subscribe(func() (bchain.EVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.NewBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.WSRPC.EthSubscribe(ctx, b.NewBlock.Channel(), "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "RskSubscribe newHeads")
		}
		b.NewBlockSubscription = sub
		glog.Info("Subscribed to newHeads")
		return sub, nil
	}); err != nil {
		return err
	}

	// new mempool transaction notifications handling
	go func() {
		for {
			t, ok := b.NewTx.Read()
			if !ok {
				break
			}
			hex := t.Hex()
			if glog.V(2) {
				glog.Info("rpc: new tx ", hex)
			}
			b.Mempool.AddTransactionToMempool(hex)
			b.PushHandler(bchain.NotificationNewTx)
		}
	}()

	// new mempool transaction subscription
	if err := b.Subscribe(func() (bchain.EVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.NewTxSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.WSRPC.EthSubscribe(ctx, b.NewTx.Channel(), "newPendingTransactions")
		if err != nil {
			return nil, errors.Annotatef(err, "RskSubscribe newPendingTransactions")
		}
		b.NewTxSubscription = sub
		glog.Info("Subscribed to newPendingTransactions")
		return sub, nil
	}); err != nil {
		return err
	}

	return nil
}

func (b *RskRPC) closeRPC() {
	b.closeRPC()
	if b.WSRPC != nil {
		b.WSRPC.Close()
	}
}

func (b *RskRPC) reconnectRPC() error {
	b.OpenRPC = func(url string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		r, err := rpc.Dial(url)
		if err != nil {
			return nil, nil, err
		}
		rc := &RskRPCClient{Client: r}
		ec := &RskClient{Client: ethclient.NewClient(r), RskRPCClient: rc}
		return rc, ec, nil
	}

	glog.Info("Reconnecting RPC")
	b.closeRPC()
	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL)
	if err != nil {
		return err
	}
	b.RPC = rc
	b.Client = ec

	wsrc, wsec, wserr := b.OpenRPC(b.ChainConfig.WSURL)
	if wserr != nil {
		return wserr
	}

	b.WSRPC = wsrc
	b.WSClient = wsec

	return b.subscribeEvents()
}
