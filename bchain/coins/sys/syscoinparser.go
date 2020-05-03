package syscoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"
	"math/big"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/txscript"
	"github.com/martinboehm/btcutil"
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
// TxFromMsgTx converts syscoin wire Tx to bchain.Tx
func (p *SyscoinParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	vin := make([]bchain.Vin, len(t.TxIn))
	for i, in := range t.TxIn {
		if blockchain.IsCoinBaseTx(t) {
			vin[i] = bchain.Vin{
				Coinbase: hex.EncodeToString(in.SignatureScript),
				Sequence: in.Sequence,
			}
			break
		}
		s := bchain.ScriptSig{
			Hex: hex.EncodeToString(in.SignatureScript),
			// missing: Asm,
		}
		vin[i] = bchain.Vin{
			Txid:      in.PreviousOutPoint.Hash.String(),
			Vout:      in.PreviousOutPoint.Index,
			Sequence:  in.Sequence,
			ScriptSig: s,
		}
	}
	vout := make([]bchain.Vout, len(t.TxOut))
	for i, out := range t.TxOut {
		addrs := []string{}
		if parseAddresses {
			addrs, _, _ = p.OutputScriptToAddressesFunc(out.PkScript)
		}
		s := bchain.ScriptPubKey{
			Hex:       hex.EncodeToString(out.PkScript),
			Addresses: addrs,
			// missing: Asm,
			// missing: Type,
		}
		var vs big.Int
		vs.SetInt64(out.Value)
		vout[i] = bchain.Vout{
			ValueSat:     vs,
			N:            uint32(i),
			ScriptPubKey: s,
		}
	}
	tx := bchain.Tx{
		Txid:     t.TxHash().String(),
		Version:  t.Version,
		LockTime: t.LockTime,
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	p.LoadAssets(&tx)
	return tx
}
// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *SyscoinParser) ParseTxFromJson(msg json.RawMessage) (*Tx, error) {
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
	p.LoadAssets(&tx)
	return &tx, nil
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
		return bchain.SyscoinMask
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
func (p *SyscoinParser) GetAllocationFromTx(tx *bchain.Tx) (wire.AssetAllocationType, error) {
	var sptData []byte
	for i, output := range tx.Vout {
		addrDesc, err := p.GetAddrDescFromVout(&output)
		if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
			continue
		}
		if(addrDesc[0] == txscript.OP_RETURN) {
			script, err := p.GetScriptFromAddrDesc(addrDesc)
			if err != nil {
				return err
			}
			sptData = p.TryGetOPReturn(script)
			if sptData == nil {
				return nil
			}
			break
		}
	}
	r := bytes.NewReader(sptData)
	var assetAllocation wire.AssetAllocationType
	err := assetAllocation.Deserialize(r)
	if err != nil {
		return nil, err
	}
	return assetAllocation, nil
}
func (p *SyscoinParser) GetAssetFromTx(tx *bchain.Tx) (wire.AssetType, error) {
	var sptData []byte
	for i, output := range tx.Vout {
		addrDesc, err := p.GetAddrDescFromVout(&output)
		if err != nil || len(addrDesc) == 0 || len(addrDesc) > maxAddrDescLen {
			continue
		}
		if(addrDesc[0] == txscript.OP_RETURN) {
			script, err := p.GetScriptFromAddrDesc(addrDesc)
			if err != nil {
				return err
			}
			sptData = p.TryGetOPReturn(script)
			if sptData == nil {
				return nil
			}
			break
		}
	}
	r := bytes.NewReader(sptData)
	var asset wire.AssetType
	err := asset.Deserialize(r)
	if err != nil {
		return nil, err
	}
	return asset, nil
}
func (p *SyscoinParser) LoadAssets(tx *bchain.Tx) error {
    if p.IsSyscoinTx(tx.Version) {
        allocation, err := p.GetAllocationFromTx(tx);
		if err != nil {
			return err
		}
        for k, v := range allocation.voutAssets {
            nAsset := k
            for ,voutAsset := range v {
				// store in vout
				tx.vout[voutAsset.N].AssetInfo = bchain.AssetInfo{AssetGuid: nAsset, ValueSat: *big.NewInt(voutAsset.nValue)}
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
		l = p.BaseParser.PackVaruint(uint(len(txAssetIndex.Txid)), varBuf)
		buf = append(buf, varBuf[:l]...)
		buf = append(buf, txAssetIndex.Txid...)
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
			ll, al := p.BaseParser.UnpackVaruint(buf[l:])
			l += al
			txIndex.Txid = append([]byte(nil), buf[l:l+int(ll)]...)
			l += int(ll)
			txAssetIndexes[i] = &txIndex
		}
	}
	return txAssetIndexes
}

func (p *SyscoinParser) AppendAssetInfoDetails(assetInfoDetails *bchain.AssetInfoDetails, buf []byte, varBuf []byte) []byte {
	l = d.chainParser.PackVarint32(assetInfoDetails.Decimals, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, []byte(assetInfoDetails.Symbol)...)
	return buf
}

func (p *SyscoinParser) UnpackAssetInfoDetails(assetInfoDetails *bchain.AssetInfoDetails, buf []byte) int {
	decimals, l := p.BaseParser.UnpackVarint32(buf)
	symbolBytes, al := append([]byte(nil), buf[l:]...)
	assetInfoDetails = &bchain.AssetInfoDetails{Symbol: string(symbolBytes), Decimals: decimals}
	return l + al
}

func (p *SyscoinParser) AppendAssetInfo(assetInfo *bchain.AssetInfo, buf []byte, varBuf []byte, details bool) []byte {
	varBuf = p.BaseParser.PackUint(assetInfo.AssetGuid)
	buf = append(buf, varBuf...)	
	if(assetInfo.AssetGuid > 0) {
		l = p.BaseParser.PackBigint(assetInfo.ValueSat, varBuf)
		buf = append(buf, varBuf[:l]...)
		if details {
			buf = p.AppendAssetInfoDetails(txi.AssetInfo.Details, buf, varBuf)	
		}
	}
	return buf
}

func (p *SyscoinParser) UnpackAssetInfo(assetInfo *bchain.AssetInfo, buf []byte, details bool) int {
	assetInfo.AssetGuid = p.BaseParser.UnpackUint(buf)	
	l := 4
	if(assetInfo.AssetGuid > 0) {
		valueSat, al := p.BaseParser.UnpackBigint(buf[l:])
		assetInfo.ValueSat = &valueSat
		l += al
		if details {
			al = p.UnpackAssetInfoDetails(assetInfo.Details, buf)
			l += al
		}
	}
	return l
}

func (p *SyscoinParser) AppendTxInput(txi *bchain.TxInput, buf []byte, varBuf []byte) []byte {
	buf := p.BitcoinParser.AppendTxInput(txi, buf, varBuf)
	buf = p.AppendAssetInfo(&txi.AssetInfo, buf, varBuf, true)
	return buf
}

func (p *SyscoinParser) AppendTxOutput(txo *bchain.TxOutput, buf []byte, varBuf []byte) []byte {
	buf := p.BitcoinParser.AppendTxInput(txo, buf, varBuf)
	buf = p.AppendAssetInfo(&txi.AssetInfo, buf, varBuf, true)
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
		ab.Utxos = make([]bchain.Utxo, 0, len(buf[l:])/txidUnpackedLen+3)
		// ab.UtxosMap = make(map[string]int, cap(ab.Utxos))
		for len(buf[l:]) >= txidUnpackedLen+3 {
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
			ll := p.UnpackAssetInfo(&u.AssetInfo, buf[l:], false)
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



func (p *BaseParser) PackTxIndexes(txi []TxIndexes) []byte {
	buf := make([]byte, 0, 36)
	bvout := make([]byte, vlq.MaxLen32)
	// store the txs in reverse order for ordering from newest to oldest
	for j := len(txi) - 1; j >= 0; j-- {
		t := &txi[j]
		varBuf := p.BaseParser.PackUint(uint32(t.Type))
		buf = append(buf, varBuf...)
		buf = append(buf, []byte(t.BtxID)...)
		for i, index := range t.Indexes {
			index <<= 1
			if i == len(t.Indexes)-1 {
				index |= 1
			}
			l := p.BaseParser.PackVarint32(index, bvout)
			buf = append(buf, bvout[:l]...)
		}
	}
	return buf
}

func (p *SyscoinParser) PackAsset(asset *bchain.Asset) ([]byte, error) {
	buf := make([]byte, 0, 32)
	varBuf := make([]byte, vlq.MaxLen64)
	l := p.BaseParser.PackVaruint(uint(asset.Transactions), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = p.BaseParser.PackVaruint(uint(len(asset.AddrDesc)), varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, []byte(asset.AddrDesc)...)
	var buffer bytes.Buffer
	err := asset.AssetObj.Serialize(&buffer)
	if err != nil {
		return nil, err
	}
	buf = append(buf, buffer.Bytes()...)
	return buf, nil
}

func (p *SyscoinParser) UnpackAsset(buf []byte) (*bchain.Asset, error) {
	var asset bchain.Asset
	transactions, l := p.BaseParser.UnpackVaruint(buf)
	asset.Transactions = uint32(transactions)
	addrDescBytes, l := p.BaseParser.UnpackVaruint(buf)
	asset.AddrDesc = append([]byte(nil), buf[l:l+addrDescBytes]...)
	l += addrDescBytes
	r := bytes.NewReader(buf[l:])
	err := asset.AssetObj.Deserialize(r)
	if err != nil {
		return nil, err
	}
	return &asset, nil
}