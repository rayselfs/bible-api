package middleware

import (
	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		roles := c.GetHeader("X-Roles")
		permissions := c.GetHeader("X-Permissions")

		c.Set("userID", userID)
		c.Set("roles", roles)
		c.Set("permissions", permissions)
		c.Next()
	}
}
