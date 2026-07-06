package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrRemoteBackoff = errors.New("remote server requested backoff")
var ErrUnsafeRemoteURL = errors.New("unsafe remote URL")

type HTTPFetcherOptions struct {
	UserAgent string
	Timeout   time.Duration
	Logger    *slog.Logger
}

type HTTPFetcher struct {
	client    *http.Client
	limiter   *HostRateLimiter
	userAgent string
	logger    *slog.Logger
}

type BinaryResponse struct {
	Bytes       []byte
	ContentType string
}

func NewHTTPFetcher(options HTTPFetcherOptions) *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{
			Timeout:       options.Timeout,
			Transport:     safeTransport(),
			CheckRedirect: validateRedirect,
		},
		limiter:   NewHostRateLimiter(),
		userAgent: options.UserAgent,
		logger:    options.Logger,
	}
}

func (f *HTTPFetcher) FetchText(ctx context.Context, rawURL string, policy FetchPolicy) (string, error) {
	var lastErr error
	attempts := policy.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if err := f.limiter.Wait(ctx, rawURL, policy); err != nil {
			return "", err
		}

		body, retryable, err := f.fetchTextOnce(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == attempts-1 {
			break
		}

		if f.logger != nil {
			f.logger.Warn("fetch failed; waiting before retry", "url", rawURL, "attempt", attempt+1, "error", err)
		}
		timer := time.NewTimer(policy.RetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}

	return "", lastErr
}

func (f *HTTPFetcher) FetchBytes(ctx context.Context, rawURL string, policy FetchPolicy) (BinaryResponse, error) {
	var lastErr error
	attempts := policy.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if err := f.limiter.Wait(ctx, rawURL, policy); err != nil {
			return BinaryResponse{}, err
		}

		body, retryable, err := f.fetchBytesOnce(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == attempts-1 {
			break
		}

		if f.logger != nil {
			f.logger.Warn("fetch failed; waiting before retry", "url", rawURL, "attempt", attempt+1, "error", err)
		}
		timer := time.NewTimer(policy.RetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return BinaryResponse{}, ctx.Err()
		case <-timer.C:
		}
	}

	return BinaryResponse{}, lastErr
}

func (f *HTTPFetcher) fetchTextOnce(ctx context.Context, rawURL string) (string, bool, error) {
	if err := ValidateRemoteURL(rawURL); err != nil {
		return "", false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false, err
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	request.Header.Set("Accept-Charset", "utf-8")
	request.Header.Set("Connection", "keep-alive")

	response, err := f.client.Do(request)
	if err != nil {
		return "", true, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusTooManyRequests {
		return "", false, fmt.Errorf("%w: %s returned HTTP %d", ErrRemoteBackoff, rawURL, response.StatusCode)
	}
	if response.StatusCode >= 500 {
		return "", true, fmt.Errorf("%s returned HTTP %d", rawURL, response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", false, fmt.Errorf("%s returned HTTP %d", rawURL, response.StatusCode)
	}

	const maxBodyBytes = 8 * 1024 * 1024
	limited := io.LimitReader(response.Body, maxBodyBytes+1)
	bytes, err := io.ReadAll(limited)
	if err != nil {
		return "", true, err
	}
	if len(bytes) > maxBodyBytes {
		return "", false, fmt.Errorf("response body exceeded %d bytes: %s", maxBodyBytes, rawURL)
	}
	return string(bytes), false, nil
}

func (f *HTTPFetcher) fetchBytesOnce(ctx context.Context, rawURL string) (BinaryResponse, bool, error) {
	if err := ValidateRemoteURL(rawURL); err != nil {
		return BinaryResponse{}, false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return BinaryResponse{}, false, err
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	request.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	request.Header.Set("Accept-Charset", "utf-8")
	request.Header.Set("Connection", "keep-alive")

	response, err := f.client.Do(request)
	if err != nil {
		return BinaryResponse{}, true, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusTooManyRequests {
		return BinaryResponse{}, false, fmt.Errorf("%w: %s returned HTTP %d", ErrRemoteBackoff, rawURL, response.StatusCode)
	}
	if response.StatusCode >= 500 {
		return BinaryResponse{}, true, fmt.Errorf("%s returned HTTP %d", rawURL, response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return BinaryResponse{}, false, fmt.Errorf("%s returned HTTP %d", rawURL, response.StatusCode)
	}

	const maxBodyBytes = 16 * 1024 * 1024
	limited := io.LimitReader(response.Body, maxBodyBytes+1)
	bytes, err := io.ReadAll(limited)
	if err != nil {
		return BinaryResponse{}, true, err
	}
	if len(bytes) > maxBodyBytes {
		return BinaryResponse{}, false, fmt.Errorf("response body exceeded %d bytes: %s", maxBodyBytes, rawURL)
	}
	contentType := response.Header.Get("Content-Type")
	if semi := strings.Index(contentType, ";"); semi >= 0 {
		contentType = contentType[:semi]
	}
	return BinaryResponse{
		Bytes:       bytes,
		ContentType: strings.TrimSpace(strings.ToLower(contentType)),
	}, false, nil
}

func safeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		dialAddress, err := validatedDialAddress(ctx, address)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, dialAddress)
	}
	return transport
}

func validateRedirect(request *http.Request, _ []*http.Request) error {
	return ValidateRemoteURL(request.URL.String())
}

func ValidateRemoteURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsafeRemoteURL, rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme %q", ErrUnsafeRemoteURL, parsed.Scheme)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("%w: private host %q", ErrUnsafeRemoteURL, host)
	}
	if ip := net.ParseIP(host); ip != nil && !isSafePublicIP(ip) {
		return fmt.Errorf("%w: private ip %q", ErrUnsafeRemoteURL, host)
	}
	return nil
}

func isSafeRemoteURL(rawURL string) bool {
	return ValidateRemoteURL(rawURL) == nil
}

func validatedDialAddress(ctx context.Context, address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip := net.ParseIP(host); ip != nil {
		if !isSafePublicIP(ip) {
			return "", fmt.Errorf("%w: private ip %q", ErrUnsafeRemoteURL, host)
		}
		return joinDialAddress(ip, port), nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("%w: no addresses for %q", ErrUnsafeRemoteURL, host)
	}
	for _, resolved := range ips {
		if !isSafePublicIP(resolved.IP) {
			return "", fmt.Errorf("%w: private resolved ip %q for %q", ErrUnsafeRemoteURL, resolved.IP.String(), host)
		}
	}
	return joinDialAddress(ips[0].IP, port), nil
}

func joinDialAddress(ip net.IP, port string) string {
	if port == "" {
		return ip.String()
	}
	return net.JoinHostPort(ip.String(), port)
}

func isSafePublicIP(ip net.IP) bool {
	if ipv4 := ip.To4(); ipv4 != nil {
		a, b, c, d := ipv4[0], ipv4[1], ipv4[2], ipv4[3]
		if a == 0 ||
			a == 10 ||
			a == 127 ||
			(a == 169 && b == 254) ||
			(a == 172 && b >= 16 && b <= 31) ||
			(a == 192 && b == 168) ||
			(a >= 224 && a <= 239) ||
			a >= 240 ||
			(a == 100 && b >= 64 && b <= 127) ||
			(a == 192 && b == 0 && c == 0) ||
			(a == 192 && b == 0 && c == 2) ||
			(a == 198 && (b == 18 || b == 19)) ||
			(a == 198 && b == 51 && c == 100) ||
			(a == 203 && b == 0 && c == 113) ||
			(a == 255 && b == 255 && c == 255 && d == 255) {
			return false
		}
		return true
	}

	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	return !ip.IsPrivate()
}
