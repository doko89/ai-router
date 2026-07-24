package compression

// Compression levels.
const (
	CompressionOff        = "off"
	CompressionLite       = "lite"
	CompressionStandard   = "standard"
	CompressionAggressive = "aggressive"
)

// PipelineConfig controls which compression stages are active per level.
// Each stage can be independently toggled. The defaults below match the
// behaviour of the legacy monolithic compressor.
type PipelineConfig struct {
	// Stages
	ProtectTech  bool // protect code fences, inline code, URLs, JSON
	StripANSI    bool // strip ANSI escape sequences
	FillerRules  bool // remove filler words and pleasantries (EN + ID)
	ContextRules bool // condense context-setting phrases
	StructRules  bool // compress verbose structures
	DictRules    bool // dictionary substitution (abbreviations)
	DedupLines   bool // deduplicate repeated lines
	UltraRules   bool // aggressive abbreviation rules (EN + ID)
	Truncate     bool // head/tail truncation of repeated content
	CollapseWS   bool // collapse whitespace
}

// DefaultPipeline returns the standard pipeline config for a given level.
func DefaultPipeline(level string) PipelineConfig {
	switch normalizeLevel(level) {
	case CompressionLite:
		return PipelineConfig{
			FillerRules:  true,
			ContextRules: true,
			CollapseWS:   true,
		}
	case CompressionStandard:
		return PipelineConfig{
			StripANSI:    true,
			FillerRules:  true,
			ContextRules: true,
			StructRules:  true,
			DictRules:    true,
			DedupLines:   true,
			CollapseWS:   true,
		}
	case CompressionAggressive:
		return PipelineConfig{
			ProtectTech:  true,
			StripANSI:    true,
			FillerRules:  true,
			ContextRules: true,
			StructRules:  true,
			DictRules:    true,
			DedupLines:   true,
			UltraRules:   true,
			Truncate:     true,
			CollapseWS:   true,
		}
	default:
		return PipelineConfig{}
	}
}
