package embed

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoDetect_WithOllama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := AutoDetect(srv.URL, "", "")
	assert.Equal(t, "ollama", e.Name())
}

func TestAutoDetect_WithOpenAIKey(t *testing.T) {
	// Ollama is not available (bad port), OpenAI key is set
	e := AutoDetect("http://127.0.0.1:19998", "sk-test-openai-key", "")
	assert.Equal(t, "openai", e.Name())
}

func TestAutoDetect_WithCohereKey(t *testing.T) {
	// Ollama not available, no OpenAI key, Cohere key set
	e := AutoDetect("http://127.0.0.1:19997", "", "cohere-test-key")
	assert.Equal(t, "cohere", e.Name())
}

func TestAutoDetect_FallbackToTFIDF(t *testing.T) {
	// None of the API embedders are available/configured
	e := AutoDetect("http://127.0.0.1:19996", "", "")
	assert.Equal(t, "tfidf", e.Name())
}
