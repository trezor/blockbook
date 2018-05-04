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
	"github.com/martinboehm/golang-socketio"
	"github.com/martinboehm/golang-socketio/transport"
)

var (
	// verifier functionality
	verifylog = flag.String("verifylog", "/Users/mxb2/Downloads/messageLogBtc.log", "path to logfile containing socket.io requests/responses")
	wsurl     = flag.String("wsurl", "wss://blockbook-dev:8336", "URL of socket.io interface to verify")
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

func verifyGetAddressHistory(t *testing.T, id int, lrs *logRequestResponse, bbResStr string, stat *verifyStats, ws *gosocketio.Client, bbRequest map[string]json.RawMessage) {
	type reqParamsData struct {
		Start            int  `json:"start"`
		End              int  `json:"end"`
		QueryMempoolOnly bool `json:"queryMempoolOnly"`
		From             int  `json:"from"`
		To               int  `json:"to"`
	}
	bbResponse := resultGetAddressHistory{}
	logResponse := resultGetAddressHistory{}
	if err := unmarshalResponses(t, id, lrs, bbResStr, &bbResponse, &logResponse); err != nil {
		return
	}
	// parse request
	addr, rr, err := unmarshalGetAddressRequest(bbRequest["params"])
	if err != nil {
		t.Log(id, ": getAddressHistory error unmarshal BB request ", err)
		return
	}
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
				if logItem.Tx.Hash != bbItem.Tx.Hash || logItem.Tx.Hex != bbItem.Tx.Hex {
					t.Log("getAddressHistory", id, "mismatch in tx", i, "bb:", bbItem.Tx.Hash,
						"log:", logItem.Tx.Hash)

					// if multiple addresses are specified, BlockBook returns transactions in different order
					// which causes problems in paged responses
					// we have to get all transactions from blockbook and check that they are in the logged response
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
						t.Log(id, ", getAddressHistory: rq marshall error ", err)
						return
					}
					res, err := ws.Ack("message", json.RawMessage(rrq), time.Second*30)
					if err != nil {
						t.Log(id, ", getAddressHistory: ws.Ack error ", err)
						return
					}
					bbFullResponse := resultGetAddressHistory{}
					t.Log(id, ": bbResponse", bbResponse.Result.TotalCount, "bbFullResponse", bbFullResponse.Result.TotalCount)
					t.Log(string(rrq))
					err = json.Unmarshal([]byte(res), &bbFullResponse)
					if err != nil {
						t.Log(id, ": getAddressHistory error unmarshal BB response ", err)
						return
					}
					return
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
	// t.Log(id, ",", method, ": response ", res)
	stat := getStat(method, stats)
	stat.Count++
	stat.TotalLogNs += lrs.LogElapsedTime
	stat.TotalBlockbookNs += ts
	switch method {
	// case "getAddressTxids":
	case "getAddressHistory":
		verifyGetAddressHistory(t, id, lrs, res, stat, ws, req)
	case "getBlockHeader":
		verifyGetBlockHeader(t, id, lrs, res, stat)
	case "getDetailedTransaction":
	case "getInfo":
		verifyGetInfo(t, id, lrs, res, stat)
	case "estimateSmartFee":
		verifyEstimateSmartFee(t, id, lrs, res, stat)
	// case "estimateFee":
	// case "sendTransaction":
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
