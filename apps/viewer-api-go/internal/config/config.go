package config

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr           string
	DataDir        string
	AllowedOrigins []string
}

func Load() (Config, error) {
	defaultDataDir := filepath.Clean("../../data")
	addr := envOrDefault("VIEWER_API_GO_ADDR", "127.0.0.1:18080")
	dataDir := envOrDefault("VIEWER_API_DATA_DIR", envOrDefault("DATA_DIR", defaultDataDir))

	flag.StringVar(&addr, "addr", addr, "HTTP listen address")
	flag.StringVar(&dataDir, "data-dir", dataDir, "viewer data directory")
	flag.Parse()

	allowedOrigins, err := ParseAllowedOrigins(os.Getenv("VIEWER_API_ALLOWED_ORIGINS"))
	if err != nil {
		return Config{}, err
	}
	return Config{Addr: addr, DataDir: dataDir, AllowedOrigins: allowedOrigins}, nil
}

func ParseAllowedOrigins(configured string) ([]string, error) {
	if strings.TrimSpace(configured) == "" {
		return nil, nil
	}
	items := strings.Split(configured, ",")
	origins := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, raw := range items {
		origin, err := parseOrigin(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("VIEWER_API_ALLOWED_ORIGINS の%d件目が不正です: %w", index+1, err)
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
}

func parseOrigin(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("空の値は指定できません")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("有効なURL originではありません")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("schemeはhttpまたはhttpsを指定してください")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("hostを指定してください")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("userinfoは指定できません")
	}
	if parsed.Path != "" {
		if parsed.Path == "/" && parsed.RawPath == "" {
			return "", fmt.Errorf(`末尾の"/"は削除してください（例: http://host.example:5173）`)
		}
		return "", fmt.Errorf("pathは指定できません")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", fmt.Errorf("queryは指定できません")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("fragmentは指定できません")
	}
	if parsed.Opaque != "" {
		return "", fmt.Errorf("有効なURL originではありません")
	}

	host := strings.ToLower(parsed.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := parsed.Port(); port != "" {
		host = net.JoinHostPort(parsed.Hostname(), port)
		if !strings.Contains(parsed.Hostname(), ":") {
			host = strings.ToLower(host)
		}
	}
	return strings.ToLower(parsed.Scheme) + "://" + host, nil
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
