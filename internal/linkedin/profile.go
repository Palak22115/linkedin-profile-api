package linkedin

import (
	"context"
	"encoding/json"
	"net/url"
)

// Section names on the identity/dash API. Each is a Rest.li collection queried
// with ?q=viewee&profileUrn=<urn>. Empty sections return an empty collection,
// not an error.
var profileSections = []string{
	"profilePositions",
	"profileEducations",
	"profileSkills",
	"profileCertifications",
	"profileLanguages",
	"profileProjects",
	"profileVolunteerExperiences",
	"profileHonors",
	"profilePublications",
	"profileCourses",
}

// RawProfile is the unparsed Voyager payload set for one profile: the core
// entity plus every section collection, keyed by section name ("core" for the
// profile entity itself).
type RawProfile struct {
	Resolved *Resolved
	Parts    map[string]json.RawMessage
}

// CoreProfile fetches the profile entity by URN.
func (c *Client) CoreProfile(ctx context.Context, urn string) (json.RawMessage, error) {
	// Path segment must contain the literal, percent-encoded URN.
	path := "/identity/dash/profiles/" + url.PathEscape(urn)
	return c.getJSON(ctx, path, nil)
}

// Section fetches one profile section collection by name.
func (c *Client) Section(ctx context.Context, name, urn string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("q", "viewee")
	q.Set("profileUrn", urn)
	return c.getJSON(ctx, "/identity/dash/"+name, q)
}

// FetchAll resolves the vanity then pulls the core profile and every section,
// pacing every request. Individual section failures that are "not found" or
// empty are tolerated; auth/rate-limit/block errors abort.
func (c *Client) FetchAll(ctx context.Context, r *Resolved) (*RawProfile, error) {
	out := &RawProfile{Resolved: r, Parts: make(map[string]json.RawMessage, len(profileSections)+1)}

	core, err := c.CoreProfile(ctx, r.URN())
	if err != nil {
		return nil, err
	}
	out.Parts["core"] = core

	for _, name := range profileSections {
		body, err := c.Section(ctx, name, r.URN())
		if err != nil {
			// A dead session or block on one call means every later call will
			// fail too — surface it.
			if isFatal(err) {
				return nil, err
			}
			continue
		}
		out.Parts[name] = body
	}
	return out, nil
}

func isFatal(err error) bool {
	switch err {
	case ErrSessionDead, ErrRateLimited, ErrBlocked:
		return true
	default:
		return false
	}
}
