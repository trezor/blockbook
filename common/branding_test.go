package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// trezorFooterJSON returns a full Trezor-style footer object for test fixtures.
func trezorFooterJSON() map[string]interface{} {
	return map[string]interface{}{
		"created_by_label": "Created by SatoshiLabs",
		"created_by_href":  "https://satoshilabs.com/",
		"terms_label":      "Terms of Use",
		"brand_label":      "Trezor",
		"brand_href":       "https://trezor.io/",
		"suite_label":      "Suite",
		"suite_href":       "https://trezor.io/trezor-suite",
		"support_label":    "Support",
		"support_href":     "https://trezor.io/support",
		"send_tx_label":    "Send Transaction",
		"send_tx_href":     "/sendtx",
		"promo_label":      "Don't have a Trezor? Get one!",
		"promo_href":       "https://trezor.io/compare",
	}
}

func mergeBranding(br map[string]interface{}) map[string]interface{} {
	br["footer"] = trezorFooterJSON()
	return br
}

func TestLoadBrandingDefaults(t *testing.T) {
	dir := t.TempDir()
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "Trezor",
		"about":                 "Blockbook - blockchain indexer for Trezor Suite https://trezor.io/trezor-suite. Do not use for any other purpose.",
		"tos_link":              "https://trezor.io/terms-of-use",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "Exchange rates provided by Coingecko.",
		"logo_url":              "/static/img/logo.svg",
		"favicon_url":           "/static/favicon.ico",
		"logo_width_px":         128,
		"logo_right_padding_px": 140,
	})
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"version":  "0.6.0",
		"branding": br,
	})
	b, err := LoadBranding(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.BrandName != "Trezor" {
		t.Fatalf("BrandName = %q", b.BrandName)
	}
	if b.TOSLink != "https://trezor.io/terms-of-use" {
		t.Fatalf("TOSLink = %q", b.TOSLink)
	}
	if b.Footer.TermsLabel != "Terms of Use" {
		t.Fatalf("Footer.TermsLabel = %q", b.Footer.TermsLabel)
	}
}

func TestLoadBrandingOverride(t *testing.T) {
	dir := t.TempDir()
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "Trezor",
		"about":                 "about-default",
		"tos_link":              "https://trezor.io/terms-of-use",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "credit-default",
		"logo_url":              "/static/img/logo.svg",
		"favicon_url":           "/static/favicon.ico",
		"logo_width_px":         128,
		"logo_right_padding_px": 140,
	})
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"version":  "0.6.0",
		"branding": br,
	})
	override := map[string]interface{}{
		"branding": map[string]interface{}{
			"brand_name":        "Edge",
			"about":             "about-override",
			"fiat_rates_credit": "",
		},
	}
	writeJSON(t, filepath.Join(dir, "environ.overrides.json"), override)

	b, err := LoadBranding(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.BrandName != "Edge" {
		t.Fatalf("BrandName = %q", b.BrandName)
	}
	if b.About != "about-override" {
		t.Fatalf("About = %q", b.About)
	}
	if b.FiatRatesCredit != "" {
		t.Fatalf("FiatRatesCredit = %q", b.FiatRatesCredit)
	}
	if b.TOSLink != "https://trezor.io/terms-of-use" {
		t.Fatalf("TOSLink should remain from base: %q", b.TOSLink)
	}
}

func TestLoadBrandingMissingFile(t *testing.T) {
	_, err := LoadBranding(filepath.Join(t.TempDir(), "nonexistent-subdir"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadBrandingEmptyAbout(t *testing.T) {
	dir := t.TempDir()
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "X",
		"about":                 "   ",
		"tos_link":              "https://trezor.io/terms-of-use",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "",
		"logo_url":              "/x",
		"favicon_url":           "/f",
		"logo_width_px":         1,
		"logo_right_padding_px": 1,
	})
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"branding": br,
	})
	_, err := LoadBranding(dir)
	if err == nil {
		t.Fatal("expected error for empty about")
	}
}

func TestLoadBrandingOptionalFooterSlots(t *testing.T) {
	dir := t.TempDir()
	foot := trezorFooterJSON()
	foot["suite_label"] = ""
	foot["suite_href"] = ""
	foot["promo_label"] = ""
	foot["promo_href"] = ""
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "X",
		"about":                 "y",
		"tos_link":              "https://trezor.io/terms-of-use",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "",
		"logo_url":              "/x",
		"favicon_url":           "/f",
		"logo_width_px":         1,
		"logo_right_padding_px": 1,
	})
	br["footer"] = foot
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"version":  "0.6.0",
		"branding": br,
	})
	b, err := LoadBranding(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Footer.SuiteLabel != "" || b.Footer.PromoLabel != "" {
		t.Fatalf("expected empty suite/promo labels, got suite=%q promo=%q", b.Footer.SuiteLabel, b.Footer.PromoLabel)
	}
}

func TestLoadBrandingFooterPartialPairError(t *testing.T) {
	dir := t.TempDir()
	foot := trezorFooterJSON()
	foot["suite_href"] = ""
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "X",
		"about":                 "y",
		"tos_link":              "https://trezor.io/terms-of-use",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "",
		"logo_url":              "/x",
		"favicon_url":           "/f",
		"logo_width_px":         1,
		"logo_right_padding_px": 1,
	})
	br["footer"] = foot
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"version":  "0.6.0",
		"branding": br,
	})
	_, err := LoadBranding(dir)
	if err == nil {
		t.Fatal("expected error when suite_label is set but suite_href is empty")
	}
}

func TestLoadBrandingInvalidTOS(t *testing.T) {
	dir := t.TempDir()
	br := mergeBranding(map[string]interface{}{
		"brand_name":            "X",
		"about":                 "y",
		"tos_link":              "not-a-url",
		"github_repo_url":       "https://github.com/trezor/blockbook",
		"fiat_rates_credit":     "",
		"logo_url":              "/x",
		"favicon_url":           "/f",
		"logo_width_px":         1,
		"logo_right_padding_px": 1,
	})
	writeJSON(t, filepath.Join(dir, "environ.json"), map[string]interface{}{
		"branding": br,
	})
	_, err := LoadBranding(dir)
	if err == nil {
		t.Fatal("expected error for invalid tos_link")
	}
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
