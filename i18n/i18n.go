// Package i18n holds the user-facing strings of the web UI in each supported
// locale and the lookup used from templates.
//
// Templates call it as the "T" function, passing the request locale first:
//
//	{{T $.Lang "login.signin"}}
//	{{T $.Lang "mail.unread.count" .N}}
//
// The locale is put on every render by the language middleware (see main.go),
// so $.Lang is present in the layout, the embedded page and every partial.
package i18n

import "fmt"

// Default is the locale used when none is chosen or when a key is missing from
// the requested locale.
const Default = "vi"

// catalogs maps a locale code to its message table. Add a locale by adding a
// file like vi.go/en.go and registering its table here.
var catalogs = map[string]map[string]string{
	"vi": viMessages,
	"en": enMessages,
	"zh": zhMessages,
}

// Lang is one offered locale, for the language switcher.
type Lang struct {
	Code string
	Name string
}

// Langs lists the supported locales in the order the switcher shows them.
var Langs = []Lang{
	{"vi", "Tiếng Việt"},
	{"en", "English"},
	{"zh", "简体中文"},
}

// Supported reports whether code is a locale the app ships.
func Supported(code string) bool {
	_, ok := catalogs[code]
	return ok
}

// Resolve picks a valid locale from a candidate (such as a cookie or the config
// default), falling back to Default when the candidate is empty or unknown.
func Resolve(code string) string {
	if Supported(code) {
		return code
	}
	return Default
}

// T returns the message for key in the given locale. An unknown locale falls
// back to the default; a key missing from the default falls back to the key
// itself, so a forgotten string shows up visibly rather than blank. With args
// the message is treated as a printf format and filled in.
func T(lang any, key string, args ...any) string {
	// lang is any so templates can pass $.Lang even on a page rendered before the
	// language middleware set it (e.g. an early error page); a nil or non-string
	// value falls back to the default catalog.
	code, _ := lang.(string)
	s, ok := catalogs[code][key]
	if !ok {
		if s, ok = catalogs[Default][key]; !ok {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
