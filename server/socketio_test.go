package server

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/martinboehm/golang-socketio"
	"github.com/martinboehm/golang-socketio/transport"
)

var (
	// verifier functionality
	verifylog = flag.String("verifylog", "", "path to logfile containing socket.io requests/responses")
	wsurl     = flag.String("wsurl", "", "URL of socket.io interface to verify")
)

type verifyStats struct {
	Count            int
	SuccessCount     int
	TotalLogNs       int64
	TotalBlockbookNs int64
}

type logMessage struct {
	ID  int             `json:"id"`
	Et  int64           `json:"et"`
	Res json.RawMessage `json:"res"`
	Req json.RawMessage `json:"req"`
}

type logRequestResponse struct {
	Request, Response json.RawMessage
	LogElapsedTime    int64
}

func getStat(m string, stats map[string]*verifyStats) *verifyStats {
	s, ok := stats[m]
	if !ok {
		s = &verifyStats{}
		stats[m] = s
	}
	return s
}

func unmarshalResponses(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, bbResponse interface{}, logResponse interface{}) error {
	err := json.Unmarshal([]byte(bbResStr), bbResponse)
	if err != nil {
		t.Log(id, ": error unmarshal BB request ", err)
		return err
	}
	err = json.Unmarshal([]byte(lrs.Response), logResponse)
	if err != nil {
		t.Log(id, ": error unmarshal log request ", err)
		return err
	}
	return nil
}

func getFullAddressHistory(addr []string, rr addrOpts, ws *gosocketio.Client) (*resultGetAddressHistory, error) {
	rr.From = 0
	rr.To = 100000000
	rq := map[string]interface{}{
		"method": "getAddressHistory",
		"params": []interface{}{
			addr,
			rr,
		},
	}
	rrq, err := json.Marshal(rq)
	if err != nil {
		return nil, err
	}
	res, err := ws.Ack("message", json.RawMessage(rrq), time.Second*30)
	if err != nil {
		return nil, err
	}
	bbResponse := resultGetAddressHistory{}
	err = json.Unmarshal([]byte(res), &bbResponse)
	if err != nil {
		return nil, err
	}
	return &bbResponse, nil
}

func equalAddressHistoryItem(logItem addressHistoryItem, bbItem addressHistoryItem) error {
	if logItem.Tx.Hash != bbItem.Tx.Hash {
		return errors.Errorf("Different hash bb: %v log: %v", bbItem.Tx.Hash, logItem.Tx.Hash)
	}
	if logItem.Tx.Hex != bbItem.Tx.Hex {
		return errors.Errorf("Different hex bb: %v log: %v", bbItem.Tx.Hex, logItem.Tx.Hex)
	}
	// Addresses do not match, bb getAddressHistory does not return input addresses
	return nil
}
func verifyGetAddressHistory(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats, ws *gosocketio.Client, bbRequest map[string]json.RawMessage) {
	bbResponse := resultGetAddressHistory{}
	logResponse := resultGetAddressHistory{}
	var bbFullResponse *resultGetAddressHistory
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	// parse request to check params
	addr, rr, err := unmarshalGetAddressRequest(bbRequest["params"])
	if err != nil {
		t.Log(id, ": getAddressHistory error unmarshal BB request ", err)
		return
	}
	// mempool transactions are not comparable
	if !rr.QueryMempoolOnly {
		if (logResponse.Result.TotalCount != bbResponse.Result.TotalCount) ||
			len(logResponse.Result.Items) != len(bbResponse.Result.Items) {
			t.Log("getAddressHistory", id, "mismatch bb:", bbResponse.Result.TotalCount, len(bbResponse.Result.Items),
				"log:", logResponse.Result.TotalCount, len(logResponse.Result.Items))
			return
		}
		if logResponse.Result.TotalCount > 0 {
			for i, logItem := range logResponse.Result.Items {
				bbItem := bbResponse.Result.Items[i]
				if err = equalAddressHistoryItem(logItem, bbItem); err != nil {
					// if multiple addresses are specified, BlockBook returns transactions in different order
					// which causes problems in paged responses
					// we have to get all transactions from blockbook and check that they are in the logged response
					var err1 error
					if bbFullResponse == nil {
						bbFullResponse, err1 = getFullAddressHistory(addr, rr, ws)
						if err1 != nil {
							t.Log("getFullAddressHistory error", err)
							return
						}
						if bbFullResponse.Result.TotalCount != logResponse.Result.TotalCount {
							t.Log("getFullAddressHistory count mismatch", bbFullResponse.Result.TotalCount, logResponse.Result.TotalCount)
							return
						}
					}
					found := false
					for _, bbFullItem := range bbFullResponse.Result.Items {
						if err1 = equalAddressHistoryItem(logItem, bbFullItem); err1 == nil {
							found = true
							break
						}
					}
					if !found {
						t.Log("getAddressHistory", id, "addresses", addr, "mismatch ", err)
						// bf, _ := json.Marshal(bbFullResponse.Result)
						// bl, _ := json.Marshal(logResponse.Result)
						// t.Log("{ \"bf\":", string(bf), ",\"bl\":", string(bl), "}")
						return
					}
				}
			}
		}
	}
	stat.SuccessCount++
}

func verifyGetInfo(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats) {
	bbResponse := resultGetInfo{}
	logResponse := resultGetInfo{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	if logResponse.Result.Blocks <= bbResponse.Result.Blocks &&
		logResponse.Result.Testnet == bbResponse.Result.Testnet &&
		logResponse.Result.Network == bbResponse.Result.Network {
		stat.SuccessCount++
	} else {
		t.Log("getInfo", id, "mismatch bb:", bbResponse.Result.Blocks, bbResponse.Result.Testnet, bbResponse.Result.Network,
			"log:", logResponse.Result.Blocks, logResponse.Result.Testnet, logResponse.Result.Network)
	}
}

func verifyGetBlockHeader(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats) {
	bbResponse := resultGetBlockHeader{}
	logResponse := resultGetBlockHeader{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	if logResponse.Result.Hash == bbResponse.Result.Hash {
		stat.SuccessCount++
	} else {
		t.Log("getBlockHeader", id, "mismatch bb:", bbResponse.Result.Hash,
			"log:", logResponse.Result.Hash)
	}
}

func verifyEstimateSmartFee(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats) {
	bbResponse := resultEstimateSmartFee{}
	logResponse := resultEstimateSmartFee{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	// it is not possible to compare fee directly, it changes over time,
	// verify that the BB fee is in a reasonable range
	if bbResponse.Result > 0 && bbResponse.Result < 1e-3 {
		stat.SuccessCount++
	} else {
		t.Log("estimateSmartFee", id, "mismatch bb:", bbResponse.Result,
			"log:", logResponse.Result)
	}
}

func verifySendTransaction(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats) {
	bbResponse := resultSendTransaction{}
	logResponse := resultSendTransaction{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	bbResponseError := resultError{}
	err := json.Unmarshal([]byte(bbResStr), &bbResponseError)
	if err != nil {
		t.Log(id, ": error unmarshal resultError ", err)
		return
	}
	// it is not possible to repeat sendTransaction, expect error
	if bbResponse.Result == "" && bbResponseError.Error.Message != "" {
		stat.SuccessCount++
	} else {
		t.Log("sendTransaction", id, "problem:", bbResponse.Result, bbResponseError)
	}
}

func verifyGetDetailedTransaction(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats) {
	bbResponse := resultGetDetailedTransaction{}
	logResponse := resultGetDetailedTransaction{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	equalInputs := func() error {
		if len(bbResponse.Result.Inputs) != len(logResponse.Result.Inputs) {
			return errors.Errorf("mismatch number of inputs %v %v", len(bbResponse.Result.Inputs), len(logResponse.Result.Inputs))
		}
		for i, bbi := range bbResponse.Result.Inputs {
			li := logResponse.Result.Inputs[i]

			if bbi.OutputIndex != li.OutputIndex ||
				bbi.Sequence != li.Sequence ||
				bbi.Satoshis != li.Satoshis {
				return errors.Errorf("mismatch input  %v %v, %v %v, %v %v", bbi.OutputIndex, li.OutputIndex, bbi.Sequence, li.Sequence, bbi.Satoshis, li.Satoshis)
			}
			// both must be null or both must not be null
			if bbi.Address != nil && li.Address != nil {
				if *bbi.Address != *li.Address {
					return errors.Errorf("mismatch input Address %v %v", *bbi.Address, *li.Address)
				}
			} else if bbi.Address != li.Address {
				// bitcore does not parse bech P2WPKH and P2WSH addresses
				if bbi.Address == nil || (*bbi.Address)[0:3] != "bc1" {
				return errors.Errorf("mismatch input Address %v %v", bbi.Address, li.Address)
			}
			}
			// both must be null or both must not be null
			if bbi.Script != nil && li.Script != nil {
				if *bbi.Script != *li.Script {
					return errors.Errorf("mismatch input Script %v %v", *bbi.Script, *li.Script)
				}
			} else if bbi.Script != li.Script {
				return errors.Errorf("mismatch input Script %v %v", bbi.Script, li.Script)
			}
		}
		return nil
	}
	equalOutputs := func() error {
		if len(bbResponse.Result.Outputs) != len(logResponse.Result.Outputs) {
			return errors.Errorf("mismatch number of outputs %v %v", len(bbResponse.Result.Outputs), len(logResponse.Result.Outputs))
		}
		for i, bbo := range bbResponse.Result.Outputs {
			lo := logResponse.Result.Outputs[i]
			if bbo.Satoshis != lo.Satoshis {
				return errors.Errorf("mismatch output Satoshis %v %v", bbo.Satoshis, lo.Satoshis)
			}
			// both must be null or both must not be null
			if bbo.Script != nil && lo.Script != nil {
				if *bbo.Script != *lo.Script {
					return errors.Errorf("mismatch output Script %v %v", *bbo.Script, *lo.Script)
				}
			} else if bbo.Script != lo.Script {
				return errors.Errorf("mismatch output Script %v %v", bbo.Script, lo.Script)
			}
			// both must be null or both must not be null
			if bbo.Address != nil && lo.Address != nil {
				if *bbo.Address != *lo.Address {
					return errors.Errorf("mismatch output Address %v %v", *bbo.Address, *lo.Address)
				}
			} else if bbo.Address != lo.Address {
				// bitcore does not parse bech P2WPKH and P2WSH addresses
				if bbo.Address == nil || (*bbo.Address)[0:3] != "bc1" {
				return errors.Errorf("mismatch output Address %v %v", bbo.Address, lo.Address)
			}
		}
		}
		return nil
	}
	// the tx in the log could have been still in mempool with Height -1
	if (bbResponse.Result.Height != logResponse.Result.Height && logResponse.Result.Height != -1) ||
		bbResponse.Result.Hash != logResponse.Result.Hash {
		t.Log("getDetailedTransaction", id, "mismatch bb:", bbResponse.Result.Hash, bbResponse.Result.Height,
			"log:", logResponse.Result.Hash, logResponse.Result.Height)
		return
	}
	// the tx in the log could have been still in mempool with BlockTimestamp 0
	if bbResponse.Result.BlockTimestamp != logResponse.Result.BlockTimestamp && logResponse.Result.BlockTimestamp != 0 {
		t.Log("getDetailedTransaction", id, "mismatch BlockTimestamp:", bbResponse.Result.BlockTimestamp,
			"log:", logResponse.Result.BlockTimestamp)
		return
	}
	if bbResponse.Result.Hex != logResponse.Result.Hex {
		t.Log("getDetailedTransaction", id, "mismatch Hex:", bbResponse.Result.Hex,
			"log:", logResponse.Result.Hex)
		return
	}
	if err := equalInputs(); err != nil {
		t.Log("getDetailedTransaction", id, err)
		return
	}
	if err := equalOutputs(); err != nil {
		t.Log("getDetailedTransaction", id, err)
		return
	}
	stat.SuccessCount++
}

func verifyMessage(t *testing.T, ws *gosocketio.Client, id int, lrs *logRequestResponse, stats map[string]*verifyStats) {
	req := make(map[string]json.RawMessage)
	err := json.Unmarshal(lrs.Request, &req)
	if err != nil {
		t.Log(id, ": error unmarshal request ", err)
		return
	}
	method := strings.Trim(string(req["method"]), "\"")
	if method == "" {
		t.Log(id, ": there is no method specified in request")
		return
	}
	// send the message to blockbook
	start := time.Now()
	res, err := ws.Ack("message", lrs.Request, time.Second*30)
	if err != nil {
		t.Log(id, ",", method, ": ws.Ack error ", err)
		getStat("ackError", stats).Count++
		return
	}
	ts := time.Since(start).Nanoseconds()
	stat := getStat(method, stats)
	stat.Count++
	stat.TotalLogNs += lrs.LogElapsedTime
	stat.TotalBlockbookNs += ts
	switch method {
	case "getAddressHistory":
		verifyGetAddressHistory(t, id, lrs, res, stat, ws, req)
	case "getBlockHeader":
		verifyGetBlockHeader(t, id, lrs, res, stat)
	case "getDetailedTransaction":
		verifyGetDetailedTransaction(t, id, lrs, res, stat)
	case "getInfo":
		verifyGetInfo(t, id, lrs, res, stat)
	case "estimateSmartFee":
		verifyEstimateSmartFee(t, id, lrs, res, stat)
	case "sendTransaction":
		verifySendTransaction(t, id, lrs, res, stat)
	// case "getAddressTxids":
	// case "estimateFee":
	// case "getMempoolEntry":
	default:
		t.Log(id, ",", method, ": unknown/unverified method", method)
	}
}

func Test_VerifyLog(t *testing.T) {
	if *verifylog == "" || *wsurl == "" {
		t.Skip("skipping test, flags verifylog or wsurl not specified")
	}
	t.Log("Verifying log", *verifylog, "against service", *wsurl)
	tr := transport.GetDefaultWebsocketTransport()
	tr.WebsocketDialer = websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	ws, err := gosocketio.Dial(*wsurl, tr)
	if err != nil {
		t.Fatal("Dial error ", err)
		return
	}
	defer ws.Close()
	file, err := os.Open(*verifylog)
	if err != nil {
		t.Fatal("File read error", err)
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1<<25)
	scanner.Buffer(buf, 1<<25)
	scanner.Split(bufio.ScanLines)
	line := 0
	stats := make(map[string]*verifyStats)
	pairs := make(map[int]*logRequestResponse, 0)
	for scanner.Scan() {
		line++
		msg := logMessage{}
		err := json.Unmarshal(scanner.Bytes(), &msg)
		if err != nil {
			t.Log("Line ", line, ": json error ", err)
			continue
		}
		lrs, exists := pairs[msg.ID]
		if !exists {
			lrs = &logRequestResponse{}
			pairs[msg.ID] = lrs
		}
		if msg.Req != nil {
			if lrs.Request != nil {
				t.Log("Line ", line, ": duplicate request with id ", msg.ID)
				continue
			}
			lrs.Request = msg.Req
		} else if msg.Res != nil {
			if lrs.Response != nil {
				t.Log("Line ", line, ": duplicate response with id ", msg.ID)
				continue
			}
			lrs.Response = msg.Res
			lrs.LogElapsedTime = msg.Et
		}
		if lrs.Request != nil && lrs.Response != nil {
			verifyMessage(t, ws, msg.ID, lrs, stats)
			delete(pairs, msg.ID)
		}
	}
	var keys []string
	for k := range stats {
		keys = append(keys, k)
	}
	failures := 0
	sort.Strings(keys)
	t.Log("Processed", line, "lines")
	for _, k := range keys {
		s := stats[k]
		failures += s.Count - s.SuccessCount
		t.Log("Method:", k, "\tCount:", s.Count, "\tSuccess:", s.SuccessCount,
			"\tTime log:", s.TotalLogNs, "\tTime BB:", s.TotalBlockbookNs,
			"\tTime BB/log", float64(s.TotalBlockbookNs)/float64(s.TotalLogNs))
	}
	if failures != 0 {
		t.Error("Number of failures:", failures)
	}
}
