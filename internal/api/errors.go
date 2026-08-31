package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Palak22115/linkedin-profile-api/internal/linkedin"
)

// errorBody is the JSON shape returned for every non-2xx response.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = msg
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(b)
}

// writeScrapeError maps a scraper/LinkedIn error to an HTTP response.
func writeScrapeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, linkedin.ErrNotFound):
		writeError(w, http.StatusNotFound, "profile_not_found",
			"The profile does not exist or is not publicly visible.")
	case errors.Is(err, linkedin.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "upstream_rate_limited",
			"LinkedIn is rate limiting the backend. Try again later.")
	case errors.Is(err, linkedin.ErrSessionDead):
		writeError(w, http.StatusServiceUnavailable, "session_expired",
			"The backend LinkedIn session is invalid. The operator must refresh it.")
	case errors.Is(err, linkedin.ErrBlocked):
		writeError(w, http.StatusBadGateway, "upstream_blocked",
			"LinkedIn blocked the request (anti-bot). Try again later.")
	default:
		var apiErr *linkedin.APIError
		if errors.As(err, &apiErr) {
			writeError(w, http.StatusBadGateway, "upstream_error",
				"Unexpected response from LinkedIn.")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
