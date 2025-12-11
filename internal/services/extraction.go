package services

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ExtractionService handles ZIP file extraction
type ExtractionService struct {
	cfg *config.Config
	log *zap.Logger
}

// NewExtractionService creates a new extraction service
func NewExtractionService(cfg *config.Config, log *zap.Logger) *ExtractionService {
	return &ExtractionService{
		cfg: cfg,
		log: log,
	}
}

// ExtractUpload extracts a ZIP file for an upload
func (s *ExtractionService) ExtractUpload(uploadID uint) error {
	var upload models.Upload
	if err := database.DB.First(&upload, uploadID).Error; err != nil {
		return fmt.Errorf("upload not found: %w", err)
	}

	// Update import status to extracting
	var importRecord models.Import
	if err := database.DB.Where("upload_id = ?", uploadID).First(&importRecord).Error; err != nil {
		return fmt.Errorf("import record not found: %w", err)
	}

	importRecord.Status = "extracting"
	importRecord.ProgressPercent = 10
	if err := database.DB.Save(&importRecord).Error; err != nil {
		return fmt.Errorf("failed to update import status: %w", err)
	}

	// Open ZIP file
	zipReader, err := zip.OpenReader(upload.StoredPath)
	if err != nil {
		importRecord.Status = "failed"
		errorMsg := fmt.Sprintf("failed to open ZIP file: %v", err)
		importRecord.ErrorMessage = &errorMsg
		database.DB.Save(&importRecord)
		return fmt.Errorf("failed to open ZIP file: %w", err)
	}
	defer zipReader.Close()

	// Create extraction directory using upload UUID
	extractDir := filepath.Join(s.cfg.Directories.ExtractedDir, upload.UUID)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}

	var totalSize int64
	var fileCount int
	var extractedFiles []models.Extraction

	// Extract files with path traversal protection
	for _, file := range zipReader.File {
		// Validate file count limit
		if fileCount >= s.cfg.Upload.MaxExtractedFiles {
			errorMsg := fmt.Sprintf("exceeded max extracted files: %d", s.cfg.Upload.MaxExtractedFiles)
			importRecord.Status = "failed"
			importRecord.ErrorMessage = &errorMsg
			database.DB.Save(&importRecord)
			return fmt.Errorf("exceeded max extracted files limit")
		}

		// Sanitize file path
		sanitizedPath, err := s.sanitizePath(file.Name, extractDir)
		if err != nil {
			s.log.Warn("Skipping file with invalid path",
				zap.String("file", file.Name),
				zap.Error(err),
			)
			continue
		}

		// Check total extraction size
		if totalSize+file.UncompressedSize64 > uint64(s.cfg.Upload.MaxExtractionSize) {
			errorMsg := fmt.Sprintf("exceeded max extraction size: %d", s.cfg.Upload.MaxExtractionSize)
			importRecord.Status = "failed"
			importRecord.ErrorMessage = &errorMsg
			database.DB.Save(&importRecord)
			return fmt.Errorf("exceeded max extraction size limit")
		}

		// Extract file
		if err := s.extractFile(file, sanitizedPath); err != nil {
			s.log.Warn("Failed to extract file",
				zap.String("file", file.Name),
				zap.Error(err),
			)
			continue
		}

		// Determine file type
		fileType := s.determineFileType(file.Name)

		// Create extraction record
		extraction := models.Extraction{
			UploadID:    uploadID,
			FilePath:    sanitizedPath,
			FileType:    fileType,
			FileSize:    int64(file.UncompressedSize64),
			ExtractedAt: time.Now().UTC(),
			Status:      "extracted",
		}
		extractedFiles = append(extractedFiles, extraction)

		totalSize += int64(file.UncompressedSize64)
		fileCount++

		// Update progress
		progress := 10 + int(float64(fileCount)/float64(len(zipReader.File))*30) // 10-40%
		importRecord.ProgressPercent = progress
		database.DB.Save(&importRecord)
	}

	// Save extraction records in batch
	if len(extractedFiles) > 0 {
		if err := database.DB.CreateInBatches(extractedFiles, 100).Error; err != nil {
			return fmt.Errorf("failed to create extraction records: %w", err)
		}
	}

	// Update import status to parsing
	importRecord.Status = "parsing"
	importRecord.ProgressPercent = 40
	if err := database.DB.Save(&importRecord).Error; err != nil {
		return fmt.Errorf("failed to update import status: %w", err)
	}

	s.log.Info("Extraction completed",
		zap.Uint("upload_id", uploadID),
		zap.Int("files_extracted", fileCount),
		zap.Int64("total_size", totalSize),
	)

	return nil
}

// sanitizePath sanitizes file paths to prevent directory traversal attacks
func (s *ExtractionService) sanitizePath(filePath, baseDir string) (string, error) {
	// Clean the path
	cleaned := filepath.Clean(filePath)

	// Remove any leading separators
	cleaned = strings.TrimPrefix(cleaned, string(filepath.Separator))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "\\")

	// Reject paths with ..
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path contains '..': %s", filePath)
	}

	// Reject absolute paths
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed: %s", filePath)
	}

	// Join with base directory
	fullPath := filepath.Join(baseDir, cleaned)

	// Ensure the resolved path is within base directory
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute file path: %w", err)
	}

	if !strings.HasPrefix(fullAbs, baseAbs) {
		return "", fmt.Errorf("path outside base directory: %s", filePath)
	}

	return fullPath, nil
}

// extractFile extracts a single file from the ZIP archive
func (s *ExtractionService) extractFile(file *zip.File, destPath string) error {
	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file from ZIP
	srcFile, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in ZIP: %w", err)
	}
	defer srcFile.Close()

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy file content
	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

// determineFileType determines the type of extracted file
func (s *ExtractionService) determineFileType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	// Check for conversation JSON files
	if ext == ".json" && (strings.Contains(filePath, "conversation") || 
		strings.Contains(filePath, "chat") || 
		strings.Contains(filePath, "message")) {
		return "conversation"
	}

	// Check for media files
	mediaExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".mp3", ".mp4", ".wav", ".pdf"}
	for _, mediaExt := range mediaExts {
		if ext == mediaExt {
			return "media"
		}
	}

	return "other"
}

