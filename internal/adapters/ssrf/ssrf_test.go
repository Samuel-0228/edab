package ssrf

import (
	"context"
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"10.254.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false}, // example.com
	}

	for _, tt := range tests {
		parsed := net.ParseIP(tt.ip)
		actual := IsBlockedIP(parsed)
		if actual != tt.blocked {
			t.Errorf("IsBlockedIP(%s) = %v, expected %v", tt.ip, actual, tt.blocked)
		}
	}
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"localhost blocked", "http://localhost:8080/secret", true},
		{"loopback IP blocked", "http://127.0.0.1:9090", true},
		{"private IP blocked", "http://192.168.1.100/admin", true},
		{"cloud metadata hostname blocked", "http://metadata.google.internal/computeMetadata/v1", true},
		{"invalid scheme blocked", "ftp://example.com/file", true},
		{"file scheme blocked", "file:///etc/passwd", true},
		{"credentials in URL blocked", "http://user:pass@example.com", true},
		{"public domain allowed", "https://httpbin.org/get", false},
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
