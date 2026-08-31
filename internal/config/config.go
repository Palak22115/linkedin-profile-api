// Package config loads runtime configuration from environment variables,
// with optional support for a local .env file during development.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	Port      string
	APIKey    string        // clients must send this as the X-API-Key header (empty disables auth)
	Cookie    string        // full Cookie header value for www.linkedin.com
	JSESSION  string        // JSESSIONID value; csrf-token header is this without surrounding quotes
	UserAgent string        // browser User-Agent used for all LinkedIn requests
	Proxy     string        // optional outbound proxy URL for LinkedIn requests
	CacheTTL  time.Duration // 0 disables the response cache
}

// CSRFToken returns the JSESSIONID value with any surrounding quotes stripped,
// which is what LinkedIn expects in the csrf-token request header.
func (c Config) CSRFToken() string {
	return strings.Trim(c.JSESSION, `"`)
}

// Load reads configuration from the process environment. If a .env file exists
// in the working directory it is loaded first (without overriding real env vars).
func Load() (Config, error) {
	loadDotEnv(".env")

	cfg := Config{
		Port:      envOr("PORT", "8080"),
		APIKey:    os.Getenv("API_KEY"),
		Cookie:    os.Getenv("LINKEDIN_COOKIE"),
		JSESSION:  os.Getenv("JSESSIONID"),
		UserAgent: envOr("LINKEDIN_USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
		Proxy:     firstEnv("LINKEDIN_PROXY", "HTTPS_PROXY", "https_proxy"),
	}

	if ttl := os.Getenv("CACHE_TTL"); ttl != "" {
		d, err := time.ParseDuration(ttl)
		if err != nil {
			return Config{}, fmt.Errorf("invalid CACHE_TTL %q: %w", ttl, err)
		}
		cfg.CacheTTL = d
	}

	if cfg.Cookie == "" {
		return Config{}, fmt.Errorf("LINKEDIN_COOKIE is required")
	}
	if cfg.JSESSION == "" {
		// Fall back to reading JSESSIONID out of the cookie string.
		if v := cookieValue(cfg.Cookie, "JSESSIONID"); v != "" {
			cfg.JSESSION = v
		} else {
			return Config{}, fmt.Errorf("JSESSIONID is required (set it or include it in LINKEDIN_COOKIE)")
		}
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstEnv returns the value of the first set (non-empty) env var among keys.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// cookieValue extracts a single cookie's value from a Cookie header string.
func cookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

// loadDotEnv parses a simple KEY=VALUE file. Lines starting with # are ignored.
// Existing environment variables are not overwritten. Missing file is not an error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip surrounding quotes only when they are a matched pair, so that
		// quotes inside a value (e.g. a Cookie header) are preserved.
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
