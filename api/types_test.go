//go:build unittest

package api

import (
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"testing"
)

func TestAmount_MarshalJSON(t *testing.T) {
	type amounts struct {
		A1  Amount  `json:"a1"`
		A2  Amount  `json:"a2,omitempty"`
		PA1 *Amount `json:"pa1"`
		PA2 *Amount `json:"pa2,omitempty"`
	}
	tests := []struct {
		name string
		a    amounts
		want string
	}{
		{
			name: "empty",
			want: `{"a1":"0","a2":"0","pa1":null}`,
		},
		{
			name: "1",
			a: amounts{
				A1:  (Amount)(*big.NewInt(123456)),
				A2:  (Amount)(*big.NewInt(787901)),
				PA1: (*Amount)(big.NewInt(234567)),
				PA2: (*Amount)(big.NewInt(890123)),
			},
			want: `{"a1":"123456","a2":"787901","pa1":"234567","pa2":"890123"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(&tt.a)
			if err != nil {
				t.Errorf("json.Marshal() error = %v", err)
				return
			}
			if !reflect.DeepEqual(string(b), tt.want) {
				t.Errorf("json.Marshal() = %v, want %v", string(b), tt.want)
			}
			var parsed amounts
			err = json.Unmarshal(b, &parsed)
			if err != nil {
				t.Errorf("json.Unmarshal() error = %v", err)
				return
			}
			if !reflect.DeepEqual(parsed, tt.a) {
				t.Errorf("json.Unmarshal() = %v, want %v", parsed, tt.a)
			}
		})
	}
}

func TestBalanceHistories_SortAndAggregate(t *testing.T) {
	tests := []struct {
		name        string
		a           BalanceHistories
		groupByTime uint32
		want        BalanceHistories
	}{
		{
			name:        "empty",
			a:           []BalanceHistory{},
			groupByTime: 3600,
			want:        []BalanceHistory{},
		},
		{
			name: "one",
			a: []BalanceHistory{
				{
					ReceivedSat:   (*Amount)(big.NewInt(1)),
					SentSat:       (*Amount)(big.NewInt(2)),
					SentToSelfSat: (*Amount)(big.NewInt(1)),
					Time:          1521514812,
					Txid:          "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
					Txs:           1,
				},
			},
			groupByTime: 3600,
			want: []BalanceHistory{
				{
					ReceivedSat:   (*Amount)(big.NewInt(1)),
					SentSat:       (*Amount)(big.NewInt(2)),
					SentToSelfSat: (*Amount)(big.NewInt(1)),
					Time:          1521514800,
					Txs:           1,
				},
			},
		},
		{
			name: "aggregate",
			a: []BalanceHistory{
				{
					ReceivedSat:   (*Amount)(big.NewInt(1)),
					SentSat:       (*Amount)(big.NewInt(2)),
					SentToSelfSat: (*Amount)(big.NewInt(0)),
					Time:          1521504812,
					Txid:          "0011223344556677889900112233445566778899001122334455667788990011",
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(3)),
					SentSat:       (*Amount)(big.NewInt(4)),
					SentToSelfSat: (*Amount)(big.NewInt(2)),
					Time:          1521504812,
					Txid:          "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(5)),
					SentSat:       (*Amount)(big.NewInt(6)),
					SentToSelfSat: (*Amount)(big.NewInt(3)),
					Time:          1521514812,
					Txid:          "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(7)),
					SentSat:       (*Amount)(big.NewInt(8)),
					SentToSelfSat: (*Amount)(big.NewInt(3)),
					Time:          1521504812,
					Txid:          "00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840",
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(9)),
					SentSat:       (*Amount)(big.NewInt(10)),
					SentToSelfSat: (*Amount)(big.NewInt(5)),
					Time:          1521534812,
					Txid:          "0011223344556677889900112233445566778899001122334455667788990011",
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(11)),
					SentSat:       (*Amount)(big.NewInt(12)),
					SentToSelfSat: (*Amount)(big.NewInt(6)),
					Time:          1521534812,
					Txid:          "1122334455667788990011223344556677889900112233445566778899001100",
					Txs:           1,
				},
			},
			groupByTime: 3600,
			want: []BalanceHistory{
				{
					ReceivedSat:   (*Amount)(big.NewInt(11)),
					SentSat:       (*Amount)(big.NewInt(14)),
					SentToSelfSat: (*Amount)(big.NewInt(5)),
					Time:          1521504000,
					Txs:           2,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(5)),
					SentSat:       (*Amount)(big.NewInt(6)),
					SentToSelfSat: (*Amount)(big.NewInt(3)),
					Time:          1521514800,
					Txs:           1,
				},
				{
					ReceivedSat:   (*Amount)(big.NewInt(20)),
					SentSat:       (*Amount)(big.NewInt(22)),
					SentToSelfSat: (*Amount)(big.NewInt(11)),
					Time:          1521532800,
					Txs:           2,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.SortAndAggregate(tt.groupByTime); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BalanceHistories.SortAndAggregate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAmount_Compare(t *testing.T) {
	tests := []struct {
		name string
		a    *Amount
		b    *Amount
		want int
	}{
		{
			name: "nil-nil",
			a:    nil,
			b:    nil,
			want: 0,
		},
		{
			name: "20-nil",
			a:    (*Amount)(big.NewInt(20)),
			b:    nil,
			want: 1,
		},
		{
			name: "nil-20",
			a:    nil,
			b:    (*Amount)(big.NewInt(20)),
			want: -1,
		},
		{
			name: "18-20",
			a:    (*Amount)(big.NewInt(18)),
			b:    (*Amount)(big.NewInt(20)),
			want: -1,
		},
		{
			name: "20-20",
			a:    (*Amount)(big.NewInt(20)),
			b:    (*Amount)(big.NewInt(20)),
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Compare(tt.b); got != tt.want {
				t.Errorf("Amount.Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokens_Sort(t *testing.T) {
	tests := []struct {
		name     string
		unsorted Tokens
		sorted   Tokens
	}{
		{
			name: "one",
			unsorted: Tokens{
				{
					Name:      "a",
					Contract:  "0x1",
					BaseValue: 12.34,
				},
			},
			sorted: Tokens{
				{
					Name:      "a",
					Contract:  "0x1",
					BaseValue: 12.34,
				},
			},
		},
		{
			name: "mix",
			unsorted: Tokens{
				{
					Name:      "",
					Contract:  "0x6",
					BaseValue: 0,
				},
				{
					Name:      "",
					Contract:  "0x5",
					BaseValue: 0,
				},
				{
					Name:      "b",
					Contract:  "0x2",
					BaseValue: 1,
				},
				{
					Name:      "d",
					Contract:  "0x4",
					BaseValue: 0,
				},
				{
					Name:      "a",
					Contract:  "0x1",
					BaseValue: 12.34,
				},
				{
					Name:      "c",
					Contract:  "0x3",
					BaseValue: 0,
				},
			},
			sorted: Tokens{
				{
					Name:      "a",
					Contract:  "0x1",
					BaseValue: 12.34,
				},
				{
					Name:      "b",
					Contract:  "0x2",
					BaseValue: 1,
				},
				{
					Name:      "c",
					Contract:  "0x3",
					BaseValue: 0,
				},
				{
					Name:      "d",
					Contract:  "0x4",
					BaseValue: 0,
				},
				{
					Name:      "",
					Contract:  "0x5",
					BaseValue: 0,
				},
				{
					Name:      "",
					Contract:  "0x6",
					BaseValue: 0,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.unsorted)
			if !reflect.DeepEqual(tt.unsorted, tt.sorted) {
				t.Errorf("Tokens Sort got %v, want %v", tt.unsorted, tt.sorted)
			}
		})
	}
}
