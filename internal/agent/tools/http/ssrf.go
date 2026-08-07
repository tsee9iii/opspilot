package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// Policy controls which destinations the http.check tool may reach.
//
// Fail-closed rules (see validate):
//   - loopback, link-local (incl. cloud metadata), RFC1918 private, CGNAT,
//     unspecified, multicast and reserved ranges are DENIED by default;
//   - a destination is allowed when it matches an explicit allow rule
//     (AllowedEndpoints / AllowedHosts / AllowedCIDRs), or when AllowPrivate
//     is set for a restricted range;
//   - when any allowlist is configured, unlisted public destinations are also
//     denied, so configuring a policy tightens the tool to exactly that set;
//   - with no allowlist configured, public destinations remain reachable so
//     the health-check feature still works, but internal ranges never are.
type Policy struct {
	// AllowedEndpoints is an exact-URL allowlist.
	AllowedEndpoints []string
	// AllowedHosts is a hostname allowlist.
	AllowedHosts []string
	// AllowedCIDRs is a CIDR allowlist.
	AllowedCIDRs []string
	// AllowPrivate opts into restricted ranges (loopback, link-local, RFC1918).
	AllowPrivate bool
}

// target is a validated destination with the connection pinned to a single
// resolved IP, so DNS rebinding cannot redirect the request after validation.
type target struct {
	scheme     string
	hostHeader string
	ip         string
	port       string
}

// restrictedNetworks are the address ranges a health-check tool must never
// reach by default. They are the SSRF surface: the host itself, internal
// services, cloud metadata and network infrastructure.
var restrictedNetworks = func() []*net.IPNet {
	blocks := []string{
		"0.0.0.0/8",      // "this" network (unspecified)
		"10.0.0.0/8",     // RFC1918 private
		"100.64.0.0/10",  // carrier-grade NAT
		"127.0.0.0/8",    // loopback
		"169.254.0.0/16", // link-local (incl. 169.254.169.254 cloud metadata)
		"172.16.0.0/12",  // RFC1918 private
		"192.0.0.0/24",   // IETF protocol assignments
		"192.168.0.0/16", // RFC1918 private
		"198.18.0.0/15",  // benchmarking
		"224.0.0.0/4",    // multicast
		"240.0.0.0/4",    // reserved
		"::1/128",        // IPv6 loopback
		"::/128",         // IPv6 unspecified
		"fc00::/7",       // IPv6 unique local (RFC4193)
		"fe80::/10",      // IPv6 link-local
		"ff00::/8",       // IPv6 multicast
	}
	nets := make([]*net.IPNet, 0, len(blocks))
	for _, b := range blocks {
		if _, n, err := net.ParseCIDR(b); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

func isRestrictedIP(ip net.IP) bool {
	for _, n := range restrictedNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// validate enforces the destination policy against every resolved IP. host is
// the original hostname (used for allowlist matching), ips the addresses the
// hostname resolved to (or the single IP literal).
func (p Policy) validate(host string, ips []net.IP) error {
	allowlistConfigured := len(p.AllowedEndpoints) > 0 || len(p.AllowedHosts) > 0 || len(p.AllowedCIDRs) > 0

	for _, ip := range ips {
		restricted := isRestrictedIP(ip)
		explicitlyAllowed := p.ipListed(ip) || p.hostListed(host)
		switch {
		case restricted && p.AllowPrivate:
			// Operator explicitly opted into private ranges.
		case restricted && !explicitlyAllowed:
			return fmt.Errorf("address %s is in a restricted network range", ip)
		case !restricted && allowlistConfigured && !explicitlyAllowed:
			return fmt.Errorf("address %s is not in the configured allowlist", ip)
		}
	}
	return nil
}

// ipListed reports whether ip is covered by the CIDR allowlist.
func (p Policy) ipListed(ip net.IP) bool {
	for _, c := range p.AllowedCIDRs {
		if _, n, err := net.ParseCIDR(c); err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// hostListed reports whether host is an allowlisted hostname or the hostname
// of an allowlisted endpoint URL.
func (p Policy) hostListed(host string) bool {
	for _, h := range p.AllowedHosts {
		if host == h {
			return true
		}
	}
	for _, raw := range p.AllowedEndpoints {
		if u, err := url.Parse(raw); err == nil && u.Hostname() == host {
			return true
		}
	}
	return false
}

// validateTarget parses rawURL, applies the policy to every resolved address
// and returns a pinned target. The connection is later dialed to exactly this
// IP:port so a DNS rebinding race cannot bypass the policy.
func (t *HTTPCheckTool) validateTarget(ctx context.Context, raw string) (target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return target{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return target{}, fmt.Errorf("unsupported scheme %q: only http:// and https:// are allowed", u.Scheme)
	}
	if u.Host == "" {
		return target{}, errors.New("invalid URL: missing host")
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := t.policy.validate(host, []net.IP{ip}); err != nil {
			return target{}, err
		}
		return target{scheme: u.Scheme, hostHeader: host, ip: ip.String(), port: port}, nil
	}

	ips, err := t.resolveHosts(ctx, host)
	if err != nil {
		return target{}, fmt.Errorf("DNS lookup failed: %w", err)
	}
	if len(ips) == 0 {
		return target{}, errors.New("DNS lookup failed: no addresses resolved")
	}
	// Every resolved address must satisfy the policy; then the request is
	// pinned to the first validated one.
	if err := t.policy.validate(host, ips); err != nil {
		return target{}, err
	}
	return target{scheme: u.Scheme, hostHeader: host, ip: ips[0].String(), port: port}, nil
}

// resolveHosts resolves host to its IP addresses using the injected resolver
// (tests) or the system resolver.
func (t *HTTPCheckTool) resolveHosts(ctx context.Context, host string) ([]net.IP, error) {
	if t.resolveHost != nil {
		return t.resolveHost(ctx, host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}
