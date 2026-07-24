package compression

import (
	"strings"
)

// Compressor is the interface each stage of the compression pipeline implements.
type Compressor interface {
	Compress(ctx *Context) error
}

// Context carries the state through the compression pipeline.
type Context struct {
	// Input/output: the text being compressed (mutated in place by stages).
	Text string

	// Metadata
	Level string
	Role  string // "user", "assistant", "system"
	Orig  string // pre-compression original (for inflation guard)

	// Protected blocks (set by Protect stage, restored after rules apply).
	Protected []protectedBlock

	// Metric
	BytesSaved int
}

// Pipeline orchestrates a sequence of Compressor stages.
type Pipeline struct {
	stages []Compressor
}

// NewPipeline creates a pipeline from the given stages.
func NewPipeline(stages ...Compressor) *Pipeline {
	return &Pipeline{stages: stages}
}

// CompressText runs the full pipeline on a single text string.
// This is the primary entry point for compressing a single message.
func (p *Pipeline) CompressText(text, level, role string) string {
	level = normalizeLevel(level)
	if level == CompressionOff || text == "" {
		return text
	}

	ctx := &Context{
		Text:  text,
		Level: level,
		Role:  role,
		Orig:  text,
	}

	for _, stage := range p.stages {
		if err := stage.Compress(ctx); err != nil {
			// On error, return original to avoid data loss.
			return text
		}
	}

	// Inflation guard: if compression made it larger, return original.
	if len(ctx.Text) > len(ctx.Orig) {
		return ctx.Orig
	}
	return ctx.Text
}

// CompressToolResult compresses tool output (file content, command output).
// Only safe operations: ANSI stripping, line dedup, truncation.
// No natural-language filler rules that could corrupt code identifiers.
func (p *Pipeline) CompressToolResult(text, level string) string {
	level = normalizeLevel(level)
	if level == CompressionOff || text == "" {
		return text
	}

	orig := text
	text = stripANSI(text)

	if level == CompressionStandard || level == CompressionAggressive {
		text = dedupLines(text)
	}

	if level == CompressionAggressive {
		text = truncateHeadTail(text)
	}

	if len(text) > len(orig) {
		return orig
	}
	return text
}

// --- Stage implementations ---

// protectStage wraps technical content protection.
type protectStage struct{}

func (s *protectStage) Compress(ctx *Context) error {
	ctx.Text, ctx.Protected = protectTechnicalContent(ctx.Text)
	return nil
}

// restoreStage restores protected content after rules have been applied.
type restoreStage struct{}

func (s *restoreStage) Compress(ctx *Context) error {
	if len(ctx.Protected) > 0 {
		ctx.Text = restoreProtectedContent(ctx.Text, ctx.Protected)
	}
	return nil
}

// stripANSIStage strips ANSI escape sequences.
type stripANSIStage struct{}

func (s *stripANSIStage) Compress(ctx *Context) error {
	ctx.Text = stripANSI(ctx.Text)
	return nil
}

// rulesStage applies a set of regex rules to the text.
type rulesStage struct {
	rules   []rule
	roleCtx ruleContext
}

func (s *rulesStage) Compress(ctx *Context) error {
	msgCtx := roleToContext(ctx.Role)
	ctx.Text = applyRules(ctx.Text, s.rules, msgCtx)
	return nil
}

// dedupLinesStage deduplicates repeated lines.
type dedupLinesStage struct{}

func (s *dedupLinesStage) Compress(ctx *Context) error {
	ctx.Text = dedupLines(ctx.Text)
	return nil
}

// truncateStage removes repeated head/tail content.
type truncateStage struct{}

func (s *truncateStage) Compress(ctx *Context) error {
	ctx.Text = truncateHeadTail(ctx.Text)
	return nil
}

// collapseWSStage collapses whitespace.
type collapseWSStage struct{}

func (s *collapseWSStage) Compress(ctx *Context) error {
	ctx.Text = collapseWhitespace(ctx.Text)
	return nil
}

// --- Pipeline builders ---

// NewDefaultPipeline builds a pipeline from the default stage config for a level.
func NewDefaultPipeline(level string) *Pipeline {
	cfg := DefaultPipeline(level)
	return NewPipeline(buildStages(cfg)...)
}

func buildStages(cfg PipelineConfig) []Compressor {
	var stages []Compressor

	if cfg.ProtectTech {
		stages = append(stages, &protectStage{})
	}
	if cfg.StripANSI {
		stages = append(stages, &stripANSIStage{})
	}
	if cfg.FillerRules {
		stages = append(stages, &rulesStage{rules: fillerRules})
		stages = append(stages, &rulesStage{rules: idFillerRules})
	}
	if cfg.ContextRules {
		stages = append(stages, &rulesStage{rules: contextRules})
		stages = append(stages, &rulesStage{rules: idContextRules})
	}
	if cfg.StructRules {
		stages = append(stages, &rulesStage{rules: structuralRules})
		stages = append(stages, &rulesStage{rules: idStructuralRules})
	}
	if cfg.DictRules {
		stages = append(stages, &rulesStage{rules: dictRules})
		stages = append(stages, &rulesStage{rules: idDictRules})
	}
	if cfg.DedupLines {
		stages = append(stages, &rulesStage{rules: dedupRules})
		stages = append(stages, &dedupLinesStage{})
	}
	if cfg.UltraRules {
		stages = append(stages, &rulesStage{rules: ultraRules})
		stages = append(stages, &rulesStage{rules: idUltraRules})
	}
	if cfg.Truncate {
		stages = append(stages, &truncateStage{})
	}

	// Collapse whitespace always runs after protection is restored
	if cfg.ProtectTech {
		stages = append(stages, &restoreStage{})
	}
	if cfg.CollapseWS {
		stages = append(stages, &collapseWSStage{})
	}

	return stages
}

// roleToContext maps a role string to a rule context.
func roleToContext(role string) ruleContext {
	switch role {
	case "user":
		return ctxUser
	case "assistant":
		return ctxAssistant
	case "system":
		return ctxSystem
	default:
		return ctxAll
	}
}

// normalizeLevel handles common typos in compression level config values.
func normalizeLevel(level string) string {
	switch strings.ToLower(level) {
	case "agressive", "agresive":
		return CompressionAggressive
	case "standart", "standar":
		return CompressionStandard
	default:
		return strings.ToLower(level)
	}
}
