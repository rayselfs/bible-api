package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// NewReadinessHandler checks outbound health to avoid waiting on Dapr's app channel.
// @Summary Dapr outbound readiness
// @Description Checks local Dapr outbound health only, without acquiring a database connection.
// @Tags Operations
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /ready [get]
func NewReadinessHandler(port int, token string) gin.HandlerFunc {
	client := &http.Client{
		Timeout:       500 * time.Millisecond,
		Transport:     &http.Transport{MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1.0/healthz/outbound", port)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
		if err == nil {
			if token != "" {
				req.Header.Set("dapr-api-token", token)
			}
			resp, requestErr := client.Do(req)
			if resp != nil {
				defer resp.Body.Close()
			}
			if requestErr == nil && resp.StatusCode == http.StatusNoContent {
				c.JSON(http.StatusOK, gin.H{"status": "UP"})
				return
			}
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN"})
	}
}
