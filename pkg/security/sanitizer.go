package security

import (
	"regexp"
	"strings"
)

var (
	disallowedChars = regexp.MustCompile(`[^a-zA-Z0-9\s\-_.=+/]`)
	multipleSpaces  = regexp.MustCompile(`\s+`)
)

// SanitizeOptions sanitizes compiler options or CLI arguments using a strict whitelist.
func SanitizeOptions(options string) string {
	if options == "" {
		return ""
	}
	cleaned := disallowedChars.ReplaceAllString(options, "")
	cleaned = multipleSpaces.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}
