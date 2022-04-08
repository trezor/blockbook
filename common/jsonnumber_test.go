//go:build unittest

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
		{"NaN", JSONNumber("dsfafdasf"), []byte(`"dsfafdasf"`), false},
		{"empty", JSONNumber(""), []byte("0"), false},
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
		d       []byte
		want    JSONNumber
		wantErr bool
	}{
		{"0", []byte("0"), JSONNumber("0"), false},
		{"1", []byte("1"), JSONNumber("1"), false},
		{"1 quotes", []byte(`"1"`), JSONNumber("1"), false},
		{"2", []byte("12341234.43214123"), JSONNumber("12341234.43214123"), false},
		{"3", []byte("1.23e+57"), JSONNumber("1.23e+57"), false},
		{"NaN", []byte(`"dsfafdasf"`), JSONNumber("dsfafdasf"), false},
		{"empty", []byte(`""`), JSONNumber(""), false},
		{"really empty", []byte(""), JSONNumber(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got JSONNumber
			if err := got.UnmarshalJSON(tt.d); (err != nil) != tt.wantErr {
				t.Errorf("JSONNumber.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JSONNumber.UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
