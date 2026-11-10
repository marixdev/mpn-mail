// Package basepath lets the program run under a path of its own, such as /webmail/, behind a reverse proxy that cuts the path off
// before it passes a request on. The proxy says which path it cut in the X-Forwarded-Prefix header; the value is taken for each
// request, so the same process can answer on a port of its own (no header, no prefix) and under a path at the same time.
//
// The program listens on the loopback address only, so the header comes from the proxy in front of it. It is still checked: a value
// that is not a single short lower-case path segment is taken as no prefix, so it can never put markup or another site's address
// into a page, a redirect or a cookie.
package basepath

import (
	"regexp"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// Header is the name of the header the proxy sets.
const Header = "X-Forwarded-Prefix"

const localsKey = "lilmail.basepath"

var valid = regexp.MustCompile(`^/[a-z0-9_-]{1,32}$`)

// Clean returns v when it is a prefix this program accepts (one segment, "/webmail"), and "" otherwise.
func Clean(v string) string {
	if valid.MatchString(v) {
		return v
	}
	return ""
}

// From returns the prefix of this request: "" or a value such as "/webmail". It never ends with a slash.
func From(c *fiber.Ctx) string {
	if v, ok := c.Locals(localsKey).(string); ok {
		return v
	}
	return Clean(c.Get(Header))
}

// Path puts the prefix of this request in front of an absolute path of the program: Path(c, "/inbox") is "/webmail/inbox" under the
// prefix and "/inbox" without it.
func Path(c *fiber.Ctx, p string) string { return From(c) + p }

// Middleware records the prefix for the request, hands it to every template as .Base (so a template writes {{$.Base}}/inbox), and
// moves the cookies the request sets under the prefix. A cookie with Path=/ would be sent to every page of the site the program is
// mounted on, including the site's own code, which has nothing to do with the mail. Put it before anything that renders or sets a cookie.
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		base := Clean(c.Get(Header))
		c.Locals(localsKey, base)
		_ = c.Bind(fiber.Map{"Base": base})
		err := c.Next()
		if base != "" {
			scopeCookies(c, base)
		}
		return err
	}
}

// scopeCookies rewrites the Set-Cookie headers of the response that say Path=/ (or no path) to say Path=<base>. A cookie with a path of
// its own is left alone. The path has no trailing slash: it then matches /webmail and /webmail/x, and not /webmailer.
func scopeCookies(c *fiber.Ctx, base string) {
	h := &c.Response().Header
	var cookies []*fasthttp.Cookie
	h.VisitAllCookie(func(_, value []byte) {
		ck := &fasthttp.Cookie{}
		if err := ck.ParseBytes(value); err == nil {
			cookies = append(cookies, ck)
		}
	})
	if len(cookies) == 0 {
		return
	}
	h.DelAllCookies()
	for _, ck := range cookies {
		if p := string(ck.Path()); p == "" || p == "/" {
			ck.SetPath(base)
		}
		h.SetCookie(ck)
	}
}
