package liquid

import (
	"strconv"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/martinboehm/btcd/txscript"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xdab5bffa
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{57}
	MainNetParams.ScriptHashAddrID = []byte{39}
	// BLINDED_ADDRESS 12
}

// LiquidParser handle
type LiquidParser struct {
	*btc.BitcoinLikeParser
	baseparser                      *bchain.BaseParser
	origOutputScriptToAddressesFunc btc.OutputScriptToAddressesFunc
}

// NewLiquidParser returns new LiquidParser instance
func NewLiquidParser(params *chaincfg.Params, c *btc.Configuration) *LiquidParser {
	p := &LiquidParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
	p.origOutputScriptToAddressesFunc = p.OutputScriptToAddressesFunc
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main GameCredits network,
// and the test GameCredits network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	default:
		return &MainNetParams
	}
}

// PackTx packs transaction to byte array using protobuf
func (p *LiquidParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *LiquidParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// GetAddrDescForUnknownInput processes inputs that were not found in txAddresses - they are bitcoin transactions
// create a special script for the input in the form OP_INVALIDOPCODE <txid> <vout varint>
func (p *LiquidParser) GetAddrDescForUnknownInput(tx *bchain.Tx, input int) bchain.AddressDescriptor {
	var iTxid string
	s := make([]byte, 0, 40)
	if len(tx.Vin) > input {
		iTxid = tx.Vin[input].Txid
		btxID, err := p.PackTxid(iTxid)
		if err == nil {
			buf := make([]byte, vlq.MaxLen64)
			l := vlq.PutInt(buf, int64(tx.Vin[input].Vout))
			s = append(s, txscript.OP_INVALIDOPCODE)
			s = append(s, btxID...)
			s = append(s, buf[:l]...)
		}
	}
	glog.Info("tx ", tx.Txid, ", encountered Bitcoin tx ", iTxid)
	return s
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *LiquidParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	// minimum length of the special script OP_INVALIDOPCODE <txid> <index varint> is 34 bytes (1 byte opcode, 32 bytes tx, 1 byte vout)
	if len(script) > 33 && script[0] == txscript.OP_INVALIDOPCODE {
		txid, _ := p.UnpackTxid(script[1:33])
		vout, _ := vlq.Int(script[33:])
		return []string{
			"Bitcoin tx " + txid + ":" + strconv.Itoa(int(vout)),
		}, false, nil
	}
	return p.origOutputScriptToAddressesFunc(script)
}
