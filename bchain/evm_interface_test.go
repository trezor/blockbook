package bchain

import (
	"context"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
)

type recordingEVMBatchRPCClient struct {
	calls []string
}

func (c *recordingEVMBatchRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (EVMClientSubscription, error) {
	c.calls = append(c.calls, "subscribe")
	return nil, nil
}

func (c *recordingEVMBatchRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	c.calls = append(c.calls, "call:"+method)
	return nil
}

func (c *recordingEVMBatchRPCClient) CallContextWithIntent(ctx context.Context, intent EVMRPCIntent, result interface{}, method string, args ...interface{}) error {
	c.calls = append(c.calls, "intent:"+method)
	return nil
}

func (c *recordingEVMBatchRPCClient) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	c.calls = append(c.calls, "batch")
	return nil
}

func (c *recordingEVMBatchRPCClient) Close() {}

func TestDualEVMRPCClientRoutesByIntent(t *testing.T) {
	callClient := &recordingEVMBatchRPCClient{}
	subClient := &recordingEVMBatchRPCClient{}
	client := &DualEVMRPCClient{CallClient: callClient, SubClient: subClient}

	if err := client.CallContext(context.Background(), nil, "eth_call"); err != nil {
		t.Fatal(err)
	}
	if err := client.BatchCallContext(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := client.CallContextWithIntent(context.Background(), EVMRPCIntentDefault, nil, "eth_blockNumber"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.EthSubscribe(context.Background(), nil, "newHeads"); err != nil {
		t.Fatal(err)
	}
	if err := client.CallContextWithIntent(context.Background(), EVMRPCIntentChainTip, nil, "eth_getBlockByHash"); err != nil {
		t.Fatal(err)
	}

	if want := []string{"call:eth_call", "batch", "intent:eth_blockNumber"}; !reflect.DeepEqual(callClient.calls, want) {
		t.Fatalf("call client calls = %v, want %v", callClient.calls, want)
	}
	if want := []string{"subscribe", "intent:eth_getBlockByHash"}; !reflect.DeepEqual(subClient.calls, want) {
		t.Fatalf("subscription client calls = %v, want %v", subClient.calls, want)
	}
}
