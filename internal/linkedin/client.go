// Package linkedin is a minimal reverse-engineered client for LinkedIn's
// internal "Voyager" JSON API. It authenticates with a copied browser session
// cookie and never uses a browser or headless automation.
package linkedin

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
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

// Client talks to the Voyager API. It is safe for concurrent use; requests are
// serialised through a pacing lock so we never burst LinkedIn.
type Client struct {
	http      *http.Client
	cookie    string
	csrf      string
	userAgent string

	mu   sync.Mutex
	last time.Time
	rnd  *rand.Rand
}

// New builds a Client from an already-assembled Cookie header, the CSRF token
// (JSESSIONID without quotes) and a browser User-Agent string.
//
// The seed cookie is loaded into a cookie jar so that short-lived cookies
// LinkedIn rotates (notably __cf_bm, ~30 min, and lidc) are refreshed from
// Set-Cookie responses instead of going stale — important for a long-running
// hosted deployment.
func New(cookie, csrfToken, userAgent string) *Client {
	jar, _ := cookiejar.New(nil)
	if u, err := url.Parse(baseURL); err == nil {
		jar.SetCookies(u, parseCookieHeader(cookie))
	}
	hc := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		// Do not follow redirects: LinkedIn's anti-bot edge answers with a
		// 302 to the same URL, which we want to detect rather than loop on.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{
		http:      hc,
		cookie:    cookie,
		csrf:      csrfToken,
		userAgent: userAgent,
		rnd:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// parseCookieHeader turns a "k=v; k2=v2" Cookie header into []*http.Cookie.
func parseCookieHeader(header string) []*http.Cookie {
	var out []*http.Cookie
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		k, v, ok := strings.Cut(part, "=")
		if !ok || k == "" {
			continue
		}
		out = append(out, &http.Cookie{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
	}
	return out
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
