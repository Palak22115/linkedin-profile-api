// Package parse maps raw Voyager payloads into the public model.Profile schema.
package parse

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/palak-kasoundhan/linkedin-profile-api/internal/linkedin"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/model"
)

// Build turns a fetched RawProfile into the public schema.
func Build(raw *linkedin.RawProfile, sourceURL string) (*model.Profile, error) {
	p := &model.Profile{
		PublicIdentifier: raw.Resolved.Vanity,
		ProfileID:        raw.Resolved.ID,
		SourceURL:        sourceURL,
		RetrievedAt:      time.Now().UTC(),
		Experience:       []model.Position{},
		Education:        []model.Education{},
		Skills:           []model.Skill{},
		Certifications:   []model.Certification{},
		Languages:        []model.Language{},
	}

	if err := applyCore(p, raw.Parts["core"], raw.Resolved.HTML); err != nil {
		return nil, err
	}
	applyExperience(p, raw.Parts["profilePositions"])
	applyEducation(p, raw.Parts["profileEducations"])
	applySkills(p, raw.Parts["profileSkills"])
	applyCertifications(p, raw.Parts["profileCertifications"])
	applyLanguages(p, raw.Parts["profileLanguages"])
	applyProjects(p, raw.Parts["profileProjects"])
	applyVolunteering(p, raw.Parts["profileVolunteerExperiences"])
	applyHonors(p, raw.Parts["profileHonors"])
	applyPublications(p, raw.Parts["profilePublications"])
	applyCourses(p, raw.Parts["profileCourses"])

	// Flag sections LinkedIn commonly hides for non-connections.
	for name, label := range map[string]string{
		"profileSkills":    "skills",
		"profileLanguages": "languages",
	} {
		if _, ok := raw.Parts[name]; !ok {
			p.Partial = true
			p.Notes = append(p.Notes, "section not retrieved: "+label)
		}
	}
	return p, nil
}

// ---------- core ----------

type coreEnvelope struct {
	Data struct {
		PublicIdentifier string `json:"publicIdentifier"`
		FirstName        string `json:"firstName"`
		LastName         string `json:"lastName"`
		Headline         string `json:"headline"`
		Summary          string `json:"summary"`
		Premium          bool   `json:"premium"`
		Influencer       bool   `json:"influencer"`
		ShowVerification bool   `json:"showVerificationBadge"`
		Location         struct {
			CountryCode string `json:"countryCode"`
		} `json:"location"`
		ProfilePicture    *rawPicture `json:"profilePicture"`
		BackgroundPicture *rawPicture `json:"backgroundPicture"`
	} `json:"data"`
}

type rawPicture struct {
	DisplayImage struct {
		VectorImage *rawVector `json:"vectorImage"`
	} `json:"displayImage"`
}

type rawVector struct {
	RootURL   string        `json:"rootUrl"`
	Artifacts []rawArtifact `json:"artifacts"`
}

type rawArtifact struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Segment string `json:"fileIdentifyingUrlPathSegment"`
}

func applyCore(p *model.Profile, raw json.RawMessage, html []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var env coreEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	d := env.Data
	if d.PublicIdentifier != "" {
		p.PublicIdentifier = d.PublicIdentifier
	}
	p.FirstName = oneLine(d.FirstName)
	p.LastName = oneLine(d.LastName)
	p.FullName = oneLine(d.FirstName + " " + d.LastName)
	p.Headline = oneLine(d.Headline)
	p.About = normalizeText(d.Summary)
	p.Premium = d.Premium
	p.Influencer = d.Influencer
	p.Verified = d.ShowVerification
	p.Location.CountryCode = d.Location.CountryCode
	if loc := locationFromHTML(html, d.Headline); loc != "" {
		p.Location.Full = loc
	}
	p.ProfilePicture = buildImage(d.ProfilePicture)
	p.BackgroundPicture = buildImage(d.BackgroundPicture)
	return nil
}

func buildImage(pic *rawPicture) *model.Image {
	if pic == nil || pic.DisplayImage.VectorImage == nil {
		return nil
	}
	vi := pic.DisplayImage.VectorImage
	if vi.RootURL == "" || len(vi.Artifacts) == 0 {
		return nil
	}
	arts := append([]rawArtifact(nil), vi.Artifacts...)
	sort.Slice(arts, func(i, j int) bool { return arts[i].Width > arts[j].Width })
	out := &model.Image{}
	for _, a := range arts {
		if a.Segment == "" {
			continue
		}
		out.Variants = append(out.Variants, model.ImageVariant{
			URL:    vi.RootURL + a.Segment,
			Width:  a.Width,
			Height: a.Height,
		})
	}
	if len(out.Variants) == 0 {
		return nil
	}
	out.URL = out.Variants[0].URL
	return out
}

var (
	// A text node in the RSC stream / server-rendered DOM.
	childrenRe = regexp.MustCompile(`"children":\["([^"\[\]]{2,120})"\]`)
	tagTextRe  = regexp.MustCompile(`>([\p{L}][^<>{}]{2,100}, [^<>{}]{2,100})</`)
	locNoise   = regexp.MustCompile(`(?i)connection|follower|contact info|mutual|message|linkedin|premium|verified`)
)

// locationFromHTML extracts the top-card location string. The top card renders
// text nodes in order: name, headline, location, "Contact info" — so the
// location is the text node immediately following the headline.
func locationFromHTML(html []byte, headline string) string {
	if len(html) == 0 {
		return ""
	}
	window := html
	if len(window) > 500_000 {
		window = window[:500_000]
	}
	// Undo the backslash-escaping used inside the embedded RSC string literal.
	s := strings.ReplaceAll(string(window), `\"`, `"`)

	// Anchor on the headline text node when we can: the location renders as the
	// very next text node. Use a prefix of the headline up to the first "&" or
	// "|" so we don't have to reproduce LinkedIn's unicode escaping.
	if anchor := headlineAnchor(headline); anchor != "" {
		if i := strings.Index(s, `"children":["`+anchor); i >= 0 {
			if m := childrenRe.FindStringSubmatch(s[i+len(anchor):]); m != nil {
				if cand := cleanLoc(m[1], headline); cand != "" {
					return cand
				}
			}
		}
	}

	// Fallback: first comma-separated place-like text node, then DOM text.
	for _, m := range childrenRe.FindAllStringSubmatch(s, -1) {
		if cand := cleanLoc(m[1], headline); cand != "" && strings.Count(cand, ",") >= 1 && !strings.Contains(cand, " and ") {
			return cand
		}
	}
	for _, m := range tagTextRe.FindAllStringSubmatch(s, -1) {
		if cand := cleanLoc(m[1], headline); cand != "" && !strings.Contains(cand, " and ") {
			return cand
		}
	}
	return ""
}

func headlineAnchor(headline string) string {
	if headline == "" {
		return ""
	}
	a := headline
	if i := strings.IndexAny(a, "&|"); i > 0 {
		a = a[:i]
	}
	a = strings.TrimSpace(a)
	if len(a) < 6 {
		return ""
	}
	return a
}

func cleanLoc(s, headline string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == headline || len(s) > 120 {
		return ""
	}
	if strings.ContainsAny(s, "|") || locNoise.MatchString(s) {
		return ""
	}
	if headline != "" && (strings.Contains(headline, s) || strings.Contains(s, headline)) {
		return ""
	}
	return s
}

// normalizeText normalises LinkedIn's \r\n line endings for multi-line fields
// (about, descriptions) while preserving paragraph breaks.
func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

// oneLine flattens a value that should be a single line: stray line breaks that
// members sometimes paste into titles/names ("1\rst position") are collapsed to
// single spaces.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\r\n\t") {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// ---------- shared date helpers ----------

type rawDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type rawDateRange struct {
	Start *rawDate `json:"start"`
	End   *rawDate `json:"end"`
}

func ym(d *rawDate) *model.YearMonth {
	if d == nil || d.Year == 0 {
		return nil
	}
	return &model.YearMonth{Year: d.Year, Month: d.Month}
}

func dr(r *rawDateRange) *model.DateRange {
	if r == nil {
		return nil
	}
	out := &model.DateRange{Start: ym(r.Start), End: ym(r.End)}
	if out.Start == nil && out.End == nil {
		return nil
	}
	return out
}

// each decodes a section collection and calls fn for every entity of typeName.
func each(raw json.RawMessage, typeName string, fn func(json.RawMessage)) {
	if len(raw) == 0 {
		return
	}
	col, err := linkedin.ParseCollection(raw)
	if err != nil {
		return
	}
	for _, e := range col.Ordered(typeName) {
		fn(e)
	}
}

// ---------- sections ----------

func applyExperience(p *model.Profile, raw json.RawMessage) {
	type rawPos struct {
		Title           string        `json:"title"`
		CompanyName     string        `json:"companyName"`
		CompanyURN      string        `json:"companyUrn"`
		Description     string        `json:"description"`
		LocationName    string        `json:"locationName"`
		GeoLocationName string        `json:"geoLocationName"`
		DateRange       *rawDateRange `json:"dateRange"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Position", func(e json.RawMessage) {
		var r rawPos
		if json.Unmarshal(e, &r) != nil {
			return
		}
		loc := r.LocationName
		if loc == "" {
			loc = r.GeoLocationName
		}
		pos := model.Position{
			Title:       oneLine(r.Title),
			CompanyName: oneLine(r.CompanyName),
			CompanyURN:  r.CompanyURN,
			Location:    oneLine(loc),
			Description: normalizeText(r.Description),
			DateRange:   dr(r.DateRange),
		}
		pos.Current = r.DateRange != nil && r.DateRange.Start != nil && r.DateRange.End == nil
		p.Experience = append(p.Experience, pos)
	})
}

func applyEducation(p *model.Profile, raw json.RawMessage) {
	type rawEdu struct {
		SchoolName   string        `json:"schoolName"`
		SchoolURN    string        `json:"schoolUrn"`
		DegreeName   string        `json:"degreeName"`
		FieldOfStudy string        `json:"fieldOfStudy"`
		Grade        string        `json:"grade"`
		Activities   string        `json:"activities"`
		Description  string        `json:"description"`
		DateRange    *rawDateRange `json:"dateRange"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Education", func(e json.RawMessage) {
		var r rawEdu
		if json.Unmarshal(e, &r) != nil {
			return
		}
		p.Education = append(p.Education, model.Education{
			SchoolName:   oneLine(r.SchoolName),
			SchoolURN:    r.SchoolURN,
			DegreeName:   oneLine(r.DegreeName),
			FieldOfStudy: oneLine(r.FieldOfStudy),
			Grade:        oneLine(r.Grade),
			Activities:   normalizeText(r.Activities),
			Description:  normalizeText(r.Description),
			DateRange:    dr(r.DateRange),
		})
	})
}

func applySkills(p *model.Profile, raw json.RawMessage) {
	type rawSkill struct {
		Name string `json:"name"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Skill", func(e json.RawMessage) {
		var r rawSkill
		if json.Unmarshal(e, &r) != nil || oneLine(r.Name) == "" {
			return
		}
		p.Skills = append(p.Skills, model.Skill{Name: oneLine(r.Name)})
	})
}

func applyCertifications(p *model.Profile, raw json.RawMessage) {
	type rawCert struct {
		Name          string        `json:"name"`
		Authority     string        `json:"authority"`
		LicenseNumber string        `json:"licenseNumber"`
		URL           string        `json:"url"`
		DateRange     *rawDateRange `json:"dateRange"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Certification", func(e json.RawMessage) {
		var r rawCert
		if json.Unmarshal(e, &r) != nil {
			return
		}
		p.Certifications = append(p.Certifications, model.Certification{
			Name:          oneLine(r.Name),
			Authority:     oneLine(r.Authority),
			LicenseNumber: oneLine(r.LicenseNumber),
			URL:           strings.TrimSpace(r.URL),
			DateRange:     dr(r.DateRange),
		})
	})
}

func applyLanguages(p *model.Profile, raw json.RawMessage) {
	type rawLang struct {
		Name        string `json:"name"`
		Proficiency string `json:"proficiency"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Language", func(e json.RawMessage) {
		var r rawLang
		if json.Unmarshal(e, &r) != nil || oneLine(r.Name) == "" {
			return
		}
		p.Languages = append(p.Languages, model.Language{
			Name:        oneLine(r.Name),
			Proficiency: humanizeEnum(r.Proficiency),
		})
	})
}

func applyProjects(p *model.Profile, raw json.RawMessage) {
	type rawProj struct {
		Title       string        `json:"title"`
		Name        string        `json:"name"`
		Description string        `json:"description"`
		URL         string        `json:"url"`
		DateRange   *rawDateRange `json:"dateRange"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Project", func(e json.RawMessage) {
		var r rawProj
		if json.Unmarshal(e, &r) != nil {
			return
		}
		name := r.Title
		if name == "" {
			name = r.Name
		}
		p.Projects = append(p.Projects, model.Project{
			Name:        oneLine(name),
			Description: normalizeText(r.Description),
			URL:         strings.TrimSpace(r.URL),
			DateRange:   dr(r.DateRange),
		})
	})
}

func applyVolunteering(p *model.Profile, raw json.RawMessage) {
	type rawVol struct {
		Role        string        `json:"role"`
		CompanyName string        `json:"companyName"`
		Cause       string        `json:"cause"`
		Description string        `json:"description"`
		DateRange   *rawDateRange `json:"dateRange"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.VolunteerExperience", func(e json.RawMessage) {
		var r rawVol
		if json.Unmarshal(e, &r) != nil {
			return
		}
		p.Volunteering = append(p.Volunteering, model.Volunteering{
			Role:         oneLine(r.Role),
			Organization: oneLine(r.CompanyName),
			Cause:        humanizeEnum(r.Cause),
			Description:  normalizeText(r.Description),
			DateRange:    dr(r.DateRange),
		})
	})
}

func applyHonors(p *model.Profile, raw json.RawMessage) {
	type rawHonor struct {
		Title       string   `json:"title"`
		Issuer      string   `json:"issuer"`
		Description string   `json:"description"`
		IssuedOn    *rawDate `json:"issuedOn"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Honor", func(e json.RawMessage) {
		var r rawHonor
		if json.Unmarshal(e, &r) != nil {
			return
		}
		p.Honors = append(p.Honors, model.Honor{
			Title:       oneLine(r.Title),
			Issuer:      oneLine(r.Issuer),
			Description: normalizeText(r.Description),
			Date:        ym(r.IssuedOn),
		})
	})
}

func applyPublications(p *model.Profile, raw json.RawMessage) {
	type rawPub struct {
		Name        string   `json:"name"`
		Publisher   string   `json:"publisher"`
		Description string   `json:"description"`
		URL         string   `json:"url"`
		PublishedOn *rawDate `json:"publishedOn"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Publication", func(e json.RawMessage) {
		var r rawPub
		if json.Unmarshal(e, &r) != nil {
			return
		}
		p.Publications = append(p.Publications, model.Publication{
			Name:        oneLine(r.Name),
			Publisher:   oneLine(r.Publisher),
			Description: normalizeText(r.Description),
			URL:         strings.TrimSpace(r.URL),
			Date:        ym(r.PublishedOn),
		})
	})
}

func applyCourses(p *model.Profile, raw json.RawMessage) {
	type rawCourse struct {
		Name   string `json:"name"`
		Number string `json:"number"`
	}
	each(raw, "com.linkedin.voyager.dash.identity.profile.Course", func(e json.RawMessage) {
		var r rawCourse
		if json.Unmarshal(e, &r) != nil || oneLine(r.Name) == "" {
			return
		}
		p.Courses = append(p.Courses, model.Course{
			Name:   oneLine(r.Name),
			Number: oneLine(r.Number),
		})
	})
}

// humanizeEnum turns "NATIVE_OR_BILINGUAL" into "Native or bilingual".
func humanizeEnum(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, " ") {
		return s
	}
	low := strings.ToLower(strings.ReplaceAll(s, "_", " "))
	if low == "" {
		return ""
	}
	return strings.ToUpper(low[:1]) + low[1:]
}
