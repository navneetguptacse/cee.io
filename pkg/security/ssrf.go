package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var blockedCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"0.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, ipNet)
		}
	}
}

// ValidateCallbackURL ensures a webhook URL does not target private or local infrastructure (SSRF protection).
func ValidateCallbackURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid callback URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("callback URL scheme must be http or https")
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" ||
		strings.HasSuffix(hostname, ".internal") ||
		strings.HasSuffix(hostname, ".local") ||
		strings.HasSuffix(hostname, ".localhost") {
		return fmt.Errorf("callback URL points to prohibited internal hostname")
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		ip := net.ParseIP(hostname)
		if ip == nil {
			return fmt.Errorf("failed to resolve callback hostname: %w", err)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		for _, blockedNet := range blockedCIDRs {
			if blockedNet.Contains(ip) {
				return fmt.Errorf("callback URL resolves to prohibited IP (%s)", ip.String())
			}
		}
	}

	return nil
}
