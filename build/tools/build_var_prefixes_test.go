package build

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// canonicalPrefixFile is the single source of truth shared with the Makefile and
// .github/actions/export-env-vars/action.yml. Path is relative to this package
// directory (build/tools).
const canonicalPrefixFile = "../bb-build-var-prefixes.txt"

func loadCanonicalPrefixes(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(canonicalPrefixFile)
	if err != nil {
		t.Fatalf("open %s: %v", canonicalPrefixFile, err)
	}
	defer f.Close()

	set := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", canonicalPrefixFile, err)
	}
	return set
}

// TestCanonicalPrefixesCoverTemplateConsumers guards against drift between the
// shared bb-build-var-prefixes.txt and the build-time variable prefixes that
// templates.go relies on. If a prefix used here is removed or renamed in the
// canonical file (and thus dropped from the Makefile/action forwarding), this
// test fails instead of the breakage surfacing only at build time.
func TestCanonicalPrefixesCoverTemplateConsumers(t *testing.T) {
	canonical := loadCanonicalPrefixes(t)

	usedByTemplates := []string{
		devRPCURLHTTPPrefix,
		devRPCURLWSPrefix,
		devMQURLPrefix,
		prodRPCURLHTTPPrefix,
		prodRPCURLWSPrefix,
		prodMQURLPrefix,
		"BB_RPC_BIND_HOST_", // inline literal in LoadConfig
		"BB_RPC_ALLOW_IP_",  // inline literal in LoadConfig
	}

	for _, prefix := range usedByTemplates {
		if !canonical[prefix] {
			t.Errorf("prefix %q used by templates.go is missing from %s", prefix, canonicalPrefixFile)
		}
	}
}
