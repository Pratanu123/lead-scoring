package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	LocalModel          = "local-hash-embedding-v1"
	DefaultRemoteModel  = "text-embedding-3-small"
	embeddingDimensions = 1536
)

type Result struct {
	Model  string
	Vector string
}

type Embedder interface {
	Model() string
	Embed(ctx context.Context, text string) (Result, error)
}

type LocalEmbedder struct{}

func NewLocalEmbedder() *LocalEmbedder {
	return &LocalEmbedder{}
}

func (e *LocalEmbedder) Model() string {
	return LocalModel
}

func (e *LocalEmbedder) Embed(_ context.Context, text string) (Result, error) {
	return Result{
		Model:  LocalModel,
		Vector: deterministicVector(text),
	}, nil
}

type RemoteEmbedder struct {
	apiURL string
	apiKey string
	model  string
	client *http.Client
}

func NewRemoteEmbedder(apiURL string, apiKey string, model string) *RemoteEmbedder {
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultRemoteModel
	}
	return &RemoteEmbedder{
		apiURL: strings.TrimSpace(apiURL),
		apiKey: strings.TrimSpace(apiKey),
		model:  model,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *RemoteEmbedder) Model() string {
	return e.model
}

func (e *RemoteEmbedder) Embed(ctx context.Context, text string) (Result, error) {
	requestPayload := map[string]any{
		"model": e.model,
		"input": text,
	}

	body, err := json.Marshal(requestPayload)
	if err != nil {
		return Result{}, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.apiURL, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("call embedding api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Result{}, fmt.Errorf("embedding api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var response struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Result{}, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return Result{}, fmt.Errorf("embedding api returned no vectors")
	}

	model := e.model
	if strings.TrimSpace(response.Model) != "" {
		model = strings.TrimSpace(response.Model)
	}

	return Result{
		Model:  model,
		Vector: formatVector(response.Data[0].Embedding),
	}, nil
}

func deterministicVector(content string) string {
	vector := make([]float64, embeddingDimensions)
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		tokens = []string{"empty"}
	}

	for _, token := range tokens {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(token))
		sum := hash.Sum64()
		index := int(sum % embeddingDimensions)
		weight := 1.0
		if sum%2 == 0 {
			weight = -1.0
		}
		vector[index] += weight
	}

	return formatVector(normalize(vector))
}

func normalize(vector []float64) []float64 {
	var magnitude float64
	for _, value := range vector {
		magnitude += value * value
	}
	magnitude = math.Sqrt(magnitude)
	if magnitude == 0 {
		magnitude = 1
	}

	normalized := make([]float64, len(vector))
	for i, value := range vector {
		normalized[i] = value / magnitude
	}
	return normalized
}

func formatVector(vector []float64) string {
	values := make([]string, len(vector))
	for i, value := range vector {
		values[i] = fmt.Sprintf("%.6f", value)
	}
	return "[" + strings.Join(values, ",") + "]"
}
