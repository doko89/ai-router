# AI Router Compression v2 Roadmap

## Objective

Mengembangkan sistem kompresi AI Router dari sekadar **regex-based text compression** menjadi **context-aware compression pipeline** yang mampu mengurangi token tanpa bantuan LLM.

Target:

* Tidak menjalankan model tambahan
* CPU/RAM friendly
* Aman untuk coding agent
* Menjaga kualitas reasoning
* Menjaga cache hit setinggi mungkin
* Mudah di-extend

---

# Current Architecture

```
Request
    │
Regex Compression
    │
Upstream Provider
```

Kelemahan:

* Semua jenis data diperlakukan sama
* Tidak memahami state percakapan
* Tidak memahami tool output
* Tidak memahami source code
* Tidak memahami history

---

# New Architecture

```
Request
    │
Protect Technical Content
    │
Static Prefix Preservation
    │
Conversation State Manager
    │
Tool Output Compressor
    │
Content Deduplication
    │
Code Context Optimizer
    │
Retrieval Engine
    │
Dictionary Compression
    │
Language Compression
    │
Token Budget Validator
    │
Upstream Provider
```

---

# Phase 1 — Refactor

## Goal

Pisahkan seluruh sistem compression menjadi pipeline modular.

### Folder

```
compression/

    pipeline.go

    protect.go

    language.go

    tool.go

    state.go

    dedup.go

    metrics.go

    dictionary.go

    budget.go

    ast.go

    retrieval.go
```

---

## Compression Pipeline

```go
type Compressor interface {
    Compress(ctx *CompressionContext) error
}
```

```go
Pipeline

↓

Protect()

↓

State()

↓

Tool()

↓

Dedup()

↓

Dictionary()

↓

Language()

↓

Budget()
```

---

# Phase 2 — Protect Technical Content

Selalu lindungi:

* code block
* inline code
* JSON
* XML
* YAML
* URL
* diff
* stack trace
* UUID
* hash
* function name
* class name
* package path
* tool arguments

Semua level compression wajib memakai protection.

Bukan hanya aggressive.

---

# Phase 3 — Static Prefix

Jangan pernah mengubah:

* System Prompt
* Tool Definition
* Repository Rules
* MCP Tool Schema

Karena bagian ini biasanya mempunyai cache hit paling tinggi.

Compression hanya berjalan setelah prefix.

---

# Phase 4 — Conversation State

Buat state machine.

Contoh:

```
Task

Current Error

Current Files

Architecture Decisions

Pending TODO

Latest Test

Latest Patch
```

History lama yang sudah tidak dipakai dibuang.

---

## Rules

Keep

* system
* latest user instruction
* unresolved task
* unresolved error
* latest test
* latest patch
* latest tool result
* architecture decision

Remove

* greeting
* acknowledgement
* duplicated plan
* completed action
* obsolete test
* obsolete diff
* repeated explanation

---

# Phase 5 — Tool Compression

## read_file

Jika file sama:

```
unchanged
```

Jika berubah:

```
send diff
```

Jika hash berbeda:

```
send changed block
```

---

## git diff

Gunakan:

```
git diff --unified=2
```

---

## compiler

Kelompokkan error.

```
5 identical errors

↓

1 summary
```

---

## test

Buang:

PASS

Pertahankan:

* FAIL
* panic
* stack
* exit code

---

## grep

Limit:

* max file
* max hit
* prioritize symbol

---

## log

Remove

* timestamp
* ansi
* progress
* duplicate

Keep

* error
* warning
* transition
* panic

---

# Phase 6 — Content Deduplication

Hash semua block besar.

```
SHA256
```

Jika block identik muncul lagi:

gunakan cache internal.

Jangan kirim ulang apabila masih berada dalam active context.

---

# Phase 7 — Delta Encoding

Daripada:

```
entire file
```

gunakan:

```
diff
```

Checkpoint setiap:

* 5 edit
* atau perubahan >30%
* atau context reset

---

# Phase 8 — AST Compression

Gunakan Tree-sitter.

Cari:

* symbol
* function
* class
* method
* caller
* callee

Kirim hanya subtree yang relevan.

Jangan kirim seluruh file.

---

# Phase 9 — Retrieval

Tanpa embedding.

Gunakan:

* BM25
* filename
* symbol
* stacktrace
* recent file
* import graph

Ranking:

```
Exact Symbol

↓

Stack Trace

↓

Filename

↓

BM25

↓

Recent
```

---

# Phase 10 — Dictionary Compression

Untuk string yang sangat repetitif.

Contoh:

```
internal/worker/pool.go

↓

§1
```

Gunakan hanya jika token benar-benar berkurang.

---

# Phase 11 — Language Compression

Regex menjadi tahap terakhir.

Tetap dipakai.

Tetapi bukan penyumbang utama compression.

---

# Phase 12 — Token Metrics

Tambahkan metric:

```
Original Token

Compressed Token

Saved Token

Saved %

Compression Time

Cacheable Prefix

History Removed

Tool Saved

Dictionary Saved
```

Jangan memakai byte sebagai ukuran utama.

---

# Compression Levels

## Lite

* whitespace
* ansi
* duplicate
* filler

Target:

10–20%

---

## Standard

Lite

*

state pruning

tool compression

dedup

Target:

25–45%

---

## Aggressive

Standard

*

AST

retrieval

dictionary

Target:

40–70%

---

# Performance Goals

Latency tambahan:

<20 ms

Memory:

<64 MB

Tidak memakai:

* GPU
* LLM
* Embedding model

---

# Priority

## P0

* Refactor pipeline
* Protect technical content
* Static prefix
* Token metrics

---

## P1

* Tool compression
* State pruning
* Deduplication

---

## P2

* Delta encoding
* Dictionary compression

---

## P3

* AST
* BM25 retrieval

---

# Success Criteria

* Token berkurang minimal 40% pada workflow coding panjang.
* Cache hit tetap tinggi (target tidak turun signifikan dibanding implementasi saat ini).
* Tidak ada perubahan pada kode, JSON, tool arguments, atau identifier akibat kompresi.
* Overhead latency tetap rendah (<20 ms untuk request normal).
* Semua tahap kompresi bersifat modular dan dapat diaktif/nonaktifkan melalui konfigurasi.

---

# Final Deliverable

Compression v2 akan menjadi **context-aware compression engine**, bukan lagi sekadar kumpulan regex.

Arsitektur baru harus mendukung penambahan strategi kompresi di masa depan tanpa mengubah pipeline utama.
