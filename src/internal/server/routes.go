package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (a *API) RegisterRoutes() *gin.Engine {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "UP",
		})
	})

	r.Group("/api/bible").
		GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler)).
		GET("/versions", a.handleGetAllVersions).
		GET("/version/:version_id", a.handleGetVersionContent)

	return r
}
