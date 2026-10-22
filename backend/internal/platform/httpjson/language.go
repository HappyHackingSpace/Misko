package httpjson

import (
	"net/http"
	"slices"
	"strings"
)

// Language picks the response language for user-facing catalog text: the
// `lang` query value when supported, otherwise the first supported language in
// Accept-Language, otherwise the fallback (the first supported language).
// Quality values are ignored; browsers list languages in preference order.
func Language(r *http.Request, supported []string) string {
	if len(supported) == 0 {
		return ""
	}
	match := func(tag string) (string, bool) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if i := strings.IndexAny(tag, ";-_"); i >= 0 {
			tag = tag[:i]
		}
		return tag, slices.Contains(supported, tag)
	}
	if lang, ok := match(r.URL.Query().Get("lang")); ok {
		return lang
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		if lang, ok := match(part); ok {
			return lang
		}
	}
	return supported[0]
}
