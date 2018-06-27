// +build integration

package eth

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"testing"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewEthereumRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	return c, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Ethereum Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	t.TryConnect()

	rpcTest = t

	os.Exit(m.Run())
}

func TestEthRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestEthRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestEthRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestEthRPC_getBestHeader(t *testing.T) {
	type fields struct {
		b *EthereumRPC
	}
	tests := []struct {
		name    string
		fields  fields
		want    *ethtypes.Header
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fields.b.getBestHeader()
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.getBestHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// the header is always different, do not compare what we got
		})
	}
}

func TestEthRPC_GetBestBlockHash(t *testing.T) {
	type fields struct {
		b *EthereumRPC
	}
	tests := []struct {
		name    string
		fields  fields
		want    int
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
			want: 66,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBestBlockHash()
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetBestBlockHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// the hash is always different, compare only the length of hash
			if len(got) != tt.want {
				t.Errorf("EthRPC.GetBestBlockHash() = %v, len %v, want len %v", got, len(got), tt.want)
			}
		})
	}
}

func TestEthRPC_GetBestBlockHeight(t *testing.T) {
	type fields struct {
		b *EthereumRPC
	}
	tests := []struct {
		name    string
		fields  fields
		want    uint32
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
			want: 1000000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBestBlockHeight()
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetBestBlockHeight() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got < tt.want {
				t.Errorf("EthRPC.GetBestBlockHeight() = %v, want at least %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_GetBlockHeader(t *testing.T) {
	bh, err := rpcTest.Client.(*EthereumRPC).getBestHeader()
	if err != nil {
		panic(err)
	}
	type fields struct {
		b *EthereumRPC
	}
	type args struct {
		hash string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *bchain.BlockHeader
		wantErr error
	}{
		{
			name: "2870000",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
			args: args{
				hash: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
			},
			want: &bchain.BlockHeader{
				Hash:          "0xeccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
				Height:        2870000,
				Confirmations: int(uint32(bh.Number.Uint64()) - 2870000 + 1),
			},
		},
		{
			name: "ErrBlockNotFound",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
			args: args{
				hash: "0xeccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903896",
			},
			wantErr: bchain.ErrBlockNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlockHeader(tt.args.hash)
			if err != tt.wantErr {
				t.Errorf("EthRPC.GetBlockHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EthRPC.GetBlockHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_EstimateFee(t *testing.T) {
	type fields struct {
		b *EthereumRPC
	}
	type args struct {
		blocks int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    float64
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
			args: args{
				blocks: 10,
			},
			want: 1., // check that there is some estimate
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.EstimateFee(tt.args.blocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.EstimateFee() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got < tt.want {
				t.Errorf("EthRPC.EstimateFee() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_GetMempool(t *testing.T) {
	type fields struct {
		b *EthereumRPC
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: rpcTest.Client.(*EthereumRPC),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetMempool()
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetMempool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			t.Logf("EthRPC.GetMempool() returned %v transactions", len(got))
		})
	}
}
