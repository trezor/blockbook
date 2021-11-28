//go:build integration

package server

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"os"
	"reflect"
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
	newSocket = flag.Bool("newsocket", false, "Create new socket.io connection for each request")
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

func equalTx(logTx resTx, bbTx resTx) error {
	if logTx.Hash != bbTx.Hash {
		return errors.Errorf("Different Hash bb: %v log: %v", bbTx.Hash, logTx.Hash)
	}
	if logTx.Hex != bbTx.Hex {
		return errors.Errorf("Different Hex bb: %v log: %v", bbTx.Hex, logTx.Hex)
	}
	if logTx.BlockTimestamp != bbTx.BlockTimestamp && logTx.BlockTimestamp != 0 {
		return errors.Errorf("Different BlockTimestamp bb: %v log: %v", bbTx.BlockTimestamp, logTx.BlockTimestamp)
	}
	if logTx.FeeSatoshis != bbTx.FeeSatoshis {
		return errors.Errorf("Different FeeSatoshis bb: %v log: %v", bbTx.FeeSatoshis, logTx.FeeSatoshis)
	}
	if logTx.Height != bbTx.Height && logTx.Height != -1 {
		return errors.Errorf("Different Height bb: %v log: %v", bbTx.Height, logTx.Height)
	}
	if logTx.InputSatoshis != bbTx.InputSatoshis {
		return errors.Errorf("Different InputSatoshis bb: %v log: %v", bbTx.InputSatoshis, logTx.InputSatoshis)
	}
	if logTx.Locktime != bbTx.Locktime {
		return errors.Errorf("Different Locktime bb: %v log: %v", bbTx.Locktime, logTx.Locktime)
	}
	if logTx.OutputSatoshis != bbTx.OutputSatoshis {
		return errors.Errorf("Different OutputSatoshis bb: %v log: %v", bbTx.OutputSatoshis, logTx.OutputSatoshis)
	}
	if logTx.Version != bbTx.Version {
		return errors.Errorf("Different Version bb: %v log: %v", bbTx.Version, logTx.Version)
	}
	if len(logTx.Inputs) != len(bbTx.Inputs) {
		return errors.Errorf("Different number of Inputs bb: %v log: %v", len(bbTx.Inputs), len(logTx.Inputs))
	}
	// blockbook parses bech addresses, it is ok for bitcore to return nil address and blockbook parsed address
	for i := range logTx.Inputs {
		if logTx.Inputs[i].Satoshis != bbTx.Inputs[i].Satoshis ||
			(bbTx.Inputs[i].Address == nil && logTx.Inputs[i].Address != bbTx.Inputs[i].Address) ||
			(logTx.Inputs[i].Address != nil && *logTx.Inputs[i].Address != *bbTx.Inputs[i].Address) ||
			logTx.Inputs[i].OutputIndex != bbTx.Inputs[i].OutputIndex ||
			logTx.Inputs[i].Sequence != bbTx.Inputs[i].Sequence {
			return errors.Errorf("Different Inputs bb: %+v log: %+v", bbTx.Inputs, logTx.Inputs)
		}
	}
	if len(logTx.Outputs) != len(bbTx.Outputs) {
		return errors.Errorf("Different number of Outputs bb: %v log: %v", len(bbTx.Outputs), len(logTx.Outputs))
	}
	// blockbook parses bech addresses, it is ok for bitcore to return nil address and blockbook parsed address
	for i := range logTx.Outputs {
		if logTx.Outputs[i].Satoshis != bbTx.Outputs[i].Satoshis ||
			(bbTx.Outputs[i].Address == nil && logTx.Outputs[i].Address != bbTx.Outputs[i].Address) ||
			(logTx.Outputs[i].Address != nil && *logTx.Outputs[i].Address != *bbTx.Outputs[i].Address) {
			return errors.Errorf("Different Outputs bb: %+v log: %+v", bbTx.Outputs, logTx.Outputs)
		}
	}
	return nil
}

func equalAddressHistoryItem(logItem addressHistoryItem, bbItem addressHistoryItem) error {
	if err := equalTx(logItem.Tx, bbItem.Tx); err != nil {
		return err
	}
	if !reflect.DeepEqual(logItem.Addresses, bbItem.Addresses) {
		return errors.Errorf("Different Addresses bb: %v log: %v", bbItem.Addresses, logItem.Addresses)
	}
	if logItem.Satoshis != bbItem.Satoshis {
		return errors.Errorf("Different Satoshis bb: %v log: %v", bbItem.Satoshis, logItem.Satoshis)
	}
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
						err1 = equalAddressHistoryItem(logItem, bbFullItem)
						if err1 == nil {
							found = true
							break
						}
						if err1.Error()[:14] != "Different Hash" {
							t.Log(err1)
						}
					}
					if !found {
						t.Log("getAddressHistory", id, "addresses", addr, "mismatch ", err)
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
	if bbResponse.Result > 0 && bbResponse.Result < .1 {
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
	if err := equalTx(logResponse.Result, bbResponse.Result); err != nil {
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

func connectSocketIO(t *testing.T) *gosocketio.Client {
	tr := transport.GetDefaultWebsocketTransport()
	tr.WebsocketDialer = websocket.Dialer{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	ws, err := gosocketio.Dial(*wsurl, tr)
	if err != nil {
		t.Fatal("Dial error ", err)
		return nil
	}
	return ws
}

func Test_VerifyLog(t *testing.T) {
	if *verifylog == "" || *wsurl == "" {
		t.Skip("skipping test, flags verifylog or wsurl not specified")
	}
	t.Log("Verifying log", *verifylog, "against service", *wsurl)
	var ws *gosocketio.Client
	if !*newSocket {
		ws = connectSocketIO(t)
		defer ws.Close()
	}
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
			if *newSocket {
				ws = connectSocketIO(t)
			}
			verifyMessage(t, ws, msg.ID, lrs, stats)
			if *newSocket {
				ws.Close()
			}
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
