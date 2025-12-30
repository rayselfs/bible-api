package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type API struct {
	store *models.Store
}

func NewAPI(store *models.Store) *API {
	return &API{
		store: store,
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
	versions, err := a.store.GetAllVersions(c)
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
	contentChan, errorChan := a.store.StreamBibleContent(c, ctx, uint(id))

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

// HandleGetVectors Stream Bible vectors by version ID
// @Summary      Stream Bible vectors (Float32Array binary) by version ID
// @Description  Stream all verse vectors for the specified version ID using chunked transfer encoding
// @Tags         Bible
// @Produce      application/octet-stream
// @Param        version_id  path      int  true  "Version ID"
// @Success      200        {string}  string "Successfully streaming vectors"
// @Failure      400        {object}  ErrorResponse "Invalid input parameters"
// @Failure      500        {object}  ErrorResponse "Internal server error"
// @Router       /api/bible/v1/vectors/{version_id} [get]
func (a *API) HandleGetVectors(c *gin.Context) {
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

	appLogger.Infof("Starting to stream Bible vectors for version ID: %d", id)

	// Set headers for binary stream
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Connection", "keep-alive")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	contentChan, errorChan := a.store.StreamVectorsForVersion(c, ctx, uint(id))

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Streaming not supported"})
		return
	}

	for {
		select {
		case content, ok := <-contentChan:
			if !ok {
				return
			}
			if _, err := c.Writer.Write(content); err != nil {
				appLogger.Errorf("Error writing vectors: %v", err)
				return
			}
			flusher.Flush()

		case err := <-errorChan:
			if err != nil {
				appLogger.Errorf("Error streaming vectors: %v", err)
				// Cannot write JSON error if we already started writing binary
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// UpdateVerseRequest represents the request body for updating a verse
type UpdateVerseRequest struct {
	Text string `json:"text" binding:"required"`
}

// HandleUpdateVerse updates a verse's text (Embedding update disabled for migration)
// @Summary      Update verse content (Text Only)
// @Description  Update verse text. NOTE: Vector embedding is NOT updated automatically. You must run the python script to regenerate vectors.
// @Tags         Bible
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "Verse ID"
// @Param        body body      UpdateVerseRequest true "Update Request"
// @Success      200  {object}  map[string]interface{} "Success"
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/bible/v1/verse/{id} [post]
func (a *API) HandleUpdateVerse(c *gin.Context) {
	verseIDStr := c.Param("id")
	verseID, err := strconv.Atoi(verseIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid verse ID"})
		return
	}

	// Check permissions
	permissionsStr, exists := c.Get("permissions")
	if !exists {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied: missing permissions"})
		return
	}

	permissions, ok := permissionsStr.(string)
	if !ok {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied: invalid permissions format"})
		return
	}

	if !utils.HasPermission(permissions, "bible:edit") && !utils.HasPermission(permissions, "bible:admin") {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied: requires 'bible:edit' or 'bible:admin' permission"})
		return
	}

	var req UpdateVerseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Better: Just update text.
	if err := a.store.DB.Model(&models.Verses{}).Where("id = ?", verseID).Update("text", req.Text).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update verse text"})
		return
	}

	// Signal version update
	// We need version ID.
	var result struct{ VersionID uint }
	a.store.DB.Raw("SELECT b.version_id FROM verses v JOIN chapters c ON v.chapter_id = c.id JOIN books b ON c.book_id = b.id WHERE v.id = ?", verseID).Scan(&result)
	if result.VersionID > 0 {
		a.store.DB.Model(&models.Versions{}).Where("id = ?", result.VersionID).Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP"))
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Verse updated successfully (Text only, Vector stale)",
		"id":      verseID,
	})
}
