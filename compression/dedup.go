package compression

import (
	"regexp"
	"strings"
)

var wsRegexp = regexp.MustCompile(`\s+`)

// dedupLines removes consecutive duplicate lines, preserving order.
func dedupLines(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}

	seen := make(map[string]bool, len(lines))
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			result = append(result, line)
			continue
		}
		key := wsRegexp.ReplaceAllString(trimmed, " ")
		if !seen[key] {
			seen[key] = true
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// collapseWhitespace normalizes whitespace within lines while preserving
// leading indentation. Trailing whitespace and blank lines are trimmed.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		collapsed := wsRegexp.ReplaceAllString(line, " ")
		lines[i] = strings.TrimRight(collapsed, " ")
	}
	// Trim trailing empty lines but keep leading content structure.
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
