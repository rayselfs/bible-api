package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/pkg/openai"

	"github.com/gin-gonic/gin"
	oai "github.com/openai/openai-go/v2"
)

type API struct {
	store         *models.Store
	openAIService *openai.OpenAIService
}

func NewAPI(store *models.Store, oaiClient *oai.Client, httpClient *http.Client, openAIModelName string) *API {
	openAIService := openai.NewOpenAIService(oaiClient, openAIModelName)

	return &API{
		store:         store,
		openAIService: openAIService,
	}
}

// ErrorResponse represents standard error response format
type ErrorResponse struct {
	Error string `json:"error" example:"error message"`
}

// handleGetAllVersions Get all Bible versions
// @Summary      Get all Bible versions
// @Description  List all available Bible versions in the system
// @Tags         Bible
// @Produce      json
// @Success      200        {array}   models.VersionListItem "Successfully retrieved version list"
// @Failure      500        {object}  ErrorResponse  "Internal server error"
// @Router       /api/bible/v1/versions [get]
func (a *API) HandleGetAllVersions(c *gin.Context) {
	versions, err := a.store.GetAllVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve versions"})
		return
	}

	c.JSON(http.StatusOK, versions)
}

// handleGetVersionContent Stream complete Bible content by version ID
// @Summary      Stream complete Bible content
// @Description  Stream all books, chapters and verses content for the specified version ID using Server-Sent Events
// @Tags         Bible
// @Produce      text/event-stream
// @Param        version_id  path      int  true  "Version ID"
// @Success      200        {string}  string "Successfully streaming Bible content"
// @Failure      400        {object}  ErrorResponse "Invalid input parameters"
// @Failure      500        {object}  ErrorResponse "Internal server error"
// @Router       /api/bible/v1/version/{version_id} [get]
func (a *API) HandleGetVersionContent(c *gin.Context) {
	appLogger := logger.GetAppLogger()

	versionID := c.Param("version_id")
	if versionID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "version_id parameter is required"})
		return
	}

	id, err := strconv.Atoi(versionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid version_id parameter"})
		return
	}

	appLogger.Infof("Starting to stream Bible content for version ID: %d", id)

	// Set up Server-Sent Events headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	// Start streaming
	contentChan, errorChan := a.store.StreamBibleContent(ctx, uint(id))

	// Create a flusher to ensure immediate delivery
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		appLogger.Error("Streaming not supported")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Streaming not supported"})
		return
	}

	// Send initial event
	fmt.Fprintf(c.Writer, "data: %s\n\n", `{"type":"start","message":"開始傳輸聖經內容"}`)
	flusher.Flush()

	bookCount := 0
	for {
		select {
		case content, ok := <-contentChan:
			if !ok {
				// Channel closed, send completion event
				fmt.Fprintf(c.Writer, "data: %s\n\n", fmt.Sprintf(`{"type":"complete","total_books":%d,"message":"傳輸完成"}`, bookCount))
				flusher.Flush()
				appLogger.Infof("Successfully streamed Bible content for version %d, total books: %d", id, bookCount)
				return
			}

			// Send book data
			fmt.Fprintf(c.Writer, "data: %s\n\n", string(content))
			flusher.Flush()
			bookCount++

		case err := <-errorChan:
			if err != nil {
				appLogger.Errorf("Error streaming Bible content: %v", err)
				fmt.Fprintf(c.Writer, "data: %s\n\n", fmt.Sprintf(`{"type":"error","message":"傳輸錯誤: %s"}`, err.Error()))
				flusher.Flush()
				return
			}

		case <-ctx.Done():
			appLogger.Warnf("Streaming timeout for version %d", id)
			fmt.Fprintf(c.Writer, "data: %s\n\n", `{"type":"timeout","message":"傳輸超時"}`)
			flusher.Flush()
			return

		case <-c.Request.Context().Done():
			// Client disconnected
			appLogger.Infof("Client disconnected while streaming version %d", id)
			return
		}
	}
}

// HandleSearch 執行混合搜尋
// @Summary      Search Bible verses
// @Description  Perform hybrid (keyword + semantic) search for Bible verses
// @Tags         Bible
// @Produce      json
// @Param        q          query     string  true  "Search query"
// @Param        version    query     string  true  "Version code to filter (e.g., CUV)"
// @Param        top        query     int     false "Number of results to return (default: 10)"
// @Success      200        {array}   models.SearchResult "Successfully retrieved search results"
// @Failure      400        {object}  ErrorResponse  "Invalid input parameters"
// @Failure      500        {object}  ErrorResponse  "Internal server error"
// @Router       /api/bible/v1/search [get]
func (a *API) HandleSearch(c *gin.Context) {
	queryText := c.Query("q")
	if queryText == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "query (q) is required"})
		return
	}
	versionFilter := c.Query("version")
	if versionFilter == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "version is required"})
		return
	}
	topK, _ := strconv.Atoi(c.DefaultQuery("top", "10"))
	if topK <= 0 {
		topK = 10
	}

	ctx := c.Request.Context()
	queryVector64, err := a.openAIService.GetEmbedding(ctx, queryText)
	if err != nil {
		logger.GetAppLogger().Errorf("Failed to get query embedding: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to process search query"})
		return
	}

	queryVector := make([]float32, len(queryVector64))
	for i, v := range queryVector64 {
		queryVector[i] = float32(v)
	}

	results, err := a.store.SearchVerses(ctx, queryText, queryVector, versionFilter, topK)
	if err != nil {
		logger.GetAppLogger().Errorf("Failed to execute Hybrid Search: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve search results"})
		return
	}

	c.JSON(http.StatusOK, results)
}
