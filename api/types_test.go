// +build unittest

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
