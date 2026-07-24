package compression

import "regexp"

// dictRules: English dictionary substitution — replace verbose terms with shorter equivalents.
var dictRules = []rule{
	{re: regexp.MustCompile(`(?i)\bsimultaneously\b`), with: "sync", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bapproximately\b`), with: "approx", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bimmediately\b`), with: "now", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\baccordingly\b`), with: "so", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bconsequently\b`), with: "so", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bnevertheless\b`), with: "still", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bnonetheless\b`), with: "still", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bspecifically\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bessentially\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bprimarily\b`), with: "mostly", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bpotentially\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\btypically\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bgenerally\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bcurrently\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\brecently\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bpreviously\b`), with: "prev", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bappropriate\b`), with: "right", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bnecessary\b`), with: "needed", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bparticular\b`), with: "specific", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bavailable\b`), with: "avail", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bcomplete\b`), with: "done", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bensure\b`), with: "make sure", context: ctxAll},
}

// idDictRules: Indonesian dictionary substitution.
var idDictRules = []rule{
	{re: regexp.MustCompile(`(?i)\b(?:melakukan|melaksanakan|menjalankan)\b`), with: "lakukan", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:memberikan|menyediakan|menghasilkan)\b`), with: "beri", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:mendapatkan|memperoleh|menerima)\b`), with: "dapat", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:menemukan|mengetahui|mengidentifikasi)\b`), with: "temukan", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:membutuhkan|memerlukan|mengharuskan)\b`), with: "perlu", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:memungkinkan|memperbolehkan|mengizinkan)\b`), with: "boleh", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:menunjukkan|memperlihatkan|menampilkan)\b`), with: "tunjuk", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:memastikan|menjamin|meyakinkan)\b`), with: "pastikan", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:mempertimbangkan|memikirkan|mengevaluasi)\b`), with: "pikir", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:menyelesaikan|menuntaskan|merampungkan)\b`), with: "selesaikan", context: ctxAll},
}
