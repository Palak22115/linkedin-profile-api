// Command probe exercises the LinkedIn client end to end against one profile
// URL and dumps the raw Voyager payloads. Development/recon tool only.
//
//	go run ./cmd/probe "https://www.linkedin.com/in/prashant-ravi-a5a688b3/"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/palak-kasoundhan/linkedin-profile-api/internal/config"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/linkedin"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/parse"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe <linkedin profile url or vanity>")
		os.Exit(2)
	}
	input := os.Args[1]

	cfg, err := config.Load()
	must(err)

	c := linkedin.New(cfg.Cookie, cfg.CSRFToken(), cfg.UserAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	vanity, err := linkedin.ParseVanity(input)
	must(err)
	fmt.Println("vanity: ", vanity)

	res, err := c.Resolve(ctx, vanity)
	must(err)
	fmt.Println("id:     ", res.ID)
	fmt.Println("urn:    ", res.URN())
	fmt.Printf("html:    %d bytes\n\n", len(res.HTML))

	raw, err := c.FetchAll(ctx, res)
	must(err)

	for name, part := range raw.Parts {
		summarize(name, part)
	}

	// Persist the full dump for offline schema work.
	_ = os.MkdirAll("recon", 0o755)
	dump := map[string]json.RawMessage{}
	for k, v := range raw.Parts {
		dump[k] = v
	}
	f, _ := os.Create("recon/probe_out.json")
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(dump)
	f.Close()
	fmt.Println("\nwrote recon/probe_out.json")

	// Parse into the public schema and print it.
	profile, err := parse.Build(raw, "https://www.linkedin.com/in/"+res.Vanity+"/")
	must(err)
	fmt.Println("\n--- parsed model.Profile ---")
	out, _ := json.MarshalIndent(profile, "", "  ")
	fmt.Println(string(out))
}

// summarize prints a one-line shape summary of a Voyager collection/entity.
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
	switch name {
	case "core":
		fmt.Printf("  %-28s firstName=%q headline=%q included=%d\n", name, v.Data.FirstName, trunc(v.Data.Headline, 40), len(v.Included))
	default:
		n := len(v.Data.Elements)
		if n == 0 {
			n = len(v.Data.Star)
		}
		fmt.Printf("  %-28s elements=%d total=%d included=%d\n", name, n, v.Data.Paging.Total, len(v.Included))
	}
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
