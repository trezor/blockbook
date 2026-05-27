package avalanche

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// Header represents a block header in the Avalanche blockchain.
type Header struct {
	RpcHash     common.Hash      `json:"hash"             gencodec:"required"`
	ParentHash  common.Hash      `json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash      `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address   `json:"miner"            gencodec:"required"`
	Root        common.Hash      `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash      `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash      `json:"receiptsRoot"     gencodec:"required"`
	Bloom       types.Bloom      `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int         `json:"difficulty"       gencodec:"required"`
	Number      *big.Int         `json:"number"           gencodec:"required"`
	GasLimit    uint64           `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64           `json:"gasUsed"          gencodec:"required"`
	Time        uint64           `json:"timestamp"        gencodec:"required"`
	Extra       []byte           `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash      `json:"mixHash"`
	Nonce       types.BlockNonce `json:"nonce"`
	ExtDataHash common.Hash      `json:"extDataHash"      gencodec:"required"`

	// BaseFee was added by EIP-1559 and is ignored in legacy headers.
	BaseFee *big.Int `json:"baseFeePerGas" rlp:"optional"`

	// ExtDataGasUsed was added by Apricot Phase 4 and is ignored in legacy
	// headers.
	//
	// It is not a uint64 like GasLimit or GasUsed because it is not possible to
	// correctly encode this field optionally with uint64.
	ExtDataGasUsed *big.Int `json:"extDataGasUsed" rlp:"optional"`

	// BlockGasCost was added by Apricot Phase 4 and is ignored in legacy
	// headers.
	BlockGasCost *big.Int `json:"blockGasCost" rlp:"optional"`

	// BlobGasUsed was added by EIP-4844 and is ignored in legacy headers.
	BlobGasUsed *uint64 `json:"blobGasUsed" rlp:"optional"`

	// ExcessBlobGas was added by EIP-4844 and is ignored in legacy headers.
	ExcessBlobGas *uint64 `json:"excessBlobGas" rlp:"optional"`

	// ParentBeaconRoot was added by EIP-4788 and is ignored in legacy headers.
	ParentBeaconRoot *common.Hash `json:"parentBeaconBlockRoot" rlp:"optional"`
}

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		ParentHash       common.Hash      `json:"parentHash"       gencodec:"required"`
		UncleHash        common.Hash      `json:"sha3Uncles"       gencodec:"required"`
		Coinbase         common.Address   `json:"miner"            gencodec:"required"`
		Root             common.Hash      `json:"stateRoot"        gencodec:"required"`
		TxHash           common.Hash      `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash      common.Hash      `json:"receiptsRoot"     gencodec:"required"`
		Bloom            types.Bloom      `json:"logsBloom"        gencodec:"required"`
		Difficulty       *hexutil.Big     `json:"difficulty"       gencodec:"required"`
		Number           *hexutil.Big     `json:"number"           gencodec:"required"`
		GasLimit         hexutil.Uint64   `json:"gasLimit"         gencodec:"required"`
		GasUsed          hexutil.Uint64   `json:"gasUsed"          gencodec:"required"`
		Time             hexutil.Uint64   `json:"timestamp"        gencodec:"required"`
		Extra            hexutil.Bytes    `json:"extraData"        gencodec:"required"`
		MixDigest        common.Hash      `json:"mixHash"`
		Nonce            types.BlockNonce `json:"nonce"`
		ExtDataHash      common.Hash      `json:"extDataHash"      gencodec:"required"`
		BaseFee          *hexutil.Big     `json:"baseFeePerGas" rlp:"optional"`
		ExtDataGasUsed   *hexutil.Big     `json:"extDataGasUsed" rlp:"optional"`
		BlockGasCost     *hexutil.Big     `json:"blockGasCost" rlp:"optional"`
		BlobGasUsed      *hexutil.Uint64  `json:"blobGasUsed" rlp:"optional"`
		ExcessBlobGas    *hexutil.Uint64  `json:"excessBlobGas" rlp:"optional"`
		ParentBeaconRoot *common.Hash     `json:"parentBeaconBlockRoot" rlp:"optional"`
		Hash             common.Hash      `json:"hash"`
	}
	var enc Header
	enc.ParentHash = h.ParentHash
	enc.UncleHash = h.UncleHash
	enc.Coinbase = h.Coinbase
	enc.Root = h.Root
	enc.TxHash = h.TxHash
	enc.ReceiptHash = h.ReceiptHash
	enc.Bloom = h.Bloom
	enc.Difficulty = (*hexutil.Big)(h.Difficulty)
	enc.Number = (*hexutil.Big)(h.Number)
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Time = hexutil.Uint64(h.Time)
	enc.Extra = h.Extra
	enc.MixDigest = h.MixDigest
	enc.Nonce = h.Nonce
	enc.ExtDataHash = h.ExtDataHash
	enc.BaseFee = (*hexutil.Big)(h.BaseFee)
	enc.ExtDataGasUsed = (*hexutil.Big)(h.ExtDataGasUsed)
	enc.BlockGasCost = (*hexutil.Big)(h.BlockGasCost)
	enc.BlobGasUsed = (*hexutil.Uint64)(h.BlobGasUsed)
	enc.ExcessBlobGas = (*hexutil.Uint64)(h.ExcessBlobGas)
	enc.ParentBeaconRoot = h.ParentBeaconRoot
	enc.Hash = h.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		RpcHash          *common.Hash      `json:"hash"`
		ParentHash       *common.Hash      `json:"parentHash"       gencodec:"required"`
		UncleHash        *common.Hash      `json:"sha3Uncles"       gencodec:"required"`
		Coinbase         *common.Address   `json:"miner"            gencodec:"required"`
		Root             *common.Hash      `json:"stateRoot"        gencodec:"required"`
		TxHash           *common.Hash      `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash      *common.Hash      `json:"receiptsRoot"     gencodec:"required"`
		Bloom            *types.Bloom      `json:"logsBloom"        gencodec:"required"`
		Difficulty       *hexutil.Big      `json:"difficulty"       gencodec:"required"`
		Number           *hexutil.Big      `json:"number"           gencodec:"required"`
		GasLimit         *hexutil.Uint64   `json:"gasLimit"         gencodec:"required"`
		GasUsed          *hexutil.Uint64   `json:"gasUsed"          gencodec:"required"`
		Time             *hexutil.Uint64   `json:"timestamp"        gencodec:"required"`
		Extra            *hexutil.Bytes    `json:"extraData"        gencodec:"required"`
		MixDigest        *common.Hash      `json:"mixHash"`
		Nonce            *types.BlockNonce `json:"nonce"`
		ExtDataHash      *common.Hash      `json:"extDataHash"      gencodec:"required"`
		BaseFee          *hexutil.Big      `json:"baseFeePerGas" rlp:"optional"`
		ExtDataGasUsed   *hexutil.Big      `json:"extDataGasUsed" rlp:"optional"`
		BlockGasCost     *hexutil.Big      `json:"blockGasCost" rlp:"optional"`
		BlobGasUsed      *hexutil.Uint64   `json:"blobGasUsed" rlp:"optional"`
		ExcessBlobGas    *hexutil.Uint64   `json:"excessBlobGas" rlp:"optional"`
		ParentBeaconRoot *common.Hash      `json:"parentBeaconBlockRoot" rlp:"optional"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.RpcHash == nil {
		return errors.New("missing required field 'hash' for Header")
	}
	h.RpcHash = *dec.RpcHash
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.UncleHash == nil {
		return errors.New("missing required field 'sha3Uncles' for Header")
	}
	h.UncleHash = *dec.UncleHash
	if dec.Coinbase == nil {
		return errors.New("missing required field 'miner' for Header")
	}
	h.Coinbase = *dec.Coinbase
	if dec.Root == nil {
		return errors.New("missing required field 'stateRoot' for Header")
	}
	h.Root = *dec.Root
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionsRoot' for Header")
	}
	h.TxHash = *dec.TxHash
	if dec.ReceiptHash == nil {
		return errors.New("missing required field 'receiptsRoot' for Header")
	}
	h.ReceiptHash = *dec.ReceiptHash
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Header")
	}
	h.Bloom = *dec.Bloom
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Header")
	}
	h.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = (*big.Int)(dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Time == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Time = uint64(*dec.Time)
	if dec.Extra == nil {
		return errors.New("missing required field 'extraData' for Header")
	}
	h.Extra = *dec.Extra
	if dec.MixDigest != nil {
		h.MixDigest = *dec.MixDigest
	}
	if dec.Nonce != nil {
		h.Nonce = *dec.Nonce
	}
	if dec.ExtDataHash == nil {
		return errors.New("missing required field 'extDataHash' for Header")
	}
	h.ExtDataHash = *dec.ExtDataHash
	if dec.BaseFee != nil {
		h.BaseFee = (*big.Int)(dec.BaseFee)
	}
	if dec.ExtDataGasUsed != nil {
		h.ExtDataGasUsed = (*big.Int)(dec.ExtDataGasUsed)
	}
	if dec.BlockGasCost != nil {
		h.BlockGasCost = (*big.Int)(dec.BlockGasCost)
	}
	if dec.BlobGasUsed != nil {
		h.BlobGasUsed = (*uint64)(dec.BlobGasUsed)
	}
	if dec.ExcessBlobGas != nil {
		h.ExcessBlobGas = (*uint64)(dec.ExcessBlobGas)
	}
	if dec.ParentBeaconRoot != nil {
		h.ParentBeaconRoot = dec.ParentBeaconRoot
	}
	return nil
}

// Hash returns the block hash of the header
func (h *Header) Hash() common.Hash {
	return h.RpcHash
}
