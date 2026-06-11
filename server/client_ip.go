package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// cloudflareEdgeCIDRs are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/, fetched 2026-06). When Cloudflare peer
// verification is enabled the CF-Connecting-* headers are trusted only when the
// TCP peer falls inside one of these ranges (or is a loopback/private proxy
// fronting Cloudflare). Cloudflare changes these rarely; operators can override
// the list via env if it drifts.
var cloudflareEdgeCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

type clientIPConfig struct {
	trustedProxies     []netip.Prefix
	cloudflarePrefixes []netip.Prefix
	trustedEnvName     string
	cloudflareEnvName  string
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
	return cfg, nil
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
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	const minIPv4Bits = 8
	const minIPv6Bits = 16
	var prefixes []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
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
		bits := p.Bits()
		if p.Addr().Is4() && bits < minIPv4Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv4)", envName, raw, bits, minIPv4Bits)
		}
		if p.Addr().Is6() && !p.Addr().Is4In6() && bits < minIPv6Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv6)", envName, raw, bits, minIPv6Bits)
		}
		prefixes = append(prefixes, p.Masked())
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
//	"<cidr>,<cidr>,..."    -> use these CIDRs instead of the built-in list
//
// A non-empty result means verification is enabled and resolveClientIP trusts
// the CF headers only when the TCP peer is inside one of the prefixes (or a
// loopback/private proxy fronting Cloudflare). Returning nil disables it; only
// the explicit "off" spellings do that -- a custom value that parses to no
// CIDRs is rejected so a typo cannot silently disable verification.
func parseCloudflareProxies(envName, value string) ([]netip.Prefix, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "builtin", "default":
		return parseCIDRList(envName, cloudflareEdgeCIDRs)
	case "off", "none", "false", "0", "disabled":
		return nil, nil
	default:
		prefixes, err := parseCIDRList(envName, strings.Split(value, ","))
		if err != nil {
			return nil, err
		}
		if len(prefixes) == 0 {
			return nil, fmt.Errorf("%s: no CIDRs in %q; use \"builtin\", \"off\", or a comma-separated CIDR list", envName, value)
		}
		return prefixes, nil
	}
}

// parseCIDRList parses CIDRs into masked prefixes, skipping blanks and rejecting
// IPv4-mapped notation, mirroring parseTrustedProxies' validation (minus the
// minimum-width check, since Cloudflare's published ranges are intentionally
// wide and the resulting set is only ever matched against the TCP peer).
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

// resolveClientIP returns the per-IP rate-limit address for the request and
// whether that attribution is trustworthy enough to add to an IP blocklist
// (blockSafe). trustedProxies governs X-Real-Ip; cloudflareProxies governs
// CF-Connecting-* (empty disables verification and trusts those headers from any
// peer, the legacy behavior). When neither header is trusted for this peer it
// falls back to the bare TCP peer address.
//
// blockSafe centralizes the spoof-protection decision so callers never have to
// re-inspect headers: a CF-Connecting-* value is block-safe only when peer
// verification is enabled (otherwise it is forgeable); X-Real-Ip is block-safe
// because it is only honored from a verified trusted proxy; the bare TCP peer is
// block-safe unless the request also carried a CF-Connecting-* header we did not
// trust (a spoof attempt, or a real but unrecognized Cloudflare edge -- blocking
// the peer would be wrong in both cases).
func resolveClientIP(r *http.Request, trustedProxies, cloudflareProxies []netip.Prefix) (string, bool) {
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
			if ip, ok := parseIP(r.Header.Get("CF-Connecting-IPv6")); ok {
				return ip, cfBlockSafe
			}
			if ip, ok := parseIP(r.Header.Get("CF-Connecting-IP")); ok {
				return ip, cfBlockSafe
			}
		}
	}

	// Trust X-Real-Ip only when the TCP peer is on a private/loopback network
	// (an upstream proxy on the same host or LAN) or in a configured trusted
	// CIDR. For direct internet peers the header is attacker-controlled and
	// would let any client spoof their IP past the per-IP rate limiter.
	if remoteOK && isTrustedProxy(remote, trustedProxies) {
		if ip, ok := parseIP(r.Header.Get("X-Real-Ip")); ok {
			return ip, true
		}
	}

	hadCFHeader := r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("CF-Connecting-IPv6") != ""
	if remoteOK {
		return remote.String(), !hadCFHeader
	}
	return strings.TrimSpace(r.RemoteAddr), !hadCFHeader
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

func isTrustedProxy(addr netip.Addr, extras []netip.Prefix) bool {
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
		return true
	}
	for _, p := range extras {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
