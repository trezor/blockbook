package api

import "strings"

// Text contains static overridable texts used in explorer and API.
var Text struct {
	BlockbookAbout, TOSLink string
}

// SetBranding sets BlockbookAbout and TOSLink from branding loaded at process startup.
// Callers must pass values already validated by common.LoadBranding (about non-empty, tos_link a valid absolute URL).
func SetBranding(about, tosLink string) {
	Text.BlockbookAbout = strings.TrimSpace(about)
	Text.TOSLink = strings.TrimSpace(tosLink)
}
