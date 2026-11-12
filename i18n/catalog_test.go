package i18n

import (
	"fmt"
	"regexp"
	"testing"
)

// Every locale has every Vietnamese key with the same fmt directives, and no key of its own.
func TestCatalogsMatch(t *testing.T) {
	directives := regexp.MustCompile(`%(?:\[\d+\])?[+#\- 0]*(?:\d+)?(?:\.\d+)?[a-zA-Z%]`)
	for _, l := range Langs {
		cat := catalogs[l.Code]
		for k, v := range viMessages {
			s, ok := cat[k]
			if !ok || s == "" {
				t.Errorf("%s: missing %q", l.Code, k)
				continue
			}
			if a, b := directives.FindAllString(v, -1), directives.FindAllString(s, -1); fmt.Sprint(a) != fmt.Sprint(b) {
				t.Errorf("%s %s: formats %v, Vietnamese %v", l.Code, k, b, a)
			}
		}
		for k := range cat {
			if _, ok := viMessages[k]; !ok {
				t.Errorf("%s: key %q is not in Vietnamese", l.Code, k)
			}
		}
	}
	if T("zh", "folder.inbox") != "收件箱" || Pick("", "", "zh-CN,zh;q=0.9", "vi") != "zh" {
		t.Error("Chinese is not selected")
	}
}
