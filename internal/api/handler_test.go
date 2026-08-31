package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palak-kasoundhan/linkedin-profile-api/internal/linkedin"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/model"
)

type stubFetcher struct {
	profile *model.Profile
	err     error
	gotArg  string
}

func (s *stubFetcher) Profile(_ context.Context, input string) (*model.Profile, error) {
	s.gotArg = input
	return s.profile, s.err
}

func do(h http.Handler, method, url string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestProfile_OK(t *testing.T) {
	stub := &stubFetcher{profile: &model.Profile{FullName: "Jane Doe", PublicIdentifier: "jane"}}
	h := New(stub, "")

	rr := do(h, "GET", "/api/profile?url=https://www.linkedin.com/in/jane/", nil)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body)
	}
	if stub.gotArg != "https://www.linkedin.com/in/jane/" {
		t.Errorf("scraper got %q", stub.gotArg)
	}
	var got model.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got.FullName != "Jane Doe" {
		t.Errorf("body = %s", rr.Body)
	}
}

func TestProfile_MissingURL(t *testing.T) {
	rr := do(New(&stubFetcher{}, ""), "GET", "/api/profile", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "missing_parameter") {
		t.Errorf("body = %s", rr.Body)
	}
}

func TestProfile_ErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
		code string
	}{
		{linkedin.ErrNotFound, http.StatusNotFound, "profile_not_found"},
		{linkedin.ErrRateLimited, http.StatusTooManyRequests, "upstream_rate_limited"},
		{linkedin.ErrSessionDead, http.StatusServiceUnavailable, "session_expired"},
		{linkedin.ErrBlocked, http.StatusBadGateway, "upstream_blocked"},
		{linkedin.ErrInvalidURL, http.StatusBadRequest, "invalid_url"},
	}
	for _, c := range cases {
		h := New(&stubFetcher{err: c.err}, "")
		rr := do(h, "GET", "/api/profile?url=x", nil)
		if rr.Code != c.want {
			t.Errorf("%v: status = %d, want %d", c.err, rr.Code, c.want)
		}
		if !strings.Contains(rr.Body.String(), c.code) {
			t.Errorf("%v: body = %s, want code %s", c.err, rr.Body, c.code)
		}
	}
}

func TestAPIKey(t *testing.T) {
	h := New(&stubFetcher{profile: &model.Profile{}}, "secret")

	if rr := do(h, "GET", "/api/profile?url=x", nil); rr.Code != http.StatusUnauthorized {
		t.Errorf("no key: status = %d", rr.Code)
	}
	if rr := do(h, "GET", "/api/profile?url=x", map[string]string{"X-API-Key": "wrong"}); rr.Code != http.StatusUnauthorized {
		t.Errorf("bad key: status = %d", rr.Code)
	}
	if rr := do(h, "GET", "/api/profile?url=x", map[string]string{"X-API-Key": "secret"}); rr.Code != 200 {
		t.Errorf("good key: status = %d", rr.Code)
	}
	// health is not gated
	if rr := do(h, "GET", "/health", nil); rr.Code != 200 {
		t.Errorf("health gated: status = %d", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	rr := do(New(&stubFetcher{}, ""), "GET", "/health", nil)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"status": "ok"`) {
		t.Errorf("health body = %s", rr.Body)
	}
}
