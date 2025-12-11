package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// UploadService handles file upload operations
type UploadService struct {
	cfg *config.Config
	log *zap.Logger
}

// NewUploadService creates a new upload service
func NewUploadService(cfg *config.Config, log *zap.Logger) *UploadService {
	return &UploadService{
		cfg: cfg,
		log: log,
	}
}

// UploadFile handles file upload, validation, and storage
func (s *UploadService) UploadFile(originalFilename string, fileReader io.Reader, fileSize int64) (*models.Upload, error) {
	// Validate file size
	if fileSize > s.cfg.Upload.MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d", fileSize, s.cfg.Upload.MaxFileSize)
	}

	// Generate UUID for file storage
	fileUUID := uuid.New().String()
	
	// Create stored filename
	storedFilename := fmt.Sprintf("%s.zip", fileUUID)
	storedPath := filepath.Join(s.cfg.Directories.UploadsDir, storedFilename)

	// Create temp file for atomic write
	tempPath := storedPath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()
	defer os.Remove(tempPath) // Clean up temp file on error

	// Copy file content and calculate hash
	hash := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hash)
	
	written, err := io.Copy(multiWriter, fileReader)
	if err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	if written != fileSize {
		return nil, fmt.Errorf("file size mismatch: expected %d, wrote %d", fileSize, written)
	}

	// Close temp file before rename
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Calculate file hash
	fileHash := hex.EncodeToString(hash.Sum(nil))

	// Check for duplicate upload
	var existingUpload models.Upload
	if err := database.DB.Where("file_hash = ?", fileHash).First(&existingUpload).Error; err == nil {
		s.log.Info("Duplicate file detected", zap.String("hash", fileHash), zap.Uint("existing_id", existingUpload.ID))
		return &existingUpload, fmt.Errorf("file already uploaded (ID: %d)", existingUpload.ID)
	}

	// Atomic rename: temp file → final file
	if err := os.Rename(tempPath, storedPath); err != nil {
		return nil, fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Create Upload and Import records in transaction
	var upload models.Upload
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Create Upload record
		upload = models.Upload{
			UUID:            fileUUID,
			OriginalFilename: originalFilename,
			StoredPath:      storedPath,
			FileSize:        fileSize,
			FileHash:        fileHash,
			MimeType:        "application/zip",
			UploadedAt:      time.Now().UTC(),
			Status:          "pending",
		}

		if err := tx.Create(&upload).Error; err != nil {
			return fmt.Errorf("failed to create upload record: %w", err)
		}

		// Create Import record
		importRecord := models.Import{
			UploadID:        upload.ID,
			StartedAt:       time.Now().UTC(),
			Status:          "pending",
			ProgressPercent: 0,
		}

		if err := tx.Create(&importRecord).Error; err != nil {
			return fmt.Errorf("failed to create import record: %w", err)
		}

		return nil
	})

	if err != nil {
		// Clean up file on database error
		os.Remove(storedPath)
		return nil, err
	}

	s.log.Info("File uploaded successfully",
		zap.Uint("upload_id", upload.ID),
		zap.String("uuid", upload.UUID),
		zap.String("hash", fileHash),
		zap.Int64("size", fileSize),
	)

	return &upload, nil
}

// GetUpload retrieves an upload by ID
func (s *UploadService) GetUpload(id uint) (*models.Upload, error) {
	var upload models.Upload
	if err := database.DB.First(&upload, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("upload not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get upload: %w", err)
	}
	return &upload, nil
}

// ListUploads lists uploads with pagination
func (s *UploadService) ListUploads(page, limit int) ([]models.Upload, int64, error) {
	var uploads []models.Upload
	var total int64

	offset := (page - 1) * limit

	// Get total count
	if err := database.DB.Model(&models.Upload{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count uploads: %w", err)
	}

	// Get paginated results
	if err := database.DB.
		Order("uploaded_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&uploads).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list uploads: %w", err)
	}

	return uploads, total, nil
}

// DeleteUpload deletes an upload and its associated data
func (s *UploadService) DeleteUpload(id uint) error {
	var upload models.Upload
	if err := database.DB.First(&upload, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("upload not found: %d", id)
		}
		return fmt.Errorf("failed to get upload: %w", err)
	}

	// Soft delete (GORM handles this)
	if err := database.DB.Delete(&upload).Error; err != nil {
		return fmt.Errorf("failed to delete upload: %w", err)
	}

	// Note: File cleanup can be done by a background job
	// For now, we just mark it as deleted in the database

	s.log.Info("Upload deleted", zap.Uint("upload_id", id))
	return nil
}

