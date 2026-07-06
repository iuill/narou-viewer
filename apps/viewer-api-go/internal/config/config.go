package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr    string
	DataDir string
}

func Load() Config {
	defaultDataDir := filepath.Clean("../../data")
	addr := envOrDefault("VIEWER_API_GO_ADDR", "127.0.0.1:18080")
	dataDir := envOrDefault("VIEWER_API_DATA_DIR", envOrDefault("DATA_DIR", defaultDataDir))

	flag.StringVar(&addr, "addr", addr, "HTTP listen address")
	flag.StringVar(&dataDir, "data-dir", dataDir, "viewer data directory")
	flag.Parse()

	return Config{Addr: addr, DataDir: dataDir}
}

func envOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func FetcherAPIBaseURL() string {
	configured := strings.TrimSpace(os.Getenv("NOVEL_FETCHER_API_BASE_URL"))
	if configured != "" {
		return configured
	}
	return "http://127.0.0.1:33006"
}
