package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AnalysisService handles analysis generation
type AnalysisService struct {
	cfg *config.Config
	log *zap.Logger
}

// NewAnalysisService creates a new analysis service
func NewAnalysisService(cfg *config.Config, log *zap.Logger) *AnalysisService {
	return &AnalysisService{
		cfg: cfg,
		log: log,
	}
}

// AnalysisDimensions are the 9 core analysis dimensions
var AnalysisDimensions = []string{
	"meaning",
	"signals",
	"shadows",
	"lies",
	"truths",
	"questionable_truths",
	"actionable_items",
	"doubts",
	"topics_of_interest",
}

// GenerateAnalysisForDate generates 9-dimensional analysis for a specific date
func (s *AnalysisService) GenerateAnalysisForDate(date string, force bool) error {
	// Verify Thread records exist for the date
	var threads []models.Thread
	if err := database.DB.Where("date = ?", date).Find(&threads).Error; err != nil {
		return fmt.Errorf("failed to get threads for date: %w", err)
	}

	if len(threads) == 0 {
		return fmt.Errorf("no threads found for date: %s", date)
	}

	// Check if analysis already exists
	if !force {
		var existing []models.Analysis
		if err := database.DB.Where("date = ?", date).Find(&existing).Error; err == nil && len(existing) > 0 {
			s.log.Info("Analysis already exists for date", zap.String("date", date))
			return nil
		}
	}

	// Create analysis directory
	analysisDir := filepath.Join(s.cfg.Directories.AnalysisDir, date)
	if err := os.MkdirAll(analysisDir, 0755); err != nil {
		return fmt.Errorf("failed to create analysis directory: %w", err)
	}

	// Get messages for this date from all threads
	var messages []models.Message
	for _, thread := range threads {
		var threadMessages []models.Message
		if thread.StartMessageID != nil && thread.EndMessageID != nil {
			if err := database.DB.Where("id >= ? AND id <= ?", *thread.StartMessageID, *thread.EndMessageID).
				Order("timestamp ASC").
				Find(&threadMessages).Error; err != nil {
				s.log.Warn("Failed to get messages for thread",
					zap.Uint("thread_id", thread.ID),
					zap.Error(err),
				)
				continue
			}
		}
		messages = append(messages, threadMessages...)
	}

	// Generate analyses for each dimension
	for _, dimension := range AnalysisDimensions {
		if err := s.generateDimensionAnalysis(date, dimension, messages, threads); err != nil {
			s.log.Warn("Failed to generate dimension analysis",
				zap.String("date", date),
				zap.String("dimension", dimension),
				zap.Error(err),
			)
			continue
		}
	}

	// Generate synthesis and summary
	if err := s.generateSynthesis(date, messages); err != nil {
		s.log.Warn("Failed to generate synthesis", zap.String("date", date), zap.Error(err))
	}

	if err := s.generateSummary(date, messages); err != nil {
		s.log.Warn("Failed to generate summary", zap.String("date", date), zap.Error(err))
	}

	s.log.Info("Analysis generation completed", zap.String("date", date))
	return nil
}

// generateDimensionAnalysis generates analysis for a specific dimension
func (s *AnalysisService) generateDimensionAnalysis(date, dimension string, messages []models.Message, threads []models.Thread) error {
	// Create template analysis data
	analysisData := map[string]interface{}{
		"dimension": dimension,
		"date":      date,
		"message_count": len(messages),
		"thread_count": len(threads),
		"content":   s.generateTemplateContent(dimension, messages),
	}

	analysisDataJSON, _ := json.Marshal(analysisData)

	// Create markdown content
	markdownContent := s.generateMarkdownContent(dimension, analysisData)

	// Get first thread for thread_id reference
	var threadID *uint
	if len(threads) > 0 {
		threadID = &threads[0].ID
	}

	// Create or update analysis record
	var analysis models.Analysis
	err := database.DB.Where("date = ? AND analysis_type = ?", date, dimension).First(&analysis).Error
	
	if err == gorm.ErrRecordNotFound {
		// Create new analysis
		analysis = models.Analysis{
			Date:            &date,
			ThreadID:        threadID,
			AnalysisType:    dimension,
			AnalysisData:    string(analysisDataJSON),
			MarkdownContent: markdownContent,
			IsAIEnhanced:    false,
			CreatedAt:       time.Now().UTC(),
		}
		if err := database.DB.Create(&analysis).Error; err != nil {
			return fmt.Errorf("failed to create analysis record: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check existing analysis: %w", err)
	} else {
		// Update existing analysis
		analysis.AnalysisData = string(analysisDataJSON)
		analysis.MarkdownContent = markdownContent
		updatedAt := time.Now().UTC()
		analysis.UpdatedAt = &updatedAt
		if err := database.DB.Save(&analysis).Error; err != nil {
			return fmt.Errorf("failed to update analysis record: %w", err)
		}
	}

	// Save markdown file
	analysisDir := filepath.Join(s.cfg.Directories.AnalysisDir, date)
	filePath := filepath.Join(analysisDir, fmt.Sprintf("%s.md", dimension))
	if err := os.WriteFile(filePath, []byte(markdownContent), 0644); err != nil {
		return fmt.Errorf("failed to write markdown file: %w", err)
	}

	return nil
}

// generateTemplateContent generates template content for a dimension
func (s *AnalysisService) generateTemplateContent(dimension string, messages []models.Message) string {
	// Extract user messages for analysis
	var userMessages []string
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessages = append(userMessages, msg.Content)
		}
	}

	// Generate basic template based on dimension
	templates := map[string]string{
		"meaning":             "Core themes and underlying messages extracted from conversations.",
		"signals":             "Behavioral patterns and communication styles observed.",
		"shadows":             "Unconscious patterns and potential blind spots identified.",
		"lies":                "Self-deceptions and rationalizations detected.",
		"truths":               "Authentic expressions and validated experiences noted.",
		"questionable_truths": "Beliefs requiring further examination identified.",
		"actionable_items":    "Concrete steps for growth and optimization extracted.",
		"doubts":              "Uncertainties and unresolved questions documented.",
		"topics_of_interest":  "Recurring themes and areas of passion identified.",
	}

	content := templates[dimension]
	if len(userMessages) > 0 {
		content += fmt.Sprintf("\n\nBased on %d user messages analyzed.", len(userMessages))
	}

	return content
}

// generateMarkdownContent generates markdown content for analysis
func (s *AnalysisService) generateMarkdownContent(dimension string, data map[string]interface{}) string {
	content := fmt.Sprintf("# %s Analysis\n\n", capitalizeFirst(dimension))
	
	if date, ok := data["date"].(string); ok {
		content += fmt.Sprintf("**Date:** %s\n\n", date)
	}
	
	if msgCount, ok := data["message_count"].(int); ok {
		content += fmt.Sprintf("**Messages Analyzed:** %d\n\n", msgCount)
	}
	
	content += "---\n\n"
	
	if contentData, ok := data["content"].(string); ok {
		content += contentData
	}
	
	return content
}

// generateSynthesis generates synthesis analysis
func (s *AnalysisService) generateSynthesis(date string, messages []models.Message) error {
	analysisData := map[string]interface{}{
		"type":         "synthesis",
		"date":         date,
		"message_count": len(messages),
		"content":      "Integrated view combining insights from all analysis dimensions.",
	}

	analysisDataJSON, _ := json.Marshal(analysisData)
	markdownContent := fmt.Sprintf("# Synthesis\n\n**Date:** %s\n\n---\n\nIntegrated analysis combining all dimensions.", date)

	var analysis models.Analysis
	err := database.DB.Where("date = ? AND analysis_type = ?", date, "synthesis").First(&analysis).Error
	
	if err == gorm.ErrRecordNotFound {
		analysis = models.Analysis{
			Date:            &date,
			AnalysisType:    "synthesis",
			AnalysisData:    string(analysisDataJSON),
			MarkdownContent: markdownContent,
			CreatedAt:       time.Now().UTC(),
		}
		if err := database.DB.Create(&analysis).Error; err != nil {
			return err
		}
	} else {
		analysis.AnalysisData = string(analysisDataJSON)
		analysis.MarkdownContent = markdownContent
		updatedAt := time.Now().UTC()
		analysis.UpdatedAt = &updatedAt
		if err := database.DB.Save(&analysis).Error; err != nil {
			return err
		}
	}

	// Save file
	analysisDir := filepath.Join(s.cfg.Directories.AnalysisDir, date)
	filePath := filepath.Join(analysisDir, "synthesis.md")
	return os.WriteFile(filePath, []byte(markdownContent), 0644)
}

// generateSummary generates summary analysis
func (s *AnalysisService) generateSummary(date string, messages []models.Message) error {
	analysisData := map[string]interface{}{
		"type":         "summary",
		"date":         date,
		"message_count": len(messages),
		"content":      "Quick overview and key insights.",
	}

	analysisDataJSON, _ := json.Marshal(analysisData)
	markdownContent := fmt.Sprintf("# Summary\n\n**Date:** %s\n\n---\n\nQuick overview of key insights from the day's conversations.", date)

	var analysis models.Analysis
	err := database.DB.Where("date = ? AND analysis_type = ?", date, "summary").First(&analysis).Error
	
	if err == gorm.ErrRecordNotFound {
		analysis = models.Analysis{
			Date:            &date,
			AnalysisType:    "summary",
			AnalysisData:    string(analysisDataJSON),
			MarkdownContent: markdownContent,
			CreatedAt:       time.Now().UTC(),
		}
		if err := database.DB.Create(&analysis).Error; err != nil {
			return err
		}
	} else {
		analysis.AnalysisData = string(analysisDataJSON)
		analysis.MarkdownContent = markdownContent
		updatedAt := time.Now().UTC()
		analysis.UpdatedAt = &updatedAt
		if err := database.DB.Save(&analysis).Error; err != nil {
			return err
		}
	}

	// Save file
	analysisDir := filepath.Join(s.cfg.Directories.AnalysisDir, date)
	filePath := filepath.Join(analysisDir, "summary.md")
	return os.WriteFile(filePath, []byte(markdownContent), 0644)
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

