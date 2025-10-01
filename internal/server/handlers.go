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
	return &API{store: store}
}

// ErrorResponse 代表標準的錯誤回傳格式
type ErrorResponse struct {
	Error string `json:"error" example:"error message"`
}

// handleGetAllVersions 獲取所有聖經版本
// @Summary      取得所有聖經版本
// @Description  列出系統中所有可用的聖經版本
// @Tags         Bible
// @Produce      json
// @Success      200        {array}   models.VersionListItem "成功取得版本列表"
// @Failure      500        {object}  ErrorResponse  "伺服器內部錯誤"
// @Router       /versions [get]
func (a *API) handleGetAllVersions(c *gin.Context) {
	versions, err := a.store.GetAllVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve versions"})
		return
	}

	c.JSON(http.StatusOK, versions)
}

// handleGetVersionContent 透過版本 ID 獲取全部經文內容
// @Summary      取得全部經文內容
// @Description  根據指定的版本 ID，取得該版本的所有書卷、章節和經文內容
// @Tags         Bible
// @Produce      json
// @Param        version_id  path      int  true  "版本 ID"
// @Success      200        {object}  models.BibleContentAPI "成功取得經文內容"
// @Failure      400        {object}  ErrorResponse "輸入參數無效"
// @Failure      500        {object}  ErrorResponse "伺服器內部錯誤"
// @Router       /version/{version_id} [get]
func (a *API) handleGetVersionContent(c *gin.Context) {
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
