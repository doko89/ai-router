package compression

// Metrics tracks compression savings for a single request.
type Metrics struct {
	OrigBytes int
	CompBytes int
}

// SavingsPercent returns the percentage of bytes saved.
func (m Metrics) SavingsPercent() float64 {
	if m.OrigBytes == 0 {
		return 0
	}
	return float64(m.OrigBytes-m.CompBytes) / float64(m.OrigBytes) * 100
}

// BytesSaved returns the absolute number of bytes saved.
func (m Metrics) BytesSaved() int {
	return m.OrigBytes - m.CompBytes
}

// Inflated returns true if compression increased the size.
func (m Metrics) Inflated() bool {
	return m.CompBytes > m.OrigBytes
}
