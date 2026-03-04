package common

import "testing"

func TestLookupEnvWithArchiveFallback_PrefersExactAlias(t *testing.T) {
	const prefix = "BB_TEST_URL_"
	t.Setenv(prefix+"base", "https://base")
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := LookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected env lookup to succeed")
	}
	if got != "https://base" {
		t.Fatalf("unexpected value: got %q, want %q", got, "https://base")
	}
}

func TestLookupEnvWithArchiveFallback_UsesArchiveFallback(t *testing.T) {
	const prefix = "BB_TEST_URL_"
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := LookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected archive fallback to succeed")
	}
	if got != "https://base-archive" {
		t.Fatalf("unexpected value: got %q, want %q", got, "https://base-archive")
	}
}

func TestLookupEnvWithArchiveFallback_NoDoubleArchiveSuffix(t *testing.T) {
	const prefix = "BB_TEST_URL_"
	t.Setenv(prefix+"base_archive_archive", "https://invalid")

	if _, ok := LookupEnvWithArchiveFallback(prefix, "base_archive"); ok {
		t.Fatal("unexpected lookup success for alias_archive_archive fallback")
	}
}
