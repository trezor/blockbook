package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
)

type cmd struct {
	ID     uint32      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type cmdError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *cmdError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

type cmdResult struct {
	ID     uint32          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *cmdError       `json:"error"`
}

// JSONRPC is a simple JSON-RPC HTTP client.
type JSONRPC struct {
	counter  uint32
	Client   http.Client
	URL      string
	User     string
	Password string
}

// Call constructs a JSON-RPC request, sends it over HTTP client, and unmarshals
// the response result.
func (c *JSONRPC) Call(method string, result interface{}, params ...interface{}) error {
	b, err := json.Marshal(&cmd{
		ID:     c.nextID(),
		Method: method,
		Params: params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.URL, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.User, c.Password)
	res, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	d := json.NewDecoder(res.Body)
	r := cmdResult{}
	if err = d.Decode(&r); err != nil {
		return err
	}
	if r.Error != nil {
		return r.Error
	}
	return json.Unmarshal(r.Result, result)
}

func (c *JSONRPC) nextID() uint32 {
	return atomic.AddUint32(&c.counter, 1)
}
