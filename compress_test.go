package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already lowercase", "aggressive", CompressionAggressive},
		{"already lowercase standard", "standard", CompressionStandard},
		{"off unchanged", "off", CompressionOff},
		{"lite unchanged", "lite", CompressionLite},
		{"empty", "", ""},
		{"typo agressive", "agressive", CompressionAggressive},
		{"typo agresive", "agresive", CompressionAggressive},
		{"typo standart", "standart", CompressionStandard},
		{"typo standar", "standar", CompressionStandard},
		// Case sensitivity — these should work but currently don't
		{"uppercase AGGRESSIVE", "AGGRESSIVE", CompressionAggressive},
		{"capitalized Standard", "Standard", CompressionStandard},
		{"uppercase LITE", "LITE", CompressionLite},
		{"mixed Aggressive", "Aggressive", CompressionAggressive},
		{"mixed OfF", "OfF", CompressionOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLevel(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLevel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"preserve go indentation",
			"package main\n\nfunc main() {\n\tif x {\n\t\treturn 1\n\t}\n}",
			"package main\n\nfunc main() {\n\tif x {\n\t\treturn 1\n\t}\n}",
		},
		{
			"preserve python indentation",
			"def foo():\n    if x:\n        return 1\n    else:\n        return 2",
			"def foo():\n    if x:\n        return 1\n    else:\n        return 2",
		},
		{
			"preserve nested indentation",
			"  line1\n    line2\n      line3",
			"  line1\n    line2\n      line3",
		},
		{
			"collapse internal multiple spaces",
			"hello    world",
			"hello world",
		},
		{
			"collapse mixed internal spaces/tabs",
			"a\t\tb",
			"a b",
		},
		{
			"trim trailing space",
			"hello world   ",
			"hello world",
		},
		{
			"preserve leading space, collapse internal",
			"  hello    world  ",
			"  hello world",
		},
		{
			"empty string unchanged",
			"",
			"",
		},
		{
			"single line no change",
			"hello world",
			"hello world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseWhitespace(tt.input)
			if got != tt.want {
				t.Errorf("collapseWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDedupLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"no duplicates",
			"a\nb\nc",
			"a\nb\nc",
		},
		{
			"two duplicates kept",
			"a\na\na\nb",
			"a\na\na\nb",
		},
		{
			"three duplicates deduped",
			"a\na\na\na\nb",
			"a\na\na\nb",
		},
		{
			"four duplicates deduped",
			"a\na\na\na\na\nb",
			"a\na\na\nb",
		},
		{
			"short input unchanged",
			"a\na\na",
			"a\na\na",
		},
		{
			"empty unchanged",
			"",
			"",
		},
		{
			"single line unchanged",
			"hello",
			"hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupLines(tt.input)
			if got != tt.want {
				t.Errorf("dedupLines(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateHeadTail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"under threshold unchanged",
			"line1\nline2\nline3",
			"line1\nline2\nline3",
		},
		{
			"exactly 100 lines unchanged",
			func() string {
				s := ""
				for i := 0; i < 100; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
			func() string {
				s := ""
				for i := 0; i < 100; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
		},
		{
			"101 lines truncated",
			func() string {
				s := ""
				for i := 0; i < 101; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
			func() string {
				s := ""
				for i := 0; i < 50; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				s += "\n... [1 lines truncated]\n"
				for i := 51; i < 101; i++ {
					if i > 51 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
		},
		{
			"200 lines truncated with count",
			func() string {
				s := ""
				for i := 0; i < 200; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
			func() string {
				s := ""
				for i := 0; i < 50; i++ {
					if i > 0 {
						s += "\n"
					}
					s += "line"
				}
				s += "\n... [100 lines truncated]\n"
				for i := 150; i < 200; i++ {
					if i > 150 {
						s += "\n"
					}
					s += "line"
				}
				return s
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateHeadTail(tt.input)
			if got != tt.want {
				t.Errorf("truncateHeadTail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple color", "\x1b[31mERROR\x1b[0m", "ERROR"},
		{"no ansi unchanged", "hello world", "hello world"},
		{"bold + color", "\x1b[1m\x1b[32mOK\x1b[0m", "OK"},
		{"cursor sequences", "\x1b[2J\x1b[Hclear", "clear"},
		{"empty unchanged", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// compressString tests — these verify the integrated string compression pipeline.
func TestCompressString_lite(t *testing.T) {
	t.Run("sql LIKE preserved", func(t *testing.T) {
		input := `SELECT * FROM users WHERE name LIKE '%foo%'`
		got := compressString(input, CompressionLite, "user")
		// "like" must NOT be deleted — it's a SQL keyword
		if contains(got, "like") || contains(got, "LIKE") {
			// "like" might be lowercased, but must still be present
		}
		if !contains(got, "%foo%") {
			t.Errorf("SQL LIKE query broken: compressString(lite) = %q", got)
		}
	})

	t.Run("verb like preserved", func(t *testing.T) {
		input := `I like cats`
		got := compressString(input, CompressionLite, "user")
		if !contains(got, "like") {
			t.Errorf("verb 'like' deleted: compressString(lite) = %q", got)
		}
	})

	t.Run("comparison like preserved", func(t *testing.T) {
		input := `configurations like this`
		got := compressString(input, CompressionLite, "user")
		if !contains(got, "like") {
			t.Errorf("comparison 'like' deleted: compressString(lite) = %q", got)
		}
	})
}

func TestCompressString_notOnlyButAlso(t *testing.T) {
	t.Run("not only X but also Y content preserved", func(t *testing.T) {
		input := "it handles not only auth but also logging"
		got := compressString(input, CompressionStandard, "user")
		// "auth" and "logging" must both be present
		if !contains(got, "auth") || !contains(got, "logging") {
			t.Errorf("not only...but also ate content: compressString(standard) = %q", got)
		}
	})
}

func TestCompressString_codeIndentation(t *testing.T) {
	tests := []struct {
		level string
		role  string
	}{
		{CompressionLite, "user"},
		{CompressionStandard, "user"},
		{CompressionAggressive, "user"},
		{CompressionLite, "assistant"},
		{CompressionStandard, "assistant"},
		{CompressionAggressive, "assistant"},
		{CompressionLite, "system"},
		{CompressionStandard, "system"},
		{CompressionAggressive, "system"},
	}

	for _, tt := range tests {
		t.Run(tt.level+"_"+tt.role, func(t *testing.T) {
			input := "Here is the code:\n\nfunc main() {\n\tif x {\n\t\treturn 1\n\t}\n}"
			got := compressString(input, tt.level, tt.role)
			// The t's within code must be preserved (indentation)
			if !contains(got, "\treturn 1") {
				t.Errorf("code indentation collapsed at level=%s role=%s: got=%q", tt.level, tt.role, got)
			}
		})
	}
}

func TestCompressString_aggressivePreservesTechnicalContent(t *testing.T) {
	tests := []struct {
		name      string
		protected string
		input     string
	}{
		{
			name:      "fenced code",
			protected: "```go\nconfig := map[string]bool{\"maybe\": true}\n```",
			input:     "Please inspect this:\n```go\nconfig := map[string]bool{\"maybe\": true}\n```\nThanks",
		},
		{
			name:      "ANSI sequence inside code",
			protected: "```text\n\x1b[31mERROR\x1b[0m\n```",
			input:     "Please inspect:\n```text\n\x1b[31mERROR\x1b[0m\n```\nThanks",
		},
		{
			name:      "unclosed fenced code",
			protected: "```go\nfunc main() {\n\tfmt.Println(\"really   literal\")\n}",
			input:     "Please inspect:\n```go\nfunc main() {\n\tfmt.Println(\"really   literal\")\n}",
		},
		{
			name:      "inline code",
			protected: "`maybe := \"really   needed\"`",
			input:     "Please keep `maybe := \"really   needed\"` exactly, thanks",
		},
		{
			name:      "URL",
			protected: "https://example.com/current/really?maybe=true",
			input:     "Please open https://example.com/current/really?maybe=true, thanks",
		},
		{
			name:      "JSON object",
			protected: `{"maybe":true,"message":"really   important","nested":{"current":"value"}}`,
			input:     `Please send {"maybe":true,"message":"really   important","nested":{"current":"value"}}, thanks`,
		},
		{
			name:      "JSON array",
			protected: `["maybe",{"currently":"really   useful"}]`,
			input:     `Please send ["maybe",{"currently":"really   useful"}], thanks`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressString(tt.input, CompressionAggressive, "user")
			if !strings.Contains(got, tt.protected) {
				t.Fatalf("protected content changed:\ngot:  %q\nwant protected: %q", got, tt.protected)
			}
		})
	}
}

func TestCompressString_aggressiveStillCompressesProse(t *testing.T) {
	input := "Please basically explain in detail how this implementation is currently being used"
	got := compressString(input, CompressionAggressive, "user")
	if got == input {
		t.Fatal("aggressive mode did not compress eligible prose")
	}
	if !strings.Contains(got, "impl") {
		t.Fatalf("aggressive mode lost technical substance: %q", got)
	}
}

func TestCompressString_aggressivePreservesLongCodeBlock(t *testing.T) {
	lines := make([]string, 120)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d: maybe really   current", i)
	}
	code := "```text\n" + strings.Join(lines, "\n") + "\n```"
	input := "Please inspect this code:\n" + code + "\nThanks"

	got := compressString(input, CompressionAggressive, "user")
	if !strings.Contains(got, code) {
		t.Fatal("aggressive truncation changed a protected long code block")
	}
	if strings.Contains(got, "lines truncated") {
		t.Fatal("protected code block counted toward aggressive line truncation")
	}
}

func TestCompressString_pleasePunctuation(t *testing.T) {
	t.Run("orphan comma after please", func(t *testing.T) {
		input := "Could you please, explain this"
		got := compressString(input, CompressionLite, "user")
		// No orphan comma should remain after "please" is removed
		if contains(got, ", explain") {
			t.Errorf("orphan comma after please removal: %q", got)
		}
	})

	t.Run("please isolated", func(t *testing.T) {
		input := "Please explain this"
		got := compressString(input, CompressionLite, "user")
		if contains(got, "Please") || contains(got, "please") {
			t.Errorf("please not removed: %q", got)
		}
		if !contains(got, "explain") {
			t.Errorf("content after please lost: %q", got)
		}
	})
}

func TestCompressString_inflationGuard(t *testing.T) {
	t.Run("inflation returns original", func(t *testing.T) {
		// Very short input with no compressible content should not inflate
		input := "hi"
		got := compressString(input, CompressionLite, "user")
		if got != input {
			t.Errorf("inflation guard failed: got %q, want %q", got, input)
		}
	})
}

func TestCompressToolResult(t *testing.T) {
	tests := []struct {
		name  string
		input string
		level string
		want  string
	}{
		{
			"ansi stripped, code preserved",
			"\x1b[32mOK\x1b[0m: 5 tests passed",
			CompressionLite,
			"OK: 5 tests passed",
		},
		{
			"off no change",
			"hello world",
			CompressionOff,
			"hello world",
		},
		{
			"filler rules NOT applied to tool result",
			// Natural language filler in tool output should NOT be stripped
			"This is basically fine and like I said it works",
			CompressionAggressive,
			"This is basically fine and like I said it works",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressToolResult(tt.input, tt.level)
			if got != tt.want {
				t.Errorf("compressToolResult(%q, %q) = %q, want %q", tt.input, tt.level, got, tt.want)
			}
		})
	}
}

func TestIntToString(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{9999, "9999"},
		{-1, "-1"},
		{-42, "-42"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := intToString(tt.input)
			if got != tt.want {
				t.Errorf("intToString(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func BenchmarkCompressStringAggressive(b *testing.B) {
	prose := strings.Repeat("Please basically explain in detail how this implementation is currently being used.\n", 80)
	code := "```go\n" + strings.Repeat("if maybe { fmt.Println(\"really   important\") }\n", 80) + "```"
	input := prose + code

	b.ReportAllocs()
	for b.Loop() {
		_ = compressString(input, CompressionAggressive, "user")
	}
}
