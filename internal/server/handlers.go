package server

import (
	"net/http"
	"strconv"

	"hhc/bible-api/internal/models"

	"github.com/gin-gonic/gin"
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
