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

	GenesisBlockTime       = 1414776286
	SwitchToMTPBlockHeader = 1544443200
	MTPL                   = 64
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	// mainnet
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	MainNetParams.AddressMagicLen = 1
	MainNetParams.PubKeyHashAddrID = []byte{0x52}
	MainNetParams.ScriptHashAddrID = []byte{0x07}

	// testnet
	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	TestNetParams.AddressMagicLen = 1
	TestNetParams.PubKeyHashAddrID = []byte{0x41}
	TestNetParams.ScriptHashAddrID = []byte{0xb2}

	// regtest
	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
}

// ZcoinParser handle
type ZcoinParser struct {
	*btc.BitcoinParser
}

// NewZcoinParser returns new ZcoinParser instance
func NewZcoinParser(params *chaincfg.Params, c *btc.Configuration) *ZcoinParser {
	return &ZcoinParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
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
	return p.BaseParser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *ZcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.BaseParser.UnpackTx(buf)
}

// ParseBlock parses raw block to our Block struct
func (p *ZcoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	reader := bytes.NewReader(b)

	// parse standard block header first
	header, err := parseBlockHeader(reader)
	if err != nil {
		return nil, err
	}

	// then MTP header
	if isMTP(header) {
		mtpHeader := MTPBlockHeader{}
		mtpHashData := MTPHashData{}

		// header
		err = binary.Read(reader, binary.LittleEndian, &mtpHeader)
		if err != nil {
			return nil, err
		}

		// hash data
		err = binary.Read(reader, binary.LittleEndian, &mtpHashData)
		if err != nil {
			return nil, err
		}

		// proof
		for i := 0; i < MTPL*3; i++ {
			var numberProofBlocks uint8

			err = binary.Read(reader, binary.LittleEndian, &numberProofBlocks)
			if err != nil {
				return nil, err
			}

			for j := uint8(0); j < numberProofBlocks; j++ {
				var mtpData [16]uint8

				err = binary.Read(reader, binary.LittleEndian, mtpData[:])
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// parse txs
	ntx, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, ntx)

	for i := uint64(0); i < ntx; i++ {
		tx := wire.MsgTx{}

		err := tx.BtcDecode(reader, 0, wire.WitnessEncoding)
		if err != nil {
			return nil, err
		}

		txs[i] = p.TxFromMsgTx(&tx, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: header.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

func parseBlockHeader(r io.Reader) (*wire.BlockHeader, error) {
	h := &wire.BlockHeader{}
	err := h.Deserialize(r)
	return h, err
}

func isMTP(h *wire.BlockHeader) bool {
	epoch := h.Timestamp.Unix()

	// the genesis block never be MTP block
	return epoch > GenesisBlockTime && epoch >= SwitchToMTPBlockHeader
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
