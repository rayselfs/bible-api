package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (a *API) RegisterRoutes() *gin.Engine {
	r := gin.New()

	// Use structured logging middleware
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "UP",
		})
	}).
		GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Group("/api/bible/v1").
		GET("/versions", a.HandleGetAllVersions).
		GET("/version/:version_id", a.HandleGetVersionContent).
		GET("/search", a.HandleSearch)

	return r
}
