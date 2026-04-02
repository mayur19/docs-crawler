// Package chunk provides strategies for splitting Documents into Chunks
// suitable for embedding and retrieval.
package chunk

import (
	"math"
	"strings"
)

// EstimateTokens approximates the token count for the given text using
// the formula ceil(wordCount * 1.3).
func EstimateTokens(text string) int {
	words := len(strings.Fields(text))
	if words == 0 {
		return 0
	}
	return int(math.Ceil(float64(words) * 1.3))
}
