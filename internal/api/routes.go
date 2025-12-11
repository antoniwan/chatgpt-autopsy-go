package api

import (
	"chatgpt-autopsy-go/internal/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *gin.Engine, handler *Handler, cfg *config.Config) {
	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// System endpoints
		v1.GET("/health", handler.HealthCheck)
		v1.GET("/ready", handler.ReadyCheck)

		// Upload endpoints
		v1.POST("/upload", handler.UploadFile)
		uploads := v1.Group("/uploads")
		{
			uploads.GET("", handler.ListUploads)
			uploads.GET("/:id", handler.GetUpload)
			uploads.DELETE("/:id", handler.DeleteUpload)
		}

		// Conversation endpoints
		conversations := v1.Group("/conversations")
		{
			conversations.GET("", handler.ListConversations)
			conversations.GET("/:id", handler.GetConversation)
		}

		// Analysis endpoints
		v1.GET("/dates", handler.ListDates)
		
		analysis := v1.Group("/analysis")
		{
			analysis.GET("/:date/:type", handler.GetAnalysis)
		}
	}
}

// SetupMiddleware configures all middleware
func SetupMiddleware(router *gin.Engine, cfg *config.Config) {
	// Request ID middleware
	router.Use(RequestIDMiddleware())

	// Recovery middleware (needs logger, will be set in main)
	// router.Use(RecoveryMiddleware(log))

	// CORS middleware
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:8080"}
	router.Use(CORSMiddleware(allowedOrigins))

	// Rate limiting
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit.RequestsPerMinute/60.0), cfg.RateLimit.BurstSize)
	router.Use(RateLimitMiddleware(limiter))
}

