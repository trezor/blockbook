//go:build unittest

package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
)

func TestWebsocketErrorLogIsBounded(t *testing.T) {
	metrics, err := common.GetMetrics("WsErrorLog")
	if err != nil {
		t.Fatal(err)
	}
	s := &WebsocketServer{metrics: metrics, is: &common.InternalState{}}
	c := &websocketChannel{id: 1, out: make(chan *WsRes, outChannelSize), alive: true}
	// encoding/json copies the whole offending literal into its error text
	req := &WsReq{ID: "1", Method: "getBlockHash", Params: json.RawMessage(`{"height":` + strings.Repeat("1", 16000) + `}`)}
	send := func() string {
		s.onRequest(c, req)
		res := <-c.out
		e, ok := res.Data.(resultError)
		if !ok {
			t.Fatalf("expected an error response, got %T", res.Data)
		}
		return e.Error.Message
	}

	t.Run("error text is truncated", func(t *testing.T) {
		lines, bytes := glog.Stats.Error.Lines(), glog.Stats.Error.Bytes()
		msg := send()
		if n := len(msg); n > 1100 {
			t.Errorf("response error message is %d bytes, want <= 1100", n)
		}
		if glog.Stats.Error.Lines()-lines != 1 {
			t.Fatalf("expected one error line, got %d", glog.Stats.Error.Lines()-lines)
		}
		if n := glog.Stats.Error.Bytes() - bytes; n > 1500 {
			t.Errorf("error line is %d bytes, want <= 1500", n)
		}
	})

	t.Run("error lines are rate limited", func(t *testing.T) {
		lines := glog.Stats.Error.Lines()
		for i := 0; i < 1000; i++ {
			if send() == "" {
				t.Fatal("empty error message")
			}
		}
		// the loop may straddle a second boundary, which grants a second budget
		if n := glog.Stats.Error.Lines() - lines; n > 20 {
			t.Errorf("1000 bad requests logged %d error lines, want <= 20", n)
		}
	})
}
