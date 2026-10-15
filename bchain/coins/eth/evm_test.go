package eth

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// Sepolia block 11856337, the first Glamsterdam block (adds blockAccessListHash and slotNumber)
const sepoliaGlamsterdamHeader = `{"hash":"0xa03f956aeb69d3fa234d9894c4309f4bb089cca9b6440cb45066c9ba39222588","parentHash":"0x862fbae116b7dca00b550ad371540de5ec8f627065bead949809ee6228b8f3eb","sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347","miner":"0x3826539cbd8d68dcf119e80b994557b4278cec9f","stateRoot":"0xfa3dce0649b084038be0d2e88b0740fdd50631fc32109c0f7766fa242baf2c39","transactionsRoot":"0x4370e25f6d627e5e9efca9cb5405f38c5ae86043e5b400ea92992d51aba12b6c","receiptsRoot":"0x636dbd7afafef168dee179457d140e251ddb3c203596d148338d1ee813f1f034","logsBloom":"0x8014a800082084804806100090009a2028220011620000168200608488100546c288100801c811022248210a01200222421950128ec100000c2892242226031a711c0551a00a220002602a58404900c0e00002c06600081121030400948540010a20000122284012608548c102604c1830800040e00c22483468a51004090118a500008009344a0009a080504206014121402041100a20081084004504201641028000810000410a803001480001041e00005000a0d0c1308a40020a02044001490f008601010220003d04004ba2000252120200040880384000800001c0694000d0101900b01810ac4814002ca0800450001800884500400207080200010208","difficulty":"0x0","number":"0xb4e9d1","gasLimit":"0x3938700","gasUsed":"0xd1dda4","timestamp":"0x6ac4fd60","extraData":"0x626573752032362e392e31","mixHash":"0x4ba9dfc9ce033a38b9f2d6d4b549e4b3a398dbcd8dd7c514d944f42c9a1a4405","nonce":"0x0000000000000000","baseFeePerGas":"0x3d7386e5","withdrawalsRoot":"0x5127063b341e2ad8b2236316204563d3025b6b9b61290fec666a0a0bba4286a7","blobGasUsed":"0x80000","excessBlobGas":"0xc6e7614","parentBeaconBlockRoot":"0xa982af12aa814307469427e140dd62cf6b6cbbcf0539fbab10336132e47b0743","requestsHash":"0xec88bf0d3fe6b86b583cf638c5635cb64bc842fee1e220f0e8be964a4d368c15","blockAccessListHash":"0xe6048b5d71a01b46691aa7d24238ba7880b789ae42173cab10a041b72e768a55","slotNumber":"0xac6000"}`

const sepoliaGlamsterdamHash = "0xa03f956aeb69d3fa234d9894c4309f4bb089cca9b6440cb45066c9ba39222588"

type headerService struct{}

func (headerService) GetBlockByNumber(number string, full bool) (json.RawMessage, error) {
	return json.RawMessage(sepoliaGlamsterdamHeader), nil
}

func (headerService) NewHeads(ctx context.Context) (*rpc.Subscription, error) {
	n, _ := rpc.NotifierFromContext(ctx)
	sub := n.CreateSubscription()
	go n.Notify(sub.ID, json.RawMessage(sepoliaGlamsterdamHeader))
	return sub, nil
}

func newHeaderRPC(t *testing.T) *rpc.Client {
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	if err := srv.RegisterName("eth", headerService{}); err != nil {
		t.Fatal(err)
	}
	return rpc.DialInProc(srv)
}

// header with fields unknown to go-ethereum -> node-reported hash
func TestEthereumClientHeaderByNumberKeepsNodeHash(t *testing.T) {
	c := &EthereumClient{Client: ethclient.NewClient(newHeaderRPC(t))}
	h, err := c.HeaderByNumber(context.Background(), big.NewInt(11856337))
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Hash(); got != sepoliaGlamsterdamHash {
		t.Errorf("Hash() = %s, want %s", got, sepoliaGlamsterdamHash)
	}
	if got := h.Number().Uint64(); got != 11856337 {
		t.Errorf("Number() = %d, want 11856337", got)
	}
}

// newHeads header -> node-reported hash
func TestEthereumNewBlockKeepsNodeHash(t *testing.T) {
	nb := NewEthereumNewBlock()
	sub, err := newHeaderRPC(t).EthSubscribe(context.Background(), nb.Channel(), "newHeads")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	h, ok := nb.Read()
	if !ok {
		t.Fatal("channel closed")
	}
	if got := h.Hash(); got != sepoliaGlamsterdamHash {
		t.Errorf("Hash() = %s, want %s", got, sepoliaGlamsterdamHash)
	}
}

// header without a hash field -> decode error
func TestEthereumHeaderWithoutHash(t *testing.T) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sepoliaGlamsterdamHeader), &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "hash")
	data, _ := json.Marshal(fields)
	var h EthereumHeader
	if err := json.Unmarshal(data, &h); err == nil {
		t.Errorf("Unmarshal succeeded, Hash() = %s", h.Hash())
	}
}
