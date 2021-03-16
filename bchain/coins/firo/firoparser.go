package firo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"

	"github.com/martinboehm/btcd/chaincfg/chainhash"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	OpZeroCoinMint      = 0xc1
	OpZeroCoinSpend     = 0xc2
	OpSigmaMint         = 0xc3
	OpSigmaSpend        = 0xc4
	OpLelantusMint      = 0xc5
	OpLelantusJMint     = 0xc6
	OpLelantusJoinSplit = 0xc7

	MainnetMagic wire.BitcoinNet = 0xe3d9fef1
	TestnetMagic wire.BitcoinNet = 0xcffcbeea
	RegtestMagic wire.BitcoinNet = 0xfabfb5da

	GenesisBlockTime       = 1414776286
	SwitchToMTPBlockHeader = 1544443200
	MTPL                   = 64

	SpendTxID = "0000000000000000000000000000000000000000000000000000000000000000"

	TransactionQuorumCommitmentType = 6
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

// FiroParser handle
type FiroParser struct {
	*btc.BitcoinParser
}

// NewFiroParser returns new FiroParser instance
func NewFiroParser(params *chaincfg.Params, c *btc.Configuration) *FiroParser {
	return &FiroParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
	}
}

// GetChainParams contains network parameters for the main Firo network,
// the regression test Firo network, the test Firo network and
// the simulation test Firo network, in this order
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
func (p *FiroParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {

	if len(addrDesc) > 0 {
		switch addrDesc[0] {
		case OpZeroCoinMint:
			return []string{"Zeromint"}, false, nil
		case OpZeroCoinSpend:
			return []string{"Zerospend"}, false, nil
		case OpSigmaMint:
			return []string{"Sigmamint"}, false, nil
		case OpSigmaSpend:
			return []string{"Sigmaspend"}, false, nil
		case OpLelantusMint:
			return []string{"LelantusMint"}, false, nil
		case OpLelantusJMint:
			return []string{"LelantusJMint"}, false, nil
		case OpLelantusJoinSplit:
			return []string{"LelantusJoinSplit"}, false, nil
		}
	}

	return p.OutputScriptToAddressesFunc(addrDesc)
}

// PackTx packs transaction to byte array using protobuf
func (p *FiroParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.BaseParser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *FiroParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.BaseParser.UnpackTx(buf)
}

// TxFromFiroMsgTx converts bitcoin wire Tx to bchain.Tx
func (p *FiroParser) TxFromFiroMsgTx(t *FiroMsgTx, parseAddresses bool) bchain.Tx {
	btx := p.TxFromMsgTx(&t.MsgTx, parseAddresses)

	// NOTE: wire.MsgTx.TxHash() doesn't include extra
	btx.Txid = t.TxHash().String()

	return btx
}

// ParseBlock parses raw block to our Block struct
func (p *FiroParser) ParseBlock(b []byte) (*bchain.Block, error) {
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
		tx := FiroMsgTx{}

		// read version and seek back
		var version uint32 = 0
		if err = binary.Read(reader, binary.LittleEndian, &version); err != nil {
			return nil, err
		}

		if _, err = reader.Seek(-4, io.SeekCurrent); err != nil {
			return nil, err
		}

		txVersion := version & 0xffff
		txType := (version >> 16) & 0xffff

		enc := wire.WitnessEncoding

		// transaction quorum commitment could not be parsed with witness flag
		if txVersion == 3 && txType == TransactionQuorumCommitmentType {
			enc = wire.BaseEncoding
		}

		if err = tx.FiroDecode(reader, 0, enc); err != nil {
			return nil, err
		}

		btx := p.TxFromFiroMsgTx(&tx, false)

		if err = p.parseFiroTx(&btx); err != nil {
			return nil, err
		}

		txs[i] = btx
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: header.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *FiroParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
	var tx bchain.Tx
	err := json.Unmarshal(msg, &tx)
	if err != nil {
		return nil, err
	}

	for i := range tx.Vout {
		vout := &tx.Vout[i]
		// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
		vout.ValueSat, err = p.AmountToBigInt(vout.JsonValue)
		if err != nil {
			return nil, err
		}
		vout.JsonValue = ""
	}

	p.parseFiroTx(&tx)

	return &tx, nil
}

func (p *FiroParser) parseFiroTx(tx *bchain.Tx) error {
	for i := range tx.Vin {
		vin := &tx.Vin[i]

		// FIXME: right now we treat zerocoin spend vin as coinbase
		// change this after blockbook support special type of vin
		if vin.Txid == SpendTxID {
			vin.Coinbase = vin.Txid
			vin.Txid = ""
			vin.Sequence = 0
			vin.Vout = 0
		}
	}

	return nil
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
