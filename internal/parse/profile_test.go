package parse

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Palak22115/linkedin-profile-api/internal/linkedin"
)

// fixtures below mirror the real identity/dash/* response shapes verified
// during recon (see recon/ dumps).

const coreFixture = `{"data":{
  "publicIdentifier":"jane-doe-123",
  "firstName":"Jane","lastName":"Doe",
  "headline":"Engineer | Builder",
  "summary":"Line one.\r\nLine two.",
  "premium":true,"influencer":false,"showVerificationBadge":true,
  "location":{"countryCode":"US"},
  "profilePicture":{"displayImage":{"vectorImage":{
    "rootUrl":"https://media.licdn.com/dms/image/v2/ABC/profile-displayphoto-shrink_",
    "artifacts":[
      {"width":100,"height":100,"fileIdentifyingUrlPathSegment":"100_100/x?e=1"},
      {"width":800,"height":800,"fileIdentifyingUrlPathSegment":"800_800/x?e=1"},
      {"width":400,"height":400,"fileIdentifyingUrlPathSegment":"400_400/x?e=1"}
    ]}}}
}}`

const positionsFixture = `{"data":{"*elements":[
  "urn:li:fsd_profilePosition:(P,1)","urn:li:fsd_profilePosition:(P,2)"]},
 "included":[
  {"$type":"com.linkedin.voyager.dash.identity.profile.Position","entityUrn":"urn:li:fsd_profilePosition:(P,2)",
   "title":"Founder","companyName":"NewCo","companyUrn":"urn:li:fsd_company:2",
   "dateRange":{"start":{"year":2020,"month":1},"end":null}},
  {"$type":"com.linkedin.voyager.dash.identity.profile.Position","entityUrn":"urn:li:fsd_profilePosition:(P,1)",
   "title":"Engineer","companyName":"OldCo","companyUrn":"urn:li:fsd_company:1",
   "locationName":"Seattle","description":"Did things.",
   "dateRange":{"start":{"year":2016,"month":6},"end":{"year":2019,"month":12}}}
 ]}`

const skillsFixture = `{"data":{"*elements":["urn:li:fsd_skill:(S,9)","urn:li:fsd_skill:(S,1)"]},
 "included":[
  {"$type":"com.linkedin.voyager.dash.identity.profile.Skill","entityUrn":"urn:li:fsd_skill:(S,1)","name":"Go"},
  {"$type":"com.linkedin.voyager.dash.identity.profile.Skill","entityUrn":"urn:li:fsd_skill:(S,9)","name":"Distributed Systems"}
 ]}`

const languagesFixture = `{"data":{"*elements":["urn:li:fsd_profileLanguage:(L,1)"]},
 "included":[
  {"$type":"com.linkedin.voyager.dash.identity.profile.Language","entityUrn":"urn:li:fsd_profileLanguage:(L,1)","name":"English","proficiency":"NATIVE_OR_BILINGUAL"}
 ]}`

func mkRaw(parts map[string]string, html string) *linkedin.RawProfile {
	m := map[string]json.RawMessage{}
	for k, v := range parts {
		m[k] = json.RawMessage(v)
	}
	return &linkedin.RawProfile{
		Resolved: &linkedin.Resolved{Vanity: "jane-doe-123", ID: "ACoAAX", HTML: []byte(html)},
		Parts:    m,
	}
}

func TestBuild(t *testing.T) {
	raw := mkRaw(map[string]string{
		"core":             coreFixture,
		"profilePositions": positionsFixture,
		"profileEducations": `{"data":{"*elements":["urn:li:fsd_profileEducation:(E,1)"]},"included":[
		 {"$type":"com.linkedin.voyager.dash.identity.profile.Education","entityUrn":"urn:li:fsd_profileEducation:(E,1)",
		  "schoolName":"MIT","degreeName":"BSc","fieldOfStudy":"CS","dateRange":{"start":{"year":2012},"end":{"year":2016}}}]}`,
		"profileSkills":    skillsFixture,
		"profileLanguages": languagesFixture,
	}, `<p>Engineer | Builder</p><p class="x">Seattle, Washington, United States</p><p>Contact info</p>`)

	p, err := Build(raw, "https://www.linkedin.com/in/jane-doe-123/")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if p.FullName != "Jane Doe" || p.Headline != "Engineer | Builder" {
		t.Errorf("identity wrong: %+v", p)
	}
	if p.About != "Line one.\nLine two." {
		t.Errorf("about not normalized: %q", p.About)
	}
	if !p.Verified || !p.Premium {
		t.Errorf("badges wrong: verified=%v premium=%v", p.Verified, p.Premium)
	}
	if p.Location.Full != "Seattle, Washington, United States" || p.Location.CountryCode != "US" {
		t.Errorf("location wrong: %+v", p.Location)
	}

	// Profile picture: variants sorted largest-first, URL = rootUrl + segment.
	if p.ProfilePicture == nil || len(p.ProfilePicture.Variants) != 3 {
		t.Fatalf("picture missing/short: %+v", p.ProfilePicture)
	}
	if p.ProfilePicture.Variants[0].Width != 800 {
		t.Errorf("variants not sorted: %+v", p.ProfilePicture.Variants)
	}
	want := "https://media.licdn.com/dms/image/v2/ABC/profile-displayphoto-shrink_800_800/x?e=1"
	if p.ProfilePicture.URL != want {
		t.Errorf("picture URL = %q, want %q", p.ProfilePicture.URL, want)
	}

	// Experience: *elements order (P,1 then P,2), current flag on the open one.
	if len(p.Experience) != 2 {
		t.Fatalf("experience count = %d", len(p.Experience))
	}
	if p.Experience[0].Title != "Engineer" || p.Experience[0].Current {
		t.Errorf("exp[0] wrong: %+v", p.Experience[0])
	}
	if p.Experience[1].Title != "Founder" || !p.Experience[1].Current {
		t.Errorf("exp[1] should be current: %+v", p.Experience[1])
	}
	if p.Experience[0].DateRange == nil || p.Experience[0].DateRange.End.Year != 2019 {
		t.Errorf("exp[0] dateRange wrong: %+v", p.Experience[0].DateRange)
	}

	// Education
	if len(p.Education) != 1 || p.Education[0].SchoolName != "MIT" || p.Education[0].FieldOfStudy != "CS" {
		t.Errorf("education wrong: %+v", p.Education)
	}

	// Skills: *elements order => Distributed Systems, Go
	if len(p.Skills) != 2 || p.Skills[0].Name != "Distributed Systems" || p.Skills[1].Name != "Go" {
		t.Errorf("skills wrong/order: %+v", p.Skills)
	}

	// Languages: enum humanized
	if len(p.Languages) != 1 || p.Languages[0].Proficiency != "Native or bilingual" {
		t.Errorf("languages wrong: %+v", p.Languages)
	}
}

// Fixtures below are trimmed from real identity/dash/* responses captured
// during recon (certifications, projects, courses, honors — the sections that
// only appear on richer profiles).
func TestBuild_RicherSections(t *testing.T) {
	raw := mkRaw(map[string]string{
		"core": coreFixture,
		"profileCertifications": `{"data":{"*elements":["urn:li:fsd_profileCertification:(C,1)"]},"included":[
		 {"$type":"com.linkedin.voyager.dash.identity.profile.Certification","entityUrn":"urn:li:fsd_profileCertification:(C,1)",
		  "name":"ISO 45001: 2018 ... Certified","authority":"TÜV SÜD","licenseNumber":"IN/18079/197576","url":null,
		  "dateRange":{"start":{"month":10,"year":2022}}}]}`,
		"profileProjects": `{"data":{"*elements":["urn:li:fsd_profileProject:(P,1)"]},"included":[
		 {"$type":"com.linkedin.voyager.dash.identity.profile.Project","entityUrn":"urn:li:fsd_profileProject:(P,1)",
		  "title":"HR policy and implementation at BPO","description":"Key learnings\r\n- a\r\n- b","dateRange":null}]}`,
		"profileCourses": `{"data":{"*elements":["urn:li:fsd_profileCourse:(K,1)"]},"included":[
		 {"$type":"com.linkedin.voyager.dash.identity.profile.Course","entityUrn":"urn:li:fsd_profileCourse:(K,1)",
		  "name":"Financial Accounting from ILLINOIS","number":null}]}`,
		"profileHonors": `{"data":{"*elements":["urn:li:fsd_profileHonor:(H,1)"]},"included":[
		 {"$type":"com.linkedin.voyager.dash.identity.profile.Honor","entityUrn":"urn:li:fsd_profileHonor:(H,1)",
		  "title":"1\rst position\r\nin  Volleyball tournament","issuer":null,"issuedOn":{"month":3,"year":2022}}]}`,
	}, "")

	p, err := Build(raw, "u")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Certifications) != 1 || p.Certifications[0].Authority != "TÜV SÜD" ||
		p.Certifications[0].LicenseNumber != "IN/18079/197576" || p.Certifications[0].DateRange.Start.Month != 10 {
		t.Errorf("certification: %+v", p.Certifications)
	}
	if len(p.Projects) != 1 || p.Projects[0].Name != "HR policy and implementation at BPO" ||
		!strings.Contains(p.Projects[0].Description, "\n- a\n") {
		t.Errorf("project: %+v", p.Projects)
	}
	if len(p.Courses) != 1 || p.Courses[0].Name != "Financial Accounting from ILLINOIS" {
		t.Errorf("course: %+v", p.Courses)
	}
	if len(p.Honors) != 1 || p.Honors[0].Date == nil || p.Honors[0].Date.Year != 2022 {
		t.Errorf("honor: %+v", p.Honors)
	}
	// stray \r / \r\n and double spaces that members paste into one-line
	// fields are flattened to single spaces
	if p.Honors[0].Title != "1 st position in Volleyball tournament" {
		t.Errorf("honor title not flattened: %q", p.Honors[0].Title)
	}
}

func TestBuild_MissingSectionsFlagged(t *testing.T) {
	raw := mkRaw(map[string]string{"core": coreFixture}, "")
	p, err := Build(raw, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Partial || len(p.Notes) == 0 {
		t.Errorf("expected Partial with notes, got %+v / %v", p.Partial, p.Notes)
	}
	// Slices must be non-nil for stable JSON ([] not null).
	if p.Experience == nil || p.Skills == nil || p.Certifications == nil {
		t.Errorf("expected empty non-nil slices")
	}
}

func TestParseVanity(t *testing.T) {
	cases := map[string]string{
		"https://www.linkedin.com/in/prashant-ravi-a5a688b3/":    "prashant-ravi-a5a688b3",
		"linkedin.com/in/john-doe":                               "john-doe",
		"https://www.linkedin.com/in/jane/details/experience/":   "jane",
		"http://in.linkedin.com/in/foo-bar?originalSubdomain=in": "foo-bar",
		"jane-doe-123": "jane-doe-123",
		"https://www.linkedin.com/in/%C3%A9lise-martin/": "élise-martin",
	}
	for in, want := range cases {
		got, err := linkedin.ParseVanity(in)
		if err != nil || got != want {
			t.Errorf("ParseVanity(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "https://example.com/in/foo", "https://www.linkedin.com/company/acme/"} {
		if _, err := linkedin.ParseVanity(bad); err == nil {
			t.Errorf("ParseVanity(%q) expected error", bad)
		}
	}
}
