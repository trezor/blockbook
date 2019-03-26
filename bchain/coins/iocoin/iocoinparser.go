package iocoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	//"blockbook/bchain/coins/utils"
	"bytes"

	"github.com/martinboehm/btcd/wire"
	"github.com/golang/glog"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet =  0xfec3bade
	TestnetMagic wire.BitcoinNet =  0xffc4bbdf
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{103}
	MainNetParams.ScriptHashAddrID = []byte{85}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111} // starting with 'x' or 'y'
	TestNetParams.ScriptHashAddrID = []byte{96}
}

// IocoinParser handle
type IocoinParser struct {
	*btc.BitcoinParser
}

// NewIocoinParser returns new IocoinParser instance
func NewIocoinParser(params *chaincfg.Params, c *btc.Configuration) *IocoinParser {
	return &IocoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Iocoin network,
// and the test Iocoin network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}

// ParseBlock parses raw block to our Block struct
// it has special handling for Auxpow blocks that cannot be parsed by standard btc wire parser
func (p *IocoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	//w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
	}
        glog.Info(" h.Version ", h.Version)
	//if (h.Version & utils.VersionAuxpow) != 0 {
	//	if err = utils.SkipAuxpow(r); err != nil {
	//		return nil, err
	//	}
	//}

	//err = utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w)
	//if err != nil {
        //  return nil, err
        //}
	ntx, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return nil, err
	}
	glog.Info("XXXX ntx ", ntx)

	//txs := make([]bchain.Tx, len(w.Transactions))
	txs := make([]bchain.Tx, ntx)
	for i := uint64(0); i < ntx; i++ {
		tx := wire.MsgTx{}

		err := tx.BtcDecode(r, 0, wire.WitnessEncoding)
		if err != nil {
			return nil, err
		}

		btx := p.TxFromMsgTx(&tx, false)


		txs[i] = btx
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: h.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}
