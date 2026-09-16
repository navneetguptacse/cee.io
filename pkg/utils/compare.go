package utils

import "strings"

// CompareOutput checks if actual output matches expected output, ignoring trailing whitespace and newlines.
func CompareOutput(actual, expected string) bool {
	normActual := strings.TrimRight(actual, " \t\r\n")
	normExpected := strings.TrimRight(expected, " \t\r\n")
	return normActual == normExpected
}

