package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/pkg/search"

	"github.com/gin-gonic/gin"
)

type API struct {
	store         *models.Store
	searchService *search.Service
}

func NewAPI(store *models.Store, aiSearchURL string) *API {
	return &API{
		store:         store,
		searchService: search.NewService(store, aiSearchURL),
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

// handleGetVersionContent Get complete Bible content by version ID
// @Summary      Get complete Bible content
// @Description  Get all books, chapters and verses content for the specified version ID
// @Tags         Bible
// @Produce      json
// @Param        version_id  path      int  true  "Version ID"
// @Success      200        {object}  models.BibleContentAPI "Successfully retrieved Bible content"
// @Failure      400        {object}  ErrorResponse "Invalid input parameters"
// @Failure      500        {object}  ErrorResponse "Internal server error"
// @Router       /api/bible/v1/version/{version_id} [get]
func (a *API) HandleGetVersionContent(c *gin.Context) {
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

	content, err := a.store.GetBibleContent(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve bible content"})
		return
	}

	c.JSON(http.StatusOK, content)
}

// handleSearch Handle search requests
// @Summary      Semantic search for Bible verses
// @Description  Use semantic search functionality to understand query meaning and return relevant verses
// @Tags         Bible
// @Accept       json
// @Produce      json
// @Param        request  body  models.SearchRequest  true  "Search request"
// @Success      200     {object}  models.SearchResponse "Search results"
// @Failure      400     {object}  ErrorResponse    "Invalid request format"
// @Failure      500     {object}  ErrorResponse    "Search failed"
// @Router       /api/bible/v1/search [post]
func (a *API) HandleSearch(c *gin.Context) {
	appLogger := logger.GetAppLogger()
	appLogger.Info("Processing search request")

	var req models.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		appLogger.Warnf("Invalid request format: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request format"})
		return
	}

	// Validate request
	if strings.TrimSpace(req.Query) == "" {
		appLogger.Warn("Empty query received")
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Query cannot be empty"})
		return
	}

	// Set default values
	if req.TopK <= 0 {
		req.TopK = 10
	}

	appLogger.Infof("Executing search for query: '%s' (TopK: %d)", req.Query, req.TopK)

	// Execute search
	results, err := a.searchService.ExecuteSearch(req)
	if err != nil {
		appLogger.Errorf("Search execution failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("Search failed: %v", err)})
		return
	}

	// Build search response
	response := models.SearchResponse{
		Query:   req.Query,
		Results: results,
		Total:   len(results),
	}

	appLogger.Infof("Search completed successfully: %d results found", len(results))
	c.JSON(http.StatusOK, response)
}
