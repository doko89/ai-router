package compression

// BudgetValidator validates that compressed content stays within token budgets.
// Phase 1 stub — full implementation in Phase 2.
type BudgetValidator struct {
	MaxTokens int
}

// NewBudgetValidator creates a budget validator with the given token limit.
func NewBudgetValidator(maxTokens int) *BudgetValidator {
	return &BudgetValidator{MaxTokens: maxTokens}
}

// Validate checks whether the text exceeds the budget.
// Returns true if within budget, false otherwise.
// This is a stub; real token counting will use tiktoken in Phase 2.
func (b *BudgetValidator) Validate(text string) bool {
	if b.MaxTokens <= 0 {
		return true
	}
	// Stub: rough character-based estimate (4 chars ≈ 1 token).
	estimatedTokens := len(text) / 4
	return estimatedTokens <= b.MaxTokens
}
