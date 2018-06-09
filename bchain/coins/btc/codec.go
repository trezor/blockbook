package btc

import (
	"encoding/json"
	"errors"
	"reflect"
)

type RPCMarshaler interface {
	Marshal(v interface{}) ([]byte, error)
}

type JSONMarshalerV2 struct{}

func (JSONMarshalerV2) Marshal(v interface{}) ([]byte, error) {
	d, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return d, nil
}

var InvalidValue = errors.New("Invalid value to marshal")

type JSONMarshalerV1 struct{}

func (JSONMarshalerV1) Marshal(v interface{}) ([]byte, error) {
	u := cmdUntypedParams{}

	switch v := v.(type) {
	case *CmdGetBlock:
		var t bool
		if v.Params.Verbosity > 0 {
			t = true
		}
		u.Method = v.Method
		u.Params = append(u.Params, v.Params.BlockHash)
		u.Params = append(u.Params, t)
	case *CmdGetRawTransaction:
		var n int
		if v.Params.Verbose {
			n = 1
		}
		u.Method = v.Method
		u.Params = append(u.Params, v.Params.Txid)
		u.Params = append(u.Params, n)
	default:
		{
			v := reflect.ValueOf(v).Elem()

			f := v.FieldByName("Method")
			if !f.IsValid() || f.Kind() != reflect.String {
				return nil, InvalidValue
			}
			u.Method = f.String()

			f = v.FieldByName("Params")
			if f.IsValid() {
				arr := make([]interface{}, f.NumField())
				for i := 0; i < f.NumField(); i++ {
					arr[i] = f.Field(i).Interface()
				}
				u.Params = arr
			}
		}
	}

	d, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	return d, nil
}

type cmdUntypedParams struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params,omitempty"`
}
