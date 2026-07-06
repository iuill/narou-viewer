package fetcher

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newTestHTTPFetcher(handler roundTripFunc) *HTTPFetcher {
	f := NewHTTPFetcher(HTTPFetcherOptions{
		UserAgent: "coverage-agent",
		Timeout:   time.Second,
		Logger:    slog.Default(),
	})
	f.client = &http.Client{Transport: handler}
	f.limiter = NewHostRateLimiter()
	return f
}

func TestFetchTextUsesHeadersAndReturnsBody(t *testing.T) {
	f := newTestHTTPFetcher(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("User-Agent") != "coverage-agent" {
			t.Fatalf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		if !strings.Contains(request.Header.Get("Accept"), "text/html") {
			t.Fatalf("Accept = %q", request.Header.Get("Accept"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("本文")),
			Header:     make(http.Header),
		}, nil
	})

	body, err := f.FetchText(context.Background(), "https://example.com/novel", FetchPolicy{})
	if err != nil {
		t.Fatalf("FetchText returned error: %v", err)
	}
	if body != "本文" {
		t.Fatalf("body = %q", body)
	}
}

func TestFetchBytesNormalizesContentType(t *testing.T) {
	f := newTestHTTPFetcher(func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.Header.Get("Accept"), "image/") {
			t.Fatalf("Accept = %q", request.Header.Get("Accept"))
		}
		header := make(http.Header)
		header.Set("Content-Type", "IMAGE/PNG; charset=binary")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("png")),
			Header:     header,
		}, nil
	})

	response, err := f.FetchBytes(context.Background(), "https://example.com/image.png", FetchPolicy{})
	if err != nil {
		t.Fatalf("FetchBytes returned error: %v", err)
	}
	if string(response.Bytes) != "png" || response.ContentType != "image/png" {
		t.Fatalf("response = %#v", response)
	}
}

func TestFetchBytesAllowsMissingContentType(t *testing.T) {
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("bytes")),
			Header:     make(http.Header),
		}, nil
	})

	response, err := f.FetchBytes(context.Background(), "https://example.com/image.bin", FetchPolicy{})
	if err != nil {
		t.Fatalf("FetchBytes returned error: %v", err)
	}
	if string(response.Bytes) != "bytes" || response.ContentType != "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestFetchBytesRetriesServerErrors(t *testing.T) {
	attempts := 0
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("temporary")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("gif")),
			Header:     make(http.Header),
		}, nil
	})

	response, err := f.FetchBytes(context.Background(), "https://example.com/retry.gif", FetchPolicy{
		MaxRetries: 1,
		RetryWait:  time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("FetchBytes returned error: %v", err)
	}
	if string(response.Bytes) != "gif" || attempts != 2 {
		t.Fatalf("response/attempts = %#v/%d", response, attempts)
	}
}

func TestFetchBytesReturnsLastRetryError(t *testing.T) {
	attempts := 0
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("temporary")),
			Header:     make(http.Header),
		}, nil
	})

	_, err := f.FetchBytes(context.Background(), "https://example.com/fail.png", FetchPolicy{
		MaxRetries: 1,
		RetryWait:  time.Nanosecond,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("FetchBytes error = %v, want HTTP 502", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestFetchBytesStopsRetryWaitWhenContextCanceled(t *testing.T) {
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("temporary")),
			Header:     make(http.Header),
		}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.FetchBytes(ctx, "https://example.com/cancel.png", FetchPolicy{
		MaxRetries: 1,
		RetryWait:  time.Hour,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchBytes error = %v, want context.Canceled", err)
	}
}

func TestFetchBytesDoesNotRetryBackoffOrClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusNotFound} {
		attempts := 0
		f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		})

		_, err := f.FetchBytes(context.Background(), "https://example.com/no-retry.png", FetchPolicy{
			MaxRetries: 2,
			RetryWait:  time.Nanosecond,
		})
		if err == nil {
			t.Fatalf("status %d returned nil error", status)
		}
		if status == http.StatusTooManyRequests && !errors.Is(err, ErrRemoteBackoff) {
			t.Fatalf("status %d error = %v, want ErrRemoteBackoff", status, err)
		}
		if attempts != 1 {
			t.Fatalf("status %d attempts = %d, want 1", status, attempts)
		}
	}
}

func TestFetchBytesRejectsOversizedBody(t *testing.T) {
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.LimitReader(infiniteAReader{}, 16*1024*1024+1)),
			Header:     make(http.Header),
		}, nil
	})

	if _, err := f.FetchBytes(context.Background(), "https://example.com/large.png", FetchPolicy{}); err == nil {
		t.Fatal("FetchBytes returned nil error for oversized body")
	}
}

func TestFetchTextRetriesServerErrors(t *testing.T) {
	attempts := 0
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("temporary")),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	body, err := f.FetchText(context.Background(), "https://example.com/retry", FetchPolicy{
		MaxRetries: 1,
		RetryWait:  time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("FetchText returned error: %v", err)
	}
	if body != "ok" || attempts != 2 {
		t.Fatalf("body/attempts = %q/%d", body, attempts)
	}
}

func TestFetchTextStopsRetryWaitWhenContextCanceled(t *testing.T) {
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("temporary")),
			Header:     make(http.Header),
		}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.FetchText(ctx, "https://example.com/cancel", FetchPolicy{
		MaxRetries: 1,
		RetryWait:  time.Hour,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchText error = %v, want context.Canceled", err)
	}
}

func TestFetchTextDoesNotRetryBackoffOrClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusNotFound} {
		attempts := 0
		f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		})

		_, err := f.FetchText(context.Background(), "https://example.com/no-retry", FetchPolicy{
			MaxRetries: 2,
			RetryWait:  time.Nanosecond,
		})
		if err == nil {
			t.Fatalf("status %d returned nil error", status)
		}
		if status == http.StatusTooManyRequests && !errors.Is(err, ErrRemoteBackoff) {
			t.Fatalf("status %d error = %v, want ErrRemoteBackoff", status, err)
		}
		if attempts != 1 {
			t.Fatalf("status %d attempts = %d, want 1", status, attempts)
		}
	}
}

func TestFetchTextRejectsOversizedBody(t *testing.T) {
	f := newTestHTTPFetcher(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.LimitReader(infiniteAReader{}, 8*1024*1024+1)),
			Header:     make(http.Header),
		}, nil
	})

	if _, err := f.FetchText(context.Background(), "https://example.com/large", FetchPolicy{}); err == nil {
		t.Fatal("FetchText returned nil error for oversized body")
	}
}

func TestIsSafeRemoteURL(t *testing.T) {
	if !isSafeRemoteURL("https://example.com/path") {
		t.Fatal("public URL was rejected")
	}
	if isSafeRemoteURL("http://127.0.0.1/path") {
		t.Fatal("loopback URL was accepted")
	}
}

func TestValidateRemoteURLRejectsUnsafeHostBoundaries(t *testing.T) {
	for _, rawURL := range []string{
		"https://localhost./path",
		"https://sub.localhost/path",
		"https:///missing-host",
		"mailto:reader@example.com",
		"://bad-url",
	} {
		if err := ValidateRemoteURL(rawURL); err == nil {
			t.Fatalf("ValidateRemoteURL(%q) returned nil error", rawURL)
		}
	}
}

func TestValidateRedirectRejectsUnsafeTargets(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost/private",
		"http://10.0.0.1/private",
		"file:///etc/passwd",
	} {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("url.Parse(%q) returned error: %v", rawURL, err)
		}
		if err := validateRedirect(&http.Request{URL: parsed}, nil); err == nil {
			t.Fatalf("validateRedirect(%q) returned nil error", rawURL)
		}
	}
}

func TestSafeTransportRejectsPrivateIPDial(t *testing.T) {
	transport := safeTransport()
	conn, err := transport.DialContext(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", "80"))
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("safe transport accepted loopback dial")
	}
	if !errors.Is(err, ErrUnsafeRemoteURL) {
		t.Fatalf("DialContext error = %v, want ErrUnsafeRemoteURL", err)
	}
}

type infiniteAReader struct{}

func (infiniteAReader) Read(p []byte) (int, error) {
	for index := range p {
		p[index] = 'a'
	}
	return len(p), nil
}
