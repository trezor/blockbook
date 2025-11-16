//go:build unittest

package server

import (
	"html/template"
	"reflect"
	"strings"
	"testing"
	"time"
)

func Test_formatInt64(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want template.HTML
	}{
		{"1", 1, "1"},
		{"13", 13, "13"},
		{"123", 123, "123"},
		{"1234", 1234, `1<span class="ns">234</span>`},
		{"91234", 91234, `91<span class="ns">234</span>`},
		{"891234", 891234, `891<span class="ns">234</span>`},
		{"7891234", 7891234, `7<span class="ns">891</span><span class="ns">234</span>`},
		{"67891234", 67891234, `67<span class="ns">891</span><span class="ns">234</span>`},
		{"567891234", 567891234, `567<span class="ns">891</span><span class="ns">234</span>`},
		{"4567891234", 4567891234, `4<span class="ns">567</span><span class="ns">891</span><span class="ns">234</span>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatInt64(tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("formatInt64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_formatTime(t *testing.T) {
	timeNow = fixedTimeNow
	tests := []struct {
		name string
		want template.HTML
	}{
		{
			name: "2020-12-23 15:16:17",
			want: `<span tt="2020-12-23 15:16:17">630 days 21 hours ago</span>`,
		},
		{
			name: "2022-08-23 11:12:13",
			want: `<span tt="2022-08-23 11:12:13">23 days 1 hour ago</span>`,
		},
		{
			name: "2022-09-14 11:12:13",
			want: `<span tt="2022-09-14 11:12:13">1 day 1 hour ago</span>`,
		},
		{
			name: "2022-09-14 14:12:13",
			want: `<span tt="2022-09-14 14:12:13">22 hours 31 mins ago</span>`,
		},
		{
			name: "2022-09-15 09:33:26",
			want: `<span tt="2022-09-15 09:33:26">3 hours 10 mins ago</span>`,
		},
		{
			name: "2022-09-15 12:23:56",
			want: `<span tt="2022-09-15 12:23:56">20 mins ago</span>`,
		},
		{
			name: "2022-09-15 12:24:07",
			want: `<span tt="2022-09-15 12:24:07">19 mins ago</span>`,
		},
		{
			name: "2022-09-15 12:43:21",
			want: `<span tt="2022-09-15 12:43:21">35 secs ago</span>`,
		},
		{
			name: "2022-09-15 12:43:56",
			want: `<span tt="2022-09-15 12:43:56">0 secs ago</span>`,
		},
		{
			name: "2022-09-16 12:43:56",
			want: `2022-09-16 12:43:56`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, _ := time.Parse("2006-01-02 15:04:05", tt.name)
			if got := timeSpan(&tm); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("formatTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_appendAmountSpan(t *testing.T) {
	tests := []struct {
		name     string
		class    string
		amount   string
		shortcut string
		txDate   string
		want     string
	}{
		{
			name:     "prim-amt 1.23456789 BTC",
			class:    "prim-amt",
			amount:   "1.23456789",
			shortcut: "BTC",
			want:     `<span class="prim-amt">1.<span class="amt-dec">234<span class="ns">567</span><span class="ns">89</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt 1432134.23456 BTC",
			class:    "prim-amt",
			amount:   "1432134.23456",
			shortcut: "BTC",
			want:     `<span class="prim-amt">1<span class="nc">432</span><span class="nc">134</span>.<span class="amt-dec">234<span class="ns">56</span></span> BTC</span>`,
		},
		{
			name:     "sec-amt 1 EUR",
			class:    "sec-amt",
			amount:   "1",
			shortcut: "EUR",
			want:     `<span class="sec-amt">1 EUR</span>`,
		},
		{
			name:     "sec-amt -1 EUR",
			class:    "sec-amt",
			amount:   "-1",
			shortcut: "EUR",
			want:     `<span class="sec-amt">-1 EUR</span>`,
		},
		{
			name:     "sec-amt 432109.23 EUR",
			class:    "sec-amt",
			amount:   "432109.23",
			shortcut: "EUR",
			want:     `<span class="sec-amt">432<span class="nc">109</span>.<span class="amt-dec">23</span> EUR</span>`,
		},
		{
			name:     "sec-amt -432109.23 EUR",
			class:    "sec-amt",
			amount:   "-432109.23",
			shortcut: "EUR",
			want:     `<span class="sec-amt">-432<span class="nc">109</span>.<span class="amt-dec">23</span> EUR</span>`,
		},
		{
			name:     "sec-amt 43141.29 EUR",
			class:    "sec-amt",
			amount:   "43141.29",
			shortcut: "EUR",
			txDate:   "2022-03-14",
			want:     `<span class="sec-amt" tm="2022-03-14">43<span class="nc">141</span>.<span class="amt-dec">29</span> EUR</span>`,
		},
		{
			name:     "sec-amt -43141.29 EUR",
			class:    "sec-amt",
			amount:   "-43141.29",
			shortcut: "EUR",
			txDate:   "2022-03-14",
			want:     `<span class="sec-amt" tm="2022-03-14">-43<span class="nc">141</span>.<span class="amt-dec">29</span> EUR</span>`,
		},
		{
			name:     "prim-amt 1.23456789 BTC",
			class:    "prim-amt",
			amount:   "1.23456789",
			shortcut: "<javascript>alert(1)</javascript>",
			want:     `<span class="prim-amt">1.<span class="amt-dec">234<span class="ns">567</span><span class="ns">89</span></span> &lt;javascript&gt;alert(1)&lt;/javascript&gt;</span>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rv strings.Builder
			appendAmountSpan(&rv, tt.class, tt.amount, tt.shortcut, tt.txDate)
			if got := rv.String(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("appendAmountSpan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_appendAmountSpanBitcoinType(t *testing.T) {
	tests := []struct {
		name     string
		class    string
		amount   string
		shortcut string
		txDate   string
		want     string
	}{
		{
			name:     "prim-amt 1.23456789 BTC",
			class:    "prim-amt",
			amount:   "1.23456789",
			shortcut: "BTC",
			want:     `<span class="prim-amt">1.<span class="amt-dec">23<span class="ns">456</span><span class="ns">789</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt 1432134.23456 BTC",
			class:    "prim-amt",
			amount:   "1432134.23456",
			shortcut: "BTC",
			want:     `<span class="prim-amt">1<span class="nc">432</span><span class="nc">134</span>.<span class="amt-dec">23<span class="ns">456</span><span class="ns">000</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt 1 BTC",
			class:    "prim-amt",
			amount:   "1",
			shortcut: "BTC",
			want:     `<span class="prim-amt">1.<span class="amt-dec">00<span class="ns">000</span><span class="ns">000</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt 0 BTC",
			class:    "prim-amt",
			amount:   "0",
			shortcut: "BTC",
			want:     `<span class="prim-amt">0 BTC</span>`,
		},
		{
			name:     "prim-amt 34.2 BTC",
			class:    "prim-amt",
			amount:   "34.2",
			shortcut: "BTC",
			want:     `<span class="prim-amt">34.<span class="amt-dec">20<span class="ns">000</span><span class="ns">000</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt -34.2345678 BTC",
			class:    "prim-amt",
			amount:   "-34.2345678",
			shortcut: "BTC",
			want:     `<span class="prim-amt">-34.<span class="amt-dec">23<span class="ns">456</span><span class="ns">780</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt -1234.2345 BTC",
			class:    "prim-amt",
			amount:   "-1234.2345",
			shortcut: "BTC",
			want:     `<span class="prim-amt">-1<span class="nc">234</span>.<span class="amt-dec">23<span class="ns">450</span><span class="ns">000</span></span> BTC</span>`,
		},
		{
			name:     "prim-amt -123.23 BTC",
			class:    "prim-amt",
			amount:   "-123.23",
			shortcut: "BTC",
			want:     `<span class="prim-amt">-123.<span class="amt-dec">23<span class="ns">000</span><span class="ns">000</span></span> BTC</span>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rv strings.Builder
			appendAmountSpanBitcoinType(&rv, tt.class, tt.amount, tt.shortcut, tt.txDate)
			if got := rv.String(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("appendAmountSpanBitcoinType() = %v, want %v", got, tt.want)
			}
		})
	}
}
