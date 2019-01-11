package xzc

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/jakm/btcutil/chaincfg"
)

const (
	OpZeroCoinMint = 0xc1

	MainnetMagic wire.BitcoinNet = 0xe3d9fef1
	TestnetMagic wire.BitcoinNet = 0xcffcbeea
	RegtestMagic wire.BitcoinNet = 0xfabfb5da

	ZCGenesisBlockTime     = 1414776286
	SwitchToMTPBlockHeader = 1544443200
	MTPL                   = 64
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics
	MainNetParams.AddressMagicLen = 1
	MainNetParams.PubKeyHashAddrID = []byte{0x52}
	MainNetParams.ScriptHashAddrID = []byte{0x07}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.AddressMagicLen = 1
	TestNetParams.PubKeyHashAddrID = []byte{0x41}
	TestNetParams.ScriptHashAddrID = []byte{0xb2}

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
}

// ZcoinParser handle
type ZcoinParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewZcoinParser returns new ZcoinParser instance
func NewZcoinParser(params *chaincfg.Params, c *btc.Configuration) *ZcoinParser {
	return &ZcoinParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Zcoin network,
// the regression test Zcoin network, the test Zcoin network and
// the simulation test Zcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *ZcoinParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	if len(addrDesc) > 0 && addrDesc[0] == OpZeroCoinMint {
		return []string{fmt.Sprintf("OP_ZEROCOINMINT %d %s", addrDesc[5], hex.EncodeToString(addrDesc[6:]))}, false, nil
	}

	return p.OutputScriptToAddressesFunc(addrDesc)
}

// PackTx packs transaction to byte array using protobuf
func (p *ZcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *ZcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// ParseBlock parses raw block to our Block struct
func (p *ZcoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	buf := bytes.NewReader(b)

	blockHeader, err := parseBlockHeader(buf)
	if err != nil {
		return nil, err
	}

	var mtpBlockHeader = &MTPBlockHeader{}
	var mtpHashData = &MTPHashData{}
	if isMTP(blockHeader) {
		err = binary.Read(buf, binary.LittleEndian, mtpBlockHeader)
		if err != nil {
			fmt.Printf("y\n")
			return nil, err
		}
		err = binary.Read(buf, binary.LittleEndian, mtpHashData)
		if err != nil {
			fmt.Printf("y\n")
			return nil, err
		}

		// var nProofMTP [MTPL * 3]uint8
		for i := 0; i < MTPL*3; i++ {
			var numberProofBlocks uint8
			err = binary.Read(buf, binary.LittleEndian, &numberProofBlocks)
			if err != nil {
				return nil, err
			}

			for j := uint8(0); j < numberProofBlocks; j++ {
				var mtpData [16]uint8
				err = binary.Read(buf, binary.LittleEndian, mtpData[:])
				if err != nil {
					return nil, err
				}
				// discard nProofMTP: dont need it now
			}
		}
	}

	// parse txs
	txCount, err := wire.ReadVarInt(buf, 0)
	if err != nil {
		return nil, err
	}

	var txs = make([]bchain.Tx, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := wire.MsgTx{}
		err := tx.BtcDecode(buf, 0, wire.WitnessEncoding)
		if err != nil {
			return nil, err
		}
		txs[i] = p.TxFromMsgTx(&tx, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: blockHeader.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

func parseBlockHeader(buf io.Reader) (*wire.BlockHeader, error) {
	var h = &wire.BlockHeader{}
	err := h.Deserialize(buf)
	return h, err
}

func isMTP(h *wire.BlockHeader) bool {
	return h.Timestamp.Unix() > ZCGenesisBlockTime && h.Timestamp.Unix() >= SwitchToMTPBlockHeader
}

type MTPHashData struct {
	HashRootMTP [16]uint8
	BlockMTP    [128][128]uint64
}

type MTPBlockHeader struct {
	VersionMTP   int32
	MTPHashValue chainhash.Hash
	Reserved1    chainhash.Hash
	Reserved2    chainhash.Hash
}
