package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaultsWhenEnvIsEmptyOrInvalid(t *testing.T) {
	t.Setenv("NOVEL_FETCHER_HOST", " ")
	t.Setenv("NOVEL_FETCHER_PORT", "invalid")
	t.Setenv("NOVEL_FETCHER_REQUEST_TIMEOUT_SECONDS", "bad")
	t.Setenv("NOVEL_FETCHER_PAGE_INTERVAL_MILLIS", "bad")

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Fatalf("Host = %q", cfg.Host)
	}
	if cfg.Port != 33006 || cfg.Addr() != "0.0.0.0:33006" {
		t.Fatalf("Port/Addr = %d/%q", cfg.Port, cfg.Addr())
	}
	if cfg.DataDir != "/data/novel-fetcher" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Fatalf("RequestTimeout = %s", cfg.RequestTimeout)
	}
	if cfg.FetchPolicy.Interval != 700*time.Millisecond {
		t.Fatalf("FetchPolicy.Interval = %s", cfg.FetchPolicy.Interval)
	}
}

func TestLoadReadsEnvOverrides(t *testing.T) {
	t.Setenv("NOVEL_FETCHER_HOST", "127.0.0.2")
	t.Setenv("NOVEL_FETCHER_PORT", "12345")
	t.Setenv("NOVEL_FETCHER_DATA_DIR", "/tmp/library")
	t.Setenv("NOVEL_FETCHER_USER_AGENT", "test-agent")
	t.Setenv("NOVEL_FETCHER_REQUEST_TIMEOUT_SECONDS", "9")
	t.Setenv("NOVEL_FETCHER_MAX_TOC_PAGES", "3")
	t.Setenv("NOVEL_FETCHER_WORK_INTERVAL_MILLIS", "11")
	t.Setenv("NOVEL_FETCHER_PAGE_INTERVAL_MILLIS", "22")
	t.Setenv("NOVEL_FETCHER_SYOSETU_WAIT_STEPS", "4")
	t.Setenv("NOVEL_FETCHER_STEPS_WAIT_MILLIS", "55")
	t.Setenv("NOVEL_FETCHER_RETRY_WAIT_MILLIS", "66")
	t.Setenv("NOVEL_FETCHER_MAX_RETRIES", "7")

	cfg := Load()

	if cfg.Addr() != "127.0.0.2:12345" {
		t.Fatalf("Addr = %q", cfg.Addr())
	}
	if cfg.DataDir != "/tmp/library" || cfg.UserAgent != "test-agent" {
		t.Fatalf("DataDir/UserAgent = %q/%q", cfg.DataDir, cfg.UserAgent)
	}
	if cfg.RequestTimeout != 9*time.Second || cfg.MaxTocPages != 3 || cfg.WorkInterval != 11*time.Millisecond {
		t.Fatalf("top-level timing values = timeout:%s toc:%d interval:%s", cfg.RequestTimeout, cfg.MaxTocPages, cfg.WorkInterval)
	}
	if cfg.FetchPolicy.Interval != 22*time.Millisecond ||
		cfg.FetchPolicy.WaitSteps != 4 ||
		cfg.FetchPolicy.StepsWait != 55*time.Millisecond ||
		cfg.FetchPolicy.RetryWait != 66*time.Millisecond ||
		cfg.FetchPolicy.MaxRetries != 7 {
		t.Fatalf("FetchPolicy = %#v", cfg.FetchPolicy)
	}
}
