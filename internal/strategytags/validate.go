package strategytags

import (
	"fmt"
	"strings"
	"unicode/utf8"
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

// NormalizePattern lowercases and trims; empty after trim is invalid for insert.
func NormalizePattern(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
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
