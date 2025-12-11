package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatgpt-autopsy-go/internal/api"
	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting ChatGPT Autopsy server")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize database
	if err := database.Initialize(cfg, logger); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	// Initialize services
	uploadService := services.NewUploadService(cfg, logger)
	extractionService := services.NewExtractionService(cfg, logger)
	parserService := services.NewParserService(cfg, logger)
	threadService := services.NewThreadService(cfg, logger)
	analysisService := services.NewAnalysisService(cfg, logger)

	// Initialize handlers
	handler := api.NewHandler(
		uploadService,
		extractionService,
		parserService,
		threadService,
		analysisService,
		logger,
	)

	// Setup Gin router
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Setup middleware
	api.SetupMiddleware(router, cfg)
	router.Use(api.LoggingMiddleware(logger))
	router.Use(api.RecoveryMiddleware(logger))

	// Setup routes
	api.SetupRoutes(router, handler, cfg)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Server starting",
			zap.String("address", srv.Addr),
			zap.Int("port", cfg.Server.Port),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

// initLogger initializes the logger based on configuration
func initLogger() (*zap.Logger, error) {
	// For now, use development config
	// In production, this would read from config
	config := zap.NewDevelopmentConfig()
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	return config.Build()
}

