package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/models"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Initialize initializes the database connection and runs migrations
func Initialize(cfg *config.Config, log *zap.Logger) error {
	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// Open database connection
	var err error
	DB, err = gorm.Open(sqlite.Open(cfg.Database.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // We'll use zap for logging
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQLite connection to configure
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	// Enable WAL mode for better concurrency
	if err := DB.Exec("PRAGMA journal_mode = WAL").Error; err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign key constraints
	if err := DB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations
	if err := runMigrations(log); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize data directories
	if err := initializeDirectories(cfg, log); err != nil {
		return fmt.Errorf("failed to initialize directories: %w", err)
	}

	log.Info("Database initialized successfully", zap.String("path", cfg.Database.Path))
	return nil
}

// runMigrations runs database migrations
func runMigrations(log *zap.Logger) error {
	models := []interface{}{
		&models.Upload{},
		&models.Import{},
		&models.Conversation{},
		&models.Message{},
		&models.Thread{},
		&models.Extraction{},
		&models.Analysis{},
		&models.SeenStatus{},
		&models.ActionableItem{},
		&models.Question{},
		&models.NoiseFlag{},
	}

	for _, model := range models {
		if err := DB.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate model %T: %w", model, err)
		}
	}

	// Create composite indexes
	if err := createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	log.Info("Database migrations completed")
	return nil
}

// createIndexes creates composite indexes that GORM doesn't create automatically
func createIndexes() error {
	indexes := []string{
		// Thread composite index
		"CREATE INDEX IF NOT EXISTS idx_threads_conversation_date ON threads(conversation_id, date)",
		
		// Message composite index
		"CREATE INDEX IF NOT EXISTS idx_messages_conversation_index ON messages(conversation_id, message_index)",
		
		// Analysis composite index
		"CREATE INDEX IF NOT EXISTS idx_analyses_date_type ON analyses(date, analysis_type)",
		
		// SeenStatus composite indexes
		"CREATE INDEX IF NOT EXISTS idx_seen_status_date_type ON seen_statuses(date, analysis_type)",
		"CREATE INDEX IF NOT EXISTS idx_seen_status_result_type_name ON seen_statuses(result_type, result_name)",
	}

	for _, indexSQL := range indexes {
		if err := DB.Exec(indexSQL).Error; err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// initializeDirectories creates all required data directories
func initializeDirectories(cfg *config.Config, log *zap.Logger) error {
	dirs := []string{
		cfg.Directories.UploadsDir,
		cfg.Directories.ExtractedDir,
		cfg.Directories.AnalysisDir,
		cfg.Directories.MessagesDir,
		filepath.Join(cfg.Directories.AnalysisDir, "cross_file_analysis"),
		filepath.Join(cfg.Directories.AnalysisDir, "questions"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Verify write permissions
		testFile := filepath.Join(dir, ".write_test")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			return fmt.Errorf("directory %s is not writable: %w", dir, err)
		}
		os.Remove(testFile)

		log.Info("Directory initialized", zap.String("path", dir))
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// RetryWithBackoff retries a database operation with exponential backoff
func RetryWithBackoff(maxRetries int, initialDelay time.Duration, fn func() error) error {
	var err error
	delay := initialDelay

	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Check if it's a database locked error
		if isDatabaseLocked(err) {
			if i < maxRetries-1 {
				time.Sleep(delay)
				delay *= 2 // Exponential backoff
				continue
			}
		}

		// If it's not a locked error or we've exhausted retries, return the error
		return err
	}

	return err
}

// isDatabaseLocked checks if the error is a database locked error
func isDatabaseLocked(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "database is locked") || contains(errStr, "database locked")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
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

