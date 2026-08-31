package linkedin

import "encoding/json"

// Collection is the decoded shape of a Voyager Rest.li collection response:
// a list of element URNs (giving order) plus a bag of decorated entities in
// the "included" array.
type Collection struct {
	ElementURNs []string
	Included    []json.RawMessage
}

// collectionEnvelope matches the wire format of an identity/dash/* collection.
type collectionEnvelope struct {
	Data struct {
		StarElements []string          `json:"*elements"`
		Elements     []json.RawMessage `json:"elements"`
	} `json:"data"`
	Included []json.RawMessage `json:"included"`
}

// ParseCollection decodes a collection response. When the entities are inlined
// in data.elements (rather than data.*elements + included) they are used directly.
func ParseCollection(raw json.RawMessage) (*Collection, error) {
	var env collectionEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	c := &Collection{ElementURNs: env.Data.StarElements}
	switch {
	case len(env.Included) > 0:
		c.Included = env.Included
	case len(env.Data.Elements) > 0:
		c.Included = env.Data.Elements
	}
	return c, nil
}

// entityHeader is the minimal set of fields present on every decorated entity.
type entityHeader struct {
	Type      string `json:"$type"`
	EntityURN string `json:"entityUrn"`
}

// Ordered returns the included entities of the given $type. When element URNs
// are present the result follows that order; otherwise natural order is kept.
func (c *Collection) Ordered(typeName string) []json.RawMessage {
	byURN := make(map[string]json.RawMessage, len(c.Included))
	var natural []json.RawMessage
	for _, raw := range c.Included {
		var h entityHeader
		if err := json.Unmarshal(raw, &h); err != nil || h.Type != typeName {
			continue
		}
		if h.EntityURN != "" {
			byURN[h.EntityURN] = raw
		}
		natural = append(natural, raw)
	}
	if len(c.ElementURNs) == 0 || len(byURN) == 0 {
		return natural
	}
	out := make([]json.RawMessage, 0, len(c.ElementURNs))
	seen := make(map[string]bool)
	for _, urn := range c.ElementURNs {
		if raw, ok := byURN[urn]; ok && !seen[urn] {
			out = append(out, raw)
			seen[urn] = true
		}
	}
	// Append any entities of this type that weren't referenced by *elements.
	for urn, raw := range byURN {
		if !seen[urn] {
			out = append(out, raw)
		}
	}
	if len(out) == 0 {
		return natural
	}
	return out
}
