package config

import (
	"flag"
	"os"
	"strings"
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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Addr != "127.0.0.1:18080" || cfg.DataDir != "../../data" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsInvalidAllowedOrigin(t *testing.T) {
	originalCommandLine := flag.CommandLine
	originalArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = originalCommandLine
		os.Args = originalArgs
	})

	flag.CommandLine = flag.NewFlagSet("config-test", flag.ContinueOnError)
	os.Args = []string{"viewer-api"}
	t.Setenv("VIEWER_API_ALLOWED_ORIGINS", "http://private-host.example:5173/")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), `末尾の"/"は削除してください`) {
		t.Fatalf("Load error = %v, want actionable trailing slash error", err)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	origins, err := ParseAllowedOrigins("HTTP://Viewer.Example.Test:5173, https://[fd00::1]:8443,http://viewer.example.test:5173")
	if err != nil {
		t.Fatalf("ParseAllowedOrigins returned error: %v", err)
	}
	if len(origins) != 2 || origins[0] != "http://viewer.example.test:5173" || origins[1] != "https://[fd00::1]:8443" {
		t.Fatalf("unexpected origins: %#v", origins)
	}
}

func TestParseAllowedOriginsEmpty(t *testing.T) {
	origins, err := ParseAllowedOrigins(" \t ")
	if err != nil || origins != nil {
		t.Fatalf("ParseAllowedOrigins = %#v, %v; want nil, nil", origins, err)
	}
}

func TestParseAllowedOriginsRejectsInvalidValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "trailing slash", value: "http://private-host.example:5173/", want: `末尾の"/"は削除してください`},
		{name: "path", value: "http://private-host.example:5173/path", want: "pathは指定できません"},
		{name: "query", value: "http://private-host.example:5173?x=1", want: "queryは指定できません"},
		{name: "fragment", value: "http://private-host.example:5173#x", want: "fragmentは指定できません"},
		{name: "userinfo", value: "http://user:password@private-host.example:5173", want: "userinfoは指定できません"},
		{name: "scheme", value: "ftp://private-host.example:5173", want: "schemeはhttpまたはhttpsを指定してください"},
		{name: "empty item", value: "http://valid.example:5173,", want: "空の値は指定できません"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAllowedOrigins(tc.value)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want message containing %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "private-host") || strings.Contains(err.Error(), "password") {
				t.Fatalf("error should not expose configured origin: %v", err)
			}
		})
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
