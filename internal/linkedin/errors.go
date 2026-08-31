package linkedin

import "errors"

var (
	// ErrSessionDead means LinkedIn rejected our session cookie (expired,
	// invalidated, or challenged). The operator must refresh LINKEDIN_COOKIE.
	ErrSessionDead = errors.New("linkedin: session cookie is invalid or expired")

	// ErrRateLimited means LinkedIn is throttling us (HTTP 429 / 999).
	ErrRateLimited = errors.New("linkedin: rate limited")

	// ErrNotFound means the profile does not exist or is not visible.
	ErrNotFound = errors.New("linkedin: profile not found")

	// ErrBlocked means LinkedIn served an anti-bot challenge / edge block
	// (e.g. a 302 redirect to the same URL, or an HTML challenge page).
	ErrBlocked = errors.New("linkedin: request blocked by anti-bot protection")

	// ErrInvalidURL means the supplied input is not a usable member profile URL.
	ErrInvalidURL = errors.New("invalid LinkedIn profile URL")
)

// APIError is returned for unexpected non-2xx responses from LinkedIn.
type APIError struct {
	Status int
	Path   string
	Body   string // truncated
}

func (e *APIError) Error() string {
	b := e.Body
	if len(b) > 300 {
		b = b[:300] + "…"
	}
	return "linkedin: unexpected status " + itoa(e.Status) + " for " + e.Path + ": " + b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
