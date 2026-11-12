package i18n

import (
	"strconv"
	"strings"
)

// Pick chooses the locale of a request. The panel's cookie (mpn_language) comes first so that changing the language in the
// panel changes it here too; then the cookie this program has always read, then the browser's Accept-Language, then fallback.
func Pick(panelCookie, langCookie, acceptLanguage, fallback string) string {
	for _, c := range []string{panelCookie, langCookie} {
		if Supported(c) {
			return c
		}
	}
	if l := fromAcceptLanguage(acceptLanguage); l != "" {
		return l
	}
	return Resolve(fallback)
}

// fromAcceptLanguage returns the supported locale the browser prefers most, honouring q weights, or "" when it names none.
func fromAcceptLanguage(header string) string {
	best, quality := "", 0.0
	for _, preference := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(preference), ";")
		code := strings.ToLower(strings.Split(parts[0], "-")[0])
		if !Supported(code) {
			continue
		}
		q := 1.0
		for _, p := range parts[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil || f < 0 || f > 1 {
					f = 0
				}
				q = f
			}
		}
		if q > quality {
			best, quality = code, q
		}
	}
	return best
}
