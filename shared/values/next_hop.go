package values

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// NextHop is a specified address, not a Core identity or a route lookup key.
type NextHop string

// IsLocal restricts an in-process adapter to its own declared address profile.
func (value NextHop) IsLocal() bool {
	return strings.HasPrefix(string(value), "local:") && value.Valid()
}

// Valid reports whether the address exactly matches a declared local or TLS profile.
// Names contain 1-63 lowercase ASCII letters, digits, or hyphens, and begin
// with a letter or digit. Validation never normalizes or resolves aliases.
func (value NextHop) Valid() bool {
	if strings.HasPrefix(string(value), "https:") {
		_, ok := value.NetworkTarget()
		return ok
	}
	const prefix = "local:"
	text := string(value)
	if len(text) < len(prefix)+1 || len(text) > len(prefix)+63 || text[:len(prefix)] != prefix {
		return false
	}
	for index, character := range []byte(text[len(prefix):]) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && character == '-' {
			continue
		}
		return false
	}
	return true
}

// NetworkTarget validates the exact https://host:port representation and returns
// host:port. It never resolves, normalizes, follows hints, or assigns an identity.
func (value NextHop) NetworkTarget() (string, bool) {
	text := string(value)
	u, err := url.Parse(text)
	if err != nil || len(text) > 280 || u.Scheme != "https" || u.Opaque != "" || u.User != nil ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(text, "%\\ \t\r\n#?") || text != "https://"+u.Host {
		return "", false
	}
	port := u.Port()
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != port {
		return "", false
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" || ip.String() != host {
			return "", false
		}
		expected := host + ":" + port
		if ip.Is6() {
			expected = "[" + host + "]:" + port
		}
		return u.Host, u.Host == expected
	}
	if len(host) == 0 || len(host) > 253 || strings.HasSuffix(host, ".") || u.Host != host+":"+port {
		return "", false
	}
	allNumeric := true
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", false
			}
			if c < '0' || c > '9' {
				allNumeric = false
			}
		}
	}
	// Numeric dotted aliases must be canonical IPs, not resolver-dependent names.
	return u.Host, !allNumeric
}
