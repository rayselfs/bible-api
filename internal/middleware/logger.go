package middleware

import (
	"time"

	"hhc/bible-api/internal/logger"

	"github.com/gin-gonic/gin"
)

// LoggerMiddleware returns a gin middleware that logs HTTP requests in structured JSON format
func LoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Create structured log entry
		logEntry := map[string]interface{}{
			"timestamp":  param.TimeStamp.Format("2006-01-02 15:04:05"),
			"level":      "INFO",
			"message":    "HTTP Request",
			"method":     param.Method,
			"path":       param.Path,
			"status":     param.StatusCode,
			"latency":    param.Latency.String(),
			"ip":         param.ClientIP,
			"user_agent": param.Request.UserAgent(),
		}

		// Add error if present
		if param.ErrorMessage != "" {
			logEntry["error"] = param.ErrorMessage
		}

		// Convert to JSON
		jsonData, err := logger.JSONMarshal(logEntry)
		if err != nil {
			return `{"timestamp": "` + time.Now().Format("2006-01-02 15:04:05") + `", "level": "ERROR", "message": "Failed to format log entry"}` + "\n"
		}

		return string(jsonData) + "\n"
	})
}
