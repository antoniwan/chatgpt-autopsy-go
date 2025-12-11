package services

import (
	"fmt"
	"sort"
	"time"

	"chatgpt-autopsy-go/internal/config"
	"chatgpt-autopsy-go/internal/database"
	"chatgpt-autopsy-go/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ThreadService handles date-based thread division
type ThreadService struct {
	cfg *config.Config
	log *zap.Logger
}

// NewThreadService creates a new thread service
func NewThreadService(cfg *config.Config, log *zap.Logger) *ThreadService {
	return &ThreadService{
		cfg: cfg,
		log: log,
	}
}

// CreateThreadsForUpload creates threads for all conversations in an upload
func (s *ThreadService) CreateThreadsForUpload(uploadID uint) error {
	var upload models.Upload
	if err := database.DB.First(&upload, uploadID).Error; err != nil {
		return fmt.Errorf("upload not found: %w", err)
	}

	var importRecord models.Import
	if err := database.DB.Where("upload_id = ?", uploadID).First(&importRecord).Error; err != nil {
		return fmt.Errorf("import record not found: %w", err)
	}

	importRecord.Status = "importing"
	importRecord.ProgressPercent = 70
	database.DB.Save(&importRecord)

	// Get all conversations for this upload
	var conversations []models.Conversation
	if err := database.DB.Where("upload_id = ?", uploadID).Find(&conversations).Error; err != nil {
		return fmt.Errorf("failed to get conversations: %w", err)
	}

	totalThreads := 0

	// Process each conversation
	for i, conv := range conversations {
		threadCount, err := s.createThreadsForConversation(conv.ID)
		if err != nil {
			s.log.Warn("Failed to create threads for conversation",
				zap.Uint("conversation_id", conv.ID),
				zap.Error(err),
			)
			continue
		}

		totalThreads += threadCount

		// Update progress
		progress := 70 + int(float64(i+1)/float64(len(conversations))*25) // 70-95%
		importRecord.ProgressPercent = progress
		database.DB.Save(&importRecord)
	}

	// Update import stats
	var stats map[string]interface{}
	if importRecord.Stats != "" {
		// Parse existing stats
		// For simplicity, we'll just update threads_count
		stats = map[string]interface{}{
			"threads_count": totalThreads,
		}
	} else {
		stats = map[string]interface{}{
			"threads_count": totalThreads,
		}
	}

	// Update import status
	importRecord.Status = "completed"
	importRecord.ProgressPercent = 100
	completedAt := time.Now().UTC()
	importRecord.CompletedAt = &completedAt
	database.DB.Save(&importRecord)

	s.log.Info("Thread creation completed",
		zap.Uint("upload_id", uploadID),
		zap.Int("threads_created", totalThreads),
	)

	return nil
}

// createThreadsForConversation creates threads for a single conversation
func (s *ThreadService) createThreadsForConversation(conversationID uint) (int, error) {
	// Get all messages for this conversation, ordered by timestamp
	var messages []models.Message
	if err := database.DB.Where("conversation_id = ?", conversationID).
		Order("timestamp ASC").
		Find(&messages).Error; err != nil {
		return 0, fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) == 0 {
		return 0, nil
	}

	// Group messages by date (YYYY-MM-DD) in UTC
	messagesByDate := make(map[string][]models.Message)
	
	for _, msg := range messages {
		// Normalize to UTC and extract date
		utcTime := msg.Timestamp.UTC()
		date := utcTime.Format("2006-01-02")
		messagesByDate[date] = append(messagesByDate[date], msg)
	}

	var threads []models.Thread

	// Create thread for each date
	for date, dateMessages := range messagesByDate {
		if len(dateMessages) == 0 {
			continue
		}

		// Check if thread already exists
		var existing models.Thread
		if err := database.DB.Where("conversation_id = ? AND date = ?", conversationID, date).
			First(&existing).Error; err == nil {
			// Thread already exists, skip
			continue
		}

		// Sort messages by timestamp
		sort.Slice(dateMessages, func(i, j int) bool {
			return dateMessages[i].Timestamp.Before(dateMessages[j].Timestamp)
		})

		// Get first and last message IDs
		startMsg := dateMessages[0]
		endMsg := dateMessages[len(dateMessages)-1]

		// Validate that messages belong to same conversation
		if startMsg.ConversationID != conversationID || endMsg.ConversationID != conversationID {
			return 0, fmt.Errorf("message conversation mismatch")
		}

		// Validate date matches message dates
		startDate := startMsg.Timestamp.UTC().Format("2006-01-02")
		endDate := endMsg.Timestamp.UTC().Format("2006-01-02")
		if startDate != date || endDate != date {
			s.log.Warn("Date mismatch in thread creation",
				zap.String("thread_date", date),
				zap.String("start_date", startDate),
				zap.String("end_date", endDate),
			)
		}

		// Validate timestamps
		if startMsg.Timestamp.After(endMsg.Timestamp) {
			return 0, fmt.Errorf("start timestamp after end timestamp")
		}

		thread := models.Thread{
			ConversationID: conversationID,
			Date:           date,
			MessageCount:   len(dateMessages),
			StartMessageID: &startMsg.ID,
			EndMessageID:   &endMsg.ID,
			StartTimestamp: startMsg.Timestamp.UTC(),
			EndTimestamp:   endMsg.Timestamp.UTC(),
		}

		threads = append(threads, thread)
	}

	// Save threads in batches
	if len(threads) > 0 {
		batchSize := 1000
		for i := 0; i < len(threads); i += batchSize {
			end := i + batchSize
			if end > len(threads) {
				end = len(threads)
			}
			if err := database.DB.CreateInBatches(threads[i:end], batchSize).Error; err != nil {
				return 0, fmt.Errorf("failed to create threads batch: %w", err)
			}
		}
	}

	return len(threads), nil
}


// GetThreadsForConversation gets all threads for a conversation
func (s *ThreadService) GetThreadsForConversation(conversationID uint) ([]models.Thread, error) {
	var threads []models.Thread
	if err := database.DB.Where("conversation_id = ?", conversationID).
		Order("date ASC").
		Find(&threads).Error; err != nil {
		return nil, fmt.Errorf("failed to get threads: %w", err)
	}
	return threads, nil
}

// GetThread gets a thread by ID
func (s *ThreadService) GetThread(threadID uint) (*models.Thread, error) {
	var thread models.Thread
	if err := database.DB.First(&thread, threadID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("thread not found: %d", threadID)
		}
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}
	return &thread, nil
}

