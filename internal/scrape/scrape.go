// Package scrape orchestrates a full profile fetch: resolve the URL, pull the
// Voyager payloads, parse them into the public schema, and cache the result.
package scrape

import (
	"context"
	"sync"
	"time"

	"github.com/palak-kasoundhan/linkedin-profile-api/internal/linkedin"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/model"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/parse"
)

// Scraper turns a LinkedIn profile URL into a model.Profile.
type Scraper struct {
	client *linkedin.Client
	cache  *cache
}

// New builds a Scraper. A ttl of 0 disables caching.
func New(client *linkedin.Client, ttl time.Duration) *Scraper {
	return &Scraper{client: client, cache: newCache(ttl)}
}

// Profile fetches and parses the profile identified by input (a LinkedIn
// profile URL or bare vanity slug).
func (s *Scraper) Profile(ctx context.Context, input string) (*model.Profile, error) {
	vanity, err := linkedin.ParseVanity(input)
	if err != nil {
		return nil, err
	}
	if p, ok := s.cache.get(vanity); ok {
		return p, nil
	}

	res, err := s.client.Resolve(ctx, vanity)
	if err != nil {
		return nil, err
	}
	raw, err := s.client.FetchAll(ctx, res)
	if err != nil {
		return nil, err
	}
	profile, err := parse.Build(raw, "https://www.linkedin.com/in/"+res.Vanity+"/")
	if err != nil {
		return nil, err
	}

	s.cache.set(vanity, profile)
	return profile, nil
}

// ---- cache ----

type cacheEntry struct {
	profile *model.Profile
	expires time.Time
}

type cache struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]cacheEntry
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, m: make(map[string]cacheEntry)}
}

func (c *cache) get(key string) (*model.Profile, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.profile, true
}

func (c *cache) set(key string, p *model.Profile) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.m[key] = cacheEntry{profile: p, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
