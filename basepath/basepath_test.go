package basepath

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOnlyAShortSingleSegmentIsAPrefix(t *testing.T) {
	for _, ok := range []string{"/webmail", "/mail", "/a", "/web-mail_2"} {
		if Clean(ok) != ok {
			t.Errorf("%q must be accepted", ok)
		}
	}
	for _, bad := range []string{"", "/", "webmail", "/webmail/", "/web/mail", "/Webmail", "/web mail", "/<script>", `/"onload=x`, "//evil.com", "/..", "/a?b", "/a#b", "https://evil.com", "/" + strings.Repeat("a", 33), "/web\r\nSet-Cookie: x=1"} {
		if Clean(bad) != "" {
			t.Errorf("%q must be taken as no prefix", bad)
		}
	}
}

func appWith() *fiber.App {
	app := fiber.New()
	app.Use(Middleware())
	app.Get("/where", func(c *fiber.Ctx) error { return c.SendString(From(c) + "|" + Path(c, "/inbox")) })
	app.Get("/cookies", func(c *fiber.Ctx) error {
		c.Cookie(&fiber.Cookie{Name: "session_id", Value: "s", Path: "/", HTTPOnly: true, Secure: true, SameSite: "Lax"})
		c.Cookie(&fiber.Cookie{Name: "_csrf", Value: "c", HTTPOnly: false, SameSite: "Lax"}) // no path at all
		c.Cookie(&fiber.Cookie{Name: "other", Value: "o", Path: "/elsewhere"})
		return c.SendString("ok")
	})
	return app
}

func get(t *testing.T, app *fiber.App, path, prefix string) (string, []string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if prefix != "" {
		req.Header.Set(Header, prefix)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b), resp.Header.Values("Set-Cookie")
}

func TestThePrefixComesFromTheHeaderOfEachRequest(t *testing.T) {
	app := appWith()
	if body, _ := get(t, app, "/where", ""); body != "|/inbox" {
		t.Errorf("without the header nothing changes: %q", body)
	}
	if body, _ := get(t, app, "/where", "/webmail"); body != "/webmail|/webmail/inbox" {
		t.Errorf("with the header: %q", body)
	}
	if body, _ := get(t, app, "/where", "/webmail/../x"); body != "|/inbox" {
		t.Errorf("a value that is not a prefix is ignored: %q", body)
	}
	if body, _ := get(t, app, "/where", ""); body != "|/inbox" {
		t.Errorf("the next request is not affected by the one before: %q", body)
	}
}

func TestCookiesMoveUnderThePrefixAndKeepTheirFlags(t *testing.T) {
	app := appWith()
	_, plain := get(t, app, "/cookies", "")
	for _, c := range plain {
		if strings.Contains(c, "/webmail") {
			t.Errorf("no prefix, no change: %s", c)
		}
	}
	_, under := get(t, app, "/cookies", "/webmail")
	if len(under) != 3 {
		t.Fatalf("three cookies expected: %v", under)
	}
	byName := map[string]string{}
	for _, c := range under {
		byName[strings.SplitN(c, "=", 2)[0]] = c
	}
	for _, name := range []string{"session_id", "_csrf"} {
		if !strings.Contains(byName[name], "path=/webmail") && !strings.Contains(byName[name], "Path=/webmail") {
			t.Errorf("%s must belong to /webmail only: %s", name, byName[name])
		}
		if strings.Contains(byName[name], "ath=/;") || strings.HasSuffix(byName[name], "ath=/") {
			t.Errorf("%s still says Path=/: %s", name, byName[name])
		}
	}
	if s := strings.ToLower(byName["session_id"]); !strings.Contains(s, "httponly") || !strings.Contains(s, "secure") || !strings.Contains(s, "samesite=lax") {
		t.Errorf("the flags of the session cookie were lost: %s", s)
	}
	if s := byName["_csrf"]; strings.Contains(strings.ToLower(s), "httponly") {
		t.Errorf("the csrf cookie must stay readable by the page: %s", s)
	}
	if s := byName["other"]; !strings.Contains(s, "/elsewhere") {
		t.Errorf("a cookie with a path of its own is left alone: %s", s)
	}
}
