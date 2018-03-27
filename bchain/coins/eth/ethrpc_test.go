package eth

import (
	"blockbook/bchain"
	"reflect"
	"testing"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

var rpcURL = "ws://10.34.3.4:18546"
var ethClient *ethclient.Client
var ethRPCClient *rpc.Client

func setupEthRPC() *EthRPC {
	if ethClient == nil {
		rc, err := rpc.Dial(rpcURL)
		if err != nil {
			panic(err)
		}
		ec := ethclient.NewClient(rc)
		ethRPCClient = rc
		ethClient = ec
	}
	return &EthRPC{
		client:  ethClient,
		rpc:     ethRPCClient,
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
		wantErr error
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
		{
			name: "ErrBlockNotFound",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				height: 1 << 31,
			},
			want:    "",
			wantErr: bchain.ErrBlockNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlockHash(tt.args.height)
			if err != tt.wantErr {
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
		wantErr error
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
		{
			name: "ErrBlockNotFound",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				hash: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903896",
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
		name        string
		fields      fields
		args        args
		want        *bchain.Block
		wantTxCount int
		wantErr     error
	}{
		{
			name: "2870000 by hash",
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
			wantTxCount: 12,
		},
		{
			name: "2870000 by height",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				height: 2870000,
			},
			want: &bchain.Block{
				BlockHeader: bchain.BlockHeader{
					Hash:          "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903895",
					Height:        2870000,
					Confirmations: int(uint32(bh.Number.Uint64()) - 2870000),
				},
			},
			wantTxCount: 12,
		},
		{
			name: "ErrBlockNotFound",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				hash: "eccd6b0031015a19cb7d4e10f28590ba65a6a54ad1baa322b50fe5ad16903896",
			},
			wantErr: bchain.ErrBlockNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetBlock(tt.args.hash, tt.args.height)
			if err != tt.wantErr {
				t.Errorf("EthRPC.GetBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == nil && tt.want == nil {
				return
			}
			if got.Hash != tt.want.Hash {
				t.Errorf("EthRPC.GetBlock().Hash = %v, want %v", got.Hash, tt.want.Hash)
				return
			}
			if got.Height != tt.want.Height {
				t.Errorf("EthRPC.GetBlock().Height = %v, want %v", got.Height, tt.want.Height)
				return
			}
			if got.Confirmations != tt.want.Confirmations {
				t.Errorf("EthRPC.GetBlock().Confirmations = %v, want %v", got.Confirmations, tt.want.Confirmations)
				return
			}
			if len(got.Txs) != tt.wantTxCount {
				t.Errorf("EthRPC.GetBlock().Txs = %v, want %v", len(got.Txs), tt.wantTxCount)
				return
			}
		})
	}
}

func TestEthRPC_GetTransaction(t *testing.T) {
	bh, err := setupEthRPC().getBestHeader()
	if err != nil {
		panic(err)
	}
	type fields struct {
		b *EthRPC
	}
	type args struct {
		txid string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *bchain.Tx
		wantErr bool
	}{
		{
			name: "1",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				txid: "e6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d",
			},
			want: &bchain.Tx{
				Blocktime:     1521515026,
				Confirmations: uint32(bh.Number.Uint64()) - 2870000,
				Hex:           "7b226e6f6e6365223a2230783239666165222c226761735072696365223a223078313261303566323030222c22676173223a2230786462626130222c22746f223a22307836383262373930336131313039386366373730633761656634616130326138356233663336303161222c2276616c7565223a22307830222c22696e707574223a223078663032356361616630303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030323235222c2268617368223a22307865366231363864366262336438656437386530336462663832386236626664316662363133663665313239636261363234393634393834353533373234633564222c22426c6f636b4e756d626572223a223078326263616630222c22426c6f636b48617368223a22307865636364366230303331303135613139636237643465313066323835393062613635613661353461643162616133323262353066653561643136393033383935222c2246726f6d223a22307864616363396336313735346130633436313666633533323364633934366538396562323732333032222c227472616e73616374696f6e496e646578223a22307831222c2276223a2230783162222c2272223a22307831626434306133313132326330333931386466366431363664373430613661336132326630386132353933346365623136383863363239373736363163383063222c2273223a22307836303766626331356331663739393561343235386635613962636363363362303430333632643139393164356566653133363163353632323265346361383966227d",
				Time:          1521515026,
				Txid:          "e6b168d6bb3d8ed78e03dbf828b6bfd1fb613f6e129cba624964984553724c5d",
				Vin: []bchain.Vin{
					{
						Addresses: []string{"dacc9c61754a0c4616fc5323dc946e89eb272302"},
					},
				},
				Vout: []bchain.Vout{
					{
						N: uint32(1),
						ScriptPubKey: bchain.ScriptPubKey{
							Addresses: []string{"682b7903a11098cf770c7aef4aa02a85b3f3601a"},
						},
					},
				},
			},
		},
		{
			name: "1",
			fields: fields{
				b: setupEthRPC(),
			},
			args: args{
				txid: "cd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
			},
			want: &bchain.Tx{
				Blocktime:     1521533434,
				Confirmations: uint32(bh.Number.Uint64()) - 2871048,
				Hex:           "7b226e6f6e6365223a22307862323663222c226761735072696365223a223078343330653233343030222c22676173223a22307835323038222c22746f223a22307835353565653131666264646330653439613962616233353861383934316164393566666462343866222c2276616c7565223a22307831626330313539643533306536303030222c22696e707574223a223078222c2268617368223a22307863643634373135313535326235313332623261656637633962653030646336663733616663353930316464653135376161623133313333356261616138353362222c22426c6f636b4e756d626572223a223078326263663038222c22426c6f636b48617368223a22307863303266396632623736633265393537643464656139643030366263643636356239303462613866383461653466343836373561383662373536326461366239222c2246726f6d223a22307833653361336436396463363662613130373337663533316564303838393534613965633839643937222c227472616e73616374696f6e496e646578223a22307861222c2276223a2230783239222c2272223a22307866373136316331373064343335373361643963386437303163646166373134666632613534386135363262306463363339323330643137383839666364343035222c2273223a22307833633439373766633930333835613237656661303033326531376234396664353735623238323663623536653364316563663231353234663261393466393135227d",
				Time:          1521533434,
				Txid:          "cd647151552b5132b2aef7c9be00dc6f73afc5901dde157aab131335baaa853b",
				Vin: []bchain.Vin{
					{
						Addresses: []string{"3e3a3d69dc66ba10737f531ed088954a9ec89d97"},
					},
				},
				Vout: []bchain.Vout{
					{
						N: uint32(10),
						ScriptPubKey: bchain.ScriptPubKey{
							Addresses: []string{"555ee11fbddc0e49a9bab358a8941ad95ffdb48f"},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.b.GetTransaction(tt.args.txid)
			if (err != nil) != tt.wantErr {
				t.Errorf("EthRPC.GetTransaction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EthRPC.GetTransaction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEthRPC_EstimateFee(t *testing.T) {
	type fields struct {
		b *EthRPC
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
				b: setupEthRPC(),
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
