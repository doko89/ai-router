package compression

import (
	"strings"
	"testing"
)

// =============================================================================
// Unit tests: normalizeLevel
// =============================================================================

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

// =============================================================================
// Unit tests: collapseWhitespace
// =============================================================================

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "inline spaces collapsed",
			input: "if  err !=  nil {",
			want:  "if err != nil {",
		},
		{
			name:  "multiple spaces collapsed",
			input: "hello    world",
			want:  "hello world",
		},
		{
			name:  "tabs and spaces",
			input: "hello   \t  world",
			want:  "hello world",
		},
		{
			name:  "trailing whitespace trimmed",
			input: "hello world   ",
			want:  "hello world",
		},
		{
			name:  "multiline code preserved structurally",
			input: "def foo():\n    x =  1\n    return  x",
			want:  "def foo():\n x = 1\n return x",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
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

// =============================================================================
// Unit tests: dedupLines
// =============================================================================

func TestDedupLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no duplicates unchanged",
			input: "line one\nline two\nline three",
			want:  "line one\nline two\nline three",
		},
		{
			name:  "consecutive duplicates removed",
			input: "dup\ndup\ndup",
			want:  "dup",
		},
		{
			name:  "non-consecutive duplicates removed",
			input: "dup\nother\ndup",
			want:  "dup\nother",
		},
		{
			name:  "empty lines preserved in order",
			input: "a\n\nb\n\nc",
			want:  "a\n\nb\n\nc",
		},
		{
			name:  "single line unchanged",
			input: "only line",
			want:  "only line",
		},
		{
			name:  "empty unchanged",
			input: "",
			want:  "",
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

// =============================================================================
// Unit tests: stripANSI
// =============================================================================

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple color", "\x1b[31mred text\x1b[0m", "red text"},
		{"bold + color", "\x1b[1;32mbold green\x1b[0m", "bold green"},
		{"cursor sequence", "\x1b[2J\x1b[Hhello", "hello"},
		{"no ansi pass-through", "plain text", "plain text"},
		{"empty", "", ""},
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

// =============================================================================
// Unit tests: truncateHeadTail
// =============================================================================

func TestTruncateHeadTail(t *testing.T) {
	t.Run("short string unchanged", func(t *testing.T) {
		input := "short\nstring\nhere"
		got := truncateHeadTail(input)
		if got != input {
			t.Errorf("short string: got %q, want %q", got, input)
		}
	})

	t.Run("under 9 lines unchanged", func(t *testing.T) {
		input := strings.Repeat("line\n", 7)
		got := truncateHeadTail(input)
		if got != input {
			t.Errorf("under 9 lines should be unchanged")
		}
	})
}

// =============================================================================
// Pipeline integration tests
// =============================================================================

func TestPipelineCompressText_lite(t *testing.T) {
	p := NewDefaultPipeline(CompressionLite)

	t.Run("SQL LIKE preserved", func(t *testing.T) {
		input := "SELECT * FROM users WHERE name LIKE 'test%'"
		got := p.CompressText(input, CompressionLite, "user")
		if !strings.Contains(got, "LIKE") {
			t.Errorf("SQL LIKE keyword should be preserved: %q", got)
		}
	})

	t.Run("verb like preserved", func(t *testing.T) {
		input := "I like programming"
		got := p.CompressText(input, CompressionLite, "user")
		if !strings.Contains(got, "like") {
			t.Errorf("verb 'like' should be preserved: %q", got)
		}
	})

	t.Run("please removed", func(t *testing.T) {
		input := "please help me with this code"
		got := p.CompressText(input, CompressionLite, "user")
		if strings.Contains(got, "please") {
			t.Errorf("'please' should be removed: %q", got)
		}
	})
}

func TestPipelineCompressText_codeIndentation(t *testing.T) {
	levels := []string{CompressionLite, CompressionStandard, CompressionAggressive}
	roles := []string{"user", "assistant", "system"}

	for _, level := range levels {
		for _, role := range roles {
			t.Run(level+"/"+role, func(t *testing.T) {
				p := NewDefaultPipeline(level)
				input := "\tfunc foo() {\n\t\tif x := 1; x > 0 {\n\t\t\treturn x\n\t\t}\n\t}"
				got := p.CompressText(input, level, role)
				// Code structural tokens must be preserved even if whitespace is normalized.
				if !strings.Contains(got, "func") || !strings.Contains(got, "return") {
					t.Errorf("code structure lost at level=%q role=%q: %q", level, role, got)
				}
			})
		}
	}
}

func TestPipelineCompressText_aggressivePreservesTechnicalContent(t *testing.T) {
	p := NewDefaultPipeline(CompressionAggressive)

	t.Run("code fence preserved", func(t *testing.T) {
		input := "here is my code:\n```go\nfunc hello() {\n\tfmt.Println(\"hi\")\n}\n```\nend"
		got := p.CompressText(input, CompressionAggressive, "user")
		if !strings.Contains(got, "```go") || !strings.Contains(got, "fmt.Println") {
			t.Errorf("code fence content not preserved: %q", got)
		}
	})

	t.Run("inline code preserved", func(t *testing.T) {
		input := "use `ctx.Err()` to check cancellation"
		got := p.CompressText(input, CompressionAggressive, "user")
		if !strings.Contains(got, "`ctx.Err()`") && !strings.Contains(got, "ctx.Err()") {
			t.Errorf("inline code not preserved: %q", got)
		}
	})

	t.Run("URL preserved", func(t *testing.T) {
		input := "see https://github.com/user/repo for details"
		got := p.CompressText(input, CompressionAggressive, "user")
		if !strings.Contains(got, "https://") {
			t.Errorf("URL not preserved: %q", got)
		}
	})

	t.Run("JSON preserved", func(t *testing.T) {
		input := "payload: {\"key\": \"value\", \"num\": 42}"
		got := p.CompressText(input, CompressionAggressive, "user")
		if !strings.Contains(got, "\"key\"") {
			t.Errorf("JSON not preserved: %q", got)
		}
	})

	t.Run("hex values preserved", func(t *testing.T) {
		input := "the address is 0xDEADBEEF and pointer is 0xFF00"
		got := p.CompressText(input, CompressionAggressive, "user")
		if !strings.Contains(got, "0xDEADBEEF") {
			t.Errorf("hex value not preserved: %q", got)
		}
	})
}

func TestPipelineCompressText_aggressiveStillCompressesProse(t *testing.T) {
	p := NewDefaultPipeline(CompressionAggressive)
	input := "I think that basically you should use the function in order to get the result because it is being used that way"
	got := p.CompressText(input, CompressionAggressive, "user")
	// Must be shorter than original
	if len(got) >= len(input) {
		t.Errorf("aggressive compression didn't reduce prose length: %d >= %d", len(got), len(input))
	}
}

func TestPipelineCompressText_inflationGuard(t *testing.T) {
	p := NewDefaultPipeline(CompressionAggressive)
	input := "hi"
	got := p.CompressText(input, CompressionAggressive, "user")
	if len(got) > len(input) {
		t.Errorf("inflation guard failed: %d > %d, got %q", len(got), len(input), got)
	}
}

func TestPipelineCompressText_identity(t *testing.T) {
	t.Run("empty string unchanged", func(t *testing.T) {
		for _, level := range []string{CompressionLite, CompressionStandard, CompressionAggressive} {
			p := NewDefaultPipeline(level)
			got := p.CompressText("", level, "user")
			if got != "" {
				t.Errorf("%q: empty should stay empty, got %q", level, got)
			}
		}
	})

	t.Run("off level unchanged", func(t *testing.T) {
		p := NewDefaultPipeline(CompressionOff)
		input := "please basically explain in detail how this works"
		got := p.CompressText(input, CompressionOff, "user")
		if got != input {
			t.Errorf("off level should return input unchanged: %q", got)
		}
	})
}

// =============================================================================
// Tool result compression tests
// =============================================================================

func TestPipelineCompressToolResult(t *testing.T) {
	p := NewDefaultPipeline(CompressionAggressive)

	t.Run("ANSI stripped", func(t *testing.T) {
		input := "\x1b[32mOK\x1b[0m test passed"
		got := p.CompressToolResult(input, CompressionAggressive)
		if strings.Contains(got, "\x1b") {
			t.Errorf("ANSI should be stripped: %q", got)
		}
	})

	t.Run("off level no change", func(t *testing.T) {
		input := "\x1b[32mOK\x1b[0m test passed"
		got := p.CompressToolResult(input, CompressionOff)
		if got != input {
			t.Errorf("off level should not change: %q != %q", got, input)
		}
	})

	t.Run("natural language rules NOT applied", func(t *testing.T) {
		input := "please basically explain in detail how function foo works"
		got := p.CompressToolResult(input, CompressionAggressive)
		if !strings.Contains(got, "please") {
			t.Errorf("tool result must preserve all text (only ANSI/dedup/trunc): %q", got)
		}
		if !strings.Contains(got, "basically") {
			t.Errorf("tool result must preserve all text (only ANSI/dedup/trunc): %q", got)
		}
	})
}

// =============================================================================
// Benchmark
// =============================================================================

func BenchmarkPipelineCompressTextAggressive(b *testing.B) {
	p := NewDefaultPipeline(CompressionAggressive)
	prose := strings.Repeat("Please basically explain in detail how this implementation is currently being used.\n", 80)
	code := "```go\n" + strings.Repeat("if maybe { fmt.Println(\"really   important\") }\n", 80) + "```"
	input := prose + code

	b.ReportAllocs()
	for b.Loop() {
		_ = p.CompressText(input, CompressionAggressive, "user")
	}
}
