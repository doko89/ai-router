package compression

import (
	"encoding/json"
	"strconv"
	"strings"
)

type protectedBlock struct {
	placeholder string
	content     string
}

type textSpan struct {
	start int
	end   int
}

// protectTechnicalContent replaces code fences, inline code, URLs, and JSON
// with placeholders so rules don't corrupt them.
func protectTechnicalContent(s string) (string, []protectedBlock) {
	spans := technicalSpans(s)
	if len(spans) == 0 {
		return s, []protectedBlock{}
	}

	blocks := make([]protectedBlock, 0, len(spans))
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, span := range spans {
		placeholder := "\x00P" + strconv.Itoa(len(blocks)) + "\x00"
		b.WriteString(s[last:span.start])
		b.WriteString(placeholder)
		blocks = append(blocks, protectedBlock{
			placeholder: placeholder,
			content:     s[span.start:span.end],
		})
		last = span.end
	}
	b.WriteString(s[last:])
	return b.String(), blocks
}

func restoreProtectedContent(s string, blocks []protectedBlock) string {
	for _, block := range blocks {
		s = strings.ReplaceAll(s, block.placeholder, block.content)
	}
	return s
}

func technicalSpans(s string) []textSpan {
	spans := make([]textSpan, 0)
	for pos := 0; pos < len(s); {
		end := protectedSpanEnd(s, pos)
		if end <= pos {
			pos++
			continue
		}
		spans = append(spans, textSpan{start: pos, end: end})
		pos = end
	}
	return spans
}

func protectedSpanEnd(s string, start int) int {
	if strings.HasPrefix(s[start:], "```") {
		if end := strings.Index(s[start+3:], "```"); end >= 0 {
			return start + 3 + end + 3
		}
		return len(s)
	}
	if s[start] == '`' {
		if end := strings.IndexByte(s[start+1:], '`'); end >= 0 {
			return start + 1 + end + 1
		}
	}
	if hasURLPrefix(s[start:]) {
		return scanURL(s, start)
	}
	if s[start] == '{' || s[start] == '[' {
		if end := scanJSON(s, start); end > start {
			return end
		}
	}
	return start
}

func hasURLPrefix(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

func scanURL(s string, start int) int {
	end := start
	for end < len(s) {
		switch s[end] {
		case ' ', '\t', '\r', '\n', '<', '>', '"':
			return trimURLPunctuation(s, start, end)
		default:
			end++
		}
	}
	return trimURLPunctuation(s, start, end)
}

func trimURLPunctuation(s string, start int, end int) int {
	for end > start {
		switch s[end-1] {
		case '.', ',', ';', ':', '!', '?':
			end--
		default:
			return end
		}
	}
	return end
}

func scanJSON(s string, start int) int {
	stack := make([]byte, 0, 4)
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 || !matchingJSONDelimiter(stack[len(stack)-1], ch) {
				return start
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				end := i + 1
				if json.Valid([]byte(s[start:end])) {
					return end
				}
				return start
			}
		}
	}
	return start
}

func matchingJSONDelimiter(open byte, close byte) bool {
	return open == '{' && close == '}' || open == '[' && close == ']'
}
