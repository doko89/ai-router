package compression

// RetrievalEngine uses BM25 to score and select relevant context.
// Phase 3 placeholder — full implementation will index conversation
// history and retrieve only the most relevant messages.
type RetrievalEngine struct{}

// NewRetrievalEngine creates a BM25-based retrieval engine.
func NewRetrievalEngine() *RetrievalEngine {
	return &RetrievalEngine{}
}

// Score ranks messages by relevance to the current query.
// Stub: returns nil in Phase 1.
func (r *RetrievalEngine) Score(query string, documents []string) []float64 {
	return nil
}

// SelectTopN returns the top N most relevant document indices.
// Stub: returns all indices in Phase 1.
func (r *RetrievalEngine) SelectTopN(query string, documents []string, n int) []int {
	if n > len(documents) {
		n = len(documents)
	}
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	return indices
}
