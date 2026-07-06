package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
)

const Version = "0.1.0"

type Config struct {
	Host           string
	Port           int
	DataDir        string
	UserAgent      string
	RequestTimeout time.Duration
	MaxTocPages    int
	WorkInterval   time.Duration
	FetchPolicy    fetcher.FetchPolicy
}

func Load() Config {
	return Config{
		Host:           envString("NOVEL_FETCHER_HOST", "0.0.0.0"),
		Port:           envInt("NOVEL_FETCHER_PORT", 33006),
		DataDir:        envString("NOVEL_FETCHER_DATA_DIR", "/data/novel-fetcher"),
		UserAgent:      envString("NOVEL_FETCHER_USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"),
		RequestTimeout: time.Duration(envInt("NOVEL_FETCHER_REQUEST_TIMEOUT_SECONDS", 30)) * time.Second,
		MaxTocPages:    envInt("NOVEL_FETCHER_MAX_TOC_PAGES", 50),
		WorkInterval:   envDurationMillis("NOVEL_FETCHER_WORK_INTERVAL_MILLIS", 2500),
		FetchPolicy: fetcher.FetchPolicy{
			Interval:   envDurationMillis("NOVEL_FETCHER_PAGE_INTERVAL_MILLIS", 700),
			WaitSteps:  envInt("NOVEL_FETCHER_SYOSETU_WAIT_STEPS", 10),
			StepsWait:  envDurationMillis("NOVEL_FETCHER_STEPS_WAIT_MILLIS", 5000),
			RetryWait:  envDurationMillis("NOVEL_FETCHER_RETRY_WAIT_MILLIS", 5000),
			MaxRetries: envInt("NOVEL_FETCHER_MAX_RETRIES", 1),
		},
	}
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDurationMillis(key string, fallback int) time.Duration {
	return time.Duration(envInt(key, fallback)) * time.Millisecond
}
