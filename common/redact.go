package common

import (
	"net/url"
	"regexp"
	"strings"
)

// urlInMessage matches an absolute URL anywhere in a message. The match stops at whitespace and at
// the quoting characters Go's http client puts around a URL, which is the shape that leaks the most
// often: Post "<url>": dial tcp ...
var urlInMessage = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://[^\s"'` + "`" + `<>]+`)

// RedactURLs reduces every URL in a message to its scheme and host. Backend and provider URLs carry
// an API key in their userinfo, path or query (a private relay, Infura, 1inch, CoinGecko), and they
// reach error messages both from our own annotations and from the http client, which renders the
// full URL into *url.Error on any dial failure or timeout. Apply this wherever a message is handed
// to an API client; logs keep the unredacted error, which is what an operator needs.
func RedactURLs(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlInMessage.ReplaceAllStringFunc(s, redactURL)
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// unparseable: drop it whole rather than keep a part that may be the key itself
		return "[redacted url]"
	}
	// Host excludes userinfo, so scheme://host drops the credentials along with the path and query.
	return u.Scheme + "://" + u.Host
}
