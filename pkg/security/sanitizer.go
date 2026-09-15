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
	// Strip all characters not in whitelist
	cleaned := disallowedChars.ReplaceAllString(options, "")
	// Collapse multiple spaces
	cleaned = multipleSpaces.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}
