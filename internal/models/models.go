package models

import (
	"time"

	"gorm.io/gorm"
)

// Upload tracks uploaded ZIP files
type Upload struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	UUID            string         `gorm:"uniqueIndex;not null" json:"uuid"`
	OriginalFilename string        `gorm:"type:varchar(255);not null" json:"original_filename"`
	StoredPath      string         `gorm:"not null" json:"stored_path"`
	FileSize        int64          `json:"file_size"`
	FileHash        string         `gorm:"uniqueIndex;not null" json:"file_hash"` // SHA256 hex digest
	MimeType        string         `gorm:"default:'application/zip'" json:"mime_type"`
	UploadedAt      time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"uploaded_at"`
	Status          string         `gorm:"type:varchar(50);not null;index" json:"status"` // pending, processing, completed, failed, deleted
	ErrorMessage    *string        `json:"error_message,omitempty"`
	Metadata        string         `gorm:"type:text" json:"metadata"` // JSON
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Import        Import         `gorm:"constraint:OnDelete:CASCADE"`
	Extractions   []Extraction   `gorm:"constraint:OnDelete:CASCADE"`
	Conversations []Conversation `gorm:"constraint:OnDelete:CASCADE"`
	Analyses      []Analysis     `gorm:"constraint:OnDelete:CASCADE"`
}

// Import tracks import process for each upload
type Import struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	UploadID        uint       `gorm:"uniqueIndex;not null" json:"upload_id"`
	StartedAt       time.Time  `gorm:"not null" json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Status          string     `gorm:"type:varchar(50);not null;index" json:"status"` // pending, extracting, parsing, importing, completed, failed
	ProgressPercent int        `gorm:"default:0" json:"progress_percent"`            // 0-100
	ErrorMessage    *string    `json:"error_message,omitempty"`
	Stats           string     `gorm:"type:text" json:"stats"` // JSON: conversations_count, messages_count, threads_count, files_extracted

	// Relationships
	Upload Upload `gorm:"constraint:OnDelete:CASCADE"`
}

// Conversation represents individual conversations from ChatGPT export
type Conversation struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	UploadID        uint       `gorm:"not null;index" json:"upload_id"`
	ConversationID  string     `gorm:"not null;index" json:"conversation_id"` // ChatGPT's original ID
	Title           *string    `gorm:"type:varchar(500)" json:"title,omitempty"`
	CreatedAt       time.Time  `gorm:"index" json:"created_at"` // From ChatGPT export
	UpdatedAt       time.Time  `json:"updated_at"`               // From ChatGPT export
	SourceFilePath  string     `json:"source_file_path"`
	MessageCount    int        `gorm:"default:0" json:"message_count"`
	Metadata        string     `gorm:"type:text" json:"metadata"` // JSON

	// Relationships
	Upload   Upload    `gorm:"constraint:OnDelete:CASCADE"`
	Messages []Message `gorm:"constraint:OnDelete:CASCADE"`
	Threads  []Thread  `gorm:"constraint:OnDelete:CASCADE"`
	Analyses []Analysis `gorm:"constraint:OnDelete:CASCADE"`
}

// Message represents individual messages within conversations
type Message struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ConversationID uint     `gorm:"not null;index" json:"conversation_id"`
	MessageID    *string    `json:"message_id,omitempty"` // ChatGPT's original message ID
	Role         string     `gorm:"type:varchar(50);not null;index" json:"role"` // user, assistant, system
	Content      string     `gorm:"type:text;not null" json:"content"`
	Timestamp    time.Time  `gorm:"not null;index" json:"timestamp"`
	MessageIndex int        `gorm:"not null;index" json:"message_index"`
	Metadata     string     `gorm:"type:text" json:"metadata"` // JSON

	// Relationships
	Conversation Conversation `gorm:"constraint:OnDelete:CASCADE"`
}

// Thread represents date-based thread divisions
type Thread struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ConversationID uint       `gorm:"not null;index" json:"conversation_id"`
	Date           string     `gorm:"type:date;not null;index" json:"date"` // YYYY-MM-DD
	MessageCount   int        `gorm:"not null" json:"message_count"`
	StartMessageID *uint      `gorm:"index" json:"start_message_id,omitempty"`
	EndMessageID   *uint      `gorm:"index" json:"end_message_id,omitempty"`
	StartTimestamp time.Time  `gorm:"not null" json:"start_timestamp"`
	EndTimestamp   time.Time  `gorm:"not null" json:"end_timestamp"`

	// Relationships
	Conversation Conversation `gorm:"constraint:OnDelete:CASCADE"`
	Analyses     []Analysis   `gorm:"constraint:OnDelete:CASCADE"`
}

// Extraction tracks extracted files from ZIP
type Extraction struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UploadID     uint       `gorm:"not null;index" json:"upload_id"`
	FilePath     string     `gorm:"not null" json:"file_path"`
	FileType     string     `gorm:"type:varchar(50);index" json:"file_type"` // conversation, media, other
	FileSize     int64      `json:"file_size"`
	ExtractedAt  time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"extracted_at"`
	Status       string     `gorm:"type:varchar(50);index" json:"status"` // extracted, parsed, failed
	ErrorMessage *string    `json:"error_message,omitempty"`

	// Relationships
	Upload Upload `gorm:"constraint:OnDelete:CASCADE"`
}

// Analysis stores analysis results
type Analysis struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	UploadID        *uint       `gorm:"index" json:"upload_id,omitempty"`
	ConversationID  *uint       `gorm:"index" json:"conversation_id,omitempty"`
	ThreadID        *uint       `gorm:"index" json:"thread_id,omitempty"`
	Date            *string     `gorm:"type:date;index" json:"date,omitempty"` // YYYY-MM-DD
	AnalysisType    string      `gorm:"type:varchar(100);not null;index" json:"analysis_type"` // meaning, signals, shadows, etc.
	AnalysisData    string      `gorm:"type:text;not null" json:"analysis_data"` // JSON
	MarkdownContent string      `gorm:"type:text" json:"markdown_content,omitempty"`
	IsAIEnhanced    bool        `gorm:"default:false" json:"is_ai_enhanced"`
	AIProvider       *string     `gorm:"type:varchar(50)" json:"ai_provider,omitempty"` // openai, anthropic
	CreatedAt        time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP;index" json:"created_at"`
	UpdatedAt        *time.Time  `json:"updated_at,omitempty"`
	Version          *string     `gorm:"type:varchar(50)" json:"version,omitempty"`

	// Relationships
	Upload       *Upload       `gorm:"constraint:OnDelete:CASCADE"`
	Conversation *Conversation `gorm:"constraint:OnDelete:CASCADE"`
	Thread       *Thread       `gorm:"constraint:OnDelete:CASCADE"`
}

// SeenStatus tracks which analysis pages user has viewed (UI metadata)
type SeenStatus struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Date         *string    `gorm:"type:date;index" json:"date,omitempty"` // YYYY-MM-DD
	AnalysisType *string    `gorm:"type:varchar(100);index" json:"analysis_type,omitempty"`
	ResultType   *string    `gorm:"type:varchar(50);index" json:"result_type,omitempty"` // actionables, synthesis, cross_file, questions
	ResultName   *string    `gorm:"type:varchar(255);index" json:"result_name,omitempty"`
	SeenAt       time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP;index" json:"seen_at"`
	UserID       *string    `gorm:"type:varchar(100);index" json:"user_id,omitempty"` // Future multi-user support
}

// ActionableItem represents extracted actionable items
type ActionableItem struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UploadID       *uint       `gorm:"index" json:"upload_id,omitempty"`
	ConversationID *uint       `gorm:"index" json:"conversation_id,omitempty"`
	MessageID      *uint       `gorm:"index" json:"message_id,omitempty"`
	AnalysisID     *uint       `gorm:"index" json:"analysis_id,omitempty"`
	Category       string      `gorm:"type:varchar(50);not null;index" json:"category"` // business, artistic, other
	Content        string      `gorm:"type:text;not null" json:"content"`
	Source         string      `gorm:"type:varchar(50);index" json:"source"` // user_message, assistant_message, analysis
	ExtractedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP;index" json:"extracted_at"`
	Metadata       string      `gorm:"type:text" json:"metadata"` // JSON
}

// Question represents extracted questions
type Question struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UploadID       *uint       `gorm:"index" json:"upload_id,omitempty"`
	ConversationID *uint       `gorm:"index" json:"conversation_id,omitempty"`
	MessageID      *uint       `gorm:"index" json:"message_id,omitempty"`
	QuestionText   string      `gorm:"type:text;not null" json:"question_text"`
	Asker          string      `gorm:"type:varchar(50);index" json:"asker"` // user, assistant
	ExtractedAt    time.Time   `gorm:"not null;default:CURRENT_TIMESTAMP;index" json:"extracted_at"`
	Metadata       string      `gorm:"type:text" json:"metadata"` // JSON
}

// NoiseFlag tracks conversations flagged as noise/low-value
type NoiseFlag struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ConversationID uint      `gorm:"uniqueIndex;not null" json:"conversation_id"`
	IsNoise       bool       `gorm:"not null;default:false;index" json:"is_noise"`
	Confidence    *float64   `json:"confidence,omitempty"` // 0.0-1.0
	Reason        *string    `gorm:"type:text" json:"reason,omitempty"`
	FlaggedAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"flagged_at"`

	// Relationships
	Conversation Conversation `gorm:"constraint:OnDelete:CASCADE"`
}

