package linkedin

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// profileIDRe matches a LinkedIn profile identifier as it appears embedded in
// the profile HTML page, e.g. urn:li:fsd_profile:ACoAAB...  or  fsd_profile:ACoAAB...
var profileIDRe = regexp.MustCompile(`fsd_profile:(ACoA[A-Za-z0-9_-]{10,})`)

// Resolved is the outcome of turning a public profile URL into the internal
// identifier the Voyager API needs, plus the HTML we fetched (kept for optional
// fallback extraction of fields the dash API returns only as URNs).
type Resolved struct {
	Vanity string // public identifier / vanity slug, e.g. "prashant-ravi-a5a688b3"
	ID     string // fsd_profile id, e.g. "ACoAAB..."
	HTML   []byte // the fetched profile page, for fallback parsing
}

// URN returns the fully-qualified profile URN.
func (r Resolved) URN() string { return "urn:li:fsd_profile:" + r.ID }

// ParseVanity extracts the vanity slug from a LinkedIn profile URL. It also
// accepts a bare slug. It rejects company/school/other URL shapes.
func ParseVanity(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("%w: empty input", ErrInvalidURL)
	}
	// Bare slug (no slashes, no dots) — accept as-is.
	if !strings.Contains(s, "/") && !strings.Contains(s, ".") {
		return cleanSlug(s), nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, "linkedin.com") {
		return "", fmt.Errorf("%w: not a linkedin.com host (%q)", ErrInvalidURL, host)
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	// Expect .../in/<slug>[/...]. Some locale URLs look like /in/<slug> too.
	for i, p := range parts {
		if p == "in" && i+1 < len(parts) {
			slug := cleanSlug(parts[i+1])
			if slug == "" {
				return "", fmt.Errorf("%w: empty profile slug", ErrInvalidURL)
			}
			return slug, nil
		}
	}
	return "", fmt.Errorf("%w: not a member profile URL (expected /in/<slug>)", ErrInvalidURL)
}

func cleanSlug(s string) string {
	s, _, _ = strings.Cut(s, "?")
	s, _, _ = strings.Cut(s, "#")
	if dec, err := url.PathUnescape(s); err == nil {
		s = dec
	}
	return strings.Trim(strings.TrimSpace(s), "/")
}

// Resolve fetches the public profile page and extracts the internal profile id.
// The vanity → id mapping is not available from a clean JSON endpoint, so we
// read it out of the server-rendered HTML (which every logged-in page request
// returns).
func (c *Client) Resolve(ctx context.Context, vanity string) (*Resolved, error) {
	page := baseURL + "/in/" + url.PathEscape(vanity) + "/"
	body, _, err := c.getRaw(ctx, page, true)
	if err != nil {
		return nil, err
	}

	// The page embeds many profile URNs (the subject plus "people also viewed"
	// recommendations). The subject's id is by far the most frequent.
	counts := map[string]int{}
	for _, m := range profileIDRe.FindAllSubmatch(body, -1) {
		counts[string(m[1])]++
	}
	if len(counts) == 0 {
		return nil, fmt.Errorf("%w: could not find profile id in page for %q", ErrNotFound, vanity)
	}
	best, bestN := "", -1
	for id, n := range counts {
		if n > bestN {
			best, bestN = id, n
		}
	}
	return &Resolved{Vanity: vanity, ID: best, HTML: body}, nil
}
