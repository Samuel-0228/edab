package ssrf

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"nil ip", "", true},
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.0.0.2", "127.0.0.2", true},
		{"loopback ipv6 ::1", "::1", true},
		{"private 10.0.0.1", "10.0.0.1", true},
		{"private 10.254.0.1", "10.254.0.1", true},
		{"private 172.16.0.1", "172.16.0.1", true},
		{"private 172.31.255.255", "172.31.255.255", true},
		{"private 192.168.1.1", "192.168.1.1", true},
		{"aws metadata 169.254.169.254", "169.254.169.254", true},
		{"link local 169.254.1.1", "169.254.1.1", true},
		{"unspecified 0.0.0.0", "0.0.0.0", true},
		{"ipv4 mapped loopback", "::ffff:127.0.0.1", true},
		{"ipv4 mapped private 10", "::ffff:10.1.2.3", true},
		{"ipv4 mapped private 172", "::ffff:172.20.1.1", true},
		{"ipv4 mapped private 192", "::ffff:192.168.1.5", true},
		{"ipv4 mapped metadata", "::ffff:169.254.169.254", true},
		{"public google dns", "8.8.8.8", false},
		{"public cloudflare dns", "1.1.1.1", false},
		{"public opendns", "208.67.222.222", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parsed net.IP
			if tt.ip != "" {
				parsed = net.ParseIP(tt.ip)
			}
			actual := IsBlockedIP(parsed)
			if actual != tt.blocked {
				t.Errorf("IsBlockedIP(%q) = %v, expected %v", tt.ip, actual, tt.blocked)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"empty string", "", true},
		{"invalid url characters", "http:// invalid-host", true},
		{"missing scheme", "localhost:8080", true},
		{"unsupported scheme ftp", "ftp://example.com/file", true},
		{"unsupported scheme file", "file:///etc/passwd", true},
		{"unsupported scheme gopher", "gopher://evil.com/1", true},
		{"credentials in URL", "http://user:pass@8.8.8.8", true},
		{"empty hostname", "http:///path", true},
		{"localhost keyword", "http://localhost:8080/secret", true},
		{"subdomain localhost", "http://api.localhost/v1", true},
		{"cloud metadata hostname google", "http://metadata.google.internal/computeMetadata/v1", true},
		{"cloud metadata hostname generic", "http://metadata/v1", true},
		{"cloud metadata hostname instance-data", "http://instance-data/latest/meta-data", true},
		{"loopback IP directly", "http://127.0.0.1:9090", true},
		{"private 10 range directly", "http://10.0.0.1:8080", true},
		{"private 172 range directly", "http://172.16.5.10:3000", true},
		{"private 192 range directly", "http://192.168.1.100/admin", true},
		{"metadata IP directly", "http://169.254.169.254/latest/meta-data", true},
		{"unspecified IP directly", "http://0.0.0.0:8080", true},
		{"public IP allowed (offline safe)", "https://8.8.8.8/dns-query", false},
		{"public IP with custom port", "http://1.1.1.1:8080/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(ctx, tt.url)
			if (err != nil) != tt.shouldErr {
				t.Errorf("ValidateURL(%q) err = %v, shouldErr = %v", tt.url, err, tt.shouldErr)
			}
		})
	}
}

func TestNewSafeHTTPClient(t *testing.T) {
	// Test default and explicit timeout
	clientDefault := NewSafeHTTPClient(0)
	if clientDefault.Timeout != 15*time.Second {
		t.Errorf("expected 15s default timeout, got %v", clientDefault.Timeout)
	}

	clientCustom := NewSafeHTTPClient(5 * time.Second)
	if clientCustom.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", clientCustom.Timeout)
	}

	// Test redirect safety
	reqSafe, _ := http.NewRequest(http.MethodGet, "https://8.8.8.8", nil)
	reqUnsafe, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/internal", nil)

	// Safe redirect passes
	err := clientCustom.CheckRedirect(reqSafe, []*http.Request{reqSafe})
	if err != nil {
		t.Fatalf("expected safe redirect to be allowed, got: %v", err)
	}

	// Unsafe redirect blocked
	err = clientCustom.CheckRedirect(reqUnsafe, []*http.Request{reqSafe})
	if err == nil {
		t.Fatal("expected unsafe redirect to loopback to be blocked")
	}

	// Max redirects exceeded
	var via []*http.Request
	for i := 0; i < 5; i++ {
		r, _ := http.NewRequest(http.MethodGet, "https://8.8.8.8/r"+string(rune('1'+i)), nil)
		via = append(via, r)
	}
	err = clientCustom.CheckRedirect(reqSafe, via)
	if err == nil {
		t.Fatal("expected redirect error after 5 redirects")
	}

	// Test dialer rejects blocked IP
	transport, ok := clientCustom.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatal("expected custom transport with DialContext")
	}

	ctx := context.Background()
	_, err = transport.DialContext(ctx, "tcp", "127.0.0.1:9999")
	if err == nil {
		t.Fatal("expected DialContext to reject 127.0.0.1")
	}

	// Test invalid host:port format in DialContext
	_, err = transport.DialContext(ctx, "tcp", "invalid-no-port")
	if err == nil {
		t.Fatal("expected error on malformed addr in DialContext")
	}

	_ = url.URL{}
}
