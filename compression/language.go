package compression

import "regexp"

// ruleContext scopes a rule to a specific message role.
type ruleContext int

const (
	ctxAll ruleContext = iota
	ctxUser
	ctxAssistant
	ctxSystem
)

// rule is a single regex-based compression rule.
type rule struct {
	re      *regexp.Regexp
	with    string
	context ruleContext
}

func applyRules(s string, rules []rule, msgCtx ruleContext) string {
	for _, r := range rules {
		if r.context != ctxAll && r.context != msgCtx {
			continue
		}
		s = r.re.ReplaceAllString(s, r.with)
	}
	return s
}

// =============================================================================
// English Rules
// =============================================================================

// fillerRules: pleasantries, hedging, filler words, polite framing, qualifiers.
var fillerRules = []rule{
	// pleasantries
	{re: regexp.MustCompile(`(?i)\b(?:thank you so much|thanks in advance|i really appreciate)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:thank you|thanks|no problem|you're welcome|you are welcome)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:i'?d be happy to|i would be happy to|i'?d be glad to|glad to help|happy to|of course|certainly|absolutely)\b[,.!?\s]*`), with: "", context: ctxAll},

	// polite framing
	{re: regexp.MustCompile(`(?i)\b(?:please|kindly|could you please|would you please|can you please|i would like you to|i want you to|i need you to)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:i was wondering if you could|would it be possible to)\b\s*`), with: "", context: ctxUser},

	// hedging
	{re: regexp.MustCompile(`(?i)\b(?:it seems like|it appears that|i think that|i believe that|probably|possibly|maybe)\b\s*`), with: "", context: ctxAll},

	// filler adverbs
	{re: regexp.MustCompile(`(?i)\b(?:basically|essentially|actually|literally|simply|currently)\b\s*`), with: "", context: ctxAll},

	// filler phrases
	{re: regexp.MustCompile(`(?i)^(?:i want to|i need to|i'?d like to|i'm looking for)\b\s*`), with: "", context: ctxUser},
	{re: regexp.MustCompile(`(?i)^(?:i am trying to|i am working on|i have been)\b\s*`), with: "", context: ctxUser},

	// redundant openers
	{re: regexp.MustCompile(`(?i)^(?:hi there|hello|good morning|hey)\s*[,.!?\s]?\s*`), with: "", context: ctxUser},

	// qualifiers & softeners
	{re: regexp.MustCompile(`(?i)\b(?:a bit|a little|somewhat|kind of|sort of)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:if possible|when you get a chance|at your convenience|just wondering)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:i guess|i suppose|more or less|in a way)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:you know|i mean)\b\s*`), with: "", context: ctxAll},

	// verbose instructions
	{re: regexp.MustCompile(`(?i)\b(?:provide a detailed explanation of|give me a comprehensive|write an in-depth|create a thorough|explain in detail)\b`), with: "explain", context: ctxAll},

	// assistant fillers
	{re: regexp.MustCompile(`(?i)^(?:here'?s|below is|this is)\s+(?:a|an|the)?\s*`), with: "", context: ctxAssistant},

	// common filler words (safe single-word removals)
	{re: regexp.MustCompile(`(?i)\bjust\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\breally\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bquite\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bwell\b\s*`), with: "", context: ctxAll},
}

// contextRules: context-setting phrase condensation.
var contextRules = []rule{
	{re: regexp.MustCompile(`(?i)\b(?:i have the following code|here is my code|here's my code|my code below|the code is below)\b\s*[:.]?\s*`), with: "Code: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:here is the file|here's the file|the file content below|here are the files)\b\s*[:.]?\s*`), with: "File: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:what i'm trying to do|my goal is|what i need is|i want to achieve)\b\s*`), with: "Goal: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:can you explain why|could you show me how|would you tell me|can you tell me)\b\s*`), with: "Explain: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:the function appears to be handling|the code seems to|the class is|this module is)\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:as you may know|as we discussed earlier|as mentioned before)\b,?\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)^(?:note that|keep in mind that|remember that)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:for the purpose of|with the goal of|in an effort to)\b`), with: "to", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\band any potential\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bfor every\b`), with: "per", context: ctxAll},
}

// structuralRules: redundant phrasing, purpose phrases, connectors, emphasis, passive voice.
var structuralRules = []rule{
	// purpose phrases
	{re: regexp.MustCompile(`(?i)\bin order to\b\s*`), with: "to ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bso as to\b\s*`), with: "to ", context: ctxAll},

	// redundant because
	{re: regexp.MustCompile(`(?i)\bdue to the fact that\b\s*`), with: "because ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bthe reason is because\b\s*`), with: "because ", context: ctxAll},

	// redundant phrasing
	{re: regexp.MustCompile(`(?i)\bmake sure to\b\s*`), with: "ensure ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bbe sure to\b\s*`), with: "ensure ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bit is important to\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\byou should\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bremember to\b\s*`), with: "", context: ctxAll},

	// list conjunctions
	{re: regexp.MustCompile(`,?\s*and also\s+`), with: ", ", context: ctxAll},
	{re: regexp.MustCompile(`,?\s*as well as\s+`), with: ", ", context: ctxAll},

	// redundant quantifiers
	{re: regexp.MustCompile(`(?i)\beach and every single\b`), with: "each", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\beach and every\b`), with: "each", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bany and all\b`), with: "all", context: ctxAll},

	// verbose connectors
	{re: regexp.MustCompile(`(?i)\b(?:furthermore|additionally|moreover|in addition)\b\s*`), with: "also ", context: ctxAll},

	// transition removal
	{re: regexp.MustCompile(`(?i)^(?:on the other hand|in contrast|however),?\s*`), with: "", context: ctxAll},

	// emphasis removal
	{re: regexp.MustCompile(`(?i)\b(?:very|really|extremely|highly|quite)\s+`), with: "", context: ctxAll},

	// passive voice → active
	{re: regexp.MustCompile(`(?i)\bis being used\b`), with: "uses", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bis being called\b`), with: "calls", context: ctxAll},

	// common compressions
	{re: regexp.MustCompile(`(?i)\bon a regular basis\b`), with: "regularly", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\ba number of\b`), with: "some", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bthe majority of\b`), with: "most", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bin the event that\b`), with: "if", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bat this point in time\b`), with: "now", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bin the near future\b`), with: "soon", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bis able to\b`), with: "can", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bhas the ability to\b`), with: "can", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bis capable of\b`), with: "can", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bwith regard to\b`), with: "about", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bin relation to\b`), with: "about", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bin addition to\b`), with: "also", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\ba lot of\b`), with: "many", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bnot only\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bbut also\b`), with: "also", context: ctxAll},
}

// ultraRules: aggressive abbreviation rules (EN).
var ultraRules = []rule{
	// leader phrases
	{re: regexp.MustCompile(`(?i)^(?:i'?ll|i will|i can|i'?d|let me|you can|we will|we can|let'?s)\s+`), with: "", context: ctxAll},

	// common abbreviations
	{re: regexp.MustCompile(`(?i)\bdatabase\b`), with: "DB", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bconfiguration\b`), with: "config", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bfunction\b`), with: "fn", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\brequest\b`), with: "req", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bresponse\b`), with: "res", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bimplementation\b`), with: "impl", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bauthentication\b`), with: "auth", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bauthorization\b`), with: "authz", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bapplication\b`), with: "app", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dependency|dependencies)\b`), with: "dep", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\brepository\b`), with: "repo", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bvariable\b`), with: "var", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\binformation\b`), with: "info", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bparameter\b`), with: "param", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bargument\b`), with: "arg", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bprevious\b`), with: "prev", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bcurrent\b`), with: "curr", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\btemporary\b`), with: "temp", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\badditional\b`), with: "extra", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:document|documentation)\b`), with: "docs", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\butilize\b`), with: "use", context: ctxAll},
}

// dedupRules: repeated context markers, summary replacements.
var dedupRules = []rule{
	{re: regexp.MustCompile(`(?i)\b(?:as we discussed earlier|as mentioned before|as previously stated|as i said before)\b[,.]?\s*`), with: "See above. ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:same question as before|i asked this earlier|this is the same question)\b[,.]?\s*`), with: "[same] ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:going back to the code above|referring back to|returning to)\b\s*`), with: "Re: ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:to summarize what we've discussed|in summary of our conversation|to recap)\b[,.]?\s*`), with: "Summary: ", context: ctxAssistant},
}

// =============================================================================
// Indonesian Rules
// =============================================================================

// idFillerRules: Indonesian filler words and phrases.
var idFillerRules = []rule{
	// pleasantries
	{re: regexp.MustCompile(`(?i)\b(?:terima kasih|makasih|matur nuwun|hatur nuhun|trima kasih|tq|thanks)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:sama-sama|kembali kasih|kembali)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dengan senang hati|senang bisa membantu|ikut senang)\b[,.!?\s]*`), with: "", context: ctxAll},

	// polite framing
	{re: regexp.MustCompile(`(?i)\b(?:mohon|tolong|tlg|bisa tolong|mohon bantuannya|minta tolong)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:saya ingin|saya mau|saya butuh|aku ingin|gw mau|gua mau|saya pengen)\b\s*`), with: "", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:bisa tolong|bisa minta|bantuin|tolongin)\b[,.!?\s]*`), with: "", context: ctxUser},

	// hedging
	{re: regexp.MustCompile(`(?i)\b(?:sepertinya|tampaknya|rasanya|kiranya|kayaknya)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:saya rasa|saya kira|saya pikir|menurut saya|menurut ku|menurut gue)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:mungkin|barangkali|bisa jadi|boleh jadi)\b\s*`), with: "", context: ctxAll},

	// filler adverbs
	{re: regexp.MustCompile(`(?i)\b(?:pada dasarnya|sebenarnya|intinya|pokoknya|pada intinya)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:secara umum|pada umumnya|umumnya|biasanya|lazimnya)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:kurang lebih|kira-kira|perkiraan)\b\s*`), with: "", context: ctxAll},

	// redundant openers
	{re: regexp.MustCompile(`(?i)^(?:halo|hai|hey|pagi|siang|sore|malam)\s*[,.!?\s]?\s*`), with: "", context: ctxUser},
	{re: regexp.MustCompile(`(?i)^(?:halo gan|halo min|halo kak|halo bro|halo sis)\s*[,.!?\s]?\s*`), with: "", context: ctxUser},
	{re: regexp.MustCompile(`(?i)^(?:assalamualaikum|salam sejahtera|salam)\s*[,.!?\s]?\s*`), with: "", context: ctxUser},

	// qualifiers & softeners
	{re: regexp.MustCompile(`(?i)\b(?:sedikit|agak|terkadang|kadang-kadang|adakalanya)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:jika memungkinkan|kalau bisa|kalau sempat|bila sempat)\b[,.!?\s]*`), with: "", context: ctxAll},

	// self-reference
	{re: regexp.MustCompile(`(?i)^(?:saya|aku|gue|gw|gua|ane|sy|aing)\s+(?:sedang|lagi|masih)\s+mencoba\b\s*`), with: "", context: ctxUser},

	// discourse fillers
	{re: regexp.MustCompile(`(?i)\b(?:kebetulan|sekadar|sejujurnya|jujur|sebenarnya|rupanya|ternyata|omong-omong|btw|fyi)\b\s*`), with: "", context: ctxAll},

	// excessive gratitude
	{re: regexp.MustCompile(`(?i)\b(?:terima kasih banyak|terima kasih sebelumnya|makasih banyak|sangat berterima kasih)\b[,.!?\s]*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:saya sangat menghargai|saya hargai|saya apresiasi)\b[,.!?\s]*`), with: "", context: ctxAll},

	// uncertainty fillers
	{re: regexp.MustCompile(`(?i)\b(?:saya rasa|saya kira|entahlah|gamfang|gak yakin|nggak yakin)\b\s*`), with: "", context: ctxAll},

	// assistant fillers
	{re: regexp.MustCompile(`(?i)^(?:berikut adalah|berikut ini|ini adalah|berikut)\s+`), with: "", context: ctxAssistant},

	// particles removal (informal)
	{re: regexp.MustCompile(`(?i)\b(?:dong|deh|sih|kok|loh|lah|tah|kah|pun|tuh|si|kek|ya)\b\s*`), with: "", context: ctxAll},
}

// idContextRules: Indonesian context condensation.
var idContextRules = []rule{
	{re: regexp.MustCompile(`(?i)\b(?:saya punya kode berikut|berikut adalah kode saya|berikut kodenya|ini kodenya|kode di bawah ini)\b\s*[:.]?\s*`), with: "Kode: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:ini filenya|berikut isi filenya|berkasnya|berikut isi berkasnya)\b\s*[:.]?\s*`), with: "File: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:apa yang saya coba lakukan|tujuan saya adalah|yang saya butuhkan adalah|saya menargetkan)\b\s*`), with: "Tujuan: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:bisa jelaskan kenapa|bisa tunjukkan bagaimana|tolong jelasin|tolong infoin|kasih tau gimana|tunjukkan cara)\b[,.!?\s]*`), with: "Jelaskan: ", context: ctxUser},
	{re: regexp.MustCompile(`(?i)\b(?:fungsi ini|kode ini|kelas ini|modul ini)\s+(?:nampaknya|tampaknya|sepertinya)\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:seperti yang anda ketahui|seperti yang kita diskusikan sebelumnya)\b,?\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)^(?:catat bahwa|perlu diingat bahwa|ingat bahwa)\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dengan tujuan untuk|dalam upaya untuk)\b`), with: "untuk", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:pertama-tama|kedua|selanjutnya|lalu|kemudian|di samping itu)\b`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:oleh karena itu|maka dari itu|dengan kata lain|sebagai catatan)\b`), with: "", context: ctxAll},
}

// idStructuralRules: Indonesian structural compression.
var idStructuralRules = []rule{
	{re: regexp.MustCompile(`(?i)\bpastikan untuk\b\s*`), with: "pastikan ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bperlu untuk\b\s*`), with: "perlu ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bdikarenakan adanya fakta bahwa\b\s*`), with: "karena ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\balasan utamanya adalah karena\b\s*`), with: "karena ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bpenting untuk\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\banda seharusnya\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bingat untuk\b\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`,?\s*dan juga\s+`), with: ", ", context: ctxAll},
	{re: regexp.MustCompile(`,?\s*serta\s+`), with: ", ", context: ctxAll},
	{re: regexp.MustCompile(`,?\s*maupun juga\s+`), with: ", ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bagar bisa untuk\b\s*`), with: "agar ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bsupaya bisa untuk\b\s*`), with: "supaya ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bbertujuan untuk\b\s*`), with: "untuk ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bmasing-masing dan setiap\b`), with: "setiap", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\btiap-tiap dan setiap\b`), with: "setiap", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bapapun dan semua\b`), with: "semua", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:selain itu|lebih lanjut lagi|tambahan lagi|dan juga)\b\s*`), with: "juga ", context: ctxAll},
	{re: regexp.MustCompile(`(?i)^(?:di sisi lain|sebaliknya|namun|akan tetapi),?\s*`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:sangat|sekali|amat|luar biasa|sungguh)\s+`), with: "", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bsedang digunakan\b`), with: "pakai", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bsedang dipanggil\b`), with: "panggil", context: ctxAll},
}

// idUltraRules: Indonesian aggressive abbreviations.
var idUltraRules = []rule{
	{re: regexp.MustCompile(`(?i)\b(?:sebelumnya|sblmny)\b`), with: "sblm", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:implementasi|implementasikan|penerapan)\b`), with: "impl", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:konfigurasi|pengaturan)\b`), with: "config", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dokumentasi|dokumentasikan)\b`), with: "docs", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:menggunakan|menggunakan|pake|memakai|memanfaatkan)\b`), with: "pakai", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dapat|bisa|mampu|dpt)\b`), with: "dpt", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:tidak|gak|nggak|tak|tdk)\b`), with: "tdk", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:namun|tetapi|tapi|tp)\b`), with: "tp", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:sebagai|sbg)\b`), with: "sbg", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:dengan|dgn)\b`), with: "dgn", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:untuk|utk|buat)\b`), with: "utk", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:atau|ato)\b`), with: "ato", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:yang|yg)\b`), with: "yg", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:kalau|klo)\b`), with: "klo", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:begitu|gitu)\b`), with: "gitu", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:bagaimana|gimana)\b`), with: "gmn", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:mengapa|kenapa|knp)\b`), with: "knp", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:sudah|sdh)\b`), with: "sdh", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:belum|blm)\b`), with: "blm", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:akan|akn)\b`), with: "akn", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:juga|jg)\b`), with: "jg", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:karena|krn)\b`), with: "krn", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:setelah|stlh)\b`), with: "stlh", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:sebelum|sblm)\b`), with: "sblm", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:terhadap|thd)\b`), with: "thd", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:tentang|ttg)\b`), with: "ttg", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:secara|scr)\b`), with: "scr", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:seperti|spt)\b`), with: "spt", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:bahwa|bwh)\b`), with: "bwh", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:hanya|cuma)\b`), with: "cma", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:saja|aja)\b`), with: "aja", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\btapi\b`), with: "tp", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bterus\b`), with: "trus", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\bbanget\b`), with: "bgt", context: ctxAll},
	{re: regexp.MustCompile(`(?i)\b(?:enggak|nggak|gak)\b`), with: "gk", context: ctxAll},
}
