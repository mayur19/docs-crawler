package embed

import "github.com/mayur19/docs-crawler/internal/pipeline"

// defaultOllamaModel is used when auto-detecting an Ollama instance.
const defaultOllamaModel = "nomic-embed-text"

// defaultOpenAIModel is used when auto-detecting via OpenAI key.
const defaultOpenAIModel = "text-embedding-3-small"

// defaultCohereModel is used when auto-detecting via Cohere key.
const defaultCohereModel = "embed-english-v3.0"

// defaultOpenAIBaseURL is the public OpenAI API base URL.
const defaultOpenAIBaseURL = "https://api.openai.com"

// defaultCohereBaseURL is the public Cohere API base URL.
const defaultCohereBaseURL = "https://api.cohere.com"

// AutoDetect picks the best available embedder using the following priority:
//  1. Ollama — if a running Ollama server is reachable at ollamaURL
//  2. OpenAI — if openAIKey is non-empty
//  3. Cohere — if cohereKey is non-empty
//  4. TF-IDF — zero-dependency fallback
func AutoDetect(ollamaURL, openAIKey, cohereKey string) pipeline.Embedder {
	if OllamaIsAvailable(ollamaURL) {
		return NewOllamaEmbedder(ollamaURL, defaultOllamaModel)
	}

	if openAIKey != "" {
		return NewOpenAIEmbedder(defaultOpenAIBaseURL, openAIKey, defaultOpenAIModel)
	}

	if cohereKey != "" {
		return NewCohereEmbedder(defaultCohereBaseURL, cohereKey, defaultCohereModel)
	}

	return NewTFIDFEmbedder()
}
