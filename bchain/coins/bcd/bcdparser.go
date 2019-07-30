package bcd

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"
	"encoding/hex"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

type BitcoinDiamondParser struct {
	*btc.BitcoinParser
}

func NewBitcoinDiamondParser(params *chaincfg.Params, c *btc.Configuration) *BitcoinDiamondParser {
	p := btc.NewBitcoinParser(params, c)
	p.BaseParser.AmountDecimalPoint = 7
	return &BitcoinDiamondParser{BitcoinParser: p}
}

func (p *BitcoinDiamondParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	r.Seek(32, 0)
	//r.ReadAt()
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

func (p *BitcoinDiamondParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)

	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
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
			Time: w.Header.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}