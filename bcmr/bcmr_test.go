package bcmr

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/bch"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

func Test_parseSignatureFromText(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "asdf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		})
	}

}

func getRegistry(url string) (*Registry, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		glog.Errorf("Error creating a new request for %v: %v", url, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Invalid response status: " + string(resp.Status))
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var data Registry
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		glog.Errorf("Error unmarshalling response from %s: %v", url, err)
		return nil, err
	}

	return &data, nil
}

func Test_download(t *testing.T) {
	registry, _ := getRegistry("https://bcmr.paytaca.com/api/registries/cade35f821c314c4f16de1f99484deb47e0320a688e2557a7f0a7d865371d695:1/")

	for _, identity := range *registry.Identities {
		for _, revision := range identity {
			for token_id, token := range revision.Token.Nfts.Parse.Types {
				println(token_id, " == ", token.Name)
			}
		}
	}
}

func hexToBytes(h string) []byte {
	b, _ := hex.DecodeString(h)
	return b
}

func setupRocksDB(t *testing.T, p bchain.BlockChainParser) *db.RocksDB {
	tmp, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, p, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	return d
}

func getRocksDb(parser bchain.BlockChainParser, extendedIndex bool, t *testing.T) *db.RocksDB {
	db := setupRocksDB(t, parser)
	return db
}

func closeAndDestroyRocksDB(t *testing.T, d *db.RocksDB) {
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	// os.RemoveAll(d.path)
}

func bcashTestnetParser() *bch.BCashParser {
	parser, err := bch.NewBCashParser(
		bch.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})
	if err != nil {
		panic(err)
	}
	return parser
}

func Test_BcmrDownloader(t *testing.T) {
	parser := bcashTestnetParser()
	d := getRocksDb(parser, false, t)
	defer closeAndDestroyRocksDB(t, d)

	tests := []struct {
		name     string
		meta     *db.BcashTokenMetaQueue
		wantErr  bool
		wantErrR bool
	}{
		{
			name: "FT Token",
			meta: &db.BcashTokenMetaQueue{
				TxId:    hexToBytes("084ec120776ea1a4e0b50c3d6332f878288898f2de28a6b8818dd5b7e7e9fa1d"),
				Vout:    0,
				Height:  885381,
				Txi:     1,
				Retries: 0,
			},
		},
		{
			name: "NFT Token",
			meta: &db.BcashTokenMetaQueue{
				TxId:    hexToBytes("c61fadba6c9cf446a7d3a8154dfd98473a0bf80ab324f2b0fa8695704340d577"),
				Vout:    2,
				Height:  879461,
				Txi:     69,
				Retries: 0,
			},
		},
		{
			name: "Failing",
			meta: &db.BcashTokenMetaQueue{
				TxId:    hexToBytes("d756d3c80e08ba9f6f3a8417d21d519b15a4a3e342775f9fe4c3b8da856ca75a"),
				Vout:    100,
				Height:  794370,
				Txi:     1109,
				Retries: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := grocksdb.NewWriteBatch()
			defer wb.Destroy()

			queue := make([]*db.BcashTokenMetaQueue, 0)
			queue = append(queue, tt.meta)
			err := d.StoreBcashTokenMetaQueue(wb, queue)
			if (err != nil) != tt.wantErr {
				t.Errorf("db.StoreBcashTokenMetaQueue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err := d.WriteBatch(wb); err != nil {
				t.Errorf("WriteBatch() error = %v", err)
			}
		})
	}

	cfg := &common.Config{CoinName: "coin-unittest"}
	metrics := &common.Metrics{}
	downloader, err := NewBcmrDownloader(d, cfg, metrics)
	if err != nil {
		t.Fatalf("failed to create BcmrDownloader: %v", err)
	}

	queue, err := d.GetAllBcashTokenMetaQueue()

	downloader.processMetaQueue(queue)

	newQueue, err := d.GetAllBcashTokenMetaQueue()
	if len(newQueue) != 1 {
		t.Errorf("Expected 1 item in the queue after processing, got %d", len(newQueue))
	}
	if newQueue[0].Retries != 1 {
		t.Errorf("Expected 1 retry for the failing item, got %d", newQueue[0].Retries)
	}

	ftMeta, err := d.GetBcashTokenMeta(hexToBytes("270fc70ef4d352ff454bcc218e42b20f246f0691af07af9a069608f14f031c48"))
	if err != nil {
		t.Errorf("GetBcashTokenMeta() error = %v", err)
	}
	if ftMeta.Decimals == 0 {
		t.Errorf("Expected decimals to be set for FT token")
	}

	nftMeta, err := d.GetBcashTokenNftMeta(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691"), hexToBytes("0a"))
	if err != nil {
		t.Errorf("GetBcashTokenNftMeta() error = %v", err)
	}
	if nftMeta.Icon == "" {
		t.Errorf("Expected icon to be set for NFT token")
	}
}

func Test_BcmrDownloader_Overwrite(t *testing.T) {
	parser := bcashTestnetParser()
	d := getRocksDb(parser, false, t)
	defer closeAndDestroyRocksDB(t, d)

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()

	tokenMetas := map[string]*db.BcashTokenMeta{
		string(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691")): {
			Height: 879460,
			Txi:    69,
			Name:   "Will be overwritten",
		},
	}

	err := d.StoreBcashTokenMetas(wb, tokenMetas)
	if err != nil {
		t.Errorf("db.StoreBcashTokenMetas() error = %v", err)
	}

	tokenNftMetas := map[string]map[string]*db.BcashTokenNftMeta{
		string(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691")): {
			string(hexToBytes("0a")): {
				Name: "Will be overwritten",
			},
		},
	}
	err = d.StoreBcashTokenNftMetas(wb, tokenNftMetas)
	if err != nil {
		t.Errorf("db.StoreBcashTokenNftMetas() error = %v", err)
	}

	queue := make([]*db.BcashTokenMetaQueue, 0)
	queue = append(queue, &db.BcashTokenMetaQueue{
		TxId:    hexToBytes("c61fadba6c9cf446a7d3a8154dfd98473a0bf80ab324f2b0fa8695704340d577"),
		Vout:    2,
		Height:  879461,
		Txi:     69,
		Retries: 0,
	})
	err = d.StoreBcashTokenMetaQueue(wb, queue)
	if err != nil {
		t.Errorf("db.StoreBcashTokenMetaQueue() error = %v", err)
		return
	}

	if err := d.WriteBatch(wb); err != nil {
		t.Errorf("WriteBatch() error = %v", err)
	}

	cfg := &common.Config{CoinName: "coin-unittest"}
	metrics := &common.Metrics{}
	downloader, err := NewBcmrDownloader(d, cfg, metrics)
	if err != nil {
		t.Fatalf("failed to create BcmrDownloader: %v", err)
	}

	loaded, err := d.GetAllBcashTokenMetaQueue()
	if err != nil {
		t.Errorf("GetAllBcashTokenMetaQueue() error = %v", err)
	}

	downloader.processMetaQueue(loaded)

	newQueue, err := d.GetAllBcashTokenMetaQueue()
	if err != nil {
		t.Errorf("GetAllBcashTokenMetaQueue() error = %v", err)
	}

	if len(newQueue) != 0 {
		t.Errorf("Expected 0 item in the queue after processing, got %d", len(newQueue))
	}

	ftMeta, err := d.GetBcashTokenMeta(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691"))
	if err != nil {
		t.Errorf("GetBcashTokenMeta() error = %v", err)
	}
	if ftMeta.Name == "Will be overwritten" {
		t.Errorf("Expected token metadata to be overwritten, got %s", ftMeta.Name)
	}

	nftMeta, err := d.GetBcashTokenNftMeta(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691"), hexToBytes("0a"))
	if err != nil {
		t.Errorf("GetBcashTokenNftMeta() error = %v", err)
	}
	if nftMeta.Name == "Will be overwritten" {
		t.Errorf("Expected NFT token metadata to be overwritten, got %s", nftMeta.Name)
	}
}

func Test_BcmrDownloader_NoOverwrite(t *testing.T) {
	parser := bcashTestnetParser()
	d := getRocksDb(parser, false, t)
	defer closeAndDestroyRocksDB(t, d)

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()

	tokenMetas := map[string]*db.BcashTokenMeta{
		string(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691")): {
			Height: 879462,
			Txi:    69,
			Name:   "New, will not overwrite",
		},
	}

	err := d.StoreBcashTokenMetas(wb, tokenMetas)
	if err != nil {
		t.Errorf("db.StoreBcashTokenMetas() error = %v", err)
	}

	tokenNftMetas := map[string]map[string]*db.BcashTokenNftMeta{
		string(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691")): {
			string(hexToBytes("0a")): {
				Name: "New, will not overwrite",
			},
		},
	}
	err = d.StoreBcashTokenNftMetas(wb, tokenNftMetas)
	if err != nil {
		t.Errorf("db.StoreBcashTokenNftMetas() error = %v", err)
	}

	queue := make([]*db.BcashTokenMetaQueue, 0)
	queue = append(queue, &db.BcashTokenMetaQueue{
		TxId:    hexToBytes("c61fadba6c9cf446a7d3a8154dfd98473a0bf80ab324f2b0fa8695704340d577"),
		Vout:    2,
		Height:  879461,
		Txi:     69,
		Retries: 0,
	})
	err = d.StoreBcashTokenMetaQueue(wb, queue)
	if err != nil {
		t.Errorf("db.StoreBcashTokenMetaQueue() error = %v", err)
		return
	}

	if err := d.WriteBatch(wb); err != nil {
		t.Errorf("WriteBatch() error = %v", err)
	}

	cfg := &common.Config{CoinName: "coin-unittest"}
	metrics := &common.Metrics{}
	downloader, err := NewBcmrDownloader(d, cfg, metrics)
	if err != nil {
		t.Fatalf("failed to create BcmrDownloader: %v", err)
	}

	loaded, err := d.GetAllBcashTokenMetaQueue()
	if err != nil {
		t.Errorf("GetAllBcashTokenMetaQueue() error = %v", err)
	}

	downloader.processMetaQueue(loaded)

	newQueue, err := d.GetAllBcashTokenMetaQueue()
	if err != nil {
		t.Errorf("GetAllBcashTokenMetaQueue() error = %v", err)
	}

	if len(newQueue) != 0 {
		t.Errorf("Expected 0 item in the queue after processing, got %d", len(newQueue))
	}

	ftMeta, err := d.GetBcashTokenMeta(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691"))
	if err != nil {
		t.Errorf("GetBcashTokenMeta() error = %v", err)
	}
	if ftMeta.Name != "New, will not overwrite" {
		t.Errorf("Expected token metadata to not be overwritten, got %s", ftMeta.Name)
	}

	nftMeta, err := d.GetBcashTokenNftMeta(hexToBytes("5a4f6b25243c1a2dabb2434e3d9e574f65c31764ce0e7eb4127a46fa74657691"), hexToBytes("0a"))
	if err != nil {
		t.Errorf("GetBcashTokenNftMeta() error = %v", err)
	}
	if nftMeta.Name != "New, will not overwrite" {
		t.Errorf("Expected NFT token metadata to not be overwritten, got %s", nftMeta.Name)
	}
}
