//go:build unittest

package db

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/bchain/coins/tron"
)

func Test_emptyInternalData(t *testing.T) {
	tests := []struct {
		name string
		data bchain.EthereumInternalData
		want bool
	}{
		{
			name: "empty CALL",
			data: bchain.EthereumInternalData{Type: bchain.CALL},
			want: true,
		},
		{
			name: "CREATE without transfers",
			data: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want: false,
		},
		{
			name: "CALL with transfer",
			data: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{{}}},
			want: false,
		},
		{
			name: "CALL with error",
			data: bchain.EthereumInternalData{Type: bchain.CALL, Error: "REVERT"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emptyInternalData(&tt.data); got != tt.want {
				t.Errorf("emptyInternalData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_internalDataEqual(t *testing.T) {
	transfer := func(t bchain.EthereumInternalTransactionType, from, to string, value int64) bchain.EthereumInternalTransfer {
		return bchain.EthereumInternalTransfer{Type: t, From: from, To: to, Value: *big.NewInt(value)}
	}
	tronParser := tron.NewTronParser(1, false)
	ethParser := eth.NewEthereumParser(1, false)
	tests := []struct {
		name     string
		parser   bchain.BlockChainParser
		stored   bchain.EthereumInternalData
		computed bchain.EthereumInternalData
		want     bool
	}{
		{
			name: "identical CALL with transfers",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 700000),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 700000),
			}},
			want: true,
		},
		{
			// the top-level type is packed as a single CALL|CREATE bit, so a
			// SELFDESTRUCT computed from the backend unpacks from the DB as CALL
			name: "stored lossy CALL equals computed SELFDESTRUCT",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", 5),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.SELFDESTRUCT, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", 5),
			}},
			want: true,
		},
		{
			// a failed contract creation carries an empty/unparseable contract, which
			// processInternalData demotes to CALL before packing; the computed CREATE
			// must be demoted the same way, otherwise it looks unequal and is
			// reconnected - and re-counted - on every sweep
			name:     "computed failed CREATE with empty contract equals stored CALL",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: ""},
			want:     true,
		},
		{
			name: "computed failed CREATE with transfer equals stored demoted CALL",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CREATE, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", 5),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "", Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CREATE, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "", 5),
			}},
			want: true,
		},
		{
			name:     "CREATE does not equal CALL",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     false,
		},
		{
			name:     "CREATE with different contract",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     false,
		},
		{
			name:     "CREATE with same contract",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			want:     true,
		},
		{
			name:   "different transfer count",
			stored: bchain.EthereumInternalData{Type: bchain.CALL},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 1),
			}},
			want: false,
		},
		{
			name: "different transfer value",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 1),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", 2),
			}},
			want: false,
		},
		{
			// the stored error message is transformed on unpack, comparing it
			// would cause endless reprocessing of failed transactions
			name:     "error is ignored",
			stored:   bchain.EthereumInternalData{Type: bchain.CALL, Error: "Error(REVERT)", Transfers: []bchain.EthereumInternalTransfer{transfer(bchain.CALL, "a", "b", 1)}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Error: "REVERT", Transfers: []bchain.EthereumInternalTransfer{transfer(bchain.CALL, "a", "b", 1)}},
			want:     true,
		},
		{
			// stored contract unpacks EIP55 checksummed, the computed one comes
			// lowercase from the trace - same address must compare equal
			name:     "eth CREATE contract differing only in case",
			parser:   ethParser,
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5C43B1eD97e52d009611D89b74fA829FE4ac56b1"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5c43b1ed97e52d009611d89b74fa829fe4ac56b1"},
			want:     true,
		},
		{
			name:   "eth transfer addresses differing only in case",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x8eB187136a5B4D3110A275c00918884F9BEcffFC", 42),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "0x8eb187136a5b4d3110a275c00918884f9becfffc", 42),
			}},
			want: true,
		},
		{
			name:     "eth CREATE with different contract",
			parser:   ethParser,
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x5C43B1eD97e52d009611D89b74fA829FE4ac56b1"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x8eb187136a5b4d3110a275c00918884f9becfffc"},
			want:     false,
		},
		{
			// the tron parser accepts both the base58 and the raw hex form,
			// mixed representations of the same address compare equal
			name:     "tron CREATE contract in mixed base58 and hex form",
			stored:   bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"},
			computed: bchain.EthereumInternalData{Type: bchain.CREATE, Contract: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"},
			want:     true,
		},
		{
			// the packed DB form stores a missing transfer address as the zero
			// address, so the unpacked zero address equals an empty computed one
			name:   "eth stored zero address equals computed empty address",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x0000000000000000000000000000000000000000", 0),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "", 0),
			}},
			want: true,
		},
		{
			name: "tron stored zero address equals computed empty address",
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", 0),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.SELFDESTRUCT, "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", "", 0),
			}},
			want: true,
		},
		{
			name:   "empty address does not equal a non-zero address",
			parser: ethParser,
			stored: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50D9090D6ce6307b7EC8904cD3dCa17b4Da56353", "0x8eB187136a5B4D3110A275c00918884F9BEcffFC", 1),
			}},
			computed: bchain.EthereumInternalData{Type: bchain.CALL, Transfers: []bchain.EthereumInternalTransfer{
				transfer(bchain.CALL, "0x50d9090d6ce6307b7ec8904cd3dca17b4da56353", "", 1),
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := tt.parser
			if parser == nil {
				parser = tronParser
			}
			if got := internalDataEqual(parser, &tt.stored, &tt.computed); got != tt.want {
				t.Errorf("internalDataEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
