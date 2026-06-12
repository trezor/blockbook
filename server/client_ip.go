package server

import (
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// embeddedCloudflareIPs holds Cloudflare's published edge ranges, kept in a
// plain text file (one CIDR per line, #-comments) so the list can be updated
// without touching Go code, and compiled in so a deployment needs no extra
// runtime file. When Cloudflare peer verification is enabled the
// CF-Connecting-* headers are trusted only when the TCP peer falls inside one
// of these ranges (or is a loopback/private proxy fronting Cloudflare).
// Operators can override the list via <NET>_CLOUDFLARE_IPS (inline CIDRs or
// @/path/to/file) if it drifts.
//
//go:embed cloudflare_ips.txt
var embeddedCloudflareIPs string

type clientIPConfig struct {
	trustedProxies     []netip.Prefix
	cloudflarePrefixes []netip.Prefix
	// trustPseudoIPv6 honors the CF-Connecting-IPv6 request header. It must be
	// enabled ONLY when the Cloudflare zone runs "Pseudo IPv4: Overwrite
	// Headers", because that is the only mode in which Cloudflare sets (and thus
	// sanitizes) CF-Connecting-IPv6. In every other mode Cloudflare forwards a
	// client-supplied CF-Connecting-IPv6 verbatim, so trusting it would let any
	// client spoof its attributed IP. Default false.
	trustPseudoIPv6   bool
	trustedEnvName    string
	cloudflareEnvName string
	pseudoIPv6EnvName string
}

func readClientIPConfig(network string) (clientIPConfig, error) {
	prefix := strings.ToUpper(network)
	cfg := clientIPConfig{}

	envName, value := lookupEnvWithFallback(prefix+"_TRUSTED_PROXIES", prefix+"_WS_TRUSTED_PROXIES")
	cfg.trustedEnvName = envName
	trusted, err := parseTrustedProxies(envName, value)
	if err != nil {
		return cfg, err
	}
	cfg.trustedProxies = trusted

	envName, value = lookupEnvWithFallback(prefix+"_CLOUDFLARE_IPS", prefix+"_WS_CLOUDFLARE_IPS")
	cfg.cloudflareEnvName = envName
	cloudflare, err := parseCloudflareProxies(envName, value)
	if err != nil {
		return cfg, err
	}
	cfg.cloudflarePrefixes = cloudflare

	cfg.pseudoIPv6EnvName = prefix + "_CLOUDFLARE_PSEUDO_IPV4"
	trustPseudo, err := parseBoolEnv(cfg.pseudoIPv6EnvName, os.Getenv(cfg.pseudoIPv6EnvName))
	if err != nil {
		return cfg, err
	}
	cfg.trustPseudoIPv6 = trustPseudo
	return cfg, nil
}

// parseBoolEnv parses an optional boolean env value, defaulting to false when
// unset/empty and failing fast on an unparseable value so a typo cannot be
// silently ignored.
func parseBoolEnv(envName, value string) (bool, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: invalid value %q (want a boolean, e.g. \"true\" or \"false\")", envName, value)
	}
	return b, nil
}

func lookupEnvWithFallback(primary, fallback string) (envName, value string) {
	if v, ok := os.LookupEnv(primary); ok {
		return primary, v
	}
	if v, ok := os.LookupEnv(fallback); ok {
		return fallback, v
	}
	return primary, ""
}

// parseTrustedProxies parses a comma-separated list of CIDRs that augment the
// loopback/RFC1918/link-local defaults for trusting X-Real-Ip. Any prefix
// broad enough to cover meaningful chunks of the public internet is rejected
// with an error so misconfiguration fails fast at startup rather than
// silently turning X-Real-Ip into an IP-spoofing primitive.
func parseTrustedProxies(envName, value string) ([]netip.Prefix, error) {
	const minIPv4Bits = 8
	const minIPv6Bits = 16
	prefixes, err := parseCIDRList(envName, strings.Split(value, ","))
	if err != nil {
		return nil, err
	}
	for _, p := range prefixes {
		bits := p.Bits()
		if p.Addr().Is4() && bits < minIPv4Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv4)", envName, p, bits, minIPv4Bits)
		}
		if p.Addr().Is6() && bits < minIPv6Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv6)", envName, p, bits, minIPv6Bits)
		}
	}
	return prefixes, nil
}

// parseCloudflareProxies parses the <NET>_CLOUDFLARE_IPS env value (or the
// legacy <NET>_WS_CLOUDFLARE_IPS fallback) used to gate trust of the
// CF-Connecting-* headers. Recognized values:
//
//	""            (unset)  -> built-in Cloudflare edge ranges (verification on)
//	"builtin"              -> built-in Cloudflare edge ranges (verification on)
//	"off" / "none" / "0"   -> disabled; CF headers are trusted from any peer
//	                          (legacy behavior, intended for an origin firewalled
//	                          to Cloudflare ranges out of band)
//	"@/path/to/file"       -> load CIDRs from the file (one per line, blank lines
//	                          and #-comments ignored) instead of the built-in list
//	"<cidr>,<cidr>,..."    -> use these CIDRs instead of the built-in list
//
// A non-empty result means verification is enabled and resolveClientIP trusts
// the CF headers only when the TCP peer is inside one of the prefixes (or a
// loopback/private proxy fronting Cloudflare). Returning nil disables it; only
// the explicit "off" spellings do that -- a custom value or file that parses to
// no CIDRs is rejected so a typo cannot silently disable verification.
func parseCloudflareProxies(envName, value string) ([]netip.Prefix, error) {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "", "builtin", "default":
		return parseCIDRSource(envName, "embedded cloudflare_ips.txt", embeddedCloudflareIPs)
	case "off", "none", "false", "0", "disabled":
		return nil, nil
	}
	if path, ok := strings.CutPrefix(trimmed, "@"); ok {
		content, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return nil, fmt.Errorf("%s: cannot read CIDR file: %w", envName, err)
		}
		return parseCIDRSource(envName, fmt.Sprintf("file %q", path), string(content))
	}
	return parseCIDRSource(envName, fmt.Sprintf("%q", value), trimmed)
}

// parseCIDRSource parses CIDR-list content (inline env value or file contents)
// and rejects an empty result so a typo cannot silently disable verification;
// source names the origin of the content for the error message.
func parseCIDRSource(envName, source, content string) ([]netip.Prefix, error) {
	prefixes, err := parseCIDRList(envName, splitCIDRList(content))
	if err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("%s: no CIDRs in %s; use \"builtin\", \"off\", \"@/path/to/file\", or a comma-separated CIDR list", envName, source)
	}
	return prefixes, nil
}

// splitCIDRList splits CIDR-list content into raw items: commas and newlines
// both separate entries, and everything from # to the end of a line is a
// comment. Blank items are skipped later by parseCIDRList.
func splitCIDRList(content string) []string {
	var raws []string
	for _, line := range strings.Split(content, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		raws = append(raws, strings.Split(line, ",")...)
	}
	return raws
}

// parseCIDRList parses CIDRs into masked prefixes, skipping blanks and rejecting
// IPv4-mapped notation. It applies no minimum-width check (Cloudflare's
// published ranges are intentionally wide and only ever matched against the TCP
// peer); parseTrustedProxies layers that check on top for trusted proxies.
func parseCIDRList(envName string, raws []string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid CIDR %q: %w", envName, raw, err)
		}
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("%s: refusing IPv4-mapped CIDR %q; use IPv4 CIDR notation", envName, raw)
		}
		prefixes = append(prefixes, p.Masked())
	}
	return prefixes, nil
}

// resolveClientIP returns the per-IP rate-limit address for the request,
// whether that attribution is trustworthy enough to add to an IP blocklist
// (blockSafe), and whether it came from a forwarding header rather than the
// bare TCP peer (fromHeader). trustedProxies governs X-Real-Ip;
// cloudflareProxies governs CF-Connecting-* (empty disables verification and
// trusts those headers from any peer, the legacy behavior). When neither
// header is trusted for this peer it falls back to the bare TCP peer address.
//
// Only CF-Connecting-IP is trusted by default: it is the single CF-* request
// header Cloudflare always overwrites with the verified visitor IP. Cloudflare
// forwards a client-supplied CF-Connecting-IPv6 verbatim unless the zone runs
// "Pseudo IPv4: Overwrite Headers", so CF-Connecting-IPv6 is honored only when
// the operator opts in with trustPseudoIPv6 (which asserts that mode is on).
// In Pseudo-IPv4 mode CF-Connecting-IP holds a synthetic pseudo-IPv4 and the
// real client is in CF-Connecting-IPv6, so trustPseudoIPv6 also makes
// CF-Connecting-IPv6 the preferred source.
//
// blockSafe centralizes the spoof-protection decision so callers never have to
// re-inspect headers: a CF-Connecting-* value is block-safe only when peer
// verification is enabled (otherwise it is forgeable); X-Real-Ip is block-safe
// because it is only honored from a verified trusted proxy; the bare TCP peer is
// block-safe unless the request also carried a CF-Connecting-* header we did not
// trust (a spoof attempt, or a real but unrecognized Cloudflare edge -- blocking
// the peer would be wrong in both cases).
//
// fromHeader lets callers recognize degenerate attribution: when it is false
// and the address is the operator's own loopback/LAN/trusted proxy, the key
// identifies shared infrastructure (or the operator's own tooling), not a
// client.
func resolveClientIP(r *http.Request, trustedProxies, cloudflareProxies []netip.Prefix, trustPseudoIPv6 bool) (ip string, blockSafe, fromHeader bool) {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	remote, remoteOK := parseAddr(host)

	// Default Cloudflare mode (no configured trusted proxies). Trust the
	// CF-Connecting-* headers either from any peer (verification disabled) or
	// only when the TCP peer is a published Cloudflare edge range or a
	// loopback/private proxy fronting Cloudflare (verification enabled). For a
	// direct public non-Cloudflare peer the headers are attacker-controlled and
	// are ignored so they cannot spoof a client IP past the limiter or blocklist.
	if len(trustedProxies) == 0 {
		cfTrusted := len(cloudflareProxies) == 0 || (remoteOK && isTrustedProxy(remote, cloudflareProxies))
		if cfTrusted {
			cfBlockSafe := len(cloudflareProxies) > 0
			// Pseudo-IPv4 Overwrite mode (opt-in): the real client is the
			// Cloudflare-set CF-Connecting-IPv6; CF-Connecting-IP is a synthetic
			// pseudo-IPv4. Prefer the IPv6 header, falling back to CF-Connecting-IP.
			if trustPseudoIPv6 {
				if ip, ok := parseIP(r.Header.Get("CF-Connecting-IPv6")); ok {
					return ip, cfBlockSafe, true
				}
			}
			// Default: CF-Connecting-IP is the only CF-* request header Cloudflare
			// always overwrites with the verified client IP. CF-Connecting-IPv6 is
			// not consulted here because Cloudflare forwards a client-supplied value
			// verbatim outside Pseudo-IPv4 mode, making it spoofable.
			if ip, ok := parseIP(r.Header.Get("CF-Connecting-IP")); ok {
				return ip, cfBlockSafe, true
			}
		}
	}

	// Trust X-Real-Ip only when the TCP peer is on a private/loopback network
	// (an upstream proxy on the same host or LAN) or in a configured trusted
	// CIDR. For direct internet peers the header is attacker-controlled and
	// would let any client spoof their IP past the per-IP rate limiter.
	if remoteOK && isTrustedProxy(remote, trustedProxies) {
		if ip, ok := parseIP(r.Header.Get("X-Real-Ip")); ok {
			return ip, true, true
		}
	}

	hadCFHeader := r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("CF-Connecting-IPv6") != ""
	if remoteOK {
		return remote.String(), !hadCFHeader, false
	}
	return strings.TrimSpace(r.RemoteAddr), !hadCFHeader, false
}

// rateLimitKey returns the key used for per-IP limiting and blocklists. IPv6 is
// aggregated to its /64 because a single client is routinely delegated a whole
// /64, so keying on the full /128 would let it evade limits by rotating the low
// 64 bits across genuine addresses. IPv4 is keyed verbatim (IPv4-mapped IPv6 is
// unmapped to its IPv4 form first, so both notations share a key); anything
// unparseable is keyed verbatim.
func rateLimitKey(ip string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return ip
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is6() {
		if p, err := addr.Prefix(64); err == nil {
			return p.String()
		}
	}
	return addr.String()
}

// blockKey returns the key used for the temporary IP blocklists (WS and REST).
// Unlike rateLimitKey it keeps IPv6 at the full /128. Rate limiting aggregates
// to /64 (so a client cannot evade limits by rotating the low 64 bits of an
// owned /64), but a long-lived hard block must not take out an entire shared
// /64: mobile carriers, CGNAT-style IPv6, and VPN exits routinely place many
// unrelated subscribers in one /64, and blocking all of them for hours because
// one address misbehaved is collateral denial of service. The connection /
// request rate limiter still aggregates to /64, so a /64-rotating abuser cannot
// gain throughput by dodging the per-/128 block. IPv4 is keyed verbatim (so
// blockKey == rateLimitKey for IPv4); IPv4-mapped IPv6 is unmapped first, and
// anything unparseable is keyed verbatim.
func blockKey(ip string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return ip
	}
	return addr.Unmap().WithZone("").String()
}

// isBlockableKey reports whether ip is safe to add to an IP blocklist. It
// refuses loopback/private/link-local addresses and any configured trusted-proxy
// or Cloudflare edge range, so a misconfiguration that collapses many clients
// onto a shared proxy/edge address (or the proxy itself) can never get that
// shared address -- and therefore every client behind it -- blocked.
func isBlockableKey(ip string, trustedProxies, cloudflareProxies []netip.Prefix) bool {
	addr, ok := parseAddr(ip)
	if !ok {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	for _, p := range trustedProxies {
		if p.Contains(addr) {
			return false
		}
	}
	for _, p := range cloudflareProxies {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// isLocalOrTrustedProxyIP reports whether ip is a loopback/private/link-local
// address or falls inside a configured trusted-proxy range -- i.e. it names the
// operator's own infrastructure rather than a client. Used together with
// resolveClientIP's fromHeader to recognize degenerate attribution (a proxy
// that forwards no client IP, or the operator's own local tooling).
func isLocalOrTrustedProxyIP(ip string, trustedProxies []netip.Prefix) bool {
	addr, ok := parseAddr(ip)
	return ok && isTrustedProxy(addr, trustedProxies)
}

func parseIP(value string) (string, bool) {
	addr, ok := parseAddr(value)
	if !ok {
		return "", false
	}
	return addr.String(), true
}

func parseAddr(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	// Unmap IPv4-mapped IPv6 (::ffff:a.b.c.d -> a.b.c.d) so both notations
	// share one rate-limit key and IPv4 prefixes match in isTrustedProxy and
	// isBlockableKey, and strip the IPv6 zone identifier so that rate-limit keys
	// are zone-free and netip.Prefix.Contains matches unzoned prefixes against
	// link-local peers.
	return addr.Unmap().WithZone(""), true
}

// isTrustedProxy reports whether a forwarding header (X-Real-Ip, or
// CF-Connecting-* in the default Cloudflare branch) may be trusted from this
// TCP peer. Loopback and RFC1918/private peers are trusted implicitly as a
// reverse proxy on the same host or LAN; this assumes the private segment in
// front of Blockbook is not attacker-reachable. Link-local peers (fe80::/10)
// are deliberately NOT implicitly trusted: a link-local address is reachable
// and spoofable by any node on the same link and is never a deliberate
// proxy-placement choice, so trusting it would let an on-link attacker forge
// X-Real-Ip. An operator who genuinely fronts Blockbook with a link-local
// proxy can still trust it by listing its prefix in <NET>_TRUSTED_PROXIES
// (matched via extras).
func isTrustedProxy(addr netip.Addr, extras []netip.Prefix) bool {
	if addr.IsLoopback() || addr.IsPrivate() {
		return true
	}
	for _, p := range extras {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
