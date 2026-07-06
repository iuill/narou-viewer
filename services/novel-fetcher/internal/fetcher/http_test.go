package fetcher

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestValidateRemoteURLRejectsLocalNetworkTargets(t *testing.T) {
	tests := []string{
		"http://localhost/image.png",
		"http://127.0.0.1/image.png",
		"http://10.0.0.1/image.png",
		"http://192.168.0.1/image.png",
		"http://[::1]/image.png",
		"file:///etc/passwd",
	}

	for _, test := range tests {
		if err := ValidateRemoteURL(test); err == nil {
			t.Fatalf("ValidateRemoteURL(%q) succeeded, want error", test)
		}
	}
}

func TestValidateRemoteURLAllowsPublicHTTPTargets(t *testing.T) {
	for _, test := range []string{"https://ncode.syosetu.com/n1234ab/", "https://kakuyomu.jp/works/123"} {
		if err := ValidateRemoteURL(test); err != nil {
			t.Fatalf("ValidateRemoteURL(%q) returned error: %v", test, err)
		}
	}
}

func TestValidatedDialAddressReturnsCheckedIP(t *testing.T) {
	address, err := validatedDialAddress(context.Background(), "8.8.8.8:443")
	if err != nil {
		t.Fatalf("validatedDialAddress returned error: %v", err)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("validated address should include host and port: %q", address)
	}
	if net.ParseIP(host) == nil {
		t.Fatalf("validated address host = %q, want IP literal", host)
	}
	if host != "8.8.8.8" {
		t.Fatalf("validated address host = %q, want checked IP", host)
	}
	if port != "443" {
		t.Fatalf("validated address port = %q, want 443", port)
	}
}

func TestValidatedDialAddressHandlesHostWithoutPort(t *testing.T) {
	address, err := validatedDialAddress(context.Background(), "8.8.4.4")
	if err != nil {
		t.Fatalf("validatedDialAddress returned error: %v", err)
	}
	if address != "8.8.4.4" {
		t.Fatalf("address = %q, want IP without port", address)
	}
}

func TestValidatedDialAddressRejectsPrivateIPv6Literal(t *testing.T) {
	if _, err := validatedDialAddress(context.Background(), net.JoinHostPort("::1", "80")); err == nil {
		t.Fatal("validatedDialAddress accepted loopback IPv6 address")
	}
}

func TestValidatedDialAddressRejectsPrivateIP(t *testing.T) {
	if _, err := validatedDialAddress(context.Background(), net.JoinHostPort("127.0.0.1", "80")); err == nil {
		t.Fatal("validatedDialAddress accepted loopback address")
	}
}

func TestJoinDialAddressHandlesIPv6(t *testing.T) {
	ip := net.ParseIP("2001:4860:4860::8888")
	address := joinDialAddress(ip, "443")
	if !strings.HasPrefix(address, "[") {
		t.Fatalf("IPv6 dial address was not bracketed: %q", address)
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("IPv6 dial address should parse: %v", err)
	}
	if portNumber, _ := strconv.Atoi(port); portNumber != 443 {
		t.Fatalf("port = %q, want 443", port)
	}
}
