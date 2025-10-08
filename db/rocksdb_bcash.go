package db

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"sort"

	"github.com/decred/dcrd/txscript/v3"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

var BCMR_PREFIX = []byte{txscript.OP_RETURN, 0x04, 0x42, 0x43, 0x4D, 0x52} // "BCMR"

// BcashToken index
type BcashToken struct {
	Standard      bchain.TokenStandard
	Txs           uint
	GenesisSupply big.Int
	Commitments   [][]byte
}

func packBcashToken(token *BcashToken, buf []byte, varBuf []byte) []byte {
	buf = buf[:0]
	if token == nil {
		return buf
	}

	// Txs
	l := packVaruint(uint(token.Txs), varBuf)
	buf = append(buf, varBuf[:l]...)

	// GenesisSupply
	l = packBigint(&token.GenesisSupply, varBuf)
	buf = append(buf, varBuf[:l]...)

	// Commitments (as varuint count, then for each: varuint length + bytes)
	l = packVaruint(uint(len(token.Commitments)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for _, c := range token.Commitments {
		cBytes := packString(string(c))
		buf = append(buf, cBytes...)
	}

	return buf
}

func unpackBcashToken(buf []byte) (*BcashToken, int, error) {
	if len(buf) == 0 {
		return nil, 0, nil
	}

	token := BcashToken{
		Standard: bchain.CashToken,
	}
	var al int

	// Txs
	txs, l := unpackVaruint(buf[al:])
	al += l
	token.Txs = uint(txs)

	// GenesisSupply
	genesisSupply, l := unpackBigint(buf[al:])
	al += l
	token.GenesisSupply = genesisSupply

	// Commitments
	commitmentsCount, l := unpackVaruint(buf[al:])
	al += l
	token.Commitments = make([][]byte, commitmentsCount)
	for i := range commitmentsCount {
		commitment, ll := unpackString(buf[al:])
		al += ll
		token.Commitments[i] = []byte(commitment)
	}

	return &token, al, nil
}

// GetBcashToken returns the BcashToken for a given category or nil if not found.
func (d *RocksDB) GetBcashToken(category []byte) (*BcashToken, error) {
	if len(category) != 32 {
		return nil, errors.New("Invalid category")
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfBcashTokens], category)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	token, _, err := unpackBcashToken(data)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// storeBcashTokens stores BcashToken data in the database.
func (d *RocksDB) storeBcashTokens(wb *grocksdb.WriteBatch, tokens map[string]*BcashToken) error {
	varBuf := make([]byte, 1024)
	buf := make([]byte, 1024)
	for category, token := range tokens {
		key := []byte(category)
		if len(key) != 32 {
			glog.Warningf("rocksdb: bcash token invalid category %s", hex.EncodeToString(key))
			continue
		}
		if token == nil {
			wb.DeleteCF(d.cfh[cfBcashTokens], key)
		} else {
			buf = packBcashToken(token, buf, varBuf)
			wb.PutCF(d.cfh[cfBcashTokens], key, buf)
		}
	}
	return nil
}

// BcashTokenMetaQueue index
type BcashTokenMetaQueue struct {
	TxId    []byte
	Vout    uint16
	Height  uint32
	Txi     uint32
	Retries uint8
}

func PackBcashTokenMetaQueueKey(meta *BcashTokenMetaQueue, buf []byte) []byte {
	buf = buf[:0]
	if meta == nil {
		return buf
	}

	// TxId
	buf = append(buf, meta.TxId[:]...)

	// Vout
	voutBytes := packUint16(meta.Vout)
	buf = append(buf, voutBytes...)

	return buf
}

func UnpackBcashTokenMetaQueueKey(buf []byte) (*BcashTokenMetaQueue, int, error) {
	if len(buf) != 34 {
		return nil, 0, errors.New("Invalid buffer length")
	}

	meta := BcashTokenMetaQueue{}
	var al int

	// TxId
	meta.TxId = bytes.Clone(buf[al : al+32])
	al += 32

	// Vout
	meta.Vout = unpackUint16(buf[al:])
	al += 2

	return &meta, al, nil
}

func PackBcashTokenMetaQueue(meta *BcashTokenMetaQueue, buf []byte) []byte {
	buf = buf[:0]
	if meta == nil {
		return buf
	}

	// Height
	buf = append(buf, packUint(meta.Height)...)

	// Txi
	buf = append(buf, packUint(meta.Txi)...)

	// Retries
	buf = append(buf, meta.Retries)

	return buf
}

func UnpackBcashTokenMetaQueue(buf []byte, meta *BcashTokenMetaQueue) (int, error) {
	if len(buf) != 9 {
		return 0, errors.New("Invalid buffer length")
	}

	var al int

	// Height
	meta.Height = unpackUint(buf[al:])
	al += 4

	// Txi
	meta.Txi = unpackUint(buf[al:])
	al += 4

	// Retries
	meta.Retries = buf[al]
	al++

	return al, nil
}

func (d *RocksDB) GetBcashTokenMetaQueueItem(item *BcashTokenMetaQueue) (*BcashTokenMetaQueue, error) {
	key := PackBcashTokenMetaQueueKey(item, make([]byte, 34))
	val, err := d.db.GetCF(d.ro, d.cfh[cfBcashTokenMetaQueue], key)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	if len(data) == 0 {
		return nil, nil
	}
	meta, _, err := UnpackBcashTokenMetaQueueKey(key)
	if err != nil {
		return nil, err
	}
	_, err = UnpackBcashTokenMetaQueue(data, meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (d *RocksDB) GetAllBcashTokenMetaQueue() ([]*BcashTokenMetaQueue, error) {
	metas := make([]*BcashTokenMetaQueue, 0)

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfBcashTokenMetaQueue])
	defer it.Close()
	for it.SeekToFirst(); it.Valid(); it.Next() {
		key := it.Key().Data()
		data := it.Value().Data()
		meta, _, err := UnpackBcashTokenMetaQueueKey(key)
		if err != nil {
			return nil, err
		}

		_, err = UnpackBcashTokenMetaQueue(data, meta)
		if err != nil {
			return nil, err
		}

		metas = append(metas, meta)
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	return metas, nil
}

func (d *RocksDB) StoreBcashTokenMetaQueue(wb *grocksdb.WriteBatch, metas []*BcashTokenMetaQueue) error {
	keyBuf := make([]byte, 34)
	dataBuf := make([]byte, 9)
	for _, meta := range metas {
		if meta == nil {
			return errors.New("Invalid meta, nil")
		}
		key := PackBcashTokenMetaQueueKey(meta, keyBuf)
		data := PackBcashTokenMetaQueue(meta, dataBuf)
		wb.PutCF(d.cfh[cfBcashTokenMetaQueue], key, data)
	}
	return nil
}

func (d *RocksDB) RemoveBcashTokenMetaQueue(wb *grocksdb.WriteBatch, metas []*BcashTokenMetaQueue) {
	buf := make([]byte, 34)
	for _, meta := range metas {
		key := PackBcashTokenMetaQueueKey(meta, buf)
		wb.DeleteCF(d.cfh[cfBcashTokenMetaQueue], key)
	}
}

func (d *RocksDB) processBcashTokens(block *bchain.Block, addresses addressesMap, bcashTokens map[string]*BcashToken, blockTxIDs *[][]byte, blockTxAddresses []*TxAddresses) error {
	// TODO: extract CashTokens activation height to chain params
	if !d.is.IsBCH() || block.Height <= 792772 {
		return nil
	}

	// process bcash token data
	genesisSupplyMap := make(map[string]uint64)
	tokenTxsMap := make(map[string]map[string]bool)
	spentCommitments := make(map[string]map[string]bool)
	createdCommitments := make(map[string]map[string]bool)
	metaQueue := make([]*BcashTokenMetaQueue, 0)

	for txi := range block.Txs {
		tx := &block.Txs[txi]
		stxID := tx.Txid
		ta := blockTxAddresses[txi]
		for i := range ta.Outputs {
			tao := &ta.Outputs[i]
			var btxID []byte
			if tao.BcashToken != nil {
				btxID, err := d.chainParser.PackTxid(tx.Txid)
				if err != nil {
					return err
				}

				sCategory := string(tao.BcashToken.Category)
				addToAddressesMap(addresses, sCategory, btxID, int32(i))

				if tokenTxsMap[sCategory] == nil {
					tokenTxsMap[sCategory] = make(map[string]bool)
				}
				tokenTxsMap[sCategory][string(stxID)] = true

				if tao.BcashToken.Nft != nil {
					if createdCommitments[sCategory] == nil {
						createdCommitments[sCategory] = make(map[string]bool)
					}
					createdCommitments[sCategory][string(tao.BcashToken.Nft.Commitment)] = true
				}

				// Check if there are no inputs with this output token category
				hasInputWithCategory := false
				for _, input := range ta.Inputs {
					if input.BcashToken != nil && bytes.Equal(input.BcashToken.Category, tao.BcashToken.Category) {
						hasInputWithCategory = true
						break
					}
				}
				if !hasInputWithCategory {
					// No input with this token category, treat as genesis supply
					genesisSupplyMap[sCategory] += tao.BcashToken.Amount.AsUint64()
				}
			}

			if bytes.HasPrefix(tao.AddrDesc, BCMR_PREFIX) {
				if len(btxID) == 0 {
					btxID, _ = d.chainParser.PackTxid(tx.Txid)
				}
				metaQueue = append(metaQueue, &BcashTokenMetaQueue{
					TxId:    btxID,
					Vout:    uint16(i),
					Height:  block.Height,
					Txi:     uint32(txi),
					Retries: 0,
				})
			}
		}

		for i := range ta.Inputs {
			tai := &ta.Inputs[i]
			if tai.BcashToken != nil {
				spendingTxid := (*blockTxIDs)[txi]

				sCategory := string(tai.BcashToken.Category)
				addToAddressesMap(addresses, sCategory, spendingTxid, ^int32(i))

				if tokenTxsMap[sCategory] == nil {
					tokenTxsMap[sCategory] = make(map[string]bool)
				}
				tokenTxsMap[sCategory][stxID] = true

				if tai.BcashToken.Nft != nil {
					if spentCommitments[sCategory] == nil {
						spentCommitments[sCategory] = make(map[string]bool)
					}
					spentCommitments[sCategory][string(tai.BcashToken.Nft.Commitment)] = true
				}
			}
		}
	}

	for sCategory, txs := range tokenTxsMap {
		bcashToken, e := bcashTokens[sCategory]
		if !e {
			bcashToken, err := d.GetBcashToken([]byte(sCategory))
			if err != nil {
				return err
			}
			if bcashToken == nil {
				bcashToken = &BcashToken{
					Standard: bchain.CashToken,
				}
			}
			bcashTokens[sCategory] = bcashToken
			d.cbs.bcashTokensMiss++
		} else {
			d.cbs.bcashTokensHit++
		}

		bcashToken, _ = bcashTokens[sCategory]

		bcashToken.Txs += uint(len(txs))
		if genesisSupplyMap[sCategory] > 0 {
			var gs big.Int
			gs.SetUint64(genesisSupplyMap[sCategory])
			bcashToken.GenesisSupply.Set(&gs)
		}

		updated := false
		// Remove commitments that were both created and spent in this block
		if created, spent := createdCommitments[sCategory], spentCommitments[sCategory]; created != nil && spent != nil {
			for commitment := range spent {
				if created[commitment] {
					delete(spent, commitment)
					delete(created, commitment)
				}
			}
		}

		// Remove spent commitments from bcashToken.Commitments efficiently
		if spent := spentCommitments[sCategory]; len(spent) > 0 && len(bcashToken.Commitments) > 0 {
			commitments := bcashToken.Commitments[:0]
			for _, c := range bcashToken.Commitments {
				if !spent[string(c)] {
					commitments = append(commitments, c)
				}
			}
			if len(commitments) != len(bcashToken.Commitments) {
				bcashToken.Commitments = commitments
				updated = true
			}
		}

		// Add new commitments that were created but not spent
		if created := createdCommitments[sCategory]; created != nil {
			for commitment := range created {
				if spentCommitments[sCategory] == nil || !spentCommitments[sCategory][commitment] {
					bcashToken.Commitments = append(bcashToken.Commitments, []byte(commitment))
					updated = true
				}
			}
		}

		if updated && len(bcashToken.Commitments) > 1 {
			// sort by length and then alphabetically
			sort.Slice(bcashToken.Commitments, func(i, j int) bool {
				if len(bcashToken.Commitments[i]) == len(bcashToken.Commitments[j]) {
					return bytes.Compare(bcashToken.Commitments[i], bcashToken.Commitments[j]) < 0
				}
				return len(bcashToken.Commitments[i]) < len(bcashToken.Commitments[j])
			})
		}
	}

	if len(metaQueue) > 0 {
		serializedMetaQueue := make([][]byte, len(metaQueue))
		for i, mq := range metaQueue {
			serializedMetaQueue[i] = append(PackBcashTokenMetaQueueKey(mq, make([]byte, 34)), PackBcashTokenMetaQueue(mq, make([]byte, 9))...)
		}
		common.BcmrMetaQueueSignal <- serializedMetaQueue
	}

	return nil
}

// BcashTokenMeta index
type BcashTokenMeta struct {
	Height      uint32
	Txi         uint32
	Name        string
	Symbol      string
	Description string
	Decimals    uint8
	Website     string
	Icon        string
}

func packBcashTokenMeta(meta *BcashTokenMeta, buf []byte) []byte {
	buf = buf[:0]
	if meta == nil {
		return buf
	}

	// Height
	buf = append(buf, packUint(meta.Height)...)

	// Txi
	buf = append(buf, packUint(meta.Txi)...)

	// Name
	nameBytes := packString(meta.Name)
	buf = append(buf, nameBytes...)

	// Symbol
	symbolBytes := packString(meta.Symbol)
	buf = append(buf, symbolBytes...)

	// Description
	descriptionBytes := packString(meta.Description)
	buf = append(buf, descriptionBytes...)

	// Decimals
	buf = append(buf, meta.Decimals)

	// Website
	websiteBytes := packString(meta.Website)
	buf = append(buf, websiteBytes...)

	// Icon
	iconBytes := packString(meta.Icon)
	buf = append(buf, iconBytes...)

	return buf
}

func unpackBcashTokenMeta(buf []byte) (*BcashTokenMeta, int, error) {
	if len(buf) == 0 {
		return nil, 0, nil
	}

	meta := BcashTokenMeta{}
	var al int

	// Height
	if al+4 > len(buf) {
		return nil, 0, errors.New("Invalid buffer length for height")
	}
	meta.Height = unpackUint(buf[al:])
	al += 4

	// Txi
	if al+4 > len(buf) {
		return nil, 0, errors.New("Invalid buffer length for txi")
	}
	meta.Txi = unpackUint(buf[al:])
	al += 4

	// Name
	name, l := unpackString(buf[al:])
	al += l
	meta.Name = name

	// Symbol
	symbol, l := unpackString(buf[al:])
	al += l
	meta.Symbol = symbol

	// Description
	description, l := unpackString(buf[al:])
	al += l
	meta.Description = description

	// Decimals
	if al >= len(buf) {
		return nil, 0, errors.New("Invalid buffer length for decimals")
	}
	meta.Decimals = buf[al]
	al++

	// Website
	website, l := unpackString(buf[al:])
	al += l
	meta.Website = website

	// Icon
	icon, l := unpackString(buf[al:])
	al += l
	meta.Icon = icon

	return &meta, al, nil
}

// GetBcashTokenMeta returns the BcashTokenMeta for a given category or nil if not found.
func (d *RocksDB) GetBcashTokenMeta(category []byte) (*BcashTokenMeta, error) {
	if len(category) != 32 {
		return nil, errors.New("Invalid category")
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfBcashTokenMeta], category)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	meta, _, err := unpackBcashTokenMeta(data)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// storeBcashTokenMetas stores BcashTokenMeta data in the database.
func (d *RocksDB) StoreBcashTokenMetas(wb *grocksdb.WriteBatch, metas map[string]*BcashTokenMeta) error {
	buf := make([]byte, 1024)
	for sCategory, meta := range metas {
		if meta == nil {
			return errors.New("Invalid meta, nil")
		}

		key := []byte(sCategory)
		buf = packBcashTokenMeta(meta, buf)
		wb.PutCF(d.cfh[cfBcashTokenMeta], key, buf)
	}
	return nil
}

func (d *RocksDB) RemoveAllBcashTokenMeta(wb *grocksdb.WriteBatch, category []byte) error {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfBcashTokenMeta])
	defer it.Close()

	prefix := category
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Key().Data()
		wb.DeleteCF(d.cfh[cfBcashTokenMeta], key)
	}
	if err := it.Err(); err != nil {
		return err
	}

	return nil
}

// BcashTokenNftMeta index
type BcashTokenNftMeta struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

func packBcashTokenNftMeta(meta *BcashTokenNftMeta, buf []byte) []byte {
	buf = buf[:0]
	if meta == nil {
		return buf
	}

	// Name
	nameBytes := packString(meta.Name)
	buf = append(buf, nameBytes...)

	// Description
	descriptionBytes := packString(meta.Description)
	buf = append(buf, descriptionBytes...)

	// Icon
	iconBytes := packString(meta.Icon)
	buf = append(buf, iconBytes...)

	return buf
}

func unpackBcashTokenNftMeta(buf []byte) (*BcashTokenNftMeta, int, error) {
	if len(buf) == 0 {
		return nil, 0, nil
	}

	meta := BcashTokenNftMeta{}
	var al int

	// Name
	name, l := unpackString(buf[al:])
	al += l
	meta.Name = name

	// Description
	description, l := unpackString(buf[al:])
	al += l
	meta.Description = description

	// Icon
	icon, l := unpackString(buf[al:])
	al += l
	meta.Icon = icon

	return &meta, al, nil
}

// GetBcashTokenNftMeta returns the BcashTokenNftMeta for a given category and commitment or nil if not found.
func (d *RocksDB) GetBcashTokenNftMeta(category []byte, commitment []byte) (*BcashTokenNftMeta, error) {
	if len(category) != 32 {
		return nil, errors.New("Invalid category")
	}
	key := append(category, commitment...)
	val, err := d.db.GetCF(d.ro, d.cfh[cfBcashTokenNftMeta], key)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	meta, _, err := unpackBcashTokenNftMeta(data)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// storeBcashTokenNftMetas stores BcashTokenNftMeta data in the database.
func (d *RocksDB) StoreBcashTokenNftMetas(wb *grocksdb.WriteBatch, metas map[string]map[string]*BcashTokenNftMeta) error {
	buf := make([]byte, 1024)
	for sCategory, commitments := range metas {
		category := []byte(sCategory)
		if len(category) != 32 {
			glog.Warningf("rocksdb: bcash token nft meta invalid category %s", hex.EncodeToString(category))
			continue
		}
		for sCommitment, meta := range commitments {
			if meta == nil {
				return errors.New("Invalid meta, nil")
			}

			key := append(category, sCommitment...)

			buf = packBcashTokenNftMeta(meta, buf)
			wb.PutCF(d.cfh[cfBcashTokenNftMeta], key, buf)
		}
	}
	return nil
}

func (d *RocksDB) RemoveAllBcashTokenNftMeta(wb *grocksdb.WriteBatch, category []byte) error {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfBcashTokenNftMeta])
	defer it.Close()

	prefix := category
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Key().Data()
		wb.DeleteCF(d.cfh[cfBcashTokenNftMeta], key)
	}
	if err := it.Err(); err != nil {
		return err
	}

	return nil
}
