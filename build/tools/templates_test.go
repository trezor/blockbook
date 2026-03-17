package build

import "testing"

func TestLookupEnvWithArchiveFallback_PrefersExactAlias(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"base", "https://base")
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected exact alias lookup to succeed")
	}
	if got != "https://base" {
		t.Fatalf("expected exact alias to win, got %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_UsesArchiveSuffixFallback(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected suffix archive fallback to succeed")
	}
	if got != "https://base-archive" {
		t.Fatalf("unexpected suffix fallback value %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_UsesArchiveInfixFallback(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"polygon_archive_bor", "https://polygon-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "polygon_bor")
	if !ok {
		t.Fatal("expected infix archive fallback to succeed")
	}
	if got != "https://polygon-archive" {
		t.Fatalf("unexpected infix fallback value %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_DoesNotDoubleArchive(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"polygon_archive_archive_bor", "https://invalid")
	t.Setenv(prefix+"polygon_archive_bor_archive", "https://invalid")

	if _, ok := lookupEnvWithArchiveFallback(prefix, "polygon_archive_bor"); ok {
		t.Fatal("unexpected lookup success for duplicate archive alias variants")
	}
}
