package btc

import (
	"encoding/json"
	"errors"
	"reflect"
)

// RPCMarshaler is used for marshalling requests to Bitcoin Type RPC interfaces
type RPCMarshaler interface {
	Marshal(v interface{}) ([]byte, error)
}

// JSONMarshalerV2 is used for marshalling requests to newer Bitcoin Type RPC interfaces
type JSONMarshalerV2 struct{}

// Marshal converts struct passed by parameter to JSON
func (JSONMarshalerV2) Marshal(v interface{}) ([]byte, error) {
	d, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// ErrInvalidValue is error returned by JSONMarshalerV1.Marshal if the passed structure contains invalid value
var ErrInvalidValue = errors.New("Invalid value to marshal")

// JSONMarshalerV1 is used for marshalling requests to legacy Bitcoin Type RPC interfaces
type JSONMarshalerV1 struct{}

// Marshal converts struct passed by parameter to JSON
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
				return nil, ErrInvalidValue
			}
			u.Method = f.String()

			f = v.FieldByName("Params")
			if f.IsValid() {
				var arr []interface{}
				switch f.Kind() {
				case reflect.Slice:
					arr = make([]interface{}, f.Len())
					for i := 0; i < f.Len(); i++ {
						arr[i] = f.Index(i).Interface()
					}
				case reflect.Struct:
					arr = make([]interface{}, f.NumField())
					for i := 0; i < f.NumField(); i++ {
						arr[i] = f.Field(i).Interface()
					}
				default:
					return nil, ErrInvalidValue
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
