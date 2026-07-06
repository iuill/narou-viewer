package config

import (
	"flag"
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("VIEWER_API_GO_TEST_VALUE", "configured")
	if got := envOrDefault("VIEWER_API_GO_TEST_VALUE", "fallback"); got != "configured" {
		t.Fatalf("expected configured value, got %q", got)
	}
	if got := envOrDefault("VIEWER_API_GO_MISSING_VALUE", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback value, got %q", got)
	}
}

func TestLoadReadsEnvironmentAndFlags(t *testing.T) {
	originalCommandLine := flag.CommandLine
	originalArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = originalCommandLine
		os.Args = originalArgs
	})

	flag.CommandLine = flag.NewFlagSet("config-test", flag.ContinueOnError)
	os.Args = []string{"viewer-api", "-addr", ":9999", "-data-dir", "/tmp/from-flag"}
	t.Setenv("VIEWER_API_GO_ADDR", ":1111")
	t.Setenv("VIEWER_API_DATA_DIR", "/tmp/from-env")

	cfg := Load()
	if cfg.Addr != ":9999" || cfg.DataDir != "/tmp/from-flag" {
		t.Fatalf("Load did not honor flags over env: %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	originalCommandLine := flag.CommandLine
	originalArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = originalCommandLine
		os.Args = originalArgs
	})

	flag.CommandLine = flag.NewFlagSet("config-test", flag.ContinueOnError)
	os.Args = []string{"viewer-api"}

	cfg := Load()
	if cfg.Addr != "127.0.0.1:18080" || cfg.DataDir != "../../data" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestFetcherAPIBaseURL(t *testing.T) {
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", " http://fetcher.test ")
	if got := FetcherAPIBaseURL(); got != "http://fetcher.test" {
		t.Fatalf("FetcherAPIBaseURL should trim configured URL, got %q", got)
	}
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", "")
	if got := FetcherAPIBaseURL(); got != "http://127.0.0.1:33006" {
		t.Fatalf("FetcherAPIBaseURL should return default URL, got %q", got)
	}
}
