package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"hhc/bible-api/internal/models"
)

// AISearchRequest AI search request structure
type AISearchRequest struct {
	Query   string `json:"query"`
	TopK    int    `json:"top_k"`
	Version string `json:"version,omitempty"`
}

// AISearchResult AI search single result structure
type AISearchResult struct {
	Rank    int     `json:"rank"`
	Score   float64 `json:"score"`
	Text    string  `json:"text"`
	Version string  `json:"version"`
	Book    int     `json:"book"`
	Chapter int     `json:"chapter"`
	Verse   int     `json:"verse"`
}

// AISearchResponse AI search response structure
type AISearchResponse struct {
	Query          string           `json:"query"`
	ModelType      string           `json:"model_type"`
	TotalResults   int              `json:"total_results"`
	Results        []AISearchResult `json:"results"`
	ProcessingTime float64          `json:"processing_time"`
}

// Service search service
type Service struct {
	store       *models.Store
	aiSearchURL string
	httpClient  *http.Client
}

// NewService create new search service
func NewService(store *models.Store, aiSearchURL string) *Service {
	return &Service{
		store:       store,
		aiSearchURL: aiSearchURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExecuteSearch execute search
func (s *Service) ExecuteSearch(req models.SearchRequest) ([]models.SearchResult, error) {
	var results []models.SearchResult

	// Database exact match
	dbResults, err := s.store.SearchDatabase(req.Query, req.Version)
	if err != nil {
		return nil, err
	}

	// Add database results to result list, set highest priority
	for i, result := range dbResults {
		result.Rank = i + 1
		result.Score = 100.0 // Exact match set highest score
		results = append(results, result)
	}

	// Second step: AI semantic search
	aiResults, err := s.executeAISearch(req)
	if err != nil {
		fmt.Println("execute ai search failed", err)
		return results, nil
	}

	// Filter out results that are already in the database
	filteredAIResults := s.filterDuplicateResults(aiResults, results)

	// Merge results
	results = append(results, filteredAIResults...)

	// Reassign ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	// If results exceed TopK, sort by score and take top TopK
	if len(results) > req.TopK {
		// Sort by score (highest first)
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		// Take top TopK results
		results = results[:req.TopK]

		// Reassign ranks
		for i := range results {
			results[i].Rank = i + 1
		}
	}

	return results, nil
}

// executeAISearch Execute AI search
func (s *Service) executeAISearch(req models.SearchRequest) ([]models.SearchResult, error) {
	fmt.Println("execute ai search")

	// Build request body
	requestBody := AISearchRequest{
		Query:   req.Query,
		TopK:    req.TopK,
		Version: req.Version,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Send HTTP request to AI search service
	resp, err := s.httpClient.Post(
		s.aiSearchURL+"/api/ai-search/v1/bible",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("AI search service request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI search service error: %d, %s", resp.StatusCode, string(body))
	}

	// Parse response
	var aiResponse AISearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode AI search response: %v", err)
	}

	// Convert to models.SearchResult
	var results []models.SearchResult
	for _, result := range aiResponse.Results {
		results = append(results, models.SearchResult{
			Rank:    result.Rank,
			Score:   result.Score,
			Text:    result.Text,
			Version: result.Version,
			Book:    result.Book,
			Chapter: result.Chapter,
			Verse:   result.Verse,
		})
	}

	fmt.Println("ai search results", results)

	return results, nil
}

// filterDuplicateResults Filter duplicate results
func (s *Service) filterDuplicateResults(aiResults []models.SearchResult, dbResults []models.SearchResult) []models.SearchResult {
	var filtered []models.SearchResult

	for _, aiResult := range aiResults {
		isDuplicate := false

		// Check if duplicate with database results
		for _, dbResult := range dbResults {
			if aiResult.Version == dbResult.Version &&
				aiResult.Book == dbResult.Book &&
				aiResult.Chapter == dbResult.Chapter &&
				aiResult.Verse == dbResult.Verse {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			filtered = append(filtered, aiResult)
		}
	}

	return filtered
}
