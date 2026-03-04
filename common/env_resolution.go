package common

import (
	"os"
	"strings"
)

const archiveSuffix = "_archive"

// LookupEnvWithArchiveFallback resolves env values for coin aliases using:
// 1) exact alias, 2) alias + "_archive" (only when alias is not already archive).
func LookupEnvWithArchiveFallback(prefix, alias string) (string, bool) {
	if alias == "" {
		return "", false
	}

	for _, candidate := range aliasCandidates(alias) {
		if value, ok := os.LookupEnv(prefix + candidate); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func aliasCandidates(alias string) []string {
	candidates := []string{alias}
	if !strings.HasSuffix(alias, archiveSuffix) {
		candidates = append(candidates, alias+archiveSuffix)
	}
	return candidates
}
