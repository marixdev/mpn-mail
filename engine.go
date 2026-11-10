package main

import (
	"fmt"
	"io/fs"
	"lilmail/config"
	"lilmail/i18n"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/template/html/v2"
)

// newEngine builds the template engine with the embedded templates and every function they use. It is a function of its own so the
// tests build exactly the engine the program runs with.
func newEngine(cfg *config.Config) *html.Engine {
	tplSub, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		log.Fatal("Failed to sub templates FS:", err)
	}
	engine := html.NewFileSystem(http.FS(tplSub), ".html")

	// String manipulation functions
	engine.AddFunc("split", strings.Split)
	engine.AddFunc("join", strings.Join)
	engine.AddFunc("lower", strings.ToLower)
	engine.AddFunc("upper", strings.ToUpper)
	engine.AddFunc("title", titleCase)
	engine.AddFunc("trim", strings.TrimSpace)
	engine.AddFunc("hasPrefix", strings.HasPrefix)
	engine.AddFunc("urlEncode", url.QueryEscape)

	// T is the i18n lookup used from every template as {{T $.Lang "key"}}.
	// $.Lang is put on each render by the language middleware.
	engine.AddFunc("T", i18n.T)

	// folderLabel localizes well-known IMAP folder names: {{folderLabel $.Lang .Name}}.
	engine.AddFunc("folderLabel", i18n.FolderLabel)

	// linkify HTML-escapes a plain-text body and turns bare URLs and email
	// addresses into clickable links. Newlines are preserved by the
	// white-space: pre-wrap CSS on the container, so we only inject anchors.
	engine.AddFunc("linkify", linkifyText)

	// Date formatting function
	engine.AddFunc("formatDate", func(t time.Time) string {
		return t.Format("Jan 02, 2006 15:04")
	})

	// File size formatting function. Accepts int (models.Attachment.Size) — the
	// template passes an int, and text/template will not coerce int -> int64.
	engine.AddFunc("formatSize", func(sizeInt int) string {
		size := int64(sizeInt)
		const unit = 1024
		if size < unit {
			return fmt.Sprintf("%d B", size)
		}
		div, exp := int64(unit), 0
		for n := size / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
	})

	// initial returns the first letter (unicode-safe, upper-cased) of the
	// preferred name, falling back to the email, for avatar bubbles.
	engine.AddFunc("initial", func(name, email string) string {
		s := strings.TrimSpace(name)
		if s == "" {
			s = strings.TrimSpace(email)
		}
		for _, r := range s {
			return strings.ToUpper(string(r))
		}
		return "?"
	})

	// caldavEnabled is a zero-argument template func so templates can show/hide
	// calendar navigation without threading a flag through every handler.
	engine.AddFunc("caldavEnabled", func() bool {
		return cfg.CalDAV.Enabled
	})

	// notificationsEnabled is a zero-argument template func so templates can
	// conditionally emit the SSE client script.  It is always registered (so
	// the template parse step never fails) but returns false by default.
	engine.AddFunc("notificationsEnabled", func() bool {
		return cfg.Notifications.Enabled
	})

	// webPushEnabled tells templates whether to register the service worker and
	// expose the "Enable push notifications" button.
	engine.AddFunc("webPushEnabled", func() bool {
		return cfg.Notifications.Enabled && cfg.Notifications.WebPush
	})

	// accountsEnabled tells templates whether to show the account-switcher UI.
	engine.AddFunc("accountsEnabled", func() bool {
		return cfg.Accounts.Enabled
	})

	engine.Reload(false) // embedded — no disk reload needed
	return engine
}
