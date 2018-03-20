package eth

import (
	"blockbook/bchain"
	"reflect"
	"testing"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var rpcURL = "ws://10.34.3.4:18546"
var client *ethclient.Client

func setupEthRPC() *EthRPC {
	if client == nil {
		ec, err := ethclient.Dial(rpcURL)
		if err != nil {
			panic(err)
		}
		client = ec
	}
	return &EthRPC{
		client:  client,
		timeout: time.Duration(25) * time.Second,
		rpcURL:  "ws://10.34.3.4:18546",
	}
}

func TestEthRPC_getBestHeader(t *testing.T) {
	type fields struct {
		b *EthRPC
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
				b: setupEthRPC(),
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
		b *EthRPC
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
				b: setupEthRPC(),
			},
			want: 64,
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
		b *EthRPC
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
				b: setupEthRPC(),
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

func TestEthRPC_GetBlockHash(t *testing.T) {
	type fields struct {
		b *EthRPC
	}
	type args struct {
		height uint32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "1000000",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				height: 1000000,
			},
			want: "6e6b2e771a3026a1981227ab4a4c8d018edb568494f17df46bcddfa427df686e",
		},
		{
			name: "2870000",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				height: 2870000,
			},
			want: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlockHash(tt.args.height)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetBlockHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EthRPC.GetBlockHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_GetBlockHeader(t *testing.T) {
	bh, err := setupEthRPC().getBestHeader()
	if err != nil {
		panic(err)
	}
	type fields struct {
		b *EthRPC
	}
	type args struct {
		hash string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *bchain.BlockHeader
		wantErr bool
	}{
		{
			name: "2870000",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				hash: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
			},
			want: &bchain.BlockHeader{
				Hash:          "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
				Height:        2870000,
				Confirmations: int(uint32(bh.Number.Uint64()) - 2870000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlockHeader(tt.args.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetBlockHeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EthRPC.GetBlockHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_GetBlock(t *testing.T) {
	bh, err := setupEthRPC().getBestHeader()
	if err != nil {
		panic(err)
	}
	type fields struct {
		b *EthRPC
	}
	type args struct {
		hash   string
		height uint32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *bchain.Block
		wantErr bool
	}{
		{
			name: "2870000",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				hash: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
			},
			want: &bchain.Block{
				BlockHeader: bchain.BlockHeader{
					Hash:          "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
					Height:        2870000,
					Confirmations: int(uint32(bh.Number.Uint64()) - 2870000),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlock(tt.args.hash, tt.args.height)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EthRPC.GetBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}
