package common

import (
	"reflect"
	"testing"
)

func TestJSONNumber_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		c       JSONNumber
		want    []byte
		wantErr bool
	}{
		{"0", JSONNumber("0"), []byte("0"), false},
		{"1", JSONNumber("1"), []byte("1"), false},
		{"2", JSONNumber("12341234.43214123"), []byte("12341234.43214123"), false},
		{"3", JSONNumber("123E55"), []byte("1.23e+57"), false},
		{"NaN", JSONNumber("dsfafdasf"), []byte("\"dsfafdasf\""), false},
		{"empty", JSONNumber(""), []byte("\"\""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONNumber.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JSONNumber.MarshalJSON() = %v, want %v", string(got), string(tt.want))
			}
		})
	}
}

func TestJSONNumber_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		c       *JSONNumber
		d       []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.UnmarshalJSON(tt.d); (err != nil) != tt.wantErr {
				t.Errorf("JSONNumber.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
