package i18n

import "testing"

func TestPick(t *testing.T) {
	for _, tc := range []struct{ panel, lang, accept, fallback, want string }{
		{"en", "", "", "vi", "en"},
		{"vi", "", "en-US,en;q=0.9", "en", "vi"},
		{"en", "vi", "", "vi", "en"},
		{"", "en", "vi", "vi", "en"},
		{"fr", "de", "en-US,en;q=0.9,vi;q=0.8", "vi", "en"},
		{"", "", "fr-FR,fr;q=0.9,vi;q=0.5", "en", "vi"},
		{"", "", "en;q=0, vi;q=0.2", "en", "vi"},
		{"", "", "fr-FR", "en", "en"},
		{"", "", "", "", Default},
		{"", "", "en;q=abc", "vi", "vi"},
	} {
		if got := Pick(tc.panel, tc.lang, tc.accept, tc.fallback); got != tc.want {
			t.Errorf("Pick(%q,%q,%q,%q) = %q, want %q", tc.panel, tc.lang, tc.accept, tc.fallback, got, tc.want)
		}
	}
}
