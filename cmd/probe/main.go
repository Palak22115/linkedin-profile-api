// Command probe exercises the LinkedIn client end to end against one profile
// URL and dumps the raw Voyager payloads. Development/recon tool only.
//
//	go run ./cmd/probe "https://www.linkedin.com/in/prashant-ravi-a5a688b3/"
//	go run ./cmd/probe @recon/probe_out.json   # offline: re-parse a saved dump
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Palak22115/linkedin-profile-api/internal/config"
	"github.com/Palak22115/linkedin-profile-api/internal/linkedin"
	"github.com/Palak22115/linkedin-profile-api/internal/parse"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe <linkedin url|vanity> | @<saved dump.json>")
		os.Exit(2)
	}
	arg := os.Args[1]

	var raw *linkedin.RawProfile
	var vanity string

	if strings.HasPrefix(arg, "@") {
		raw, vanity = loadDump(strings.TrimPrefix(arg, "@"))
	} else {
		raw, vanity = liveFetch(arg)
	}

	for name, part := range raw.Parts {
		summarize(name, part)
	}

	profile, err := parse.Build(raw, "https://www.linkedin.com/in/"+vanity+"/")
	must(err)

	fmt.Println("\n--- parsed section counts ---")
	fmt.Printf("  experience=%d education=%d skills=%d certifications=%d languages=%d\n",
		len(profile.Experience), len(profile.Education), len(profile.Skills), len(profile.Certifications), len(profile.Languages))
	fmt.Printf("  projects=%d volunteering=%d honors=%d publications=%d courses=%d\n",
		len(profile.Projects), len(profile.Volunteering), len(profile.Honors), len(profile.Publications), len(profile.Courses))

	fmt.Println("\n--- parsed model.Profile ---")
	out, _ := json.MarshalIndent(profile, "", "  ")
	fmt.Println(string(out))
}

func liveFetch(input string) (*linkedin.RawProfile, string) {
	cfg, err := config.Load()
	must(err)
	c := linkedin.New(cfg.Cookie, cfg.CSRFToken(), cfg.UserAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	vanity, err := linkedin.ParseVanity(input)
	must(err)
	fmt.Println("vanity: ", vanity)

	res, err := c.Resolve(ctx, vanity)
	must(err)
	fmt.Println("id:     ", res.ID)
	fmt.Printf("html:    %d bytes\n\n", len(res.HTML))

	raw, err := c.FetchAll(ctx, res)
	must(err)

	_ = os.MkdirAll("recon", 0o755)
	dump := struct {
		Vanity string                     `json:"_vanity"`
		ID     string                     `json:"_id"`
		Parts  map[string]json.RawMessage `json:"parts"`
	}{res.Vanity, res.ID, raw.Parts}
	f, _ := os.Create("recon/probe_out.json")
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(dump)
	f.Close()
	fmt.Println("wrote recon/probe_out.json")

	return raw, res.Vanity
}

func loadDump(path string) (*linkedin.RawProfile, string) {
	b, err := os.ReadFile(path)
	must(err)

	// Accept both the new {_vanity,_id,parts} shape and a bare {section: json}.
	var wrapped struct {
		Vanity string                     `json:"_vanity"`
		ID     string                     `json:"_id"`
		Parts  map[string]json.RawMessage `json:"parts"`
	}
	_ = json.Unmarshal(b, &wrapped)
	parts := wrapped.Parts
	if parts == nil {
		must(json.Unmarshal(b, &parts))
	}
	vanity := wrapped.Vanity
	if vanity == "" {
		vanity = "offline"
	}
	fmt.Printf("loaded %s  (%d sections, id=%q)\n\n", path, len(parts), wrapped.ID)
	return &linkedin.RawProfile{
		Resolved: &linkedin.Resolved{Vanity: vanity, ID: wrapped.ID},
		Parts:    parts,
	}, vanity
}

func summarize(name string, raw json.RawMessage) {
	var v struct {
		Data struct {
			Elements []json.RawMessage `json:"elements"`
			Star     []string          `json:"*elements"`
			Paging   struct {
				Total int `json:"total"`
			} `json:"paging"`
			FirstName string `json:"firstName"`
			Headline  string `json:"headline"`
		} `json:"data"`
		Included []json.RawMessage `json:"included"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Printf("  %-28s <unparseable: %v>\n", name, err)
		return
	}
	if name == "core" {
		fmt.Printf("  %-28s firstName=%q headline=%q\n", name, v.Data.FirstName, trunc(v.Data.Headline, 40))
		return
	}
	n := len(v.Data.Elements)
	if n == 0 {
		n = len(v.Data.Star)
	}
	fmt.Printf("  %-28s elements=%d total=%d included=%d\n", name, n, v.Data.Paging.Total, len(v.Included))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
