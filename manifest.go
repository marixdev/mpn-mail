package main

import (
	"encoding/json"
	"lilmail/basepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// prefixManifest puts the prefix in front of the start page, the scope and the icons of the web manifest, which name them by absolute
// path. With no prefix the file is returned as it is.
func prefixManifest(raw []byte, base string) ([]byte, error) {
	if base == "" {
		return raw, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for _, key := range []string{"start_url", "scope", "id"} {
		if v, ok := m[key].(string); ok && strings.HasPrefix(v, "/") {
			m[key] = base + v
		}
	}
	if icons, ok := m["icons"].([]any); ok {
		for _, i := range icons {
			if icon, ok := i.(map[string]any); ok {
				if src, ok := icon["src"].(string); ok && strings.HasPrefix(src, "/") {
					icon["src"] = base + src
				}
			}
		}
	}
	return json.MarshalIndent(m, "", "  ")
}

// serveManifest answers /assets/site.webmanifest for the prefix of this request.
func serveManifest(c *fiber.Ctx) error {
	raw, err := assetsFS.ReadFile("assets/site.webmanifest")
	if err != nil {
		return fiber.ErrNotFound
	}
	out, err := prefixManifest(raw, basepath.From(c))
	if err != nil {
		return fiber.ErrInternalServerError
	}
	c.Set("Content-Type", "application/manifest+json")
	c.Set("Cache-Control", "no-cache") // the content depends on the prefix of the request
	return c.Send(out)
}
