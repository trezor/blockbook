package bchain

import (
	"encoding/hex"
	"reflect"
	"testing"
)

func TestAddressToOutputScript(t *testing.T) {
	type args struct {
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "P2PKH",
			args:    args{address: "1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			want:    "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac",
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{address: "321x69Cb9HZLWwAWGiUBT1U81r1zPLnEjL"},
			want:    "a9140394b3cf9a44782c10105b93962daa8dba304d7f87",
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{address: "bc1qrsf2l34jvqnq0lduyz0j5pfu2nkd93nnq0qggn"},
			want:    "00141c12afc6b2602607fdbc209f2a053c54ecd2c673",
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{address: "bc1qqwtn5s8vjnqdzrm0du885c46ypzt05vakmljhasx28shlv5a355sw5exgr"},
			want:    "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddressToOutputScript(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddressToOutputScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			h := hex.EncodeToString(got)
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("AddressToOutputScript() = %v, want %v", h, tt.want)
			}
		})
	}
}

func TestOutputScriptToAddresses(t *testing.T) {
	type args struct {
		script string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name:    "P2PKH",
			args:    args{script: "76a914be027bf3eac907bd4ac8cb9c5293b6f37662722088ac"},
			want:    []string{"1JKgN43B9SyLuZH19H5ECvr4KcfrbVHzZ6"},
			wantErr: false,
		},
		{
			name:    "P2SH",
			args:    args{script: "a9140394b3cf9a44782c10105b93962daa8dba304d7f87"},
			want:    []string{"321x69Cb9HZLWwAWGiUBT1U81r1zPLnEjL"},
			wantErr: false,
		},
		{
			name:    "P2WPKH",
			args:    args{script: "00141c12afc6b2602607fdbc209f2a053c54ecd2c673"},
			want:    []string{"bc1qrsf2l34jvqnq0lduyz0j5pfu2nkd93nnq0qggn"},
			wantErr: false,
		},
		{
			name:    "P2WSH",
			args:    args{script: "002003973a40ec94c0d10f6f6f0e7a62ba2044b7d19db6ff2bf60651e17fb29d8d29"},
			want:    []string{"bc1qqwtn5s8vjnqdzrm0du885c46ypzt05vakmljhasx28shlv5a355sw5exgr"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := hex.DecodeString(tt.args.script)
			got, err := OutputScriptToAddresses(b)
			if (err != nil) != tt.wantErr {
				t.Errorf("OutputScriptToAddresses() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OutputScriptToAddresses() = %v, want %v", got, tt.want)
			}
		})
	}
}
