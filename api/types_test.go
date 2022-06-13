//go:build unittest

package api

import (
	"encoding/json"
	"math/big"
	"reflect"
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
