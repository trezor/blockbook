//go:build integration

package sync

import "github.com/trezor/blockbook/bchain"

type fakeBlockChain struct {
	bchain.BlockChain
	returnFakes bool
	fakeBlocks  map[uint32]BlockID
	bestHeight  uint32
}

func (c *fakeBlockChain) GetBestBlockHash() (v string, err error) {
	return c.GetBlockHash(c.bestHeight)
}

func (c *fakeBlockChain) GetBestBlockHeight() (v uint32, err error) {
	return c.bestHeight, nil
}

func (c *fakeBlockChain) GetBlockHash(height uint32) (v string, err error) {
	if height > c.bestHeight {
		return "", bchain.ErrBlockNotFound
	}
	if c.returnFakes {
		if b, found := c.fakeBlocks[height]; found {
			return b.Hash, nil
		}
	}
	return c.BlockChain.GetBlockHash(height)
}

func (c *fakeBlockChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	if height > 0 && height > c.bestHeight {
		return nil, bchain.ErrBlockNotFound
	}
	if c.returnFakes {
		if hash == "" && height > 0 {
			var err error
			hash, err = c.GetBlockHash(height)
			if err != nil {
				return nil, err
			}
		}
	}
	b, err := c.BlockChain.GetBlock(hash, height)
	if err != nil {
		return nil, err
	}
	b.Height = height
	return b, nil
}
