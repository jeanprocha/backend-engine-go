package strategytags

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	maxPatternLen = 100
	maxLabelLen   = 100
	maxCategory   = 50
)

var allowedColorSchemes = map[string]struct{}{
	"emerald": {},
	"blue":    {},
	"amber":   {},
	"purple":  {},
}

// NormalizePattern alinha ao frontend (normalizeText): minúsculas, sem marcas diacríticas, espaços colapsados.
func NormalizePattern(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return ""
	}
	return strings.Join(strings.Fields(out), " ")
}

// SanitizeColorScheme returns a whitelisted scheme or "emerald".
func SanitizeColorScheme(s string) string {
	c := strings.ToLower(strings.TrimSpace(s))
	if _, ok := allowedColorSchemes[c]; ok {
		return c
	}
	return "emerald"
}

// ValidateForInsert checks lengths and non-empty pattern/label.
func ValidateForInsert(pattern, label, category string) error {
	p := NormalizePattern(pattern)
	if p == "" {
		return fmt.Errorf("pattern vazio")
	}
	if utf8.RuneCountInString(p) > maxPatternLen {
		return fmt.Errorf("pattern excede %d runes", maxPatternLen)
	}
	l := strings.TrimSpace(label)
	if l == "" {
		return fmt.Errorf("label vazio")
	}
	if utf8.RuneCountInString(l) > maxLabelLen {
		return fmt.Errorf("label excede %d runes", maxLabelLen)
	}
	cat := strings.TrimSpace(category)
	if utf8.RuneCountInString(cat) > maxCategory {
		return fmt.Errorf("category excede %d runes", maxCategory)
	}
	return nil
}
