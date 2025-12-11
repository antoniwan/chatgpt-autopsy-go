package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ParserService handles parsing of ChatGPT export JSON files
type ParserService struct {
	cfg *config.Config
	log *zap.Logger
}

// NewParserService creates a new parser service
func NewParserService(cfg *config.Config, log *zap.Logger) *ParserService {
	return &ParserService{
		cfg: cfg,
		log: log,
	}
}

// ChatGPTExport represents the structure of ChatGPT export JSON
type ChatGPTExport []ChatGPTConversation

// ChatGPTConversation represents a single conversation in the export
type ChatGPTConversation struct {
	Title      string                 `json:"title"`
	CreateTime float64                `json:"create_time"`
	UpdateTime float64                `json:"update_time"`
	Mapping    map[string]MessageNode `json:"mapping"`
}

// MessageNode represents a message node in the conversation tree
type MessageNode struct {
	ID       string      `json:"id"`
	Message  *MessageData `json:"message"`
	Parent   *string     `json:"parent"`
	Children []string    `json:"children"`
}

// MessageData represents the actual message content
type MessageData struct {
	ID       string                 `json:"id"`
	Author   AuthorData             `json:"author"`
	Content  ContentData             `json:"content"`
	Status   string                 `json:"status"`
	Metadata map[string]interface{} `json:"metadata"`
}

// AuthorData represents message author information
type AuthorData struct {
	Role     string                 `json:"role"`
	Name     *string                `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}

// ContentData represents message content
type ContentData struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

// ParseUpload parses extracted files for an upload
func (s *ParserService) ParseUpload(uploadID uint) error {
	var upload models.Upload
	if err := database.DB.First(&upload, uploadID).Error; err != nil {
		return fmt.Errorf("upload not found: %w", err)
	}

	// Find conversation JSON files
	var extractions []models.Extraction
	if err := database.DB.Where("upload_id = ? AND file_type = ?", uploadID, "conversation").Find(&extractions).Error; err != nil {
		return fmt.Errorf("failed to find extraction files: %w", err)
	}

	if len(extractions) == 0 {
		return fmt.Errorf("no conversation files found for upload %d", uploadID)
	}

	var importRecord models.Import
	if err := database.DB.Where("upload_id = ?", uploadID).First(&importRecord).Error; err != nil {
		return fmt.Errorf("import record not found: %w", err)
	}

	importRecord.Status = "parsing"
	importRecord.ProgressPercent = 40
	database.DB.Save(&importRecord)

	var totalConversations int
	var totalMessages int

	// Process each conversation file
	for i, extraction := range extractions {
		conversations, messages, err := s.parseConversationFile(extraction.FilePath, uploadID)
		if err != nil {
			s.log.Warn("Failed to parse conversation file",
				zap.String("file", extraction.FilePath),
				zap.Error(err),
			)
			continue
		}

		totalConversations += conversations
		totalMessages += messages

		// Update extraction status
		extraction.Status = "parsed"
		database.DB.Save(&extraction)

		// Update progress
		progress := 40 + int(float64(i+1)/float64(len(extractions))*30) // 40-70%
		importRecord.ProgressPercent = progress
		database.DB.Save(&importRecord)
	}

	// Update import stats
	stats := map[string]interface{}{
		"conversations_count": totalConversations,
		"messages_count":      totalMessages,
		"files_extracted":     len(extractions),
	}
	statsJSON, _ := json.Marshal(stats)
	importRecord.Stats = string(statsJSON)
	importRecord.Status = "importing"
	importRecord.ProgressPercent = 70
	database.DB.Save(&importRecord)

	s.log.Info("Parsing completed",
		zap.Uint("upload_id", uploadID),
		zap.Int("conversations", totalConversations),
		zap.Int("messages", totalMessages),
	)

	return nil
}

// parseConversationFile parses a single conversation JSON file
func (s *ParserService) parseConversationFile(filePath string, uploadID uint) (int, int, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var export ChatGPTExport
	if err := json.Unmarshal(data, &export); err != nil {
		return 0, 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var conversationsCreated int
	var messagesCreated int

	// Process each conversation
	for _, conv := range export {
		convRecord, msgCount, err := s.processConversation(conv, uploadID, filePath)
		if err != nil {
			s.log.Warn("Failed to process conversation",
				zap.String("title", conv.Title),
				zap.Error(err),
			)
			continue
		}

		conversationsCreated++
		messagesCreated += msgCount

		// Update conversation message count
		convRecord.MessageCount = msgCount
		database.DB.Save(&convRecord)
	}

	return conversationsCreated, messagesCreated, nil
}

// processConversation processes a single conversation and creates database records
func (s *ParserService) processConversation(conv ChatGPTConversation, uploadID uint, sourcePath string) (*models.Conversation, int, error) {
	// Create conversation record
	conversation := models.Conversation{
		UploadID:       uploadID,
		ConversationID: s.findRootMessageID(conv.Mapping),
		Title:          &conv.Title,
		CreatedAt:      time.Unix(int64(conv.CreateTime), 0).UTC(),
		UpdatedAt:      time.Unix(int64(conv.UpdateTime), 0).UTC(),
		SourceFilePath: sourcePath,
		MessageCount:   0,
	}

	// Check if conversation already exists
	var existing models.Conversation
	if err := database.DB.Where("upload_id = ? AND conversation_id = ?", uploadID, conversation.ConversationID).
		First(&existing).Error; err == nil {
		// Conversation already exists, return it
		return &existing, existing.MessageCount, nil
	}

	// Create conversation
	if err := database.DB.Create(&conversation).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to create conversation: %w", err)
	}

	// Process messages
	messages, err := s.processMessages(conv.Mapping, conversation.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to process messages: %w", err)
	}

	// Save messages in batches
	if len(messages) > 0 {
		batchSize := 1000
		for i := 0; i < len(messages); i += batchSize {
			end := i + batchSize
			if end > len(messages) {
				end = len(messages)
			}
			if err := database.DB.CreateInBatches(messages[i:end], batchSize).Error; err != nil {
				return nil, 0, fmt.Errorf("failed to create messages batch: %w", err)
			}
		}
	}

	// Extract and save user messages by date
	if err := s.extractUserMessagesByDate(messages, conversation.ID); err != nil {
		s.log.Warn("Failed to extract user messages by date", zap.Error(err))
	}

	return &conversation, len(messages), nil
}

// processMessages processes message nodes and creates message records
func (s *ParserService) processMessages(mapping map[string]MessageNode, conversationID uint) ([]models.Message, error) {
	var messages []models.Message
	messageIndex := 0

	// Find root message (no parent)
	var rootID string
	for id, node := range mapping {
		if node.Parent == nil {
			rootID = id
			break
		}
	}

	if rootID == "" {
		return messages, nil
	}

	// Traverse message tree
	s.traverseMessages(mapping, rootID, conversationID, &messages, &messageIndex)

	// Sort by message_index
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].MessageIndex < messages[j].MessageIndex
	})

	return messages, nil
}

// traverseMessages recursively traverses the message tree
func (s *ParserService) traverseMessages(mapping map[string]MessageNode, nodeID string, conversationID uint, messages *[]models.Message, index *int) {
	node, exists := mapping[nodeID]
	if !exists || node.Message == nil {
		return
	}

	msg := node.Message
	if msg.Status != "finished_successfully" {
		// Skip incomplete messages
		for _, childID := range node.Children {
			s.traverseMessages(mapping, childID, conversationID, messages, index)
		}
		return
	}

	// Extract content parts
	var content strings.Builder
	for i, part := range msg.Content.Parts {
		if i > 0 {
			content.WriteString("\n\n")
		}
		content.WriteString(part)
	}

	// Parse timestamp (usually in metadata or use current time as fallback)
	timestamp := time.Now().UTC()
	if createTime, ok := msg.Metadata["create_time"].(float64); ok {
		timestamp = time.Unix(int64(createTime), 0).UTC()
	}

	// Create message record
	message := models.Message{
		ConversationID: conversationID,
		MessageID:      &msg.ID,
		Role:           msg.Author.Role,
		Content:        content.String(),
		Timestamp:      timestamp,
		MessageIndex:   *index,
	}

	// Serialize metadata
	if metadataJSON, err := json.Marshal(msg.Metadata); err == nil {
		message.Metadata = string(metadataJSON)
	}

	*messages = append(*messages, message)
	*index++

	// Process children
	for _, childID := range node.Children {
		s.traverseMessages(mapping, childID, conversationID, messages, index)
	}
}

// findRootMessageID finds the root message ID (conversation identifier)
func (s *ParserService) findRootMessageID(mapping map[string]MessageNode) string {
	for id, node := range mapping {
		if node.Parent == nil {
			return id
		}
	}
	// Fallback: return first key
	for id := range mapping {
		return id
	}
	return ""
}

// extractUserMessagesByDate extracts user messages and organizes them by date
func (s *ParserService) extractUserMessagesByDate(messages []models.Message, conversationID uint) error {
	// Group messages by date
	messagesByDate := make(map[string][]models.Message)
	
	for _, msg := range messages {
		if msg.Role == "user" {
			date := msg.Timestamp.Format("2006-01-02")
			messagesByDate[date] = append(messagesByDate[date], msg)
		}
	}

	// Write to date files
	for date, msgs := range messagesByDate {
		// Sort messages by timestamp
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].Timestamp.Before(msgs[j].Timestamp)
		})

		// Create file path
		filePath := filepath.Join(s.cfg.Directories.MessagesDir, fmt.Sprintf("%s.md", date))

		// Append to file (multiple conversations may contribute to same date)
		var content strings.Builder
		content.WriteString(fmt.Sprintf("# Messages for %s\n\n", date))
		
		for _, msg := range msgs {
			content.WriteString(fmt.Sprintf("## %s\n\n", msg.Timestamp.Format("15:04:05")))
			content.WriteString(msg.Content)
			content.WriteString("\n\n---\n\n")
		}

		// Append to existing file or create new
		existingContent, _ := os.ReadFile(filePath)
		if len(existingContent) > 0 {
			content.WriteString("\n\n")
			content.Write(existingContent)
		}

		if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
			return fmt.Errorf("failed to write message file: %w", err)
		}
	}

	return nil
}

