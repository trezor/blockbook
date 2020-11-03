package common

import (
	"encoding/json"
	"strconv"
)

// JSONNumber is used instead of json.Number after upgrade to go 1.14
//  to handle data which can be numbers in double quotes or possibly not numbers at all
// see https://github.com/golang/go/issues/37308
type JSONNumber string

// Float64 returns JSONNumber as float64
func (c JSONNumber) Float64() (float64, error) {
	f, err := strconv.ParseFloat(string(c), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// Int64 returns JSONNumber as int64
func (c JSONNumber) Int64() (int64, error) {
	i, err := strconv.ParseInt(string(c), 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (c JSONNumber) String() string {
	return string(c)
}

// MarshalJSON marsalls JSONNumber to []byte
// if possible, return a number without quotes, otherwise string value in quotes
// empty string is treated as number 0
func (c JSONNumber) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte("0"), nil
	}
	if f, err := c.Float64(); err == nil {
		return json.Marshal(f)
	}
	return json.Marshal(string(c))
}

// UnmarshalJSON unmarshalls JSONNumber from []byte
// if the value is in quotes, remove them
func (c *JSONNumber) UnmarshalJSON(d []byte) error {
	s := string(d)
	l := len(s)
	if l > 1 && s[0] == '"' && s[l-1] == '"' {
		*c = JSONNumber(s[1 : l-1])
	} else {
		*c = JSONNumber(s)
	}
	return nil
}
