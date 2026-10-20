package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrBlockedHost       = errors.New("url points to a blocked or restricted host")
	ErrBlockedIP         = errors.New("url resolves to a private, loopback, or cloud metadata address")
	ErrInvalidScheme     = errors.New("url scheme must be http or https")
	ErrCredentialsInURL = errors.New("credentials in url are not permitted")
	ErrRedirectBlocked   = errors.New("redirect to blocked address was prevented")
)

// ValidateURL checks if a raw URL is safe from SSRF attacks.
func ValidateURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidScheme
	}

	if parsed.User != nil {
		return ErrCredentialsInURL
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return fmt.Errorf("hostname is missing in URL")
	}

	// Check blocked hostname keywords
	if hostname == "localhost" ||
		strings.HasSuffix(hostname, ".localhost") ||
		hostname == "metadata.google.internal" ||
		hostname == "instance-data" ||
		hostname == "metadata" {
		return ErrBlockedHost
	}

	// Resolve hostname to IP addresses
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %s: %w", hostname, err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("hostname %s resolved to no addresses", hostname)
	}

	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return ErrBlockedIP
		}
	}

	return nil
}

// IsBlockedIP checks if an IP is loopback, private, link-local, multicast, or cloud metadata.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}

	// Explicit check for AWS/GCP/Azure link-local metadata IP 169.254.169.254
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}

	// IPv4 mapped IPv6 e.g. ::ffff:127.0.0.1
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 127 ||
			ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168) ||
			(ip4[0] == 169 && ip4[1] == 254) ||
			ip4[0] == 0 {
			return true
		}
	}

	return false
}

// NewSafeHTTPClient creates an http.Client with proactive SSRF defense at both
// DNS resolution, TCP dial, and HTTP redirect stages.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Validate IP at connection time to defeat DNS rebinding attacks
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			var safeIP net.IP
			for _, ip := range ips {
				if !IsBlockedIP(ip) {
					safeIP = ip
					break
				}
			}

			if safeIP == nil {
				return nil, ErrBlockedIP
			}

			safeAddr := net.JoinHostPort(safeIP.String(), port)
			return dialer.DialContext(ctx, network, safeAddr)
		},
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			// Verify target of redirect is safe
			if err := ValidateURL(req.Context(), req.URL.String()); err != nil {
				return fmt.Errorf("%w: %v", ErrRedirectBlocked, err)
			}
			return nil
		},
	}
}
