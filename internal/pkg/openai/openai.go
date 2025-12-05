package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v2"
)

// OpenAIService 處理 OpenAI embedding 相關的業務邏輯
type OpenAIService struct {
	client    *openai.Client
	modelName string
}

// NewOpenAIService 創建新的 OpenAI 服務
func NewOpenAIService(client *openai.Client, modelName string) *OpenAIService {
	return &OpenAIService{
		client:    client,
		modelName: modelName,
	}
}

// GetEmbedding 取得查詢文字的 embedding
func (s *OpenAIService) GetEmbedding(ctx context.Context, text string) ([]float64, error) {
	resp, err := s.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)},
		Model: s.modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAI SDK error: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return resp.Data[0].Embedding, nil
}
