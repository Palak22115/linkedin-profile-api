// Package model defines the public JSON response schema for the API.
package model

import "time"

// Profile is the structured representation of a LinkedIn member profile.
type Profile struct {
	PublicIdentifier  string   `json:"publicIdentifier"`
	ProfileID         string   `json:"profileId"`
	FirstName         string   `json:"firstName"`
	LastName          string   `json:"lastName"`
	FullName          string   `json:"fullName"`
	Headline          string   `json:"headline,omitempty"`
	About             string   `json:"about,omitempty"`
	Location          Location `json:"location"`
	Industry          string   `json:"industry,omitempty"`
	ProfilePicture    *Image   `json:"profilePicture,omitempty"`
	BackgroundPicture *Image   `json:"backgroundPicture,omitempty"`
	Verified          bool     `json:"verified"`
	Premium           bool     `json:"premium"`
	Influencer        bool     `json:"influencer"`
	FollowerCount     int      `json:"followerCount,omitempty"`
	ConnectionCount   int      `json:"connectionCount,omitempty"`

	Experience     []Position      `json:"experience"`
	Education      []Education     `json:"education"`
	Skills         []Skill         `json:"skills"`
	Certifications []Certification `json:"certifications"`
	Languages      []Language      `json:"languages"`
	Projects       []Project       `json:"projects,omitempty"`
	Volunteering   []Volunteering  `json:"volunteering,omitempty"`
	Honors         []Honor         `json:"honors,omitempty"`
	Publications   []Publication   `json:"publications,omitempty"`
	Courses        []Course        `json:"courses,omitempty"`

	SourceURL   string    `json:"sourceUrl"`
	RetrievedAt time.Time `json:"retrievedAt"`

	// Partial is set when one or more sections could not be retrieved (e.g.
	// hidden by LinkedIn for non-connections). Notes explains what is missing.
	Partial bool     `json:"partial"`
	Notes   []string `json:"notes,omitempty"`
}

// Location is a member's stated location.
type Location struct {
	Full        string `json:"full,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
}

// Image is a set of CDN URLs for one picture, ordered largest first.
type Image struct {
	URL      string         `json:"url"`
	Variants []ImageVariant `json:"variants,omitempty"`
}

// ImageVariant is a single rendition of an image.
type ImageVariant struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// YearMonth is a partial date; Month is 0 when unknown.
type YearMonth struct {
	Year  int `json:"year,omitempty"`
	Month int `json:"month,omitempty"`
}

// DateRange spans two partial dates. End is nil for ongoing items.
type DateRange struct {
	Start *YearMonth `json:"start,omitempty"`
	End   *YearMonth `json:"end,omitempty"`
}

// Position is one work experience entry.
type Position struct {
	Title          string     `json:"title,omitempty"`
	CompanyName    string     `json:"companyName,omitempty"`
	CompanyURN     string     `json:"companyUrn,omitempty"`
	EmploymentType string     `json:"employmentType,omitempty"`
	Location       string     `json:"location,omitempty"`
	Description    string     `json:"description,omitempty"`
	DateRange      *DateRange `json:"dateRange,omitempty"`
	Current        bool       `json:"current"`
}

// Education is one education entry.
type Education struct {
	SchoolName   string     `json:"schoolName,omitempty"`
	SchoolURN    string     `json:"schoolUrn,omitempty"`
	DegreeName   string     `json:"degreeName,omitempty"`
	FieldOfStudy string     `json:"fieldOfStudy,omitempty"`
	Grade        string     `json:"grade,omitempty"`
	Activities   string     `json:"activities,omitempty"`
	Description  string     `json:"description,omitempty"`
	DateRange    *DateRange `json:"dateRange,omitempty"`
}

// Skill is a single listed skill.
type Skill struct {
	Name string `json:"name"`
}

// Certification is one license or certification.
type Certification struct {
	Name          string     `json:"name,omitempty"`
	Authority     string     `json:"authority,omitempty"`
	LicenseNumber string     `json:"licenseNumber,omitempty"`
	URL           string     `json:"url,omitempty"`
	DateRange     *DateRange `json:"dateRange,omitempty"`
}

// Language is one listed language and proficiency.
type Language struct {
	Name        string `json:"name"`
	Proficiency string `json:"proficiency,omitempty"`
}

// Project is one listed project.
type Project struct {
	Name        string     `json:"name,omitempty"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url,omitempty"`
	DateRange   *DateRange `json:"dateRange,omitempty"`
}

// Volunteering is one volunteer experience.
type Volunteering struct {
	Role         string     `json:"role,omitempty"`
	Organization string     `json:"organization,omitempty"`
	Cause        string     `json:"cause,omitempty"`
	Description  string     `json:"description,omitempty"`
	DateRange    *DateRange `json:"dateRange,omitempty"`
}

// Honor is one honor or award.
type Honor struct {
	Title       string     `json:"title,omitempty"`
	Issuer      string     `json:"issuer,omitempty"`
	Description string     `json:"description,omitempty"`
	Date        *YearMonth `json:"date,omitempty"`
}

// Publication is one publication.
type Publication struct {
	Name        string     `json:"name,omitempty"`
	Publisher   string     `json:"publisher,omitempty"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url,omitempty"`
	Date        *YearMonth `json:"date,omitempty"`
}

// Course is one listed course.
type Course struct {
	Name   string `json:"name,omitempty"`
	Number string `json:"number,omitempty"`
}
