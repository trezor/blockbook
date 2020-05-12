package bchain

import (
	"encoding/hex"
	"encoding/json"
	"encoding/binary"
	"math/big"
	"strings"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/juju/errors"
	vlq "github.com/bsm/go-vlq"
	"blockbook/common"
)

// BaseParser implements data parsing/handling functionality base for all other parsers
type BaseParser struct {
	BlockAddressesToKeep int
	AmountDecimalPoint   int
}

// ParseBlock parses raw block to our Block struct - currently not implemented
func (p *BaseParser) ParseBlock(b []byte) (*Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

// ParseTx parses byte array containing transaction and returns Tx struct - currently not implemented
func (p *BaseParser) ParseTx(b []byte) (*Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

// GetAddrDescForUnknownInput returns nil AddressDescriptor
func (p *BaseParser) GetAddrDescForUnknownInput(tx *Tx, input int) AddressDescriptor {
	var iTxid string
	if len(tx.Vin) > input {
		iTxid = tx.Vin[input].Txid
	}
	glog.Warningf("tx %v, input tx %v not found in txAddresses", tx.Txid, iTxid)
	return nil
}

const zeros = "0000000000000000000000000000000000000000"

// AmountToBigInt converts amount in common.JSONNumber (string) to big.Int
// it uses string operations to avoid problems with rounding
func (p *BaseParser) AmountToBigInt(n common.JSONNumber) (big.Int, error) {
	var r big.Int
	s := string(n)
	i := strings.IndexByte(s, '.')
	d := p.AmountDecimalPoint
	if d > len(zeros) {
		d = len(zeros)
	}
	if i == -1 {
		s = s + zeros[:d]
	} else {
		z := d - len(s) + i + 1
		if z > 0 {
			s = s[:i] + s[i+1:] + zeros[:z]
		} else {
			s = s[:i] + s[i+1:len(s)+z]
		}
	}
	if _, ok := r.SetString(s, 10); !ok {
		return r, errors.New("AmountToBigInt: failed to convert")
	}
	return r, nil
}

// AmountToDecimalString converts amount in big.Int to string with decimal point in the place defined by the parameter d
func AmountToDecimalString(a *big.Int, d int) string {
	if a == nil {
		return ""
	}
	n := a.String()
	var s string
	if n[0] == '-' {
		n = n[1:]
		s = "-"
	}
	if d > len(zeros) {
		d = len(zeros)
	}
	if len(n) <= d {
		n = zeros[:d-len(n)+1] + n
	}
	i := len(n) - d
	ad := strings.TrimRight(n[i:], "0")
	if len(ad) > 0 {
		n = n[:i] + "." + ad
	} else {
		n = n[:i]
	}
	return s + n
}

// AmountToDecimalString converts amount in big.Int to string with decimal point in the correct place
func (p *BaseParser) AmountToDecimalString(a *big.Int) string {
	return AmountToDecimalString(a, p.AmountDecimalPoint)
}

// AmountDecimals returns number of decimal places in amounts
func (p *BaseParser) AmountDecimals() int {
	return p.AmountDecimalPoint
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *BaseParser) ParseTxFromJson(msg json.RawMessage) (*Tx, error) {
	var tx Tx
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

	return &tx, nil
}

// PackedTxidLen returns length in bytes of packed txid
func (p *BaseParser) PackedTxidLen() int {
	return 32
}

func (p *BaseParser) PackedTxIndexLen() int {
	return p.PackedTxidLen()
}

// KeepBlockAddresses returns number of blocks which are to be kept in blockaddresses column
func (p *BaseParser) KeepBlockAddresses() int {
	return p.BlockAddressesToKeep
}

// PackTxid packs txid to byte array
func (p *BaseParser) PackTxid(txid string) ([]byte, error) {
	if txid == "" {
		return nil, ErrTxidMissing
	}
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *BaseParser) UnpackTxid(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *BaseParser) PackBlockHash(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *BaseParser) UnpackBlockHash(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// GetChainType is type of the blockchain, default is ChainBitcoinType
func (p *BaseParser) GetChainType() ChainType {
	return ChainBitcoinType
}

// MinimumCoinbaseConfirmations returns minimum number of confirmations a coinbase transaction must have before it can be spent
func (p *BaseParser) MinimumCoinbaseConfirmations() int {
	return 0
}

// PackTx packs transaction to byte array using protobuf
func (p *BaseParser) PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error) {
	var err error
	pti := make([]*ProtoTransaction_VinType, len(tx.Vin))
	for i, vi := range tx.Vin {
		hex, err := hex.DecodeString(vi.ScriptSig.Hex)
		if err != nil {
			return nil, errors.Annotatef(err, "Vin %v Hex %v", i, vi.ScriptSig.Hex)
		}
		// coinbase txs do not have Vin.txid
		itxid, err := p.PackTxid(vi.Txid)
		if err != nil && err != ErrTxidMissing {
			return nil, errors.Annotatef(err, "Vin %v Txid %v", i, vi.Txid)
		}
		pti[i] = &ProtoTransaction_VinType{
			Addresses:    vi.Addresses,
			Coinbase:     vi.Coinbase,
			ScriptSigHex: hex,
			Sequence:     vi.Sequence,
			Txid:         itxid,
			Vout:         vi.Vout,
		}
	}
	pto := make([]*ProtoTransaction_VoutType, len(tx.Vout))
	for i, vo := range tx.Vout {
		hex, err := hex.DecodeString(vo.ScriptPubKey.Hex)
		if err != nil {
			return nil, errors.Annotatef(err, "Vout %v Hex %v", i, vo.ScriptPubKey.Hex)
		}
		pto[i] = &ProtoTransaction_VoutType{
			Addresses:       vo.ScriptPubKey.Addresses,
			N:               vo.N,
			ScriptPubKeyHex: hex,
			ValueSat:        vo.ValueSat.Bytes(),
		}
	}
	pt := &ProtoTransaction{
		Blocktime: uint64(blockTime),
		Height:    height,
		Locktime:  tx.LockTime,
		Vin:       pti,
		Vout:      pto,
		Version:   tx.Version,
	}
	if pt.Hex, err = hex.DecodeString(tx.Hex); err != nil {
		return nil, errors.Annotatef(err, "Hex %v", tx.Hex)
	}
	if pt.Txid, err = p.PackTxid(tx.Txid); err != nil {
		return nil, errors.Annotatef(err, "Txid %v", tx.Txid)
	}
	return proto.Marshal(pt)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *BaseParser) UnpackTx(buf []byte) (*Tx, uint32, error) {
	var pt ProtoTransaction
	err := proto.Unmarshal(buf, &pt)
	if err != nil {
		return nil, 0, err
	}
	txid, err := p.UnpackTxid(pt.Txid)
	if err != nil {
		return nil, 0, err
	}
	vin := make([]Vin, len(pt.Vin))
	for i, pti := range pt.Vin {
		itxid, err := p.UnpackTxid(pti.Txid)
		if err != nil {
			return nil, 0, err
		}
		vin[i] = Vin{
			Addresses: pti.Addresses,
			Coinbase:  pti.Coinbase,
			ScriptSig: ScriptSig{
				Hex: hex.EncodeToString(pti.ScriptSigHex),
			},
			Sequence: pti.Sequence,
			Txid:     itxid,
			Vout:     pti.Vout,
		}
	}
	vout := make([]Vout, len(pt.Vout))
	for i, pto := range pt.Vout {
		var vs big.Int
		vs.SetBytes(pto.ValueSat)
		vout[i] = Vout{
			N: pto.N,
			ScriptPubKey: ScriptPubKey{
				Addresses: pto.Addresses,
				Hex:       hex.EncodeToString(pto.ScriptPubKeyHex),
			},
			ValueSat: vs,
		}
	}
	tx := Tx{
		Blocktime: int64(pt.Blocktime),
		Hex:       hex.EncodeToString(pt.Hex),
		LockTime:  pt.Locktime,
		Time:      int64(pt.Blocktime),
		Txid:      txid,
		Vin:       vin,
		Vout:      vout,
		Version:   pt.Version,
	}
	return &tx, pt.Height, nil
}

// IsAddrDescIndexable returns true if AddressDescriptor should be added to index
// by default all AddressDescriptors are indexable
func (p *BaseParser) IsAddrDescIndexable(addrDesc AddressDescriptor) bool {
	return true
}

// DerivationBasePath is unsupported
func (p *BaseParser) DerivationBasePath(xpub string) (string, error) {
	return "", errors.New("Not supported")
}

// DeriveAddressDescriptors is unsupported
func (p *BaseParser) DeriveAddressDescriptors(xpub string, change uint32, indexes []uint32) ([]AddressDescriptor, error) {
	return nil, errors.New("Not supported")
}

// DeriveAddressDescriptorsFromTo is unsupported
func (p *BaseParser) DeriveAddressDescriptorsFromTo(xpub string, change uint32, fromIndex uint32, toIndex uint32) ([]AddressDescriptor, error) {
	return nil, errors.New("Not supported")
}

// EthereumTypeGetErc20FromTx is unsupported
func (p *BaseParser) EthereumTypeGetErc20FromTx(tx *Tx) ([]Erc20Transfer, error) {
	return nil, errors.New("Not supported")
}

func (p *BaseParser) IsSyscoinTx(nVersion int32) bool {
	return false
}
func (p *BaseParser) IsTxIndexAsset(txIndex int32) bool {
	return false
}
func (p *BaseParser) IsSyscoinMintTx(nVersion int32) bool {
	return false
}
func (p *BaseParser) IsAssetTx(nVersion int32) bool {
    return false
}
func (p *BaseParser) IsAssetAllocationTx(nVersion int32) bool {
	return false
}
func (p *BaseParser) IsAssetSendTx(nVersion int32) bool {
	return false
}
func (p *BaseParser) IsAssetActivateTx(nVersion int32) bool {
	return false
}
func (p *BaseParser) GetAssetsMaskFromVersion(nVersion int32) AssetsMask {
	return BaseCoinMask
}
func (p *BaseParser) GetAssetTypeFromVersion(nVersion int32) TokenType {
	return SPTUnknownType
}
func (p *BaseParser) TryGetOPReturn(script []byte) []byte {
	return nil
}
func (p *BaseParser) GetMaxAddrLength() int {
	return 1024
}
func (p *BaseParser) PackAddrBalance(ab *AddrBalance, buf, varBuf []byte) []byte {
	return nil
}
func (p *BaseParser) UnpackAddrBalance(buf []byte, txidUnpackedLen int, detail AddressBalanceDetail) (*AddrBalance, error) {
	return nil, errors.New("Not supported")
}
func (p *BaseParser) PackAssetKey(assetGuid uint32, height uint32) []byte {
	return nil
}
func (p *BaseParser) UnpackAssetKey(buf []byte) (uint32, uint32) {
	return 0, 0
}
func (p *BaseParser) PackAssetTxIndex(txAsset *TxAsset) []byte {
	return nil
}
func (p *BaseParser) UnpackAssetTxIndex(buf []byte) []*TxAssetIndex {
	return nil
}
func (p *BaseParser) GetAssetFromData(sptData []byte) (*Asset, error) {
	return nil, errors.New("Not supported")
}
func (p *BaseParser) GetAllocationFromTx(tx *Tx) (*AssetAllocation, error) {
	return nil, errors.New("Not supported")
}
func (p *BaseParser) LoadAssets(tx *Tx) error {
	return errors.New("Not supported")
}
func (p *BaseParser) AppendAssetInfo(assetInfo *AssetInfo, buf []byte, varBuf []byte) []byte  {
	return nil
}
func (p *BaseParser) UnpackAssetInfo(assetInfo *AssetInfo, buf []byte) int  {
	return 0
}
const PackedHeightBytes = 4
func (p *BaseParser) PackAddressKey(addrDesc AddressDescriptor, height uint32) []byte {
	buf := make([]byte, len(addrDesc)+PackedHeightBytes)
	copy(buf, addrDesc)
	// pack height as binary complement to achieve ordering from newest to oldest block
	binary.BigEndian.PutUint32(buf[len(addrDesc):], ^height)
	return buf
}

func (p *BaseParser) UnpackAddressKey(key []byte) ([]byte, uint32, error) {
	i := len(key) - PackedHeightBytes
	if i <= 0 {
		return nil, 0, errors.New("Invalid address key")
	}
	// height is packed in binary complement, convert it
	return key[:i], ^p.UnpackUint(key[i : i+PackedHeightBytes]), nil
}

func (p *BaseParser) PackUint(i uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i)
	return buf
}

func (p *BaseParser) UnpackUint(buf []byte) uint32 {
	return binary.BigEndian.Uint32(buf)
}

func (p *BaseParser) PackVarint32(i int32, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func (p *BaseParser) PackVarint(i int, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func (p *BaseParser) PackVaruint(i uint, buf []byte) int {
	return vlq.PutUint(buf, uint64(i))
}

func (p *BaseParser) UnpackVarint32(buf []byte) (int32, int) {
	i, ofs := vlq.Int(buf)
	return int32(i), ofs
}

func (p *BaseParser) UnpackVarint(buf []byte) (int, int) {
	i, ofs := vlq.Int(buf)
	return int(i), ofs
}

func (p *BaseParser) UnpackVaruint(buf []byte) (uint, int) {
	i, ofs := vlq.Uint(buf)
	return uint(i), ofs
}

func (p *BaseParser) UnpackVarBytes(buf []byte) ([]byte, int) {
	txvalue, l := p.UnpackVaruint(buf)
	bufValue := append([]byte(nil), buf[l:l+int(txvalue)]...)
	return bufValue, (l+int(txvalue))
}

func (p *BaseParser) PackVarBytes(bufValue []byte, buf []byte, varBuf []byte) []byte {
	l := p.PackVaruint(uint(len(bufValue)), varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, bufValue...)
	return buf
}

const (
	// number of bits in a big.Word
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	// number of bytes in a big.Word
	wordBytes = wordBits / 8
	// max packed bigint words
	maxPackedBigintWords = (256 - wordBytes) / wordBytes
	maxPackedBigintBytes = 249
)

func (p *BaseParser) MaxPackedBigintBytes() int {
	return maxPackedBigintBytes
}

// big int is packed in BigEndian order without memory allocation as 1 byte length followed by bytes of big int
// number of written bytes is returned
// limitation: bigints longer than 248 bytes are truncated to 248 bytes
// caution: buffer must be big enough to hold the packed big int, buffer 249 bytes big is always safe
func (p *BaseParser) PackBigint(bi *big.Int, buf []byte) int {
	w := bi.Bits()
	lw := len(w)
	// zero returns only one byte - zero length
	if lw == 0 {
		buf[0] = 0
		return 1
	}
	// pack the most significant word in a special way - skip leading zeros
	w0 := w[lw-1]
	fb := 8
	mask := big.Word(0xff) << (wordBits - 8)
	for w0&mask == 0 {
		fb--
		mask >>= 8
	}
	for i := fb; i > 0; i-- {
		buf[i] = byte(w0)
		w0 >>= 8
	}
	// if the big int is too big (> 2^1984), the number of bytes would not fit to 1 byte
	// in this case, truncate the number, it is not expected to work with this big numbers as amounts
	s := 0
	if lw > maxPackedBigintWords {
		s = lw - maxPackedBigintWords
	}
	// pack the rest of the words in reverse order
	for j := lw - 2; j >= s; j-- {
		d := w[j]
		for i := fb + wordBytes; i > fb; i-- {
			buf[i] = byte(d)
			d >>= 8
		}
		fb += wordBytes
	}
	buf[0] = byte(fb)
	return fb + 1
}

func (p *BaseParser) UnpackBigint(buf []byte) (big.Int, int) {
	var r big.Int
	l := int(buf[0]) + 1
	r.SetBytes(buf[1:l])
	return r, l
}

func (p *BaseParser) PackTxIndexes(txi []TxIndexes) []byte {
	buf := make([]byte, 0, 32)
	bvout := make([]byte, vlq.MaxLen32)
	// store the txs in reverse order for ordering from newest to oldest
	for j := len(txi) - 1; j >= 0; j-- {
		t := &txi[j]
		buf = append(buf, []byte(t.BtxID)...)
		for i, index := range t.Indexes {
			index <<= 1
			if i == len(t.Indexes)-1 {
				index |= 1
			}
			l := p.PackVarint32(index, bvout)
			buf = append(buf, bvout[:l]...)
		}
	}
	return buf
}

func (p *BaseParser) UnpackTxIndexes(txindexes *[]int32, buf *[]byte) error {
	for {
		index, l := p.UnpackVarint32(*buf)
		*txindexes = append(*txindexes, index>>1)
		*buf = (*buf)[l:]
		if index&1 == 1 {
			return nil
		} else if len(*buf) == 0 {
			return errors.New("rocksdb: index buffer length is zero")
		}
	}
	return nil
}

func (p *BaseParser) PackTxAddresses(ta *TxAddresses, buf []byte, varBuf []byte) []byte {
	return nil
}

func (p *BaseParser) AppendTxInput(txi *TxInput, buf []byte, varBuf []byte) []byte {
	return nil
}

func (p *BaseParser) AppendTxOutput(txo *TxOutput, buf []byte, varBuf []byte) []byte {
	return nil
}

func (p *BaseParser) UnpackTxAddresses(buf []byte) (*TxAddresses, error) {
	return nil, errors.New("Not supported")
}

func (p *BaseParser) UnpackTxInput(ti *TxInput, buf []byte) int {
	return 0
}

func (p *BaseParser) UnpackTxOutput(to *TxOutput, buf []byte) int {
	return 0
}

func (p *BaseParser) PackOutpoints(outpoints []DbOutpoint) []byte {
	return nil
}

func (p *BaseParser) UnpackNOutpoints(buf []byte) ([]DbOutpoint, int, error) {
	return nil, 0, errors.New("Not supported")
}

func (p *BaseParser) PackBlockInfo(block *DbBlockInfo) ([]byte, error) {
	return nil, errors.New("Not supported")
}

func (p *BaseParser) UnpackBlockInfo(buf []byte) (*DbBlockInfo, error) {
	return nil, errors.New("Not supported")
}

func (p *BaseParser) UnpackAsset(buf []byte) (*Asset, error) {
	return nil, nil
}

func (p *BaseParser) PackAsset(asset *Asset) ([]byte, error) {
	return nil, nil
}
func (p *BaseParser) UnpackTxIndexType(buf []byte) (AssetsMask, int) {
	return AllMask, 0
}

