package aisearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"hhc/bible-api/internal/models"
)

// Service 處理 Azure AI Search 相關的業務邏輯
type Service struct {
	httpClient       *http.Client
	aiSearchBaseURL  string
	aiSearchQueryKey string
	indexName        string
	apiVersion       string
}

// NewService 創建新的 AI Search 服務
func NewService(httpClient *http.Client, aiSearchBaseURL, aiSearchQueryKey, indexName, apiVersion string) *Service {
	return &Service{
		httpClient:       httpClient,
		aiSearchBaseURL:  aiSearchBaseURL,
		aiSearchQueryKey: aiSearchQueryKey,
		indexName:        indexName,
		apiVersion:       apiVersion,
	}
}

// Search 執行混合搜尋
func (s *Service) Search(ctx context.Context, req models.AISearchRequest) (*models.AISearchResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	// 構建正確的搜尋端點 URL
	searchURL := fmt.Sprintf("%s/indexes/%s/docs/search?api-version=%s",
		s.aiSearchBaseURL, s.indexName, s.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", s.aiSearchQueryKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return nil, fmt.Errorf("AI Search API returned error status %s: %s", resp.Status, errBody.String())
	}

	var searchResp models.AISearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return &searchResp, nil
}

// BuildSearchRequest 構建搜尋請求
func (s *Service) BuildSearchRequest(queryText string, queryVector []float64, versionFilter string, topK int) models.AISearchRequest {
	req := models.AISearchRequest{
		Search: queryText, // 關鍵字搜尋
		VectorQueries: []models.VectorQuery{ // 向量搜尋
			{
				Vector: queryVector,
				Fields: "text_vector", // 要搜尋的向量欄位
				K:      topK,          // K-NN 搜尋
				Kind:   "vector",      // 向量查詢類型
			},
		},
		Top:    topK,                                                                      // 最終返回的結果數量
		Select: "verse_id, version_code, book_number, chapter_number, verse_number, text", // 返回的欄位
	}

	// 加上版本過濾
	if versionFilter != "" {
		// 使用 OData filter 語法
		req.Filter = fmt.Sprintf("version_code eq '%s'", versionFilter)
	}

	return req
}
