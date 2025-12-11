package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all configuration values
type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	Upload      UploadConfig
	Directories DirectoriesConfig
	AI          AIConfig
	Analysis    AnalysisConfig
	RateLimit   RateLimitConfig
	Logging     LoggingConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         int
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// UploadConfig holds upload configuration
type UploadConfig struct {
	MaxFileSize       int64
	MaxExtractionSize int64
	MaxExtractedFiles int
}

// DirectoriesConfig holds directory paths
type DirectoriesConfig struct {
	UploadsDir   string
	ExtractedDir string
	AnalysisDir  string
	MessagesDir  string
}

// AIConfig holds AI enhancement configuration
type AIConfig struct {
	OpenAIAPIKey        string
	AnthropicAPIKey     string
	PreferredProvider   string
	EnhancementEnabled  bool
	MaxTokensPerRequest int
	Temperature         float64
}

// AnalysisConfig holds analysis configuration
type AnalysisConfig struct {
	EnableNoiseDetection bool
	NoiseDetectionThreshold float64
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int
	BurstSize         int
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
	Output string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvInt("CHATGPT_AUTOPSY_PORT", 8080),
			Host:         getEnv("CHATGPT_AUTOPSY_HOST", "0.0.0.0"),
			ReadTimeout:  getEnvDuration("CHATGPT_AUTOPSY_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("CHATGPT_AUTOPSY_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getEnvDuration("CHATGPT_AUTOPSY_IDLE_TIMEOUT", 120*time.Second),
		},
		Database: DatabaseConfig{
			Path:            getEnv("CHATGPT_AUTOPSY_DB_PATH", "data/chatgpt_autopsy.db"),
			MaxOpenConns:    getEnvInt("CHATGPT_AUTOPSY_MAX_OPEN_CONNS", 1),
			MaxIdleConns:    getEnvInt("CHATGPT_AUTOPSY_MAX_IDLE_CONNS", 1),
			ConnMaxLifetime: getEnvDuration("CHATGPT_AUTOPSY_CONN_MAX_LIFETIME", 1*time.Hour),
		},
		Upload: UploadConfig{
			MaxFileSize:       getEnvInt64("CHATGPT_AUTOPSY_MAX_FILE_SIZE", 524288000), // 500MB
			MaxExtractionSize: getEnvInt64("CHATGPT_AUTOPSY_MAX_EXTRACTION_SIZE", 2147483648), // 2GB
			MaxExtractedFiles: getEnvInt("CHATGPT_AUTOPSY_MAX_EXTRACTED_FILES", 10000),
		},
		Directories: DirectoriesConfig{
			UploadsDir:   getEnv("CHATGPT_AUTOPSY_UPLOADS_DIR", "data/uploads"),
			ExtractedDir: getEnv("CHATGPT_AUTOPSY_EXTRACTED_DIR", "data/extracted"),
			AnalysisDir:  getEnv("CHATGPT_AUTOPSY_ANALYSIS_DIR", "data/analysis"),
			MessagesDir:  getEnv("CHATGPT_AUTOPSY_MESSAGES_DIR", "data/messages"),
		},
		AI: AIConfig{
			OpenAIAPIKey:        getEnv("OPENAI_API_KEY", ""),
			AnthropicAPIKey:     getEnv("ANTHROPIC_API_KEY", ""),
			PreferredProvider:   getEnv("CHATGPT_AUTOPSY_PREFERRED_AI_PROVIDER", "anthropic"),
			EnhancementEnabled:  getEnvBool("CHATGPT_AUTOPSY_AI_ENHANCEMENT_ENABLED", false),
			MaxTokensPerRequest: getEnvInt("CHATGPT_AUTOPSY_MAX_TOKENS_PER_REQUEST", 4000),
			Temperature:         getEnvFloat64("CHATGPT_AUTOPSY_AI_TEMPERATURE", 0.7),
		},
		Analysis: AnalysisConfig{
			EnableNoiseDetection:  getEnvBool("CHATGPT_AUTOPSY_ENABLE_NOISE_DETECTION", true),
			NoiseDetectionThreshold: getEnvFloat64("CHATGPT_AUTOPSY_NOISE_DETECTION_THRESHOLD", 0.3),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: getEnvInt("CHATGPT_AUTOPSY_REQUESTS_PER_MINUTE", 100),
			BurstSize:         getEnvInt("CHATGPT_AUTOPSY_BURST_SIZE", 10),
		},
		Logging: LoggingConfig{
			Level:  getEnv("CHATGPT_AUTOPSY_LOG_LEVEL", "info"),
			Format: getEnv("CHATGPT_AUTOPSY_LOG_FORMAT", "json"),
			Output: getEnv("CHATGPT_AUTOPSY_LOG_OUTPUT", "stdout"),
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Resolve absolute paths
	if err := cfg.resolvePaths(); err != nil {
		return nil, fmt.Errorf("failed to resolve paths: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate port range
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535, got %d", c.Server.Port)
	}

	// Validate file sizes
	if c.Upload.MaxFileSize <= 0 {
		return fmt.Errorf("max file size must be positive, got %d", c.Upload.MaxFileSize)
	}
	if c.Upload.MaxExtractionSize <= 0 {
		return fmt.Errorf("max extraction size must be positive, got %d", c.Upload.MaxExtractionSize)
	}

	// Validate database path parent exists (or can be created)
	dbDir := filepath.Dir(c.Database.Path)
	if dbDir != "." && dbDir != "" {
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			// Directory doesn't exist, but we can create it, so this is OK
		}
	}

	return nil
}

// resolvePaths resolves all directory paths to absolute paths
func (c *Config) resolvePaths() error {
	var err error

	c.Directories.UploadsDir, err = filepath.Abs(c.Directories.UploadsDir)
	if err != nil {
		return fmt.Errorf("failed to resolve uploads directory: %w", err)
	}

	c.Directories.ExtractedDir, err = filepath.Abs(c.Directories.ExtractedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve extracted directory: %w", err)
	}

	c.Directories.AnalysisDir, err = filepath.Abs(c.Directories.AnalysisDir)
	if err != nil {
		return fmt.Errorf("failed to resolve analysis directory: %w", err)
	}

	c.Directories.MessagesDir, err = filepath.Abs(c.Directories.MessagesDir)
	if err != nil {
		return fmt.Errorf("failed to resolve messages directory: %w", err)
	}

	c.Database.Path, err = filepath.Abs(c.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to resolve database path: %w", err)
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

