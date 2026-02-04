package common

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

// BrandingFooter holds explorer footer labels and hrefs.
// For each link except terms, leave label and href both empty to hide that slot (after trim).
// Terms uses tos_link; leave terms_label empty to hide the terms link.
type BrandingFooter struct {
	CreatedByLabel string `json:"created_by_label"`
	CreatedByHref  string `json:"created_by_href"`
	TermsLabel     string `json:"terms_label"`
	BrandLabel     string `json:"brand_label"`
	BrandHref      string `json:"brand_href"`
	SuiteLabel     string `json:"suite_label"`
	SuiteHref      string `json:"suite_href"`
	SupportLabel   string `json:"support_label"`
	SupportHref    string `json:"support_href"`
	SendTxLabel    string `json:"send_tx_label"`
	SendTxHref     string `json:"send_tx_href"`
	PromoLabel     string `json:"promo_label"`
	PromoHref      string `json:"promo_href"`
}

// Branding holds explorer and API branding strings loaded from configs/environ.json
// with optional overlay from configs/environ.overrides.json.
type Branding struct {
	BrandName          string         `json:"brand_name"`
	About              string         `json:"about"`
	TOSLink            string         `json:"tos_link"`
	GithubRepoURL      string         `json:"github_repo_url"`
	FiatRatesCredit    string         `json:"fiat_rates_credit"`
	LogoURL            string         `json:"logo_url"`
	FaviconURL         string         `json:"favicon_url"`
	LogoWidthPx        int            `json:"logo_width_px"`
	LogoRightPaddingPx int            `json:"logo_right_padding_px"`
	Footer             BrandingFooter `json:"footer"`
}

type environBrandingRoot struct {
	Branding *Branding `json:"branding"`
}

// brandingOverride uses pointers so empty strings and partial overrides are applied correctly.
type brandingOverride struct {
	BrandName          *string                 `json:"brand_name"`
	About              *string                 `json:"about"`
	TOSLink            *string                 `json:"tos_link"`
	GithubRepoURL      *string                 `json:"github_repo_url"`
	FiatRatesCredit    *string                 `json:"fiat_rates_credit"`
	LogoURL            *string                 `json:"logo_url"`
	FaviconURL         *string                 `json:"favicon_url"`
	LogoWidthPx        *int                    `json:"logo_width_px"`
	LogoRightPaddingPx *int                    `json:"logo_right_padding_px"`
	Footer             *footerBrandingOverride `json:"footer"`
}

type footerBrandingOverride struct {
	CreatedByLabel *string `json:"created_by_label"`
	CreatedByHref  *string `json:"created_by_href"`
	TermsLabel     *string `json:"terms_label"`
	BrandLabel     *string `json:"brand_label"`
	BrandHref      *string `json:"brand_href"`
	SuiteLabel     *string `json:"suite_label"`
	SuiteHref      *string `json:"suite_href"`
	SupportLabel   *string `json:"support_label"`
	SupportHref    *string `json:"support_href"`
	SendTxLabel    *string `json:"send_tx_label"`
	SendTxHref     *string `json:"send_tx_href"`
	PromoLabel     *string `json:"promo_label"`
	PromoHref      *string `json:"promo_href"`
}

// LoadBranding reads configs/environ.json (required branding block) and merges
// configs/environ.overrides.json when present. Returned branding has non-empty
// about and a valid absolute tos_link (ParseRequestURI).
func LoadBranding(configsDir string) (*Branding, error) {
	basePath := filepath.Join(configsDir, "environ.json")
	b, err := os.ReadFile(basePath)
	if err != nil {
		return nil, errors.Annotatef(err, "read %s", basePath)
	}
	var root environBrandingRoot
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, errors.Annotatef(err, "parse %s", basePath)
	}
	if root.Branding == nil {
		return nil, errors.Errorf("%s: missing branding", basePath)
	}
	out := cloneBranding(root.Branding)

	overridePath := filepath.Join(configsDir, "environ.overrides.json")
	if st, err := os.Stat(overridePath); err == nil && !st.IsDir() {
		ob, err := os.ReadFile(overridePath)
		if err != nil {
			return nil, errors.Annotatef(err, "read %s", overridePath)
		}
		var oroot struct {
			Branding *brandingOverride `json:"branding"`
		}
		if err := json.Unmarshal(ob, &oroot); err != nil {
			return nil, errors.Annotatef(err, "parse %s", overridePath)
		}
		if oroot.Branding != nil {
			applyBrandingOverride(out, oroot.Branding)
		}
	}

	trimBrandingStrings(out)

	if err := validateAbout(out.About); err != nil {
		return nil, err
	}
	if err := validateTOSLink(out.TOSLink); err != nil {
		return nil, err
	}
	if err := validateFooterNav(&out.Footer); err != nil {
		return nil, err
	}

	return out, nil
}

func trimBrandingStrings(b *Branding) {
	b.About = strings.TrimSpace(b.About)
	b.BrandName = strings.TrimSpace(b.BrandName)
	b.TOSLink = strings.TrimSpace(b.TOSLink)
	b.GithubRepoURL = strings.TrimSpace(b.GithubRepoURL)
	b.FiatRatesCredit = strings.TrimSpace(b.FiatRatesCredit)
	b.LogoURL = strings.TrimSpace(b.LogoURL)
	b.FaviconURL = strings.TrimSpace(b.FaviconURL)
	f := &b.Footer
	f.CreatedByLabel = strings.TrimSpace(f.CreatedByLabel)
	f.CreatedByHref = strings.TrimSpace(f.CreatedByHref)
	f.TermsLabel = strings.TrimSpace(f.TermsLabel)
	f.BrandLabel = strings.TrimSpace(f.BrandLabel)
	f.BrandHref = strings.TrimSpace(f.BrandHref)
	f.SuiteLabel = strings.TrimSpace(f.SuiteLabel)
	f.SuiteHref = strings.TrimSpace(f.SuiteHref)
	f.SupportLabel = strings.TrimSpace(f.SupportLabel)
	f.SupportHref = strings.TrimSpace(f.SupportHref)
	f.SendTxLabel = strings.TrimSpace(f.SendTxLabel)
	f.SendTxHref = strings.TrimSpace(f.SendTxHref)
	f.PromoLabel = strings.TrimSpace(f.PromoLabel)
	f.PromoHref = strings.TrimSpace(f.PromoHref)
}

func cloneBranding(src *Branding) *Branding {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func applyBrandingOverride(dst *Branding, o *brandingOverride) {
	if o.BrandName != nil {
		dst.BrandName = *o.BrandName
	}
	if o.About != nil {
		dst.About = *o.About
	}
	if o.TOSLink != nil {
		dst.TOSLink = *o.TOSLink
	}
	if o.GithubRepoURL != nil {
		dst.GithubRepoURL = *o.GithubRepoURL
	}
	if o.FiatRatesCredit != nil {
		dst.FiatRatesCredit = *o.FiatRatesCredit
	}
	if o.LogoURL != nil {
		dst.LogoURL = *o.LogoURL
	}
	if o.FaviconURL != nil {
		dst.FaviconURL = *o.FaviconURL
	}
	if o.LogoWidthPx != nil {
		dst.LogoWidthPx = *o.LogoWidthPx
	}
	if o.LogoRightPaddingPx != nil {
		dst.LogoRightPaddingPx = *o.LogoRightPaddingPx
	}
	if o.Footer != nil {
		applyFooterOverride(&dst.Footer, o.Footer)
	}
}

func applyFooterOverride(dst *BrandingFooter, o *footerBrandingOverride) {
	if o.CreatedByLabel != nil {
		dst.CreatedByLabel = *o.CreatedByLabel
	}
	if o.CreatedByHref != nil {
		dst.CreatedByHref = *o.CreatedByHref
	}
	if o.TermsLabel != nil {
		dst.TermsLabel = *o.TermsLabel
	}
	if o.BrandLabel != nil {
		dst.BrandLabel = *o.BrandLabel
	}
	if o.BrandHref != nil {
		dst.BrandHref = *o.BrandHref
	}
	if o.SuiteLabel != nil {
		dst.SuiteLabel = *o.SuiteLabel
	}
	if o.SuiteHref != nil {
		dst.SuiteHref = *o.SuiteHref
	}
	if o.SupportLabel != nil {
		dst.SupportLabel = *o.SupportLabel
	}
	if o.SupportHref != nil {
		dst.SupportHref = *o.SupportHref
	}
	if o.SendTxLabel != nil {
		dst.SendTxLabel = *o.SendTxLabel
	}
	if o.SendTxHref != nil {
		dst.SendTxHref = *o.SendTxHref
	}
	if o.PromoLabel != nil {
		dst.PromoLabel = *o.PromoLabel
	}
	if o.PromoHref != nil {
		dst.PromoHref = *o.PromoHref
	}
}

func validateAbout(about string) error {
	if about == "" {
		return errors.New("branding about is required")
	}
	return nil
}

func validateTOSLink(tos string) error {
	if tos == "" {
		return errors.New("branding tos_link is required")
	}
	if _, err := url.ParseRequestURI(tos); err != nil {
		return errors.Annotate(err, "branding tos_link is not a valid URL")
	}
	return nil
}

func validateFooterNavHref(field, h string) error {
	if h == "" {
		return errors.Errorf("branding %s is required", field)
	}
	if strings.HasPrefix(h, "/") && !strings.HasPrefix(h, "//") {
		return nil
	}
	if _, err := url.ParseRequestURI(h); err != nil {
		return errors.Annotatef(err, "branding %s is not a valid URL", field)
	}
	return nil
}

// validateOptionalFooterPair requires label and href to be both set or both empty.
// An empty pair omits that footer link in templates (overrides can hide slots).
func validateOptionalFooterPair(labelField, hrefField, label, href string) error {
	labelEmpty := label == ""
	hrefEmpty := href == ""
	if labelEmpty && hrefEmpty {
		return nil
	}
	if labelEmpty || hrefEmpty {
		return errors.Errorf("branding %s and %s must both be set or both empty", labelField, hrefField)
	}
	if err := validateFooterNavHref(hrefField, href); err != nil {
		return err
	}
	return nil
}

func validateFooterNav(f *BrandingFooter) error {
	pairs := []struct{ labelField, hrefField, label, href string }{
		{"footer.created_by_label", "footer.created_by_href", f.CreatedByLabel, f.CreatedByHref},
		{"footer.brand_label", "footer.brand_href", f.BrandLabel, f.BrandHref},
		{"footer.suite_label", "footer.suite_href", f.SuiteLabel, f.SuiteHref},
		{"footer.support_label", "footer.support_href", f.SupportLabel, f.SupportHref},
		{"footer.send_tx_label", "footer.send_tx_href", f.SendTxLabel, f.SendTxHref},
		{"footer.promo_label", "footer.promo_href", f.PromoLabel, f.PromoHref},
	}
	for _, p := range pairs {
		if err := validateOptionalFooterPair(p.labelField, p.hrefField, p.label, p.href); err != nil {
			return err
		}
	}
	// Terms row uses branding.tos_link; empty terms_label hides the link in templates.
	return nil
}
