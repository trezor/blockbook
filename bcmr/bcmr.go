package bcmr

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

type BcmrDownloader struct {
	periodSeconds int64
	db            *db.RocksDB
	provider      string // BCMR indexer deployment from https://github.com/paytaca/bcmr-indexer
	mux           sync.RWMutex
}

var DefaultBcmrProvider = "https://bcmr.paytaca.com"
var MAX_RETRIES = uint8(144) // 24 hours with 10 min interval

// NewBcmrDownloader initializes the BcmrDownloader, which handles downloading and storing BCMR token metadata, and also retries failed downloads
func NewBcmrDownloader(d *db.RocksDB, config *common.Config, metrics *common.Metrics) (*BcmrDownloader, error) {
	var bd = &BcmrDownloader{
		periodSeconds: 10 * 60, // 10 minutes
	}

	if config.BcmrProvider == "" {
		bd.provider = DefaultBcmrProvider
		glog.Infof("Using default BCMR provider %v", bd.provider)
	} else {
		bd.provider = config.BcmrProvider
		glog.Infof("Using custom BCMR provider %v", bd.provider)
	}
	bd.db = d

	go func() {
		for signal := range common.BcmrMetaQueueSignal {
			if len(signal) == 0 {
				continue
			}

			queue := make([]*db.BcashTokenMetaQueue, 0)
			for _, item := range signal {
				meta, l, err := db.UnpackBcashTokenMetaQueueKey(item[:34])
				if err != nil || l != 34 {
					glog.Errorf("Error unpacking BCMR meta queue item key %x: %+v", item, err)
					continue
				}
				l, err = db.UnpackBcashTokenMetaQueue(item[34:], meta)
				if err != nil || l != 9 {
					glog.Errorf("Error unpacking BCMR meta queue item %x: %+v", item, err)
					continue
				}

				queue = append(queue, meta)
			}

			glog.Info("Processing BCMR meta queue signal with ", len(queue), " items")
			bd.processMetaQueue(queue)
		}
	}()

	return bd, nil
}

// RunDownloader periodically processes the BCMR metadata download queue
func (bd *BcmrDownloader) RunDownloader() error {
	glog.Infof("Starting BCMR downloader...")

	for {
		unix := time.Now().Unix()
		next := unix + bd.periodSeconds
		next -= next % bd.periodSeconds

		if next-unix < bd.periodSeconds {
			next += int64(rand.Intn(3))
			time.Sleep(time.Duration(next-unix) * time.Second)
		}

		metaQueue, err := bd.db.GetAllBcashTokenMetaQueue()
		glog.Infof("BCMR metadata download queue has %d items", len(metaQueue))
		if err != nil {
			glog.Errorf("Error getting BCMR metadata download queue: %+v", err)
			continue
		}

		bd.processMetaQueue(metaQueue)
	}
}

func (bd *BcmrDownloader) processMetaQueue(metaQueue []*db.BcashTokenMetaQueue) {
	bd.mux.Lock()
	defer bd.mux.Unlock()

	if len(metaQueue) != 0 {
		updated := make([]*db.BcashTokenMetaQueue, 0)
		deleted := make([]*db.BcashTokenMetaQueue, 0)
		addedCategories := make(map[string]*db.BcashTokenMeta)
		addedNfts := make(map[string]map[string]*db.BcashTokenNftMeta)

		for _, meta := range metaQueue {
			// Process metadata
			glog.Infof("Processing meta: %s:%d", hex.EncodeToString(meta.TxId), meta.Vout)
			registry, err := bd.getRegistry(fmt.Sprintf("%s/api/registries/%s:%d/", bd.provider, hex.EncodeToString(meta.TxId), meta.Vout))
			if err != nil {
				// Remove from queue if error includes 'cannot unmarshal'
				if strings.Contains(err.Error(), "cannot unmarshal") {
					glog.Errorf("Cannot unmarshal registry for %s:%d, removing from queue", hex.EncodeToString(meta.TxId), meta.Vout)
					deleted = append(deleted, meta)
					continue
				}

				glog.Errorf("Error downloading BCMR registry %s:%d: %+v", hex.EncodeToString(meta.TxId), meta.Vout, err)
				meta.Retries += 1
				if meta.Retries > MAX_RETRIES {
					glog.Errorf("Max retries reached for BCMR registry %s:%d, removing from queue", hex.EncodeToString(meta.TxId), meta.Vout)
					deleted = append(deleted, meta)
				} else {
					updated = append(updated, meta)
				}
				continue
			}

			if registry == nil || registry.Identities == nil || len(*registry.Identities) == 0 {
				glog.Infof("No identities found in BCMR registry %s:%d, removing from queue", hex.EncodeToString(meta.TxId), meta.Vout)
				deleted = append(deleted, meta)
				continue
			}

			for categoryHex, identity := range *registry.Identities {
				categoryBytes, err := hex.DecodeString(categoryHex)
				if err != nil || categoryBytes == nil || len(categoryBytes) != 32 {
					glog.Errorf("Error decoding category hex %s: %+v", categoryHex, err)
					deleted = append(deleted, meta)
					continue
				}

				stored, err := bd.db.GetBcashTokenMeta(categoryBytes)
				if err != nil {
					glog.Errorf("Error getting stored BCMR meta queue item for %s:%d: %+v", hex.EncodeToString(meta.TxId), meta.Vout, err)
				}
				if stored != nil {
					// check if fetched meta is older than stored one
					if stored.Height > meta.Height || (stored.Height == meta.Height && stored.Txi < meta.Txi) {
						// delete from queue
						deleted = append(deleted, meta)
						continue
					}
				}

				if len(identity) == 0 {
					glog.Infof("No identity snapshots found for category %s, skipping", categoryHex)
					deleted = append(deleted, meta)
					continue
				}

				// Get identity keys and sort them alphabetically
				identityKeys := make([]string, 0, len(identity))
				for k := range identity {
					identityKeys = append(identityKeys, k)
				}
				sort.Sort(sort.Reverse(sort.StringSlice(identityKeys)))

				// take only the latest snapshot
				snapshot := identity[identityKeys[0]]

				bcashMeta := &db.BcashTokenMeta{
					Decimals: 0,
				}
				bcashMeta.Name = snapshot.Name
				if snapshot.Description != nil {
					bcashMeta.Description = *snapshot.Description
				}
				if snapshot.Token != nil {
					if snapshot.Token.Decimals != nil {
						bcashMeta.Decimals = uint8(*snapshot.Token.Decimals)
					}
					bcashMeta.Symbol = snapshot.Token.Symbol
					if snapshot.Uris != nil && len(*snapshot.Uris) > 0 {
						for _, key := range []string{"icon", "image"} {
							if url := (*snapshot.Uris)[key]; url != "" {
								if strings.HasPrefix(url, "ipfs://") {
									url = "https://ipfs.io/ipfs/" + url[len("ipfs://"):]
								}
								bcashMeta.Icon = url
								break
							}
						}

						for _, key := range []string{"website", "web", "twitter", "x", "telegram"} {
							if url := (*snapshot.Uris)[key]; url != "" {
								bcashMeta.Website = url
								break
							}
						}
					}

					if snapshot.Token.Nfts != nil {
						for commitmentHex, nftType := range snapshot.Token.Nfts.Parse.Types {
							commitmentBytes, err := hex.DecodeString(commitmentHex)
							if err != nil {
								glog.Errorf("Error decoding commitment hex %s: %+v", commitmentHex, err)
								continue
							}
							bcashNftMeta := &db.BcashTokenNftMeta{}
							bcashNftMeta.Name = nftType.Name
							if nftType.Description != nil {
								bcashNftMeta.Description = *nftType.Description
							}

							if nftType.Uris != nil && len(*nftType.Uris) > 0 {
								for _, key := range []string{"icon", "image"} {
									if url := (*nftType.Uris)[key]; url != "" {
										if strings.HasPrefix(url, "ipfs://") {
											url = "https://ipfs.io/ipfs/" + url[len("ipfs://"):]
										}
										bcashNftMeta.Icon = url
										break
									}
								}
							}

							if addedNfts[string(categoryBytes)] == nil {
								addedNfts[string(categoryBytes)] = make(map[string]*db.BcashTokenNftMeta)
							}
							addedNfts[string(categoryBytes)][string(commitmentBytes)] = bcashNftMeta
						}
					}

					addedCategories[string(categoryBytes)] = bcashMeta
				}
			}

			// successfully processed, remove from queue
			deleted = append(deleted, meta)
		}

		if len(updated) > 0 || len(deleted) > 0 || len(addedCategories) > 0 || len(addedNfts) > 0 {
			wb := grocksdb.NewWriteBatch()

			bd.db.RemoveBcashTokenMetaQueue(wb, deleted)
			bd.db.StoreBcashTokenMetaQueue(wb, updated)

			for category := range addedCategories {
				categoryHex := hex.EncodeToString([]byte(category))
				err := bd.db.RemoveAllBcashTokenMeta(wb, []byte(category))
				if err != nil {
					glog.Errorf("Error removing old BCMR metadata for category %s: %+v", categoryHex, err)
					continue
				}
				err = bd.db.RemoveAllBcashTokenNftMeta(wb, []byte(category))
				if err != nil {
					glog.Errorf("Error removing old BCMR NFT metadata for category %s: %+v", categoryHex, err)
					continue
				}
			}

			bd.db.StoreBcashTokenMetas(wb, addedCategories)
			bd.db.StoreBcashTokenNftMetas(wb, addedNfts)

			if err := bd.db.WriteBatch(wb); err != nil {
				glog.Errorf("Error updating BCMR metadata download queue: %+v", err)
			}
			wb.Destroy()
		}
	}
}

func (bd *BcmrDownloader) getRegistry(url string) (*Registry, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		glog.Errorf("Error creating a new request for %v: %v", url, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 10 * time.Second,
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
