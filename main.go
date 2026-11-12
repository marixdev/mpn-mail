package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"lilmail/basepath"
	"lilmail/config"
	"lilmail/handlers/ai"
	"lilmail/handlers/api"
	"lilmail/handlers/jsonapi"
	"lilmail/handlers/web"
	"lilmail/i18n"
	"lilmail/storage"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/session"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

// titleCase converts the first letter of each word to upper-case.
// It is used as a template function and replaces the deprecated strings.Title.
func titleCase(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			upper = true
			b.WriteRune(r)
		} else if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// uiLang resolves the UI locale for a request: the panel's language cookie, then the "lang" cookie, then the browser's
// preference, then the configured default, always clamped to a locale the app ships.
func uiLang(c *fiber.Ctx, cfg *config.Config) string {
	return i18n.Pick(c.Cookies("mpn_language"), c.Cookies("lang"), c.Get("Accept-Language"), cfg.Server.Language)
}

// linkifyRe matches, in a single left-to-right pass, bare http(s) URLs, www.
// hosts, and email addresses in plain text. A single combined pass means each
// character is consumed once, so a URL containing "@" is never re-matched as an
// email.
var linkifyRe = regexp.MustCompile(
	`(https?://[^\s<>"]+)` + // 1: absolute URL
		`|(\bwww\.[^\s<>"]+)` + // 2: www. host
		`|(\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b)`) // 3: email

// linkifyText HTML-escapes a plain-text email body and converts bare URLs and
// email addresses into safe anchors. The result is marked template.HTML so the
// anchors render; everything else has already been escaped, so this does not
// introduce an injection vector. Trailing sentence punctuation on URLs is kept
// out of the link target so "see https://x.com." behaves sensibly. Newlines are
// preserved by the white-space: pre-wrap CSS on the container.
func linkifyText(s string) template.HTML {
	esc := template.HTMLEscapeString(s)

	out := linkifyRe.ReplaceAllStringFunc(esc, func(m string) string {
		if strings.Contains(m, "@") && !strings.Contains(m, "/") {
			// Bare email address.
			return `<a href="mailto:` + m + `" class="text-link">` + m + `</a>`
		}
		// URL — pull trailing sentence punctuation back out of the link.
		trail := ""
		for len(m) > 0 && strings.ContainsRune(".,;:!?)", rune(m[len(m)-1])) {
			trail = string(m[len(m)-1]) + trail
			m = m[:len(m)-1]
		}
		href := m
		if strings.HasPrefix(strings.ToLower(m), "www.") {
			href = "http://" + m
		}
		return `<a href="` + href + `" class="text-link" target="_blank" rel="noopener noreferrer">` + m + `</a>` + trail
	})

	return template.HTML(out)
}

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:assets
var assetsFS embed.FS

// thirdPartyNotices is the generated attribution file for every third-party
// component lilmail redistributes (Go modules linked into this binary and the
// vendored JavaScript served to the browser). Their licences require the notice
// to accompany the copy, so it is embedded in the binary and served at
// /licenses.txt. Regenerate with ./scripts/gen-notices.sh.
//
//go:embed THIRD-PARTY-NOTICES.txt
var thirdPartyNotices string

var store *session.Store

// Idle tracking for the optional socket-activation exit (server.idle_exit_seconds).
// activeConns counts in-flight requests, including long-lived SSE, so an open tab
// holds the process up; lastActivity is the last time a request started or ended.
var (
	lastActivity atomic.Int64
	activeConns  atomic.Int64
)

// systemdListener returns a listener inherited from systemd socket activation
// (LISTEN_FDS), or nil when the process was not socket-activated. With socket
// activation systemd owns the port and launches the process on the first
// connection, so the process may exit when idle (see idle_exit_seconds) and be
// relaunched on demand, freeing RAM while nobody is using it.
func systemdListener() (net.Listener, error) {
	if os.Getenv("LISTEN_FDS") == "" {
		return nil, nil
	}
	if pid, _ := strconv.Atoi(os.Getenv("LISTEN_PID")); pid != 0 && pid != os.Getpid() {
		return nil, nil // the fds were passed to a different process
	}
	nfds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || nfds < 1 {
		return nil, nil
	}
	const firstFD = 3 // SD_LISTEN_FDS_START
	f := os.NewFile(uintptr(firstFD), "systemd-socket")
	ln, err := net.FileListener(f)
	if err != nil {
		return nil, fmt.Errorf("systemd socket activation: %w", err)
	}
	return ln, nil
}

// Helper function to determine if request is an API request
func isAPIRequest(c *fiber.Ctx) bool {
	if c == nil {
		return false
	}

	// Check for HTMX request first
	if c.Get("HX-Request") != "" {
		return true
	}

	// Safely check if path starts with /api
	path := c.Path()
	return len(path) >= 4 && path[:4] == "/api"
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("mpn-mail", Version)
		return
	}

	// Load configuration
	config, err := config.LoadConfig("config.toml")
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize session store now that we have the config (CookieSecure needs it).
	{
		fileStorage, err := storage.NewFileStorage("./sessions")
		if err != nil {
			log.Fatal("Failed to initialize session storage:", err)
		}
		store = session.New(session.Config{
			Storage:        fileStorage,
			Expiration:     24 * time.Hour,
			CookieSecure:   config.Server.SecureCookies, // true in TLS-terminated deployments
			CookieHTTPOnly: true,
			CookieSameSite: "Lax", // Prevents CSRF via cross-site form submissions
		})
	}

	// Template engine with the embedded templates and their functions (engine.go).
	engine := newEngine(config)

	// Initialize Fiber with template engine
	app := fiber.New(fiber.Config{
		Views:       engine,
		ViewsLayout: "layouts/main",   // Default layout
		BodyLimit:   25 * 1024 * 1024, // 25 MiB — guards compose form uploads
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}

			// Handle API requests differently
			if isAPIRequest(c) {
				return c.Status(code).JSON(fiber.Map{
					"error": err.Error(),
				})
			}

			// Render error page for regular requests
			return c.Status(code).Render("error", fiber.Map{
				"Error": err.Error(),
				"Code":  code,
			})
		},
	})

	// Add middleware
	app.Use(recover.New()) // Recover from panics
	app.Use(logger.New())  // Request logging

	// The path the reverse proxy cut off (X-Forwarded-Prefix, such as /webmail), for this request: given to every template as .Base,
	// used by every redirect, and the cookies are moved under it. Without the header nothing changes.
	app.Use(basepath.Middleware())

	// Apply security headers on every response.
	app.Use(func(c *fiber.Ctx) error {
		for h, v := range config.GetSecurityHeaders() {
			c.Set(h, v)
		}
		return c.Next()
	})

	// Resolve the UI locale once per request and expose it to every template as
	// $.Lang. c.Bind merges it into the layout, the embedded page and every
	// partial, so no handler has to thread it through.
	app.Use(func(c *fiber.Ctx) error {
		_ = c.Bind(fiber.Map{"Lang": uiLang(c, config)})
		return c.Next()
	})

	// Track request activity for the optional idle-exit watcher. Long-lived SSE
	// requests keep activeConns > 0, so an open tab holds the process up; the
	// /health probe is ignored so a liveness check does not defeat idling.
	app.Use(func(c *fiber.Ctx) error {
		if c.Path() == "/health" {
			return c.Next()
		}
		activeConns.Add(1)
		lastActivity.Store(time.Now().UnixNano())
		defer func() {
			lastActivity.Store(time.Now().UnixNano())
			activeConns.Add(-1)
		}()
		return c.Next()
	})

	// The web manifest names its start page, scope and icons by absolute path, so it is made for the prefix of the request.
	app.Get("/assets/site.webmanifest", serveManifest)

	// Serve embedded assets
	assetsSub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		log.Fatal("Failed to sub assets FS:", err)
	}
	app.Use("/assets", filesystem.New(filesystem.Config{
		Root:         http.FS(assetsSub),
		MaxAge:       int(24 * time.Hour / time.Second),
		NotFoundFile: "",
	}))

	// Service worker must be served at the root scope (not under /assets/)
	// so that it can intercept fetch requests and show push notifications for
	// the entire origin.  Served with Cache-Control: no-cache so the browser
	// always picks up updates promptly.
	app.Get("/sw.js", func(c *fiber.Ctx) error {
		swBytes, readErr := assetsFS.ReadFile("assets/sw.js")
		if readErr != nil {
			return fiber.ErrNotFound
		}
		c.Set("Content-Type", "application/javascript; charset=utf-8")
		c.Set("Cache-Control", "no-cache")
		c.Set("Service-Worker-Allowed", basepath.Path(c, "/")) // The worker may control the path the program is mounted on (the whole origin when there is no prefix).
		return c.Send(swBytes)
	})

	// Third-party notices. lilmail redistributes MIT/BSD/ISC/Apache-2.0 code
	// (Go modules compiled into this binary, plus the vendored JS served to the
	// browser); those licences require their copyright notice and licence text
	// to travel with every copy. Served unauthenticated so any user of a running
	// lilmail can read them. Regenerate with ./scripts/gen-notices.sh.
	app.Get("/licenses.txt", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/plain; charset=utf-8")
		c.Set("Cache-Control", "public, max-age=3600")
		return c.SendString(thirdPartyNotices)
	})

	// Initialize web handlers
	webAuthHandler := web.NewAuthHandler(store, config)
	webEmailHandler := web.NewEmailHandler(store, config, webAuthHandler)
	webCalendarHandler := web.NewCalendarHandler(store, config, webAuthHandler)

	// JSON/REST API (/v1/*) — machine-readable contract for rich JSON clients
	// (e.g. the Vulos OS mail surface). Additive: reuses the same engine +
	// session auth as the HTMX UI, returns 401 JSON instead of redirecting.
	//
	// A durable KV store (bbolt by default, Postgres when configured) backs
	// scheduled send (send-later): the store persists pending scheduled sends and
	// a poll-based drain delivers them at their sendAt with restart catch-up. If
	// the store cannot be opened we log and fall back to the storeless handler, so
	// the rest of the API keeps working (only send-later is unavailable).
	scheduleDBPath := filepath.Join(config.Cache.Folder, "scheduled.db")
	if kv, kvErr := storage.Open(config, scheduleDBPath); kvErr != nil {
		log.Printf("scheduled send unavailable (store open failed): %v", kvErr)
		jsonapi.New(store, config, webAuthHandler).Register(app)
	} else {
		jsonAPI := jsonapi.NewWithStore(store, config, webAuthHandler, kv)
		jsonAPI.Register(app)
		defer jsonAPI.StopScheduler()
		defer kv.Close()
	}

	// Rate limiters — applied to the three highest-risk surfaces.
	// Login: tight (brute-force). Send + AI: moderate (abuse/cost).
	// All limits are configurable via [rate_limit] in config.toml.
	loginLimiter := limiter.New(limiter.Config{
		Max:        config.RateLimit.LoginMax,
		Expiration: time.Duration(config.RateLimit.LoginWindow) * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).Render("login", fiber.Map{
				"Error": i18n.T(uiLang(c, config), "login.toomany"),
			})
		},
	})

	// Public routes
	app.Get("/login", webAuthHandler.ShowLogin)
	app.Post("/login", loginLimiter, webAuthHandler.HandleLogin)

	// Demo / screenshot mode — registered only when [demo] enabled = true.
	// Both GET and POST /demo-login immediately establish a demo session
	// (no IMAP contact) and redirect to /inbox. This lets Playwright simply
	// navigate to /demo-login and follow the redirect.
	if config.Demo.Enabled {
		app.Get("/demo-login", webAuthHandler.HandleDemoLogin)
		app.Post("/demo-login", webAuthHandler.HandleDemoLogin)
	}

	// OAuth2 login routes (public; the callback establishes the session)
	if config.OAuth2.Enabled {
		app.Get("/auth/oauth/login", webAuthHandler.HandleOAuthLogin)
		app.Get("/auth/oauth/callback", webAuthHandler.HandleOAuthCallback)
	}

	// Health check endpoint — MUST be registered BEFORE the protected group.
	// The protected group uses an empty prefix (app.Group("", SessionMiddleware)),
	// so its middleware attaches to every route registered AFTER it. Registering
	// /health here keeps it public (no 302 to /login) for liveness probes.
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Auth-gated root: logged-in users go to the inbox, logged-out visitors to
	// sign-in. (The marketing landing lives on vulos.org.)
	app.Get("/", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err == nil {
			if v := sess.Get("authenticated"); v == true {
				return c.Redirect(basepath.Path(c, "/inbox"))
			}
		}
		return c.Redirect(basepath.Path(c, "/login"))
	})

	// CSRF middleware for all web (cookie-session) protected routes.
	// Uses the double-submit cookie pattern:
	//   1. Middleware sets a JS-readable "_csrf" cookie on GET responses.
	//   2. HTMX's htmx:configRequest handler reads the cookie and sends its
	//      value as "X-CSRF-Token" on every mutating request.
	//   3. Middleware validates header == cookie value before the handler runs.
	// WebPush subscribe/unsubscribe routes are skipped.
	//
	// CORRECTION (found while typechecking assets/js/push.js, 2026-08-06): the
	// skip comment used to justify this as "they carry Authorization: Bearer
	// headers which already prevent cross-origin CSRF". That is not what
	// happens — handlers/web/push.go's HandleSubscribe/HandleUnsubscribe never
	// read the Authorization header at all; they authorize purely from
	// c.Locals("username"), set below by SessionMiddleware from the session
	// cookie. The Authorization: Bearer header push.js sends on these routes
	// is therefore inert on the server. What actually keeps these two routes
	// safe from cross-origin CSRF today is CookieSameSite: "Lax" below — a
	// forged cross-site POST/DELETE does not carry the session cookie at all,
	// so SessionMiddleware has nothing to authorize — the same protection
	// every other protected route relies on. This exemption is not currently
	// exploitable, but it rests on an inaccurate premise and should be
	// resolved deliberately (either validate the Bearer token server-side to
	// match the comment's original intent, or drop the exemption and the now-
	// pointless header) rather than left as accidentally-correct.
	csrfMiddleware := csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		CookieName:     "_csrf",
		CookieHTTPOnly: false, // must be JS-readable for double-submit pattern
		CookieSameSite: "Lax",
		CookieSecure:   config.Server.SecureCookies,
		Expiration:     24 * time.Hour,
		Next: func(c *fiber.Ctx) bool {
			// Skip for WebPush routes — see the correction above.
			return strings.HasPrefix(c.Path(), "/api/push/")
		},
	})

	// Protected routes group — session-gated + CSRF-protected
	protected := app.Group("", api.SessionMiddleware(store), csrfMiddleware)

	// Logout MUST be POST to prevent forced-logout via a crafted GET link
	// (CSRF attack). Placed in the protected group so CSRF middleware covers it.
	protected.Post("/logout", webAuthHandler.HandleLogout)

	// Main web routes
	protected.Get("/inbox", webEmailHandler.HandleInbox) // Explicit inbox route
	protected.Get("/folder/:name", webEmailHandler.HandleFolder)

	// API routes - Keep these paths exactly as they were before
	apiRoutes := protected.Group("/api")
	{
		// Email routes
		apiRoutes.Get("/email/:id", webEmailHandler.HandleEmailView)
		apiRoutes.Delete("/email/:id", webEmailHandler.HandleDeleteEmail)

		// Attachment download (ID encodes folder + UID + MIME part)
		apiRoutes.Get("/attachment/:id", webEmailHandler.HandleAttachment)

		// Folder routes - This is the important fix
		apiRoutes.Get("/folder/:name/emails", webEmailHandler.HandleFolderEmails) // Match the path in HTML

		// Composition routes
		apiRoutes.Post("/compose", webEmailHandler.HandleComposeEmail)

		// Drafts
		apiRoutes.Post("/draft", webEmailHandler.HandleSaveDraft)
		apiRoutes.Get("/drafts", webEmailHandler.HandleListDrafts)

		// Recipient autocomplete (recent senders + optional CardDAV)
		apiRoutes.Get("/autocomplete", webEmailHandler.HandleAutocomplete)

		// Mark-as-unread: removes the \Seen flag from a message.
		apiRoutes.Patch("/email/:id/unread", webEmailHandler.HandleMarkUnread)

		// Search: IMAP SEARCH returning an email-list partial.
		apiRoutes.Get("/search", webEmailHandler.HandleSearch)
	}

	// AI mail-assistant routes — registered always (gated internally on config.AI.Enabled).
	// When disabled, all /api/ai/* routes return 404 {"error":"ai_disabled"}.
	// Rate-limit AI routes before registration to guard against compute-cost abuse
	// and brute-force of the AI endpoint (limits configurable via [rate_limit]).
	aiLimiter := limiter.New(limiter.Config{
		Max:        config.RateLimit.AIMax,
		Expiration: time.Duration(config.RateLimit.AIWindow) * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		},
	})
	apiRoutes.Use("/ai", aiLimiter)
	ai.RegisterRoutes(apiRoutes, config.AI)

	// Notifications routes — registered only when notifications.enabled = true.
	// With enabled = false (the default) this block is never entered, so no
	// extra goroutines are created and no new routes appear.
	if config.Notifications.Enabled {
		// Optional VAPID Web Push.
		var vapidKeys *web.VAPIDKeys
		var pushStore *web.PushStore
		if config.Notifications.WebPush {
			var err error
			vapidKeys, err = web.LoadOrGenerateVAPIDKeys(config.Notifications.VAPIDKeyFile)
			if err != nil {
				log.Printf("webpush: VAPID key init failed (%v) — web push disabled", err)
			} else {
				log.Printf("webpush: VAPID public key loaded (%s)", config.Notifications.VAPIDKeyFile)
				cacheRoot := config.Cache.Folder
				if cacheRoot == "" {
					cacheRoot = "."
				}
				pushStore = web.NewPushStore(cacheRoot)
			}
		}

		hub := web.NewNotificationHub(store, config, webAuthHandler, vapidKeys, pushStore)
		notifHandler := web.NewNotificationsHandler(hub)
		protected.Get("/events", notifHandler.HandleSSE)

		// VAPID public key endpoint — public (no session required) so the SW can
		// fetch it before the user navigates to an authenticated page.
		if vapidKeys != nil {
			pushHandler := web.NewPushHandler(vapidKeys, pushStore)
			app.Get("/api/push/vapid-public", pushHandler.HandleVAPIDPublicKey)
			protected.Post("/api/push/subscribe", pushHandler.HandleSubscribe)
			protected.Delete("/api/push/subscribe", pushHandler.HandleUnsubscribe)
		}
	}

	// Multi-account routes — registered only when accounts.enabled = true.
	var acctHandler *web.AccountsHandler
	if config.Accounts.Enabled {
		acctStore, err := web.OpenAccountStore(config.Accounts.StoreFile)
		if err != nil {
			log.Fatalf("accounts: open store: %v", err)
		}
		// Wire the account store into the email handler so unified-inbox fetches work.
		webEmailHandler.SetAccountStore(acctStore)

		acctHandler = web.NewAccountsHandler(store, config, webAuthHandler, acctStore)

		protected.Get("/api/accounts", acctHandler.HandleListAccounts)
		protected.Post("/api/accounts", acctHandler.HandleAddAccount)
		protected.Delete("/api/accounts/:email", acctHandler.HandleDeleteAccount)
		protected.Post("/api/accounts/:email/switch", acctHandler.HandleSwitchAccount)
	}
	// Calendar routes — registered only when CalDAV is enabled.
	if config.CalDAV.Enabled {
		protected.Get("/calendar", webCalendarHandler.HandleCalendarMonth)
		protected.Get("/calendar/week", webCalendarHandler.HandleCalendarWeek)
		protected.Get("/calendar/event/:uid", webCalendarHandler.HandleEventDetail)
		protected.Post("/calendar/event", webCalendarHandler.HandleCreateEvent)
		protected.Post("/calendar/rsvp", webCalendarHandler.HandleRSVP)
	}

	// 404 Handler for undefined routes
	app.Use(func(c *fiber.Ctx) error {
		if isAPIRequest(c) {
			return c.Status(404).JSON(fiber.Map{
				"error": "Not Found",
			})
		}
		return c.Status(404).Render("error", fiber.Map{
			"Error": "Page not found",
			"Code":  404,
		})
	})

	// Optional idle-exit: under systemd socket activation the socket outlives the
	// process, so exiting when idle frees RAM and the next connection relaunches us.
	if d := time.Duration(config.Server.IdleExitSeconds) * time.Second; d > 0 {
		lastActivity.Store(time.Now().UnixNano())
		go func() {
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for range tick.C {
				if activeConns.Load() > 0 {
					continue
				}
				if time.Since(time.Unix(0, lastActivity.Load())) >= d {
					log.Printf("idle for %s with no active connections; shutting down for socket activation", d)
					_ = app.ShutdownWithTimeout(5 * time.Second)
					return
				}
			}
		}()
	}

	// Start server. Prefer a systemd socket-activated listener when one was passed.
	if ln, sockErr := systemdListener(); sockErr != nil {
		log.Fatal("Error starting server: ", sockErr)
	} else if ln != nil {
		log.Println("Starting server on a socket-activated listener…")
		if err := app.Listener(ln); err != nil {
			log.Fatal("Error starting server: ", err)
		}
	} else {
		log.Printf("Starting server on port %d...\n", config.Server.Port)
		if err := app.Listen(fmt.Sprintf(":%d", config.Server.Port)); err != nil {
			log.Fatal("Error starting server: ", err)
		}
	}
}
