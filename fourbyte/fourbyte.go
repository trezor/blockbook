package fourbyte

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/db"
)

// Coingecko is a structure that implements RatesDownloaderInterface
type FourByteSignaturesDownloader struct {
	url                string
	httpTimeoutSeconds time.Duration
	db                 *db.RocksDB
}

// NewFourByteSignaturesDownloader initializes the downloader for FourByteSignatures API.
func NewFourByteSignaturesDownloader(db *db.RocksDB, url string) (*FourByteSignaturesDownloader, error) {
	return &FourByteSignaturesDownloader{
		url:                url,
		httpTimeoutSeconds: 15 * time.Second,
		db:                 db,
	}, nil
}

// Run starts the FourByteSignatures downloader
func (fd *FourByteSignaturesDownloader) Run() {
	period := time.Hour * 24
	timer := time.NewTimer(period)
	for {
		fd.downloadSignatures()
		<-timer.C
		timer.Reset(period)
	}
}

type signatureData struct {
	Id            int    `json:"id"`
	TextSignature string `json:"text_signature"`
	HexSignature  string `json:"hex_signature"`
}

type signaturesPage struct {
	Count   int             `json:"count"`
	Next    string          `json:"next"`
	Results []signatureData `json:"results"`
}

func (fd *FourByteSignaturesDownloader) getPage(url string) (*signaturesPage, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		glog.Errorf("Error creating a new request for %v: %v", url, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: fd.httpTimeoutSeconds,
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
	var data signaturesPage
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		glog.Errorf("Error parsing 4byte signatures response from %s: %v", url, err)
		return nil, err
	}
	return &data, nil
}

func (fd *FourByteSignaturesDownloader) getPageWithRetry(url string) (*signaturesPage, error) {
	for retry := 1; retry <= 16; retry++ {
		page, err := fd.getPage(url)
		if err == nil && page != nil {
			return page, err
		}
		glog.Errorf("Error getting 4byte signatures from %s: %v, retry count %d", url, err, retry)
		timer := time.NewTimer(time.Second * time.Duration(retry))
		<-timer.C
	}
	return nil, errors.New("Too many retries to 4byte signatures")
}

func parseSignatureFromText(t string) *bchain.FourByteSignature {
	s := strings.Index(t, "(")
	e := strings.LastIndex(t, ")")
	if s < 0 || e < 0 {
		return nil
	}
	var signature bchain.FourByteSignature
	signature.Name = t[:s]
	params := t[s+1 : e]
	if len(params) > 0 {
		s = 0
		tupleDepth := 0
		// parse params as comma separated list
		// tuple is regarded as one parameter and not parsed further
		for i, c := range params {
			if c == ',' && tupleDepth == 0 {
				signature.Parameters = append(signature.Parameters, params[s:i])
				s = i + 1
			} else if c == '(' {
				tupleDepth++
			} else if c == ')' {
				tupleDepth--
			}
		}
		signature.Parameters = append(signature.Parameters, params[s:])
	}
	return &signature
}

func (fd *FourByteSignaturesDownloader) downloadSignatures() {
	period := time.Millisecond * 100
	timer := time.NewTimer(period)
	url := fd.url
	results := make([]signatureData, 0)
	glog.Info("FourByteSignaturesDownloader starting download")
	for {
		page, err := fd.getPageWithRetry(url)
		if err != nil {
			glog.Errorf("Error getting 4byte signatures from %s: %v", url, err)
			return
		}
		if page == nil {
			glog.Errorf("Empty page from 4byte signatures from %s: %v", url, err)
			return
		}
		glog.Infof("FourByteSignaturesDownloader downloaded %s with %d results", url, len(page.Results))
		if len(page.Results) > 0 {
			fourBytes, err := strconv.ParseUint(page.Results[0].HexSignature, 0, 0)
			if err != nil {
				glog.Errorf("Invalid 4byte signature %+v on page %s: %v", page.Results[0], url, err)
				return
			}
			sig, err := fd.db.GetFourByteSignature(uint32(fourBytes), uint32(page.Results[0].Id))
			if err != nil {
				glog.Errorf("db.GetFourByteSignature error %+v on page %s: %v", page.Results[0], url, err)
				return
			}
			// signature is already stored in db, break
			if sig != nil {
				break
			}
			results = append(results, page.Results...)
		}
		if page.Next == "" {
			// at the end
			break
		}
		url = page.Next
		// wait a bit to not to flood the server
		<-timer.C
		timer.Reset(period)
	}
	if len(results) > 0 {
		glog.Infof("FourByteSignaturesDownloader storing %d new signatures", len(results))
		wb := grocksdb.NewWriteBatch()
		defer wb.Destroy()

		for i := range results {
			r := &results[i]
			fourBytes, err := strconv.ParseUint(r.HexSignature, 0, 0)
			if err != nil {
				glog.Errorf("Invalid 4byte signature %+v: %v", r, err)
				return
			}
			fbs := parseSignatureFromText(r.TextSignature)
			if fbs != nil {
				fd.db.StoreFourByteSignature(wb, uint32(fourBytes), uint32(r.Id), fbs)
			} else {
				glog.Errorf("FourByteSignaturesDownloader invalid signature %s", r.TextSignature)
			}
		}

		if err := fd.db.WriteBatch(wb); err != nil {
			glog.Errorf("FourByteSignaturesDownloader failed to store signatures, %v", err)
		}

	}
	glog.Infof("FourByteSignaturesDownloader finished")
}
