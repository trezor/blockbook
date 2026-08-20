//go:build unittest

package common

import "testing"

func TestRedactURLs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// the http client's *url.Error on a dial failure or timeout - the widest leak
			name: "url.Error from the http client",
			in:   `Post "https://gas.api.infura.io/v3/SECRET_API_KEY/networks/56/suggestedGasFees": context deadline exceeded`,
			want: `Post "https://gas.api.infura.io": context deadline exceeded`,
		},
		{
			// our own annotation, errors.New(url + " " + method + ...)
			name: "own annotation",
			in:   "https://relay.example.com/v1/SECRET eth_sendRawTransaction : failed, empty result",
			want: "https://relay.example.com eth_sendRawTransaction : failed, empty result",
		},
		{
			name: "key in userinfo",
			in:   `Post "https://user:SECRET@relay.example.com/rpc": EOF`,
			want: `Post "https://relay.example.com": EOF`,
		},
		{
			name: "key in query",
			in:   "get https://api.coingecko.com/api/v3/simple/price?x_cg_pro_api_key=SECRET failed",
			want: "get https://api.coingecko.com failed",
		},
		{
			name: "host and port kept",
			in:   `Post "http://127.0.0.1:8065/v3/SECRET": connection refused`,
			want: `Post "http://127.0.0.1:8065": connection refused`,
		},
		{
			name: "websocket scheme",
			in:   `dial ws://127.0.0.1:8065/SECRET: connection reset by peer`,
			want: `dial ws://127.0.0.1:8065 connection reset by peer`,
		},
		{
			name: "several urls in one message",
			in:   "https://a.example.com/K1 failed; https://b.example.com/K2 failed",
			want: "https://a.example.com failed; https://b.example.com failed",
		},
		{
			// a plain JSON-RPC rejection must reach the client untouched: the wallet classifies on it
			name: "json-rpc rejection untouched",
			in:   "replacement transaction underpriced",
			want: "replacement transaction underpriced",
		},
		{
			name: "nonce rejection untouched",
			in:   "nonce too low: next nonce 5, tx nonce 3",
			want: "nonce too low: next nonce 5, tx nonce 3",
		},
		{
			name: "no host to keep",
			in:   "reading file:///etc/blockbook/secret.conf failed",
			want: "reading [redacted url] failed",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactURLs(tt.in); got != tt.want {
				t.Errorf("RedactURLs()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}
