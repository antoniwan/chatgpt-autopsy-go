package api

import (
	"net/http"
	"strconv"
	"time"

	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"
	"chatgpt-autopsy-go/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Handler holds all handlers and dependencies
type Handler struct {
	uploadService    *services.UploadService
	extractionService *services.ExtractionService
	parserService    *services.ParserService
	threadService    *services.ThreadService
	analysisService  *services.AnalysisService
	log              *zap.Logger
}

// NewHandler creates a new handler instance
func NewHandler(
	uploadService *services.UploadService,
	extractionService *services.ExtractionService,
	parserService *services.ParserService,
	threadService *services.ThreadService,
	analysisService *services.AnalysisService,
	log *zap.Logger,
) *Handler {
	return &Handler{
		uploadService:    uploadService,
		extractionService: extractionService,
		parserService:    parserService,
		threadService:    threadService,
		analysisService:  analysisService,
		log:              log,
	}
}

// HealthCheck handles health check endpoint
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ReadyCheck handles readiness check endpoint
func (h *Handler) ReadyCheck(c *gin.Context) {
	// Check database connection
	sqlDB, err := database.DB.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": "database_connection_failed",
		})
		return
	}

	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": "database_ping_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}

// UploadFile handles file upload
func (h *Handler) UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "INVALID_FILE", "No file provided", err)
		return
	}

	// Validate file extension
	if !isValidZipFile(file.Filename) {
		h.errorResponse(c, http.StatusBadRequest, "INVALID_FILE_TYPE", "File must be a ZIP file", nil)
		return
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "FILE_OPEN_ERROR", "Failed to open file", err)
		return
	}
	defer src.Close()

	// Upload file
	upload, err := h.uploadService.UploadFile(file.Filename, src, file.Size)
	if err != nil {
		if contains(err.Error(), "already uploaded") {
			h.errorResponse(c, http.StatusConflict, "DUPLICATE_UPLOAD", err.Error(), err)
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "UPLOAD_ERROR", "Failed to upload file", err)
		return
	}

	// Trigger async extraction
	go func() {
		if err := h.extractionService.ExtractUpload(upload.ID); err != nil {
			h.log.Error("Extraction failed", zap.Uint("upload_id", upload.ID), zap.Error(err))
			return
		}

		// Trigger parsing
		if err := h.parserService.ParseUpload(upload.ID); err != nil {
			h.log.Error("Parsing failed", zap.Uint("upload_id", upload.ID), zap.Error(err))
			return
		}

		// Trigger thread creation
		if err := h.threadService.CreateThreadsForUpload(upload.ID); err != nil {
			h.log.Error("Thread creation failed", zap.Uint("upload_id", upload.ID), zap.Error(err))
			return
		}
	}()

	c.JSON(http.StatusCreated, gin.H{
		"upload": upload,
	})
}

// ListUploads lists all uploads
func (h *Handler) ListUploads(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if limit > 500 {
		limit = 500
	}

	uploads, total, err := h.uploadService.ListUploads(page, limit)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "LIST_ERROR", "Failed to list uploads", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"uploads": uploads,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetUpload gets upload details
func (h *Handler) GetUpload(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid upload ID", err)
		return
	}

	upload, err := h.uploadService.GetUpload(uint(id))
	if err != nil {
		if contains(err.Error(), "not found") {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Upload not found", err)
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "GET_ERROR", "Failed to get upload", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload": upload,
	})
}

// DeleteUpload deletes an upload
func (h *Handler) DeleteUpload(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid upload ID", err)
		return
	}

	if err := h.uploadService.DeleteUpload(uint(id)); err != nil {
		if contains(err.Error(), "not found") {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Upload not found", err)
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "DELETE_ERROR", "Failed to delete upload", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Upload deleted successfully",
	})
}

// ListConversations lists conversations
func (h *Handler) ListConversations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if limit > 500 {
		limit = 500
	}

	var conversations []models.Conversation
	var total int64

	query := database.DB.Model(&models.Conversation{})

	// Filter by upload_id if provided
	if uploadID := c.Query("upload_id"); uploadID != "" {
		query = query.Where("upload_id = ?", uploadID)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "COUNT_ERROR", "Failed to count conversations", err)
		return
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&conversations).Error; err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "LIST_ERROR", "Failed to list conversations", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversations": conversations,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetConversation gets conversation details
func (h *Handler) GetConversation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid conversation ID", err)
		return
	}

	var conversation models.Conversation
	if err := database.DB.Preload("Messages").First(&conversation, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Conversation not found", err)
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "GET_ERROR", "Failed to get conversation", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation": conversation,
	})
}

// ListDates lists all analysis dates
func (h *Handler) ListDates(c *gin.Context) {
	var dates []string
	if err := database.DB.Model(&models.Thread{}).
		Distinct("date").
		Order("date DESC").
		Pluck("date", &dates).Error; err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "LIST_ERROR", "Failed to list dates", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"dates": dates,
	})
}

// GetAnalysis gets analysis for a date and type
func (h *Handler) GetAnalysis(c *gin.Context) {
	date := c.Param("date")
	analysisType := c.Param("type")

	var analysis models.Analysis
	if err := database.DB.Where("date = ? AND analysis_type = ?", date, analysisType).First(&analysis).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Analysis not found", err)
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "GET_ERROR", "Failed to get analysis", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"analysis": analysis,
	})
}

// errorResponse sends a standardized error response
func (h *Handler) errorResponse(c *gin.Context, status int, code, message string, err error) {
	requestID, _ := c.Get("request_id")
	
	response := gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
		"request_id": requestID,
	}

	if err != nil {
		h.log.Error("Request error",
			zap.String("code", code),
			zap.String("message", message),
			zap.Error(err),
			zap.String("request_id", requestID.(string)),
		)
	}

	c.JSON(status, response)
}

// Helper functions
func isValidZipFile(filename string) bool {
	return len(filename) > 4 && filename[len(filename)-4:] == ".zip"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

