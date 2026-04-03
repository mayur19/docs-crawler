package chunk_test

import (
	"testing"

	"github.com/mayur19/docs-crawler/internal/chunk"
	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{
			name: "empty string",
			text: "",
			want: 0,
		},
		{
			name: "single word",
			text: "hello",
			want: 2, // ceil(1 * 1.3) = ceil(1.3) = 2
		},
		{
			name: "two words",
			text: "hello world",
			want: 3, // ceil(2 * 1.3) = ceil(2.6) = 3
		},
		{
			name: "sentence",
			text: "the quick brown fox jumps over the lazy dog",
			want: 12, // ceil(9 * 1.3) = ceil(11.7) = 12
		},
		{
			name: "large text",
			text: buildLargeText(100),
			want: 130, // ceil(100 * 1.3) = 130
		},
		{
			name: "whitespace only",
			text: "   \t\n  ",
			want: 0,
		},
		{
			name: "mixed whitespace and words",
			text: "  hello   world  ",
			want: 3, // ceil(2 * 1.3) = 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunk.EstimateTokens(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

// buildLargeText generates a string with exactly n space-separated words.
func buildLargeText(n int) string {
	result := make([]byte, 0, n*6)
	for i := 0; i < n; i++ {
		if i > 0 {
			result = append(result, ' ')
		}
		result = append(result, "word"...)
	}
	return string(result)
}
