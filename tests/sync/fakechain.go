// +build integration

package sync

import (
	"blockbook/bchain"
	"errors"
)

type fakeBlockChain struct {
	bchain.BlockChain
	returnFakes    bool
	fakeBlocks     map[uint32]BlockID
	fakeBestHeight uint32
}

func (c *fakeBlockChain) GetBestBlockHash() (v string, err error) {
	if !c.returnFakes {
		return c.BlockChain.GetBestBlockHash()
	}
	if b, found := c.fakeBlocks[c.fakeBestHeight]; found {
		return b.Hash, nil
	} else {
		return "", errors.New("Not found")
	}
}

func (c *fakeBlockChain) GetBestBlockHeight() (v uint32, err error) {
	if !c.returnFakes {
		return c.BlockChain.GetBestBlockHeight()
	}
	return c.fakeBestHeight, nil
}

func (c *fakeBlockChain) GetBlockHash(height uint32) (v string, err error) {
	if c.returnFakes {
		if b, found := c.fakeBlocks[height]; found {
			return b.Hash, nil
		}
	}
	return c.BlockChain.GetBlockHash(height)
}

func (c *fakeBlockChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	if c.returnFakes {
		if hash == "" && height > 0 {
			var err error
			hash, err = c.GetBlockHash(height)
			if err != nil {
				return nil, err
			}
		}
	}
	return c.BlockChain.GetBlock(hash, height)
}
