// Package api exposes the HTTP interface: GET /api/profile?url=<linkedin url>.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Palak22115/linkedin-profile-api/internal/linkedin"
	"github.com/Palak22115/linkedin-profile-api/internal/model"
)

// ProfileFetcher fetches and parses a profile. *scrape.Scraper implements it.
type ProfileFetcher interface {
	Profile(ctx context.Context, input string) (*model.Profile, error)
}

// Handler holds the API dependencies.
type Handler struct {
	scraper   ProfileFetcher
	apiKey    string
	startedAt time.Time
}

// New returns the fully-wired HTTP handler (routing + middleware).
func New(scraper ProfileFetcher, apiKey string) http.Handler {
	h := &Handler{scraper: scraper, apiKey: apiKey, startedAt: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.index)
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /api/profile", requireAPIKey(h.apiKey, http.HandlerFunc(h.profile)))

	return recoverer(logger(mux))
}

// profileRequestTimeout bounds one upstream fetch (resolve + ~12 paced calls).
const profileRequestTimeout = 120 * time.Second

func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		writeError(w, http.StatusBadRequest, "missing_parameter",
			"query parameter 'url' is required, e.g. /api/profile?url=https://www.linkedin.com/in/<slug>/")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), profileRequestTimeout)
	defer cancel()

	profile, err := h.scraper.Profile(ctx, target)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			writeError(w, http.StatusGatewayTimeout, "upstream_timeout",
				"Timed out fetching the profile from LinkedIn.")
			return
		}
		if errors.Is(err, linkedin.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
		writeScrapeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"uptime": time.Since(h.startedAt).Round(time.Second).String(),
	})
}

func (h *Handler) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(`LinkedIn Profile API

GET /api/profile?url=<linkedin profile url>
    Header: X-API-Key: <key>   (if configured)

GET /health
`))
}

// writeJSON encodes v without HTML-escaping so CDN URLs stay readable.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
