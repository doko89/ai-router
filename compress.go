package main

import (
	"regexp"
	"strconv"
	"strings"
)

type ruleContext int

const (
	ctxAll ruleContext = iota
	ctxUser
	ctxAssistant
	ctxSystem
)

type rule struct {
	re      *regexp.Regexp
	with    string
	context ruleContext
}

// Compression levels.
const (
	CompressionOff        = "off"
	CompressionLite       = "lite"
	CompressionStandard   = "standard"
	CompressionAggressive = "aggressive"
)

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

var (
	ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	wsRegexp   = regexp.MustCompile(`\s+`)

	// fillerRules: pleasantries, hedging, filler words, polite framing, qualifiers
	fillerRules = []rule{
		// pleasantries
		{regexp.MustCompile(`(?i)\b(?:thank you so much|thanks in advance|i really appreciate)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:thank you|thanks|no problem|you're welcome|you are welcome)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:i'?d be happy to|i would be happy to|i'?d be glad to|glad to help|happy to|of course|certainly|absolutely)\b[,.!?\s]*`), "", ctxAll},

		// polite framing
		{regexp.MustCompile(`(?i)\b(?:please|kindly|could you please|would you please|can you please|i would like you to|i want you to|i need you to)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:i was wondering if you could|would it be possible to)\b\s*`), "", ctxUser},

		// hedging
		{regexp.MustCompile(`(?i)\b(?:it seems like|it appears that|i think that|i believe that|probably|possibly|maybe)\b\s*`), "", ctxAll},

		// filler adverbs
		{regexp.MustCompile(`(?i)\b(?:basically|essentially|actually|literally|simply|currently)\b\s*`), "", ctxAll},

		// filler phrases
		{regexp.MustCompile(`(?i)^(?:i want to|i need to|i'?d like to|i'm looking for)\b\s*`), "", ctxUser},
		{regexp.MustCompile(`(?i)^(?:i am trying to|i am working on|i have been)\b\s*`), "", ctxUser},

		// redundant openers
		{regexp.MustCompile(`(?i)^(?:hi there|hello|good morning|hey)\s*[,.!?\s]?\s*`), "", ctxUser},

		// qualifiers & softeners
		{regexp.MustCompile(`(?i)\b(?:a bit|a little|somewhat|kind of|sort of)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:if possible|when you get a chance|at your convenience|just wondering)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:i guess|i suppose|more or less|in a way)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:you know|i mean)\b\s*`), "", ctxAll},

		// verbose instructions
		{regexp.MustCompile(`(?i)\b(?:provide a detailed explanation of|give me a comprehensive|write an in-depth|create a thorough|explain in detail)\b`), "explain", ctxAll},

		// assistant fillers
		{regexp.MustCompile(`(?i)^(?:here'?s|below is|this is)\s+(?:a|an|the)?\s*`), "", ctxAssistant},

		// common filler words (safe single-word removals)
		{regexp.MustCompile(`(?i)\bjust\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\breally\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bquite\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bwell\b\s*`), "", ctxAll},
	}

	// contextRules: context setup, question→directive, background, meta, etc.
	contextRules = []rule{
		{regexp.MustCompile(`(?i)\b(?:i have the following code|here is my code|below is the code)\b\s*[:.]?\s*`), "Code: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:what i'm trying to do is|my objective is to|what i need is|i'm aiming to)\b\s*`), "Goal: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:can you explain why|could you show me how|would you tell me|can you tell me)\b\s*`), "Explain: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:the function appears to be handling|the code seems to|the class is|this module is)\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:as you may know|as we discussed earlier|as mentioned before)\b,?\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)^(?:note that|keep in mind that|remember that)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:for the purpose of|with the goal of|in an effort to)\b`), "to", ctxAll},
		{regexp.MustCompile(`(?i)\band any potential\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bfor every\b`), "per", ctxAll},
	}

	// structuralRules: redundant phrasing, purpose phrases, connectors, emphasis, passive voice
	structuralRules = []rule{
		// purpose phrases
		{regexp.MustCompile(`(?i)\bin order to\b\s*`), "to ", ctxAll},
		{regexp.MustCompile(`(?i)\bso as to\b\s*`), "to ", ctxAll},

		// redundant because
		{regexp.MustCompile(`(?i)\bdue to the fact that\b\s*`), "because ", ctxAll},
		{regexp.MustCompile(`(?i)\bthe reason is because\b\s*`), "because ", ctxAll},

		// redundant phrasing
		{regexp.MustCompile(`(?i)\bmake sure to\b\s*`), "ensure ", ctxAll},
		{regexp.MustCompile(`(?i)\bbe sure to\b\s*`), "ensure ", ctxAll},
		{regexp.MustCompile(`(?i)\bit is important to\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\byou should\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bremember to\b\s*`), "", ctxAll},

		// list conjunctions
		{regexp.MustCompile(`,?\s*and also\s+`), ", ", ctxAll},
		{regexp.MustCompile(`,?\s*as well as\s+`), ", ", ctxAll},

		// redundant quantifiers
		{regexp.MustCompile(`(?i)\beach and every single\b`), "each", ctxAll},
		{regexp.MustCompile(`(?i)\beach and every\b`), "each", ctxAll},
		{regexp.MustCompile(`(?i)\bany and all\b`), "all", ctxAll},

		// verbose connectors
		{regexp.MustCompile(`(?i)\b(?:furthermore|additionally|moreover|in addition)\b\s*`), "also ", ctxAll},

		// transition removal
		{regexp.MustCompile(`(?i)^(?:on the other hand|in contrast|however),?\s*`), "", ctxAll},

		// emphasis removal
		{regexp.MustCompile(`(?i)\b(?:very|really|extremely|highly|quite)\s+`), "", ctxAll},

		// passive voice → active
		{regexp.MustCompile(`(?i)\bis being used\b`), "uses", ctxAll},
		{regexp.MustCompile(`(?i)\bis being called\b`), "calls", ctxAll},
		{regexp.MustCompile(`(?i)\bis being generated\b`), "generates", ctxAll},
		{regexp.MustCompile(`(?i)\bwas created\b`), "created", ctxAll},
		{regexp.MustCompile(`(?i)\bwas generated\b`), "generated", ctxAll},
		{regexp.MustCompile(`(?i)\bwas implemented\b`), "implemented", ctxAll},

		// wordy constructions
		{regexp.MustCompile(`(?i)\ba number of\b`), "some", ctxAll},
		{regexp.MustCompile(`(?i)\bthe majority of\b`), "most", ctxAll},
		{regexp.MustCompile(`(?i)\bin the event that\b`), "if", ctxAll},
		{regexp.MustCompile(`(?i)\bat this point in time\b`), "now", ctxAll},
		{regexp.MustCompile(`(?i)\bin the near future\b`), "soon", ctxAll},
		{regexp.MustCompile(`(?i)\bis able to\b`), "can", ctxAll},
		{regexp.MustCompile(`(?i)\bhas the ability to\b`), "can", ctxAll},
		{regexp.MustCompile(`(?i)\bis capable of\b`), "can", ctxAll},
		{regexp.MustCompile(`(?i)\bwith regard to\b`), "about", ctxAll},
		{regexp.MustCompile(`(?i)\bin relation to\b`), "about", ctxAll},
		{regexp.MustCompile(`(?i)\bin addition to\b`), "also", ctxAll},
		{regexp.MustCompile(`(?i)\ba lot of\b`), "many", ctxAll},
		{regexp.MustCompile(`(?i)\bnot only\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bbut also\b`), "also", ctxAll},
	}

	// dedupRules: repeated context markers, summary replacements
	dedupRules = []rule{
		{regexp.MustCompile(`(?i)\b(?:as we discussed earlier|as mentioned before|as previously stated|as i said before)\b[,.]?\s*`), "See above. ", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:same question as before|i asked this earlier|this is the same question)\b[,.]?\s*`), "[same] ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:going back to the code above|referring back to|returning to)\b\s*`), "Re: ", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:to summarize what we've discussed|in summary of our conversation|to recap)\b[,.]?\s*`), "Summary: ", ctxAssistant},
	}

	// ultraRules: abbreviations, leader phrases, articles
	ultraRules = []rule{
		// leader phrases
		{regexp.MustCompile(`(?i)^(?:i'?ll|i will|i can|i'?d|let me|you can|we will|we can|let'?s)\s+`), "", ctxAll},

		// common abbreviations
		{regexp.MustCompile(`(?i)\bdatabase\b`), "DB", ctxAll},
		{regexp.MustCompile(`(?i)\bconfiguration\b`), "config", ctxAll},
		{regexp.MustCompile(`(?i)\bfunction\b`), "fn", ctxAll},
		{regexp.MustCompile(`(?i)\brequest\b`), "req", ctxAll},
		{regexp.MustCompile(`(?i)\bresponse\b`), "res", ctxAll},
		{regexp.MustCompile(`(?i)\bimplementation\b`), "impl", ctxAll},
		{regexp.MustCompile(`(?i)\bauthentication\b`), "auth", ctxAll},
		{regexp.MustCompile(`(?i)\bauthorization\b`), "authz", ctxAll},
		{regexp.MustCompile(`(?i)\bapplication\b`), "app", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dependency|dependencies)\b`), "dep", ctxAll},
		{regexp.MustCompile(`(?i)\brepository\b`), "repo", ctxAll},
		{regexp.MustCompile(`(?i)\bvariable\b`), "var", ctxAll},
		{regexp.MustCompile(`(?i)\binformation\b`), "info", ctxAll},
		{regexp.MustCompile(`(?i)\bparameter\b`), "param", ctxAll},
		{regexp.MustCompile(`(?i)\bargument\b`), "arg", ctxAll},
		{regexp.MustCompile(`(?i)\bprevious\b`), "prev", ctxAll},
		{regexp.MustCompile(`(?i)\bcurrent\b`), "curr", ctxAll},
		{regexp.MustCompile(`(?i)\btemporary\b`), "temp", ctxAll},
		{regexp.MustCompile(`(?i)\badditional\b`), "extra", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:document|documentation)\b`), "docs", ctxAll},
		{regexp.MustCompile(`(?i)\butilize\b`), "use", ctxAll},

	}

	// idFillerRules: Indonesian filler words and phrases
	idFillerRules = []rule{
		// pleasantries
		{regexp.MustCompile(`(?i)\b(?:terima kasih|makasih|matur nuwun|hatur nuhun|trima kasih|tq|thanks)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:sama-sama|kembali kasih|kembali)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dengan senang hati|senang bisa membantu|ikut senang)\b[,.!?\s]*`), "", ctxAll},

		// polite framing
		{regexp.MustCompile(`(?i)\b(?:mohon|tolong|tlg|bisa tolong|mohon bantuannya|minta tolong)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:saya ingin|saya mau|saya butuh|aku ingin|gw mau|gua mau|saya pengen)\b\s*`), "", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:bisa tolong|bisa minta|bantuin|tolongin)\b[,.!?\s]*`), "", ctxUser},

		// hedging
		{regexp.MustCompile(`(?i)\b(?:sepertinya|tampaknya|rasanya|kiranya|kayaknya)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:saya rasa|saya kira|saya pikir|menurut saya|menurut ku|menurut gue)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:mungkin|barangkali|bisa jadi|boleh jadi)\b\s*`), "", ctxAll},

		// filler adverbs
		{regexp.MustCompile(`(?i)\b(?:pada dasarnya|sebenarnya|intinya|pokoknya|pada intinya)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:secara umum|pada umumnya|umumnya|biasanya|lazimnya)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:kurang lebih|kira-kira|perkiraan)\b\s*`), "", ctxAll},

		// redundant openers
		{regexp.MustCompile(`(?i)^(?:halo|hai|hey|pagi|siang|sore|malam)\s*[,.!?\s]?\s*`), "", ctxUser},
		{regexp.MustCompile(`(?i)^(?:halo gan|halo min|halo kak|halo bro|halo sis)\s*[,.!?\s]?\s*`), "", ctxUser},
		{regexp.MustCompile(`(?i)^(?:assalamualaikum|salam sejahtera|salam)\s*[,.!?\s]?\s*`), "", ctxUser},

		// qualifiers & softeners
		{regexp.MustCompile(`(?i)\b(?:sedikit|agak|terkadang|kadang-kadang|adakalanya)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:jika memungkinkan|kalau bisa|kalau sempat|bila sempat)\b[,.!?\s]*`), "", ctxAll},

		// self-reference
		{regexp.MustCompile(`(?i)^(?:saya|aku|gue|gw|gua|ane|sy|aing)\s+(?:sedang|lagi|masih)\s+mencoba\b\s*`), "", ctxUser},

		// discourse fillers
		{regexp.MustCompile(`(?i)\b(?:kebetulan|sekadar|sejujurnya|jujur|sebenarnya|rupanya|ternyata|omong-omong|btw|fyi)\b\s*`), "", ctxAll},

		// excessive gratitude
		{regexp.MustCompile(`(?i)\b(?:terima kasih banyak|terima kasih sebelumnya|makasih banyak|sangat berterima kasih)\b[,.!?\s]*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:saya sangat menghargai|saya hargai|saya apresiasi)\b[,.!?\s]*`), "", ctxAll},

		// uncertainty fillers
		{regexp.MustCompile(`(?i)\b(?:saya rasa|saya kira|entahlah|gamfang|gak yakin|nggak yakin)\b\s*`), "", ctxAll},

		// assistant fillers
		{regexp.MustCompile(`(?i)^(?:berikut adalah|berikut ini|ini adalah|berikut)\s+`), "", ctxAssistant},

		// particles removal (informal)
		{regexp.MustCompile(`(?i)\b(?:dong|deh|sih|kok|loh|lah|tah|kah|pun|tuh|si|kek|ya)\b\s*`), "", ctxAll},
	}

	// idContextRules: Indonesian context condensation
	idContextRules = []rule{
		{regexp.MustCompile(`(?i)\b(?:saya punya kode berikut|berikut adalah kode saya|berikut kodenya|ini kodenya|kode di bawah ini)\b\s*[:.]?\s*`), "Kode: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:ini filenya|berikut isi filenya|berkasnya|berikut isi berkasnya)\b\s*[:.]?\s*`), "File: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:apa yang saya coba lakukan|tujuan saya adalah|yang saya butuhkan adalah|saya menargetkan)\b\s*`), "Tujuan: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:bisa jelaskan kenapa|bisa tunjukkan bagaimana|tolong jelasin|tolong infoin|kasih tau gimana|tunjukkan cara)\b[,.!?\s]*`), "Jelaskan: ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:fungsi ini|kode ini|kelas ini|modul ini)\s+(?:nampaknya|tampaknya|sepertinya)\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:seperti yang anda ketahui|seperti yang kita diskusikan sebelumnya)\b,?\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)^(?:catat bahwa|perlu diingat bahwa|ingat bahwa)\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dengan tujuan untuk|dalam upaya untuk)\b`), "untuk", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:pertama-tama|kedua|selanjutnya|lalu|kemudian|di samping itu)\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:oleh karena itu|maka dari itu|dengan kata lain|sebagai catatan)\b`), "", ctxAll},
	}

	// idStructuralRules: Indonesian structural compression
	idStructuralRules = []rule{
		{regexp.MustCompile(`(?i)\bpastikan untuk\b\s*`), "pastikan ", ctxAll},
		{regexp.MustCompile(`(?i)\bperlu untuk\b\s*`), "perlu ", ctxAll},
		{regexp.MustCompile(`(?i)\bdikarenakan adanya fakta bahwa\b\s*`), "karena ", ctxAll},
		{regexp.MustCompile(`(?i)\balasan utamanya adalah karena\b\s*`), "karena ", ctxAll},
		{regexp.MustCompile(`(?i)\bpenting untuk\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\banda seharusnya\b\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bingat untuk\b\s*`), "", ctxAll},
		{regexp.MustCompile(`,?\s*dan juga\s+`), ", ", ctxAll},
		{regexp.MustCompile(`,?\s*serta\s+`), ", ", ctxAll},
		{regexp.MustCompile(`,?\s*maupun juga\s+`), ", ", ctxAll},
		{regexp.MustCompile(`(?i)\bagar bisa untuk\b\s*`), "agar ", ctxAll},
		{regexp.MustCompile(`(?i)\bsupaya bisa untuk\b\s*`), "supaya ", ctxAll},
		{regexp.MustCompile(`(?i)\bbertujuan untuk\b\s*`), "untuk ", ctxAll},
		{regexp.MustCompile(`(?i)\bmasing-masing dan setiap\b`), "setiap", ctxAll},
		{regexp.MustCompile(`(?i)\btiap-tiap dan setiap\b`), "setiap", ctxAll},
		{regexp.MustCompile(`(?i)\bapapun dan semua\b`), "semua", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:selain itu|lebih lanjut lagi|tambahan lagi|dan juga)\b\s*`), "juga ", ctxAll},
		{regexp.MustCompile(`(?i)^(?:di sisi lain|sebaliknya|namun|akan tetapi),?\s*`), "", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:sangat|sekali|amat|luar biasa|sungguh)\s+`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bsedang digunakan\b`), "pakai", ctxAll},
		{regexp.MustCompile(`(?i)\bsedang dipanggil\b`), "panggil", ctxAll},
		{regexp.MustCompile(`(?i)\bsedang dibuat\b`), "buat", ctxAll},
		{regexp.MustCompile(`(?i)\btelah dibuat\b`), "buat", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:merupakan|terdapat|yakni|yaitu)\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bdapat\b`), "bisa", ctxAll},
		{regexp.MustCompile(`(?i)\bapabila\b`), "kalau", ctxAll},
		{regexp.MustCompile(`(?i)\bsehingga\b`), "maka", ctxAll},
		{regexp.MustCompile(`(?i)\bdalam hal\b`), "", ctxAll},
		{regexp.MustCompile(`(?i)\bdari segi\b`), "dari", ctxAll},
		{regexp.MustCompile(`(?i)\bdi mana\b`), "yang", ctxAll},
		{regexp.MustCompile(`(?i)\bhal ini\b`), "ini", ctxAll},
		{regexp.MustCompile(`(?i)\bsehubungan dengan\b`), "tentang", ctxAll},
		{regexp.MustCompile(`(?i)\bberdasarkan\b`), "dari", ctxAll},
		{regexp.MustCompile(`(?i)\bmelalui\b`), "dengan", ctxAll},
		{regexp.MustCompile(`(?i)\bdalam rangka\b`), "untuk", ctxAll},
	}

	// idDedupRules: Indonesian repetition markers
	idDedupRules = []rule{
		{regexp.MustCompile(`(?i)\b(?:seperti yang kita diskusikan tadi|seperti yang sudah dibahas|seperti yang disebutkan sebelumnya)\b[,.]?\s*`), "Sama seperti di atas. ", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:pertanyaan yang sama seperti sebelumnya|saya sudah tanya ini sebelumnya|ini pertanyaan yang sama)\b[,.]?\s*`), "[pertanyaan sama] ", ctxUser},
		{regexp.MustCompile(`(?i)\b(?:kembali ke kode di atas|kembali pada|kembali ke)\b\s*`), "Re: ", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:untuk meringkas apa yang kita bahas|ringkasan dari percakapan kita|untuk rekap)\b[,.]?\s*`), "Ringkasan: ", ctxAssistant},
	}

	// idUltraRules: Indonesian abbreviations
	idUltraRules = []rule{
		// leader phrases
		{regexp.MustCompile(`(?i)^(?:saya akan|saya bisa|biarkan saya|kamu bisa|kita bisa|mari|mari kita)\s+`), "", ctxAll},

		// abbreviations
		{regexp.MustCompile(`(?i)\b(?:basis data|database)\b`), "DB", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:konfigurasi|pengaturan)\b`), "config", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:fungsi|metode)\b`), "fn", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:permintaan|request)\b`), "req", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:tanggapan|respons|jawaban|response)\b`), "res", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:implementasi|penerapan)\b`), "impl", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:autentikasi|otentikasi|authentication)\b`), "auth", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:otorisasi|authorization)\b`), "authz", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:aplikasi|program|application)\b`), "app", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:ketergantungan|dependensi|dependencies)\b`), "dep", ctxAll},
		{regexp.MustCompile(`(?i)\binformasi\b`), "info", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:parameter|param)\b`), "param", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:argumen|argument)\b`), "arg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:variabel|variable)\b`), "var", ctxAll},
		{regexp.MustCompile(`(?i)\bsekarang\b`), "skrg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:gimana|bagaimana)\b`), "gmn", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:kenapa|mengapa)\b`), "knp", ctxAll},
		{regexp.MustCompile(`(?i)\btidak\b`), "gk", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:yang|yg)\b`), "yg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:untuk|utk)\b`), "utk", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dengan|dg)\b`), "dg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dari|dr)\b`), "dr", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:bisa|bs)\b`), "bs", ctxAll},
		{regexp.MustCompile(`(?i)\bdan\b`), "&", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:dalam|dlm)\b`), "dlm", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:pada|pd)\b`), "pd", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:sudah|sdh)\b`), "sdh", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:belum|blm)\b`), "blm", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:akan|akn)\b`), "akn", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:juga|jg)\b`), "jg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:karena|krn)\b`), "krn", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:setelah|stlh)\b`), "stlh", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:sebelum|sblm)\b`), "sblm", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:terhadap|thd)\b`), "thd", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:tentang|ttg)\b`), "ttg", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:secara|scr)\b`), "scr", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:seperti|spt)\b`), "spt", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:bahwa|bwh)\b`), "bwh", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:hanya|cuma)\b`), "cma", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:saja|aja)\b`), "aja", ctxAll},
		{regexp.MustCompile(`(?i)\btapi\b`), "tp", ctxAll},
		{regexp.MustCompile(`(?i)\bterus\b`), "trus", ctxAll},
		{regexp.MustCompile(`(?i)\bbanget\b`), "bgt", ctxAll},
		{regexp.MustCompile(`(?i)\b(?:enggak|nggak|gak)\b`), "gk", ctxAll},
	}
)

func compressString(s string, level string, role string) string {
	level = normalizeLevel(level)
	if level == CompressionOff || s == "" {
		return s
	}

	orig := s
	s = stripANSI(s)

	var ctx ruleContext
	switch role {
	case "user":
		ctx = ctxUser
	case "assistant":
		ctx = ctxAssistant
	case "system":
		ctx = ctxSystem
	default:
		ctx = ctxAll
	}

	// lite: filler + context
	s = applyRules(s, fillerRules, ctx)
	s = applyRules(s, contextRules, ctx)
	s = applyRules(s, idFillerRules, ctx)
	s = applyRules(s, idContextRules, ctx)

	if level == CompressionStandard || level == CompressionAggressive {
		// standard: +structural + dedup + ID structural + ID dedup
		s = applyRules(s, structuralRules, ctx)
		s = applyRules(s, dedupRules, ctx)
		s = applyRules(s, idStructuralRules, ctx)
		s = applyRules(s, idDedupRules, ctx)
		s = dedupLines(s)
	}

	if level == CompressionAggressive {
		// aggressive: +ultra + ID ultra + head/tail truncation
		s = applyRules(s, ultraRules, ctx)
		s = applyRules(s, idUltraRules, ctx)
		s = truncateHeadTail(s)
	}

	s = collapseWhitespace(s)

	// inflation guard
	if len(s) > len(orig) {
		return orig
	}
	return s
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

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Preserve leading whitespace (code indentation)
		trimmed := strings.TrimLeft(line, " \t")
		leading := line[:len(line)-len(trimmed)]
		// Collapse internal multi-whitespace runs to single space
		collapsed := wsRegexp.ReplaceAllString(trimmed, " ")
		// Trim trailing space
		collapsed = strings.TrimRight(collapsed, " ")
		lines[i] = leading + collapsed
	}
	return strings.Join(lines, "\n")
}

func dedupLines(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 4 {
		return s
	}

	var out []string
	dupCount := 0
	for i := 0; i < len(lines); i++ {
		if i > 0 && lines[i] == lines[i-1] {
			dupCount++
			if dupCount >= 3 {
				continue
			}
		} else {
			dupCount = 0
		}
		out = append(out, lines[i])
	}

	return strings.Join(out, "\n")
}

func truncateHeadTail(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 100 {
		return s
	}

	head := 50
	tail := 50

	var b strings.Builder
	for i := 0; i < head && i < len(lines); i++ {
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	b.WriteString("... [")
	b.WriteString(intToString(len(lines) - head - tail))
	b.WriteString(" lines truncated]\n")
	for i := len(lines) - tail; i < len(lines); i++ {
		b.WriteString(lines[i])
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func compressAnthropicRequest(req *AnthropicRequest, level string) {
	level = normalizeLevel(level)
	if level == CompressionOff || req == nil {
		return
	}

	if req.System != nil {
		if req.System.IsString {
			req.System.Str = compressString(req.System.Str, level, "system")
		} else {
			for i := range req.System.Blocks {
				if req.System.Blocks[i].Type == "text" {
					req.System.Blocks[i].Text = compressString(req.System.Blocks[i].Text, level, "system")
				}
			}
		}
	}

	for i := range req.Messages {
		role := req.Messages[i].Role
		compressContent(req.Messages[i].Content, level, role)
	}
}

func compressContent(sc *StringOrBlocks, level string, role string) {
	if sc == nil {
		return
	}
	if sc.IsString {
		sc.Str = compressString(sc.Str, level, role)
		return
	}
	for i := range sc.Blocks {
		b := &sc.Blocks[i]
		switch b.Type {
		case "text":
			b.Text = compressString(b.Text, level, role)
		case "tool_result":
			// tool_result = file content / command output — only apply safe
			// compression (ANSI strip, line dedup, truncation), NOT caveman
			// filler rules that could corrupt code identifiers.
			compressToolResultBlocks(b.Content, level)
		}
	}
}

func compressToolResultBlocks(sc *StringOrBlocks, level string) {
	if sc == nil {
		return
	}
	if sc.IsString {
		sc.Str = compressToolResult(sc.Str, level)
		return
	}
	for i := range sc.Blocks {
		if sc.Blocks[i].Type == "text" {
			sc.Blocks[i].Text = compressToolResult(sc.Blocks[i].Text, level)
		}
	}
}

// compressToolResult sanitizes file content / command output — only safe ops:
// ANSI stripping, line dedup, and head/tail truncation. No natural-language
// filler rules that could corrupt code identifiers or string literals.
func compressToolResult(s string, level string) string {
	level = normalizeLevel(level)
	if level == CompressionOff || s == "" {
		return s
	}

	orig := s
	s = stripANSI(s)

	if level == CompressionStandard || level == CompressionAggressive {
		s = dedupLines(s)
	}

	if level == CompressionAggressive {
		s = truncateHeadTail(s)
	}

	if len(s) > len(orig) {
		return orig
	}
	return s
}

func intToString(n int) string {
	return strconv.Itoa(n)
}
