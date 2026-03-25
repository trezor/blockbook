package tron

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/trezor/blockbook/bchain"
)

const (
	tronResourceBandwidth tronResourceCode = 0
	tronResourceEnergy    tronResourceCode = 1
	tronResourceVotePower tronResourceCode = 2
)

func (c *tronResourceCode) UnmarshalJSON(data []byte) error {
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*c = tronResourceCode(n)
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "0", "BANDWIDTH":
		*c = tronResourceBandwidth
	case "1", "ENERGY":
		*c = tronResourceEnergy
	case "2", "VOTE_POWER", "VOTEPOWER", "TRON_POWER", "TRONPOWER":
		*c = tronResourceVotePower
	default:
		return fmt.Errorf("unknown Tron resource code %q", s)
	}
	return nil
}

func tronNumberToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

func has0xPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1]|32) == 'x'
}

func strip0xPrefix(s string) string {
	if has0xPrefix(s) {
		return s[2:]
	}
	return s
}

func normalizeHexString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if has0xPrefix(s) {
		return s
	}
	return "0x" + s
}

func tronResourceToString(v *tronResourceCode) string {
	if v == nil {
		return ""
	}
	switch *v {
	case tronResourceEnergy:
		return "energy"
	case tronResourceBandwidth:
		return "bandwidth"
	case tronResourceVotePower:
		return "votePower"
	default:
		return ""
	}
}

func tronInt64PtrToString(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func tronInt64PtrToHexQuantity(v *int64) string {
	if v == nil {
		return ""
	}
	n := big.NewInt(*v)
	if n.Sign() < 0 {
		return ""
	}
	return "0x" + n.Text(16)
}

func tronUint64(v interface{}) (uint64, bool) {
	s := strings.TrimSpace(tronNumberToString(v))
	if s == "" {
		return 0, false
	}
	n, ok := new(big.Int).SetString(s, 0)
	if !ok || n.Sign() < 0 || !n.IsUint64() {
		return 0, false
	}
	return n.Uint64(), true
}

func tronFirstAddress(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func tronFirstInt64PtrToString(values ...*int64) string {
	for _, v := range values {
		if s := tronInt64PtrToString(v); s != "" {
			return s
		}
	}
	return ""
}

func tronNormalizeLogs(logs []*bchain.RpcLog) []*bchain.RpcLog {
	for _, l := range logs {
		if l == nil {
			continue
		}
		l.Address = normalizeHexString(l.Address)
		l.Data = normalizeHexString(l.Data)
		for i, t := range l.Topics {
			l.Topics[i] = normalizeHexString(t)
		}
	}
	return logs
}
