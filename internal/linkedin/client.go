// Package linkedin is a minimal reverse-engineered client for LinkedIn's
// internal "Voyager" JSON API. It authenticates with a copied browser session
// cookie and never uses a browser or headless automation.
package linkedin

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://www.linkedin.com"
	voyager   = baseURL + "/voyager/api"
	acceptLI  = "application/vnd.linkedin.normalized+json+2.1"
	minGap    = 1000 * time.Millisecond // minimum spacing between LinkedIn requests
	maxGapJit = 500 * time.Millisecond  // added random jitter, [0, maxGapJit)
)

// volatileCookies are the ones LinkedIn rotates and that matter for the
// anti-bot edge; we refresh them from Set-Cookie responses.
var volatileCookies = map[string]bool{"__cf_bm": true, "lidc": true, "bcookie": true, "li_at": true, "JSESSIONID": true}

// Client talks to the Voyager API. It is safe for concurrent use; requests are
// serialised through a pacing lock so we never burst LinkedIn.
type Client struct {
	http      *http.Client
	csrf      string
	userAgent string

	// cookies holds every cookie as a raw name→value pair (values may contain
	// '"', which the stdlib cookie jar rejects, so we manage the Cookie header
	// ourselves). Refreshed from Set-Cookie on volatile names.
	cookieMu sync.RWMutex
	cookies  map[string]string
	order    []string

	mu   sync.Mutex
	last time.Time
	rnd  *rand.Rand
}

// New builds a Client from an already-assembled Cookie header, the CSRF token
// (JSESSIONID without quotes) and a browser User-Agent string.
func New(cookie, csrfToken, userAgent string) *Client {
	hc := &http.Client{
		Timeout: 30 * time.Second,
		// Do not follow redirects: LinkedIn's anti-bot edge answers with a
		// 302 to the same URL, which we want to detect rather than loop on.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	c := &Client{
		http:      hc,
		csrf:      csrfToken,
		userAgent: userAgent,
		cookies:   map[string]string{},
		rnd:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := c.cookies[k]; !seen {
			c.order = append(c.order, k)
		}
		c.cookies[k] = strings.TrimSpace(v)
	}
	return c
}

// cookieHeader renders the current cookie set as a Cookie header value.
func (c *Client) cookieHeader() string {
	c.cookieMu.RLock()
	defer c.cookieMu.RUnlock()
	var b strings.Builder
	for i, k := range c.order {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(c.cookies[k])
	}
	return b.String()
}

// absorbSetCookie updates volatile cookies (e.g. __cf_bm, lidc) from a response,
// keeping the raw value verbatim.
func (c *Client) absorbSetCookie(resp *http.Response) {
	lines := resp.Header["Set-Cookie"]
	if len(lines) == 0 {
		return
	}
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()
	for _, line := range lines {
		pair, _, _ := strings.Cut(line, ";")
		k, v, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || !volatileCookies[k] || v == "" || strings.Contains(v, "delete") {
			continue
		}
		if _, seen := c.cookies[k]; !seen {
			c.order = append(c.order, k)
		}
		c.cookies[k] = v
	}
}

// pace blocks until at least minGap (+jitter) has elapsed since the last request.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := time.Until(c.last.Add(minGap + time.Duration(c.rnd.Int63n(int64(maxGapJit)))))
	if wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

// getJSON performs a paced GET against a Voyager path (relative to /voyager/api)
// with the given query params and returns the raw response body.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := voyager + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.voyagerHeaders(req)
	return c.do(req, path)
}

// getRaw performs a paced GET against an absolute linkedin.com URL, used for the
// profile HTML page during vanity→URN resolution.
func (c *Client) getRaw(ctx context.Context, absURL string, htmlHeaders bool) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, absURL, nil)
	if err != nil {
		return nil, nil, err
	}
	if htmlHeaders {
		c.htmlHeaders(req)
	} else {
		c.voyagerHeaders(req)
	}
	c.pace()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	c.absorbSetCookie(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp, err
	}
	if cerr := classify(resp, body, absURL); cerr != nil {
		return body, resp, cerr
	}
	return body, resp, nil
}

// do sends an already-built request through pacing + response classification.
func (c *Client) do(req *http.Request, path string) ([]byte, error) {
	c.pace()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.absorbSetCookie(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if cerr := classify(resp, body, path); cerr != nil {
		return body, cerr
	}
	return body, nil
}

func (c *Client) voyagerHeaders(req *http.Request) {
	h := req.Header
	h.Set("accept", acceptLI)
	h.Set("cookie", c.cookieHeader())
	h.Set("csrf-token", c.csrf)
	h.Set("x-restli-protocol-version", "2.0.0")
	h.Set("x-li-lang", "en_US")
	h.Set("user-agent", c.userAgent)
	h.Set("accept-language", "en-US,en;q=0.9")
	h.Set("referer", baseURL+"/feed/")
	h.Set("x-li-track", `{"clientVersion":"1.13.0","osName":"web","timezoneOffset":0,"deviceFormFactor":"DESKTOP","mpName":"voyager-web"}`)
}

func (c *Client) htmlHeaders(req *http.Request) {
	h := req.Header
	h.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("cookie", c.cookieHeader())
	h.Set("user-agent", c.userAgent)
	h.Set("accept-language", "en-US,en;q=0.9")
	h.Set("upgrade-insecure-requests", "1")
}

// classify inspects a response and maps anti-bot / auth / rate-limit signals to
// sentinel errors. Returns nil for a healthy 2xx response.
func classify(resp *http.Response, body []byte, path string) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// 302-to-self is served with 200 sometimes; also watch for session kill.
		if strings.Contains(string(body), `li_at=delete me`) {
			return ErrSessionDead
		}
		return nil
	case resp.StatusCode == 302 || resp.StatusCode == 303 || resp.StatusCode == 307:
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "/login") || strings.Contains(loc, "/checkpoint") || strings.Contains(loc, "/authwall") {
			return ErrSessionDead
		}
		// Redirect to (approximately) the same URL is the classic edge block.
		return ErrBlocked
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return ErrSessionDead
	case resp.StatusCode == 404 || resp.StatusCode == 410:
		return ErrNotFound
	case resp.StatusCode == 429 || resp.StatusCode == 999:
		return ErrRateLimited
	default:
		return &APIError{Status: resp.StatusCode, Path: path, Body: string(body)}
	}
}
