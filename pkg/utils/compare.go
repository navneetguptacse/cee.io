package utils

import "strings"

// CompareOutput checks if actual output matches expected output, ignoring differences in line endings (CRLF vs LF) and trailing whitespace.
func CompareOutput(actual, expected string) bool {
	return normalizeOutput(actual) == normalizeOutput(expected)
}

func normalizeOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	res := strings.Join(lines, "\n")
	return strings.TrimRight(res, " \t\n")
}
