package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCohereEmbedder_Name(t *testing.T) {
	e := NewCohereEmbedder("http://localhost", "test-key", "embed-english-v3.0")
	assert.Equal(t, "cohere", e.Name())
}

func TestCohereEmbedder_Dimensions(t *testing.T) {
	e := NewCohereEmbedder("http://localhost", "test-key", "embed-english-v3.0")
	assert.Equal(t, 0, e.Dimensions())
}

func TestCohereEmbedder_BasicEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/embed", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		var req cohereEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "embed-english-v3.0", req.Model)
		assert.Equal(t, "search_document", req.InputType)

		resp := cohereEmbedResponse{
			Embeddings: make([][]float32, len(req.Texts)),
		}
		for i := range req.Texts {
			resp.Embeddings[i] = []float32{0.1, 0.2}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewCohereEmbedder(srv.URL, "test-api-key", "embed-english-v3.0")
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("http://example.com", "Title", []string{"h1"}, "content here", 0, 1),
	}
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []float32{0.1, 0.2}, results[0].Vector)
}

func TestCohereEmbedder_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewCohereEmbedder(srv.URL, "bad-key", "embed-english-v3.0")
	_, err := e.Embed(context.Background(), makeChunks(1))
	require.Error(t, err)
}

func TestCohereEmbedder_Batching(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req cohereEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		resp := cohereEmbedResponse{
			Embeddings: make([][]float32, len(req.Texts)),
		}
		for i := range req.Texts {
			resp.Embeddings[i] = []float32{0.1}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewCohereEmbedder(srv.URL, "key", "embed-english-v3.0")
	results, err := e.Embed(context.Background(), makeChunks(100))
	require.NoError(t, err)
	require.Len(t, results, 100)
	assert.Equal(t, 2, callCount)
}
