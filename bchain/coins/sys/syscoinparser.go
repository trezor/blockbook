package syscoin

import (
	"encoding/json"
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"
	"strconv"
	"math/big"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/txscript"
	vlq "github.com/bsm/go-vlq"
	"github.com/juju/errors"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xffcae2ce
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
	SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_SYSCOIN int32 = 128
	SYSCOIN_TX_VERSION_SYSCOIN_BURN_TO_ALLOCATION int32 = 129
	SYSCOIN_TX_VERSION_ASSET_ACTIVATE int32 = 130
	SYSCOIN_TX_VERSION_ASSET_UPDATE int32 = 131
	SYSCOIN_TX_VERSION_ASSET_SEND int32 = 132
	SYSCOIN_TX_VERSION_ALLOCATION_MINT int32 = 133
	SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_ETHEREUM int32 = 134
	SYSCOIN_TX_VERSION_ALLOCATION_SEND int32 = 135
	maxAddrDescLen = 1024
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Mainnet address encoding magics
	MainNetParams.PubKeyHashAddrID = []byte{63} // base58 prefix: s
	MainNetParams.ScriptHashAddrID = []byte{5} // base68 prefix: 3
	MainNetParams.Bech32HRPSegwit = "sys"

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Regtest address encoding magics
	RegtestParams.PubKeyHashAddrID = []byte{65} // base58 prefix: t
	RegtestParams.ScriptHashAddrID = []byte{196} // base58 prefix: 2
	RegtestParams.Bech32HRPSegwit = "tsys"
}

// SyscoinParser handle
type SyscoinParser struct {
	*btc.BitcoinParser
	BaseParser *bchain.BaseParser
}

// NewSyscoinParser returns new SyscoinParser instance
func NewSyscoinParser(params *chaincfg.Params, c *btc.Configuration) *SyscoinParser {
	return &SyscoinParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		BaseParser:    &bchain.BaseParser{},
	}
}

// matches max data carrier for systx
func (p *SyscoinParser) GetMaxAddrLength() int {
	return 8000
}

// GetChainParams returns network parameters
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}

	switch chain {
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *SyscoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	tx, height, err := p.BitcoinParser.UnpackTx(buf)
	if err != nil {
		return nil, 0, err
	}
	p.LoadAssets(tx)
	return tx, height, nil
}
// TxFromMsgTx converts syscoin wire Tx to bchain.Tx
func (p *SyscoinParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	tx := p.BitcoinParser.TxFromMsgTx(t, parseAddresses)
	p.LoadAssets(&tx)
	return tx
}
// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *SyscoinParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
	tx, err := p.BaseParser.ParseTxFromJson(msg)
	if err != nil {
		return nil, err
	}
	p.LoadAssets(tx)
	return tx, nil
}
// ParseBlock parses raw block to our Block struct
// it has special handling for Auxpow blocks that cannot be parsed by standard btc wire parse
func (p *SyscoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
	}

	if (h.Version & utils.VersionAuxpow) != 0 {
		if err = utils.SkipAuxpow(r); err != nil {
			return nil, err
		}
	}

	err = utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w)
	if err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: h.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}
func (p *SyscoinParser) GetAssetTypeFromVersion(nVersion int32) bchain.TokenType {
	switch nVersion {
	case SYSCOIN_TX_VERSION_ASSET_ACTIVATE:
		return bchain.SPTAssetActivateType
	case SYSCOIN_TX_VERSION_ASSET_UPDATE:
		return bchain.SPTAssetUpdateType
	case SYSCOIN_TX_VERSION_ASSET_SEND:
		return bchain.SPTAssetSendType
	case SYSCOIN_TX_VERSION_ALLOCATION_MINT:
		return bchain.SPTAssetAllocationMintType
	case SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_ETHEREUM:
		return bchain.SPTAssetAllocationBurnToEthereumType
	case SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_SYSCOIN:
		return bchain.SPTAssetAllocationBurnToSyscoinType
	case SYSCOIN_TX_VERSION_SYSCOIN_BURN_TO_ALLOCATION:
		return bchain.SPTAssetSyscoinBurnToAllocationType
	case SYSCOIN_TX_VERSION_ALLOCATION_SEND:
		return bchain.SPTAssetAllocationSendType
	default:
		return bchain.SPTUnknownType
	}
}

func (p *SyscoinParser) GetAssetsMaskFromVersion(nVersion int32) bchain.AssetsMask {
	switch nVersion {
	case SYSCOIN_TX_VERSION_ASSET_ACTIVATE:
		return bchain.AssetActivateMask
	case SYSCOIN_TX_VERSION_ASSET_UPDATE:
		return bchain.AssetUpdateMask
	case SYSCOIN_TX_VERSION_ASSET_SEND:
		return bchain.AssetSendMask
	case SYSCOIN_TX_VERSION_ALLOCATION_MINT:
		return bchain.AssetAllocationMintMask
	case SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_ETHEREUM:
		return bchain.AssetAllocationBurnToEthereumMask
	case SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_SYSCOIN:
		return bchain.AssetAllocationBurnToSyscoinMask
	case SYSCOIN_TX_VERSION_SYSCOIN_BURN_TO_ALLOCATION:
		return bchain.AssetSyscoinBurnToAllocationMask
	case SYSCOIN_TX_VERSION_ALLOCATION_SEND:
		return bchain.AssetAllocationSendMask
	default:
		return bchain.BaseCoinMask
	}
}

func (p *SyscoinParser) IsSyscoinMintTx(nVersion int32) bool {
    return nVersion == SYSCOIN_TX_VERSION_ALLOCATION_MINT
}

func (p *SyscoinParser) IsAssetTx(nVersion int32) bool {
    return nVersion == SYSCOIN_TX_VERSION_ASSET_ACTIVATE || nVersion == SYSCOIN_TX_VERSION_ASSET_UPDATE
}

// note assetsend in core is assettx but its deserialized as allocation, we just care about balances so we can do it in same code for allocations
func (p *SyscoinParser) IsAssetAllocationTx(nVersion int32) bool {
	return nVersion == SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_ETHEREUM || nVersion == SYSCOIN_TX_VERSION_ALLOCATION_BURN_TO_SYSCOIN || nVersion == SYSCOIN_TX_VERSION_SYSCOIN_BURN_TO_ALLOCATION ||
	nVersion == SYSCOIN_TX_VERSION_ALLOCATION_SEND
}

func (p *SyscoinParser) IsAssetSendTx(nVersion int32) bool {
    return nVersion == SYSCOIN_TX_VERSION_ASSET_SEND
}

func (p *SyscoinParser) IsAssetActivateTx(nVersion int32) bool {
    return nVersion == SYSCOIN_TX_VERSION_ASSET_ACTIVATE
}

func (p *SyscoinParser) IsSyscoinTx(nVersion int32) bool {
    return p.IsAssetTx(nVersion) || p.IsAssetAllocationTx(nVersion) || p.IsSyscoinMintTx(nVersion)
}

func (p *SyscoinParser) IsTxIndexAsset(txIndex int32) bool {
    return txIndex > (SYSCOIN_TX_VERSION_ALLOCATION_SEND*10)
}

	
// TryGetOPReturn tries to process OP_RETURN script and return data
func (p *SyscoinParser) TryGetOPReturn(script []byte) []byte {
	if len(script) > 1 && script[0] == txscript.OP_RETURN {
		// trying 3 variants of OP_RETURN data
		// 1) OP_RETURN <datalen> <data>
		// 2) OP_RETURN OP_PUSHDATA1 <datalen in 1 byte> <data>
		// 3) OP_RETURN OP_PUSHDATA2 <datalen in 2 bytes> <data>
		
		var data []byte
		if len(script) < txscript.OP_PUSHDATA1 {
			data = script[2:]
		} else if script[1] == txscript.OP_PUSHDATA1 && len(script) <= 0xff {
			data = script[3:]
		} else if script[1] == txscript.OP_PUSHDATA2 && len(script) <= 0xffff {
			data = script[4:]
		}
		return data
	}
	return nil
}

// Amount compression:
// * If the amount is 0, output 0
// * first, divide the amount (in base units) by the largest power of 10 possible; call the exponent e (e is max 9)
// * if e<9, the last digit of the resulting number cannot be 0; store it as d, and drop it (divide by 10)
//   * call the result n
//   * output 1 + 10*(9*n + d - 1) + e
// * if e==9, we only know the resulting number is not zero, so output 1 + 10*(n - 1) + 9
// (this is decodable, as d is in [1-9] and e is in [0-9])

func CompressAmount(n uint64) uint64 {
    if n == 0 {
		return 0
	}
    var e uint64 = 0;
    for ((n % 10) == 0) && e < 9 {
        n /= 10
        e++
    }
    if e < 9 {
        var d uint64 = (n % 10)
        n /= 10
        return 1 + (n*9 + d - 1)*10 + e
    } else {
        return 1 + (n - 1)*10 + 9
    }
}

func DecompressAmount(x uint64) uint64 {
    // x = 0  OR  x = 1+10*(9*n + d - 1) + e  OR  x = 1+10*(n - 1) + 9
    if x == 0 {
		return 0
	}
    x--
    // x = 10*(9*n + d - 1) + e
    var e uint64 = x % 10
    x /= 10
    var n uint64 = 0
    if e < 9 {
        // x = 9*n + d - 1
        var d uint64 = (x % 9) + 1
        x /= 9
        // x = n
        n = x*10 + d
    } else {
        n = x+1
    }
    for e > 0 {
        n *= 10
        e--
    }
    return n
}

func (p *SyscoinParser) PackAllocation(a *bchain.AssetAllocationType, buf []byte) []byte {
	varBuf := make([]byte, vlq.MaxLen64)
	l := p.BaseParser.PackVaruint(uint(len(a.VoutAssets)), varBuf)

	for k, v := range a.VoutAssets {
		varBufLE := p.BaseParser.PackUintLE(k)
		buf = append(buf, varBufLE...)

		l = p.BaseParser.PackVaruint(uint(len(v)), varBuf)
		buf = append(buf, varBuf[:l]...)

		for _,voutAsset := range v {
			buf = p.PackAssetOut(&voutAsset, buf, varBuf)
		}
	}
	return buf
}

func (p *SyscoinParser) UnpackAllocation(a *bchain.AssetAllocationType, buf []byte) int {
	numAssets, l := p.BaseParser.UnpackVaruint(buf)
	a.VoutAssets = make(map[uint32][]bchain.AssetOutType, numAssets)
	for i := 0; i < int(numAssets); i++ {
		assetGuid := p.BaseParser.UnpackUintLE(buf[l:])
		l += 4
		numOutputs, ll := p.BaseParser.UnpackVaruint(buf[l:])
		l += ll
		assetOutArray, ok := a.VoutAssets[assetGuid]
		if !ok {
			assetOutArray = make([]bchain.AssetOutType, numOutputs)
			a.VoutAssets[assetGuid] = assetOutArray
		}
		for j := 0; j < int(numOutputs); j++ {
			ll = p.UnpackAssetOut(&assetOutArray[j], buf[l:])
			l += ll
		}
	}
	return l
}

func (p *SyscoinParser) UnpackAssetObj(a *bchain.AssetType, buf []byte) int {
	l := p.UnpackAllocation(&a.Allocation, buf)
	var ll int
	a.Precision = uint8(buf[l:l+1][0])
	l += 1

	a.Contract, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll
	
	a.PubData, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	symbol, ll := p.BaseParser.UnpackVarBytes(buf[l:])
	a.Symbol = string(symbol)
	l += ll

	a.UpdateFlags = uint8(buf[l:l+1][0])
	l += 1

	a.PrevContract, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.PrevPubData, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.PrevUpdateFlags = uint8(buf[l:l+1][0])
	l += 1

	balance, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	a.Balance = int64(DecompressAmount(uint64(balance)))

	totalSupply, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	a.TotalSupply = int64(DecompressAmount(uint64(totalSupply)))

	maxSupply, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	a.MaxSupply = int64(DecompressAmount(uint64(maxSupply)))

	return l
}

func (p *SyscoinParser) PackAssetObj(a *bchain.AssetType, buf []byte) []byte {
	varBuf := make([]byte, 20)
	buf = p.PackAllocation(&a.Allocation, buf)
	buf = append(buf, []byte{a.Precision}...)

	buf = p.BaseParser.PackVarBytes(a.Contract, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.PubData, buf, varBuf)

	buf = p.BaseParser.PackVarBytes([]byte(a.Symbol), buf, varBuf)

	buf = append(buf, []byte{a.UpdateFlags}...)

	buf = p.BaseParser.PackVarBytes(a.PrevContract, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.PrevPubData, buf, varBuf)

	buf = append(buf, []byte{a.PrevUpdateFlags}...)

	l := p.BaseParser.PackVaruint(uint(CompressAmount(uint64(a.Balance))), varBuf)
	buf = append(buf, varBuf[:l]...)

	l = p.BaseParser.PackVaruint(uint(CompressAmount(uint64(a.TotalSupply))), varBuf)
	buf = append(buf, varBuf[:l]...)

	l = p.BaseParser.PackVaruint(uint(CompressAmount(uint64(a.MaxSupply))), varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func (p *SyscoinParser) PackAssetOut(a *bchain.AssetOutType, buf []byte, varBuf []byte) []byte {
	l := p.BaseParser.PackVaruint(uint(a.N), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackVaruint(uint(CompressAmount(uint64(a.ValueSat))), varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func (p *SyscoinParser) UnpackAssetOut(a *bchain.AssetOutType, buf []byte) int {
	var l int
	var ll int
	n, l := p.BaseParser.UnpackVaruint(buf[l:])
	a.N = uint32(n)
	valueSat, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	a.ValueSat = int64(DecompressAmount(uint64(valueSat)))
	return l
}


func (p *SyscoinParser) UnpackMintSyscoin(a *bchain.MintSyscoinType, buf []byte) int {
	l := p.UnpackAllocation(&a.Allocation, buf)
	var ll int
	bridgeTransferId, ll := p.BaseParser.UnpackVaruint(buf[l:])
	a.BridgeTransferId = uint32(bridgeTransferId)
	l += ll

	blockNumber, ll := p.BaseParser.UnpackVaruint(buf[l:])
	a.BlockNumber = uint32(blockNumber)
	l += ll

	a.TxValue, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll
	
	a.TxParentNodes, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.TxRoot, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.TxPath, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.ReceiptValue, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.ReceiptParentNodes, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll

	a.ReceiptRoot, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll	

	a.ReceiptPath, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll	
	
	return l
}

func (p *SyscoinParser) AppendMintSyscoin(a *bchain.MintSyscoinType, buf []byte) []byte {
	varBuf := make([]byte, 4096)
	buf = p.PackAllocation(&a.Allocation, buf)

	l := p.BaseParser.PackVaruint(uint(a.BridgeTransferId), varBuf)
	buf = append(buf, varBuf[:l]...)

	l = p.BaseParser.PackVaruint(uint(a.BlockNumber), varBuf)
	buf = append(buf, varBuf[:l]...)

	buf = p.BaseParser.PackVarBytes(a.TxValue, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.TxParentNodes, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.TxRoot, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.TxPath, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.ReceiptValue, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.ReceiptParentNodes, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.ReceiptRoot, buf, varBuf)

	buf = p.BaseParser.PackVarBytes(a.ReceiptPath, buf, varBuf)
	return buf
}

func (p *SyscoinParser) UnpackSyscoinBurnToEthereum(a *bchain.SyscoinBurnToEthereumType, buf []byte) int {
	l := p.UnpackAllocation(&a.Allocation, buf)
	var ll int
	a.EthAddress, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll	
	return l
}

func (p *SyscoinParser) PackSyscoinBurnToEthereum(a *bchain.SyscoinBurnToEthereumType, buf []byte) []byte {
	buf = p.PackAllocation(&a.Allocation, buf)
	buf = append(buf, a.EthAddress...)
	return buf
}

func (p *SyscoinParser) GetAllocationFromTx(tx *bchain.Tx) (*bchain.AssetAllocationType, error) {
	var sptData []byte
	for _, output := range tx.Vout {
		addrDesc, err := p.GetAddrDescFromVout(&output)
		if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
			continue
		}
		if addrDesc[0] == txscript.OP_RETURN {
			script, err := p.GetScriptFromAddrDesc(addrDesc)
			if err != nil {
				return nil, err
			}
			sptData = p.TryGetOPReturn(script)
			if sptData == nil {
				return nil, errors.New("OP_RETURN empty")
			}
			break
		}
	}
	var assetAllocation bchain.AssetAllocationType
	l := p.UnpackAllocation(&assetAllocation, sptData)
	// should be atleast 8 bytes minimum
	if l < 8 {
		return nil, errors.New("Could not decode asset allocation")
	}
	return &assetAllocation, nil
}

func (p *SyscoinParser) GetAssetFromData(sptData []byte) (*bchain.AssetType, error) {
	var asset bchain.AssetType
	l := p.UnpackAssetObj(&asset, sptData)
	if l != len(sptData) {
		return nil, errors.New("Could not decode asset l " + strconv.Itoa(l) + " vs len " + strconv.Itoa(len(sptData)))
	}
	return &asset, nil
}
func (p *SyscoinParser) LoadAssets(tx *bchain.Tx) error {
    if p.IsSyscoinTx(tx.Version) {
        allocation, err := p.GetAllocationFromTx(tx);
		if err != nil {
			return err
		}
        for k, v := range allocation.VoutAssets {
            for _,voutAsset := range v {
				// store in vout
				tx.Vout[voutAsset.N].AssetInfo = bchain.AssetInfo{AssetGuid: k, ValueSat: big.NewInt(voutAsset.ValueSat)}
            }
        }       
	}
	return nil
}

func (p *SyscoinParser) PackAssetKey(assetGuid uint32, height uint32) []byte {
	var buf []byte
	varBuf := p.BaseParser.PackUint(assetGuid)
	buf = append(buf, varBuf...)
	// pack height as binary complement to achieve ordering from newest to oldest block
	varBuf = p.BaseParser.PackUint(^height)
	buf = append(buf, varBuf...)
	return buf
}

func (p *SyscoinParser) UnpackAssetKey(buf []byte) (uint32, uint32) {
	assetGuid := p.BaseParser.UnpackUint(buf)
	height := p.BaseParser.UnpackUint(buf[4:])
	// height is packed in binary complement, convert it
	return assetGuid, ^height
}

func (p *SyscoinParser) PackAssetTxIndex(txAsset *bchain.TxAsset) []byte {
	var buf []byte
	varBuf := make([]byte, vlq.MaxLen64)
	l := p.BaseParser.PackVaruint(uint(len(txAsset.Txs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for _, txAssetIndex := range txAsset.Txs {
		varBuf = p.BaseParser.PackUint(uint32(txAssetIndex.Type))
		buf = append(buf, varBuf...)
		buf = append(buf, txAssetIndex.BtxID...)
	}
	return buf
}

func (p *SyscoinParser) UnpackAssetTxIndex(buf []byte) []*bchain.TxAssetIndex {
	var txAssetIndexes []*bchain.TxAssetIndex
	numTxIndexes, l := p.BaseParser.UnpackVaruint(buf)
	if numTxIndexes > 0 {
		txAssetIndexes = make([]*bchain.TxAssetIndex, numTxIndexes)
		for i := uint(0); i < numTxIndexes; i++ {
			var txIndex bchain.TxAssetIndex
			txIndex.Type = bchain.AssetsMask(p.BaseParser.UnpackUint(buf[l:]))
			l += 4
			txIndex.BtxID = append([]byte(nil), buf[l:l+32]...)
			l += 32
			txAssetIndexes[i] = &txIndex
		}
	}
	return txAssetIndexes
}

func (p *SyscoinParser) AppendAssetInfoDetails(assetInfoDetails *bchain.AssetInfoDetails, buf []byte, varBuf []byte) []byte {
	l := p.BaseParser.PackVarint32(assetInfoDetails.Decimals, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, []byte(assetInfoDetails.Symbol)...)
	return buf
}

func (p *SyscoinParser) UnpackAssetInfoDetails(assetInfoDetails *bchain.AssetInfoDetails, buf []byte) int {
	decimals, l := p.BaseParser.UnpackVarint32(buf)
	symbolBytes := append([]byte(nil), buf[l:]...)
	assetInfoDetails = &bchain.AssetInfoDetails{Symbol: string(symbolBytes), Decimals: decimals}
	return l + int(len(symbolBytes))
}

func (p *SyscoinParser) AppendAssetInfo(assetInfo *bchain.AssetInfo, buf []byte, varBuf []byte, details bool) []byte {
	l := p.BaseParser.PackVaruint(uint(assetInfo.AssetGuid), varBuf)
	buf = append(buf, varBuf[:l]...)
	if assetInfo.AssetGuid > 0 {
		l = p.BaseParser.PackBigint(assetInfo.ValueSat, varBuf)
		buf = append(buf, varBuf[:l]...)
		if details {
			buf = p.AppendAssetInfoDetails(&assetInfo.Details, buf, varBuf)	
		}
	}
	return buf
}

func (p *SyscoinParser) UnpackAssetInfo(assetInfo *bchain.AssetInfo, buf []byte, details bool) int {
	assetGuid, l := p.BaseParser.UnpackVaruint(buf)
	assetInfo.AssetGuid = uint32(assetGuid)
	if assetInfo.AssetGuid > 0 {
		valueSat, al := p.BaseParser.UnpackBigint(buf[l:])
		assetInfo.ValueSat = &valueSat
		l += al
		if details {
			al = p.UnpackAssetInfoDetails(&assetInfo.Details, buf[l:])
			l += al
		}
	}
	return l
}

func (p *SyscoinParser) PackTxAddresses(ta *bchain.TxAddresses, buf []byte, varBuf []byte) []byte {
	buf = buf[:0]
	l := p.BaseParser.PackVaruint(uint(ta.Height), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackVaruint(uint(len(ta.Inputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.Inputs {
		buf = p.AppendTxInput(&ta.Inputs[i], buf, varBuf)
	}
	l = p.BaseParser.PackVaruint(uint(len(ta.Outputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.Outputs {
		buf = p.AppendTxOutput(&ta.Outputs[i], buf, varBuf)
	}
	return buf
}

func (p *SyscoinParser) UnpackTxAddresses(buf []byte) (*bchain.TxAddresses, error) {
	ta := bchain.TxAddresses{}
	height, l := p.BaseParser.UnpackVaruint(buf)
	ta.Height = uint32(height)
	inputs, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	ta.Inputs = make([]bchain.TxInput, inputs)
	for i := uint(0); i < inputs; i++ {
		l += p.UnpackTxInput(&ta.Inputs[i], buf[l:])
	}
	outputs, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	ta.Outputs = make([]bchain.TxOutput, outputs)
	for i := uint(0); i < outputs; i++ {
		l += p.UnpackTxOutput(&ta.Outputs[i], buf[l:])
	}
	return &ta, nil
}

func (p *SyscoinParser) AppendTxInput(txi *bchain.TxInput, buf []byte, varBuf []byte) []byte {
	buf = p.BitcoinParser.AppendTxInput(txi, buf, varBuf)
	buf = p.AppendAssetInfo(&txi.AssetInfo, buf, varBuf, true)
	return buf
}

func (p *SyscoinParser) AppendTxOutput(txo *bchain.TxOutput, buf []byte, varBuf []byte) []byte {
	buf = p.BitcoinParser.AppendTxOutput(txo, buf, varBuf)
	buf = p.AppendAssetInfo(&txo.AssetInfo, buf, varBuf, true)
	return buf
}


func (p *SyscoinParser) UnpackTxInput(ti *bchain.TxInput, buf []byte) int {
	l := p.BitcoinParser.UnpackTxInput(ti, buf)
	al := p.UnpackAssetInfo(&ti.AssetInfo, buf[l:], true)
	return l + al
}

func (p *SyscoinParser) UnpackTxOutput(to *bchain.TxOutput, buf []byte) int {
	l := p.BitcoinParser.UnpackTxOutput(to, buf)
	al := p.UnpackAssetInfo(&to.AssetInfo, buf[l:], true)
	return l + al
}


func (p *SyscoinParser) UnpackAddrBalance(buf []byte, txidUnpackedLen int, detail bchain.AddressBalanceDetail) (*bchain.AddrBalance, error) {
	txs, l := p.BaseParser.UnpackVaruint(buf)
	sentSat, sl := p.BaseParser.UnpackBigint(buf[l:])
	balanceSat, bl := p.BaseParser.UnpackBigint(buf[l+sl:])
	l = l + sl + bl
	ab := &bchain.AddrBalance{
		Txs:        uint32(txs),
		SentSat:    sentSat,
		BalanceSat: balanceSat,
	}
	// unpack asset balance information
	numAssetBalances, ll := p.BaseParser.UnpackVaruint(buf[l:])
	l += ll
	if numAssetBalances > 0 {
		ab.AssetBalances = make(map[uint32]*bchain.AssetBalance, numAssetBalances)
		for i := uint(0); i < numAssetBalances; i++ {
			asset, ll := p.BaseParser.UnpackVaruint(buf[l:])
			l += ll
			balancevalue, ll := p.BaseParser.UnpackBigint(buf[l:])
			l += ll
			sentvalue, ll := p.BaseParser.UnpackBigint(buf[l:])
			l += ll
			transfers, ll := p.BaseParser.UnpackVaruint(buf[l:])
			l += ll
			ab.AssetBalances[uint32(asset)] = &bchain.AssetBalance{Transfers: uint32(transfers), SentSat: &sentvalue, BalanceSat: &balancevalue}
		}
	}
	if detail != bchain.AddressBalanceDetailNoUTXO {
		// estimate the size of utxos to avoid reallocation
		ab.Utxos = make([]bchain.Utxo, 0, len(buf[l:])/txidUnpackedLen+4)
		// ab.UtxosMap = make(map[string]int, cap(ab.Utxos))
		for len(buf[l:]) >= txidUnpackedLen+4 {
			btxID := append([]byte(nil), buf[l:l+txidUnpackedLen]...)
			l += txidUnpackedLen
			vout, ll := p.BaseParser.UnpackVaruint(buf[l:])
			l += ll
			height, ll := p.BaseParser.UnpackVaruint(buf[l:])
			l += ll
			valueSat, ll := p.BaseParser.UnpackBigint(buf[l:])
			l += ll
			u := bchain.Utxo{
				BtxID:    btxID,
				Vout:     int32(vout),
				Height:   uint32(height),
				ValueSat: valueSat,
			}
			ll = p.UnpackAssetInfo(&u.AssetInfo, buf[l:], false)
			l += ll
			if detail == bchain.AddressBalanceDetailUTXO {
				ab.Utxos = append(ab.Utxos, u)
			} else {
				ab.AddUtxo(&u)
			}
		}
	}
	return ab, nil
}

func (p *SyscoinParser) PackAddrBalance(ab *bchain.AddrBalance, buf, varBuf []byte) []byte {
	buf = buf[:0]
	l := p.BaseParser.PackVaruint(uint(ab.Txs), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackBigint(&ab.SentSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackBigint(&ab.BalanceSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	
	// pack asset balance information
	l = p.BaseParser.PackVaruint(uint(len(ab.AssetBalances)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for key, value := range ab.AssetBalances {
		l = p.BaseParser.PackVaruint(uint(key), varBuf)
		buf = append(buf, varBuf[:l]...)
		l = p.BaseParser.PackBigint(value.BalanceSat, varBuf)
		buf = append(buf, varBuf[:l]...)
		l = p.BaseParser.PackBigint(value.SentSat, varBuf)
		buf = append(buf, varBuf[:l]...)
		l = p.BaseParser.PackVaruint(uint(value.Transfers), varBuf)
		buf = append(buf, varBuf[:l]...)
	}
	for _, utxo := range ab.Utxos {
		// if Vout < 0, utxo is marked as spent
		if utxo.Vout >= 0 {
			buf = append(buf, utxo.BtxID...)
			l = p.BaseParser.PackVaruint(uint(utxo.Vout), varBuf)
			buf = append(buf, varBuf[:l]...)
			l = p.BaseParser.PackVaruint(uint(utxo.Height), varBuf)
			buf = append(buf, varBuf[:l]...)
			l = p.BaseParser.PackBigint(&utxo.ValueSat, varBuf)
			buf = append(buf, varBuf[:l]...)
			buf = p.AppendAssetInfo(&utxo.AssetInfo, buf, varBuf, false)
		}
	}
	return buf
}


func (p *SyscoinParser) PackAsset(asset *bchain.Asset) []byte {
	buf := make([]byte, 0, 32)
	varBuf := make([]byte, vlq.MaxLen64)
	l := p.BaseParser.PackVaruint(uint(asset.Transactions), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackVaruint(uint(len(asset.AddrDesc)), varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, []byte(asset.AddrDesc)...)
	buf = p.PackAssetObj(&asset.AssetObj, buf)
	return buf
}

func (p *SyscoinParser) UnpackAsset(buf []byte) *bchain.Asset {
	var asset bchain.Asset
	var ll int
	transactions, l := p.BaseParser.UnpackVaruint(buf)
	asset.Transactions = uint32(transactions)
	asset.AddrDesc, ll = p.BaseParser.UnpackVarBytes(buf[l:])
	l += ll
	varBuf := buf[l:]
	l = p.UnpackAssetObj(&asset.AssetObj, varBuf)
	if l != len(varBuf) {
		return nil
	}
	return &asset
}