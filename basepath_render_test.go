package main

import (
	"io"
	"lilmail/basepath"
	"lilmail/config"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// renderApp is the real template engine and the real prefix middleware with a route that renders any page with made-up data, so the
// pages that need a mail server can still be looked at.
func renderApp(t *testing.T) *fiber.App {
	t.Helper()
	cfg := &config.Config{}
	cfg.CalDAV.Enabled = true
	cfg.Notifications.Enabled = true
	cfg.Notifications.WebPush = true
	cfg.Accounts.Enabled = true
	app := fiber.New(fiber.Config{Views: newEngine(cfg), ViewsLayout: "layouts/main"})
	app.Use(basepath.Middleware())
	app.Use(func(c *fiber.Ctx) error { _ = c.Bind(fiber.Map{"Lang": "en"}); return c.Next() })
	app.Get("/page/:name", func(c *fiber.Ctx) error {
		name := strings.ReplaceAll(c.Params("name"), "~", "/")
		data := fiber.Map{
			"Username": "user@example.com", "Email": "user@example.com", "Token": "tok", "CurrentFolder": "INBOX",
			"Title": "T", "Code": 404, "Error": "",
			"Folders":  []fiber.Map{{"Name": "INBOX"}, {"Name": "Sent Items"}},
			"Emails":   []fiber.Map{{"ID": "INBOX:1", "Subject": "s", "From": "a@b.c", "Date": "d", "Unread": true}},
			"Accounts": []fiber.Map{{"Email": "x@y.z"}},
		}
		if name == "partials/email-viewer" { // here .Email is the message, not the address of the signed-in person
			data["Email"] = fiber.Map{"ID": "INBOX:1", "Subject": "s", "From": "a@b.c", "To": "x@y.z", "Date": "d", "Body": "b",
				"Attachments": []fiber.Map{{"ID": "a1", "Name": "f.txt", "Size": 10}}}
		}
		return c.Render(name, data)
	})
	return app
}

func render(t *testing.T, app *fiber.App, page, prefix string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/page/"+page, nil)
	if prefix != "" {
		req.Header.Set(basepath.Header, prefix)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s: %v", page, err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%s: status %d: %.300s", page, resp.StatusCode, b)
	}
	return string(b)
}

// the pages that can be drawn without a mail server
// ("~" stands for "/" in the name of a template). The two partials are the ones htmx loads while the page is in use.
var pagesToCheck = []string{"login", "error", "inbox", "calendar", "calendar-week", "partials~email-list", "partials~email-viewer"}

var (
	attrRe  = regexp.MustCompile(`\s(?:href|src|action|hx-get|hx-post|hx-put|hx-patch|hx-delete)="(/[^"]*)"`)
	jsPathR = regexp.MustCompile("['\"`]/(?:api|assets|inbox|folder|login|logout|settings|calendar|events|auth|sw\\.js)\\b")
)

func TestUnderAPrefixEveryAddressOfThePageGoesThroughIt(t *testing.T) {
	app := renderApp(t)
	checked := 0
	for _, page := range pagesToCheck {
		body := render(t, app, page, "/webmail")
		for _, m := range attrRe.FindAllStringSubmatch(body, -1) {
			checked++
			if !strings.HasPrefix(m[1], "/webmail/") {
				t.Errorf("%s: %s does not go through the prefix", page, strings.TrimSpace(m[0]))
			}
		}
		if loc := jsPathR.FindAllString(body, -1); len(loc) > 0 {
			t.Errorf("%s: a script still names a path without the prefix: %v", page, loc)
		}
		if !strings.Contains(body, `name="mail-base" content="/webmail"`) && page != "error" {
			// the layout carries the prefix to the scripts
			if strings.Contains(body, "<html") {
				t.Errorf("%s: the layout does not hand the prefix to the scripts", page)
			}
		}
	}
	if checked < 25 {
		t.Errorf("only %d addresses were looked at: the check is not seeing the pages", checked)
	}
}

func TestWithoutTheHeaderThePagesAreTheOnesTheyWere(t *testing.T) {
	app := renderApp(t)
	for _, page := range pagesToCheck {
		body := render(t, app, page, "")
		if strings.Contains(body, "/webmail") {
			t.Errorf("%s: no header, yet the page names a prefix", page)
		}
		if strings.Contains(body, "mail-base") && !strings.Contains(body, `name="mail-base" content=""`) {
			t.Errorf("%s: the prefix is not empty", page)
		}
		for _, m := range attrRe.FindAllStringSubmatch(body, -1) {
			if strings.HasPrefix(m[1], "//") || strings.Contains(m[1], "{{") {
				t.Errorf("%s: odd address %s", page, m[0])
			}
		}
	}
	body := render(t, app, "login", "")
	for _, want := range []string{`href="/assets/css/mail.css"`, `action="/login"`, `src="/assets/vendor/htmx.min.js"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the login page lost %s", want)
		}
	}
}

func TestAValueThatIsNotAPrefixNeverReachesAPage(t *testing.T) {
	app := renderApp(t)
	for _, evil := range []string{`/"><script>alert(1)</script>`, "//evil.example", "/a/b", "https://evil.example", "/Webmail"} {
		body := render(t, app, "login", evil)
		if strings.Contains(body, "evil.example") || strings.Contains(body, "<script>alert") || strings.Contains(body, "/a/b") || strings.Contains(body, "/Webmail") {
			t.Errorf("%q reached the page", evil)
		}
	}
}

func TestTheManifestFollowsThePrefix(t *testing.T) {
	app := fiber.New()
	app.Use(basepath.Middleware())
	app.Get("/assets/site.webmanifest", serveManifest)
	get := func(prefix string) string {
		req := httptest.NewRequest("GET", "/assets/site.webmanifest", nil)
		if prefix != "" {
			req.Header.Set(basepath.Header, prefix)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	under := get("/webmail")
	for _, want := range []string{`"start_url": "/webmail/inbox"`, `"scope": "/webmail/"`, `"src": "/webmail/assets/icon-192.png"`, `"src": "/webmail/assets/icon-512.png"`} {
		if !strings.Contains(under, want) {
			t.Errorf("under the prefix the manifest lacks %s:\n%s", want, under)
		}
	}
	plain := get("")
	for _, want := range []string{`"start_url": "/inbox"`, `"scope": "/"`, `"src": "/assets/icon-192.png"`} {
		if !strings.Contains(plain, want) {
			t.Errorf("without the prefix the manifest lacks %s:\n%s", want, plain)
		}
	}
}

func TestTheServiceWorkerMayControlThePathItIsMountedOn(t *testing.T) {
	app := fiber.New()
	app.Use(basepath.Middleware())
	app.Get("/probe", func(c *fiber.Ctx) error { return c.SendString(basepath.Path(c, "/")) })
	for prefix, want := range map[string]string{"": "/", "/webmail": "/webmail/"} {
		req := httptest.NewRequest("GET", "/probe", nil)
		if prefix != "" {
			req.Header.Set(basepath.Header, prefix)
		}
		resp, _ := app.Test(req)
		b, _ := io.ReadAll(resp.Body)
		if string(b) != want {
			t.Errorf("prefix %q: scope %q, want %q", prefix, b, want)
		}
	}
}
