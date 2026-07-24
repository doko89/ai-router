package compression

import (
	"regexp"
	"strings"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// truncateHeadTail removes repeated prefix/suffix lines to keep the middle
// of long repeated content (e.g. stack traces, logs).
func truncateHeadTail(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 8 {
		return s
	}

	// Find prefix common to first few lines.
	prefix := commonPrefix(lines[0], lines[1])
	for i := 2; i < min(4, len(lines)); i++ {
		prefix = commonPrefix(prefix, lines[i])
	}

	// Find suffix common to last few lines.
	suffix := commonSuffix(lines[len(lines)-1], lines[len(lines)-2])
	for i := len(lines) - 3; i >= max(len(lines)-4, 0); i-- {
		suffix = commonSuffix(suffix, lines[i])
	}

	// If prefix and suffix are significant, trim head and tail.
	if len(prefix) > 0 || len(suffix) > 0 {
		// Keep first line with prefix context, then middle, then last line.
		var result []string
		if len(lines) > 3 {
			// Keep first line, one separator, middle line, one separator, last line.
			result = []string{
				strings.TrimPrefix(lines[0], prefix),
				"...",
				strings.TrimSpace(lines[len(lines)/2]),
				"...",
				strings.TrimSuffix(lines[len(lines)-1], suffix),
			}
		}
		if len(result) > 0 {
			compressed := strings.Join(result, "\n")
			if len(compressed) <= len(s) {
				return compressed
			}
		}
	}
	return s
}

func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}

func commonSuffix(a, b string) string {
	na, nb := len(a), len(b)
	n := min(na, nb)
	for i := 0; i < n; i++ {
		if a[na-1-i] != b[nb-1-i] {
			return a[na-i:]
		}
	}
	return a[na-n:]
}
