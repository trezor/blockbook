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

func verifyGetInfo(t *testing.T, id int, lrs *logRequestResponse, bbRequest map[string]json.RawMessage, bbResponseS string, stat *verifyStats) {
	bbResponse := resultGetInfo{}
	err := json.Unmarshal([]byte(bbResponseS), &bbResponse)
	if err != nil {
		t.Log(id, ": error unmarshal BB request ", err)
		return
	}
	logResponse := resultGetInfo{}
	err = json.Unmarshal([]byte(lrs.Response), &logResponse)
	if err != nil {
		t.Log(id, ": error unmarshal log request ", err)
		return
	}
	if logResponse.Result.Blocks <= bbResponse.Result.Blocks &&
		logResponse.Result.Testnet == bbResponse.Result.Testnet &&
		logResponse.Result.Network == bbResponse.Result.Network {
		stat.SuccessCount++
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
	t.Log(id, ",", method, ": response ", res)
	stat := getStat(method, stats)
	stat.Count++
	stat.TotalLogNs += lrs.LogElapsedTime
	stat.TotalBlockbookNs += ts
	switch method {
	case "getAddressTxids":
	case "getAddressHistory":
	case "getBlockHeader":
	case "getDetailedTransaction":
	case "getInfo":
		verifyGetInfo(t, id, lrs, req, res, stat)
	case "estimateSmartFee":
	case "estimateFee":
	case "sendTransaction":
	case "getMempoolEntry":
		break
	default:
		t.Log(id, ",", method, ": unknown method", method)
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
	for _, k := range keys {
		s := stats[k]
		failures += s.Count - s.SuccessCount
		t.Log("Method:", k, "\tCount:", s.Count, "\tSuccess:", s.SuccessCount, "\tTime log:", s.TotalLogNs, "\tTime BB:", s.TotalBlockbookNs)
	}
	if failures != 0 {
		t.Error("Number of failures:", failures)
	}
}
