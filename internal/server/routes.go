package server

import (
	middleware "hhc/bible-api/internal/middlewares"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (a *API) SetupRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "UP",
		})
	}).GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := r.Group("/api/bible/v1")
	v1.Use(middleware.AuthMiddleware())
	{
		v1.GET("/versions", a.HandleGetAllVersions)
		v1.GET("/content/:version_id", a.HandleGetVersionContent)
		v1.GET("/vectors/:version_id", a.HandleGetVectors)
		v1.POST("/verse/:id", a.HandleUpdateVerse)
	}
}
