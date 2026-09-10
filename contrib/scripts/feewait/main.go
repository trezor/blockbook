// feewait dumps the per-tier wait-time estimates a Blockbook host actually serves.
// Suite prefers a provider's maxWaitTimeEstimate over its own {low:4, medium:2, high:1}
// block fallback, so the tier ETA a user sees depends on which source answered.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type fee struct {
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MinWaitTimeEstimate  int    `json:"minWaitTimeEstimate"`
	MaxWaitTimeEstimate  int    `json:"maxWaitTimeEstimate"`
}

type resp struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

func main() {
	chains := strings.Split("eth,bsc,pol,arb,op,base,avax,hype,rhc", ",")
	if len(os.Args) > 1 {
		chains = strings.Split(os.Args[1], ",")
	}
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second, Proxy: http.ProxyFromEnvironment}
	fmt.Printf("%-6s %-8s %14s %14s %s\n", "chain", "tier", "minWait ms", "maxWait ms", "tip gwei")
	for _, c := range chains {
		u := (&url.URL{Scheme: "wss", Host: c + ".trezor.io", Path: "/websocket"}).String()
		conn, _, err := d.Dial(u, nil)
		if err != nil {
			fmt.Printf("%-6s dial: %v\n", c, err)
			continue
		}
		conn.WriteJSON(map[string]interface{}{"id": "f", "method": "estimateFee",
			"params": map[string]interface{}{"blocks": []int{1}, "specific": map[string]interface{}{
				"from": "0x0000000000000000000000000000000000000000",
				"to":   "0x0000000000000000000000000000000000000000", "value": "0x0"}}})
		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		_, payload, err := conn.ReadMessage()
		conn.Close()
		if err != nil {
			fmt.Printf("%-6s read: %v\n", c, err)
			continue
		}
		var r resp
		json.Unmarshal(payload, &r)
		var arr []struct {
			Eip1559 *struct {
				Low, Medium, High, Instant *fee
			} `json:"eip1559"`
		}
		if json.Unmarshal(r.Data, &arr) != nil || len(arr) == 0 || arr[0].Eip1559 == nil {
			fmt.Printf("%-6s no eip1559 block\n", c)
			continue
		}
		e := arr[0].Eip1559
		for _, t := range []struct {
			n string
			f *fee
		}{{"low", e.Low}, {"medium", e.Medium}, {"high", e.High}, {"instant", e.Instant}} {
			if t.f == nil {
				continue
			}
			fmt.Printf("%-6s %-8s %14d %14d %s\n", c, t.n, t.f.MinWaitTimeEstimate, t.f.MaxWaitTimeEstimate, t.f.MaxPriorityFeePerGas)
		}
	}
}
