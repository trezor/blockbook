package avalanche

import (
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
)

var hasherPool = sync.Pool{
	New: func() interface{} { return sha3.NewLegacyKeccak256() },
}

func rlpHash(x interface{}) (h common.Hash) {
	sha := hasherPool.Get().(crypto.KeccakState)
	defer hasherPool.Put(sha)
	sha.Reset()
	_ = rlp.Encode(sha, x)
	_, _ = sha.Read(h[:])
	return h
}

// Header represents a block header in the Avalanche blockchain.
type Header struct {
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

func (obj *Header) EncodeRLP(_w io.Writer) error {
	w := rlp.NewEncoderBuffer(_w)
	_tmp0 := w.List()
	w.WriteBytes(obj.ParentHash[:])
	w.WriteBytes(obj.UncleHash[:])
	w.WriteBytes(obj.Coinbase[:])
	w.WriteBytes(obj.Root[:])
	w.WriteBytes(obj.TxHash[:])
	w.WriteBytes(obj.ReceiptHash[:])
	w.WriteBytes(obj.Bloom[:])
	if obj.Difficulty == nil {
		_, _ = w.Write(rlp.EmptyString)
	} else {
		if obj.Difficulty.Sign() == -1 {
			return rlp.ErrNegativeBigInt
		}
		w.WriteBigInt(obj.Difficulty)
	}
	if obj.Number == nil {
		_, _ = w.Write(rlp.EmptyString)
	} else {
		if obj.Number.Sign() == -1 {
			return rlp.ErrNegativeBigInt
		}
		w.WriteBigInt(obj.Number)
	}
	w.WriteUint64(obj.GasLimit)
	w.WriteUint64(obj.GasUsed)
	w.WriteUint64(obj.Time)
	w.WriteBytes(obj.Extra)
	w.WriteBytes(obj.MixDigest[:])
	w.WriteBytes(obj.Nonce[:])
	w.WriteBytes(obj.ExtDataHash[:])
	_tmp1 := obj.BaseFee != nil
	_tmp2 := obj.ExtDataGasUsed != nil
	_tmp3 := obj.BlockGasCost != nil
	_tmp4 := obj.BlobGasUsed != nil
	_tmp5 := obj.ExcessBlobGas != nil
	_tmp6 := obj.ParentBeaconRoot != nil
	if _tmp1 || _tmp2 || _tmp3 || _tmp4 || _tmp5 || _tmp6 {
		if obj.BaseFee == nil {
			_, _ = w.Write(rlp.EmptyString)
		} else {
			if obj.BaseFee.Sign() == -1 {
				return rlp.ErrNegativeBigInt
			}
			w.WriteBigInt(obj.BaseFee)
		}
	}
	if _tmp2 || _tmp3 || _tmp4 || _tmp5 || _tmp6 {
		if obj.ExtDataGasUsed == nil {
			_, _ = w.Write(rlp.EmptyString)
		} else {
			if obj.ExtDataGasUsed.Sign() == -1 {
				return rlp.ErrNegativeBigInt
			}
			w.WriteBigInt(obj.ExtDataGasUsed)
		}
	}
	if _tmp3 || _tmp4 || _tmp5 || _tmp6 {
		if obj.BlockGasCost == nil {
			_, _ = w.Write(rlp.EmptyString)
		} else {
			if obj.BlockGasCost.Sign() == -1 {
				return rlp.ErrNegativeBigInt
			}
			w.WriteBigInt(obj.BlockGasCost)
		}
	}
	if _tmp4 || _tmp5 || _tmp6 {
		if obj.BlobGasUsed == nil {
			_, _ = w.Write([]byte{0x80})
		} else {
			w.WriteUint64((*obj.BlobGasUsed))
		}
	}
	if _tmp5 || _tmp6 {
		if obj.ExcessBlobGas == nil {
			_, _ = w.Write([]byte{0x80})
		} else {
			w.WriteUint64((*obj.ExcessBlobGas))
		}
	}
	if _tmp6 {
		if obj.ParentBeaconRoot == nil {
			_, _ = w.Write([]byte{0x80})
		} else {
			w.WriteBytes(obj.ParentBeaconRoot[:])
		}
	}
	w.ListEnd(_tmp0)
	return w.Flush()
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}
