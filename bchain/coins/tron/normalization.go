package tron

import "strings"

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
	return "0x" + strip0xPrefix(s)
}
