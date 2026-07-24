package compression

// ConversationState tracks conversation-level state for intelligent compression.
// Phase 1 stub — full implementation in Phase 2.
type ConversationState struct {
	TurnCount    int
	ToolCallMode bool
}

// NewConversationState creates a fresh state tracker.
func NewConversationState() *ConversationState {
	return &ConversationState{}
}

// RecordTurn increments the turn counter and tracks tool-call context.
func (cs *ConversationState) RecordTurn(hasToolCall bool) {
	cs.TurnCount++
	cs.ToolCallMode = hasToolCall
}

// ShouldPrune returns true if the conversation history should be pruned.
// Stub: always returns false in Phase 1.
func (cs *ConversationState) ShouldPrune() bool {
	return false
}
