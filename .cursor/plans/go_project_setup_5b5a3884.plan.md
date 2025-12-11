---
name: Go Project Setup
overview: Set up a new Go project for ChatGPT Autopsy tool with Gin web framework, GORM with SQLite database, and a clean project structure supporting both Windows and macOS development.
todos: []
---

# ChatGPT Autopsy - Go Project Setup

## Overview

Go-based system for importing, organizing, and analyzing ChatGPT conversation exports. Processes ZIP exports, extracts conversations by date, and generates 9-dimensional psychological/behavioral analyses with optional AI enhancement.

**Key Features:**

- 9-dimensional analysis framework (meaning, signals, shadows, lies, truths, questionable_truths, actionable_items, doubts, topics_of_interest)
- Date-based organization (YYYY-MM-DD)
- Seen status tracking (database + localStorage)
- Optional AI enhancement (OpenAI/Anthropic)
- Aggregated analysis extraction (actionables, questions, cross-file patterns, synthesis)
- React web interface with real-time updates

## Project Structure

```
chatgpt-autopsy-go/
├── cmd/server/main.go           # Application entry point
├── internal/
│   ├── api/                     # HTTP handlers, routes, middleware
│   ├── models/                  # GORM database models
│   ├── database/                # DB connection, migrations
│   ├── services/                # Business logic (upload, extraction, analysis, etc.)
│   └── config/                  # Configuration management
├── data/
│   ├── uploads/                 # Original ZIP files
│   ├── extracted/               # Unzipped files
│   ├── messages/                # Extracted messages by date (YYYY-MM-DD.md)
│   └── analysis/                # Analysis results (YYYY-MM-DD/, cross_file_analysis/, questions/)
├── web/                         # React frontend (optional)
├── go.mod, go.sum
├── .gitignore, README.md
└── .env.example
```

## Dependencies

**Core:**

- `github.com/gin-gonic/gin` - Web framework
- `gorm.io/gorm` + `gorm.io/driver/sqlite` - ORM and SQLite driver
- `github.com/google/uuid` - UUID generation
- `go.uber.org/zap` or `github.com/sirupsen/logrus` - Structured logging
- `golang.org/x/time/rate` - Rate limiting

**Optional (AI Enhancement):**

- `github.com/sashabaranov/go-openai` - OpenAI client
- `github.com/anthropics/anthropic-sdk-go` - Anthropic client
- `github.com/pkoukk/tiktoken-go` - Token counting

**Utilities:**

- `github.com/yuin/goldmark` - Markdown processing
- Standard library: `archive/zip`, `encoding/json`, `crypto/sha256`, `context`

## Database Models

### Core Models

**Upload** - Tracks uploaded ZIP files

- Fields: id, uuid (unique), original_filename, stored_path, file_size, file_hash (SHA256, unique), mime_type, uploaded_at, status (pending|processing|completed|failed|deleted), error_message, metadata (JSON)
- Indexes: uuid, file_hash, status, uploaded_at
- Relationships: has one Import, has many Extractions/Conversations/Analysis
- Soft deletes enabled
- Note: Soft delete cascades to related records (Import, Extraction, Conversation) handled at application level since GORM doesn't cascade soft deletes automatically

**Import** - Tracks import process

- Fields: id, upload_id (unique FK), started_at, completed_at, status (pending|extracting|parsing|importing|completed|failed), progress_percent (0-100), error_message, stats (JSON: conversations_count, messages_count, threads_count, files_extracted)
- Indexes: upload_id, status, started_at
- ON DELETE CASCADE

**Conversation** - Individual conversations from export

- Fields: id, upload_id (FK), conversation_id (ChatGPT's ID, unique per upload), title, created_at, updated_at, source_file_path, message_count, metadata (JSON)
- Indexes: upload_id, conversation_id, created_at
- Unique: (upload_id, conversation_id)
- ON DELETE CASCADE

**Message** - Individual messages

- Fields: id, conversation_id (FK), message_id (ChatGPT's ID), role (user|assistant|system), content (text), timestamp, message_index, metadata (JSON)
- Indexes: conversation_id, role, timestamp, (conversation_id, message_index)
- ON DELETE CASCADE
- Consider FTS5 for full-text search

**Thread** - Date-based thread divisions

- Fields: id, conversation_id (FK), date (YYYY-MM-DD), message_count, start_message_id (FK), end_message_id (FK), start_timestamp, end_timestamp
- Indexes: conversation_id, date, (conversation_id, date), start_message_id, end_message_id
- Unique: (conversation_id, date)
- ON DELETE CASCADE
- Constraints: start_message_id and end_message_id must belong to the same conversation_id (application-level validation)
- Constraints: start_timestamp <= end_timestamp, date must match message dates (application-level validation)

**Extraction** - Tracks extracted files

- Fields: id, upload_id (FK), file_path, file_type (conversation|media|other), file_size, extracted_at, status (extracted|parsed|failed), error_message
- Indexes: upload_id, file_type, status
- ON DELETE CASCADE

### Analysis Models

**Analysis** - Analysis results

- Fields: id, upload_id/conversation_id/thread_id (nullable FKs), date (YYYY-MM-DD, nullable), analysis_type (enum: meaning|signals|shadows|lies|truths|questionable_truths|actionable_items|doubts|topics_of_interest|synthesis|summary), analysis_data (JSON), markdown_content (text), is_ai_enhanced (bool), ai_provider (openai|anthropic), created_at, updated_at, version
- Indexes: upload_id, conversation_id, thread_id, date, analysis_type, created_at, (date, analysis_type)
- Unique: (date, analysis_type) for date-based analyses
- Constraints (application-level validation required):
  - At least one FK or date must be set
  - If thread_id is set, date must match Thread.date (denormalized for query performance)
  - If conversation_id is set, date should match conversation date range (optional validation)
  - If upload_id is set without date, this is an upload-level analysis (rare, but allowed)

**SeenStatus** - Metadata: Tracks which analysis pages user has viewed

- Fields: id, date (YYYY-MM-DD, nullable), analysis_type (nullable), result_type (actionables|synthesis|cross_file|questions, nullable), result_name (nullable), seen_at, user_id (nullable, future multi-user)
- Indexes: date, analysis_type, (date, analysis_type), result_type, result_name, (result_type, result_name), seen_at
- Unique constraints: (date, analysis_type) for date-based, (result_type, result_name) for aggregated
- Constraints (application-level validation required):
  - Exactly one of these must be set: (date AND analysis_type) OR (result_type AND result_name)
  - Cannot have both date+type AND result_type+name set
- Note: This is UI state metadata, not core analysis data

**ActionableItem** - Extracted actionables

- Fields: id, upload_id/conversation_id/message_id (nullable FKs), analysis_id (nullable FK to Analysis), category (business|artistic|other), content (text), source (user_message|assistant_message|analysis), extracted_at, metadata (JSON)
- Indexes: upload_id, conversation_id, message_id, analysis_id, category, source, extracted_at
- Constraints (application-level validation):
  - If source is "analysis", analysis_id must be set
  - If source is "user_message" or "assistant_message", message_id must be set
  - At least one FK must be set (upload_id, conversation_id, message_id, or analysis_id)

**Question** - Extracted questions

- Fields: id, upload_id/conversation_id/message_id (nullable FKs), question_text (text), asker (user|assistant), extracted_at, metadata (JSON)
- Indexes: upload_id, conversation_id, message_id, asker, extracted_at
- Constraints (application-level validation):
  - At least one FK must be set (upload_id, conversation_id, or message_id)
  - If message_id is set, asker should match message.role (application-level validation)

**NoiseFlag** - Noise detection flags

- Fields: id, conversation_id (unique FK), is_noise (bool), confidence (0.0-1.0, nullable), reason (nullable), flagged_at
- Indexes: conversation_id, is_noise
- ON DELETE CASCADE

## API Endpoints

### Upload & Import

- `POST /api/v1/upload` - Upload ZIP (multipart/form-data)
- `GET /api/v1/uploads` - List uploads (paginated)
- `GET /api/v1/uploads/:id` - Get upload details
- `DELETE /api/v1/uploads/:id` - Delete upload

### Conversations & Threads

- `GET /api/v1/conversations` - List conversations (paginated, filterable)
- `GET /api/v1/conversations/:id` - Get conversation with messages
- `GET /api/v1/conversations/:id/threads` - Get threads for conversation
- `GET /api/v1/threads` - List threads (paginated, filterable)
- `GET /api/v1/threads/:id` - Get thread details

### Analysis

**Date-based analyses (per-date, per-dimension):**

- `GET /api/v1/dates` - List all analysis dates (YYYY-MM-DD)
- `GET /api/v1/analysis/:date` - Get all analysis types for a date
- `GET /api/v1/analysis/:date/:type` - Get specific analysis for a date
  - Types: meaning, signals, shadows, lies, truths, questionable_truths, actionable_items, doubts, topics_of_interest, synthesis, summary

**Aggregated analyses (across all dates):**

- `GET /api/v1/analysis/actionables/:category` - Get actionable items (category: business, artistic, other)
- `GET /api/v1/analysis/synthesis/comprehensive` - Get comprehensive synthesis
- `GET /api/v1/analysis/cross_file/:pattern` - Get cross-file analysis (pattern: topics_over_time, relationship_patterns, work_patterns, parenting_patterns, personal_development_patterns, keywords)
- `GET /api/v1/analysis/questions/:type` - Get questions (type: all, my, chatgpt, summary)
- Note: Consistent naming - use underscores for multi-word paths (cross_file not cross-file)

**General:**

- `GET /api/v1/analysis` - List analyses (paginated, filterable by date, type, upload_id, conversation_id, thread_id)
- `POST /api/v1/analysis` - Trigger analysis (body: {"scope": "date|conversation|thread|global", "target": "YYYY-MM-DD|id|...", "types": ["meaning", ...], "force": false})
  - If analysis exists and force=false, return existing (idempotent)
  - If force=true, regenerate analysis
- `PUT /api/v1/analysis/:id` - Update analysis (for AI enhancement, manual edits)
- `DELETE /api/v1/analysis/:id` - Delete specific analysis

**Seen status (metadata):**

- `GET /api/v1/analysis/:date/:type?seen=true` - Include seen status in response (query param)
- `PUT /api/v1/analysis/:date/:type/seen` - Mark as seen (body: {"seen": true|false})
- `GET /api/v1/analysis/:date?include_seen_status=true` - Get date with all seen statuses and unseen_count
- `GET /api/v1/analysis/actionables/:category?seen=true` - Include seen status for aggregated analyses
- `PUT /api/v1/analysis/actionables/:category/seen` - Mark aggregated analysis as seen
- Similar patterns for other aggregated analyses (synthesis, cross-file, questions)

### System

- `GET /api/v1/health` - Health check
- `GET /api/v1/ready` - Readiness (database + disk space)

## Services

### upload.go

- Validate file (size, type, structure)
- Generate UUID, calculate SHA256 hash
- Atomic save (temp → rename)
- Create Upload/Import records (transaction)
- Trigger async extraction (goroutine)
- Duplicate detection (file_hash)

### extraction.go

- Validate ZIP structure
- Extract with path traversal protection
- Enforce limits (max size: 2GB, max files: 10000)
- Extract to `data/extracted/{upload_uuid}/` (use upload UUID, not date)
- Update Import status/progress
- Create Extraction records with correct file_path
- Context cancellation support

### parser.go

- Parse ChatGPT export JSON
- Expected structure: Array of {title, create_time, update_time, mapping: {message-id: {id, message: {author: {role}, content: {parts}}, parent, children}}}
- Validate structure, handle malformed JSON gracefully
- Batch create Conversation/Message records (1000 per transaction)
- Extract user messages, organize by date (YYYY-MM-DD)
- Save to `data/messages/YYYY-MM-DD.md`
- Calculate token counts

### thread.go

- Group messages by date (YYYY-MM-DD) based on message timestamps
- UTC normalization (convert all timestamps to UTC before grouping)
- Validate that start_message_id and end_message_id belong to the same conversation
- Validate that Thread.date matches the actual message dates in the thread
- Create Thread records with message ranges
- Handle edge cases (midnight boundaries, missing timestamps, timezone differences)
- Batch creation for performance (1000 threads per transaction)
- Update Import.progress_percent during processing

### analysis.go

- **Dependency check:** Verify Thread records exist for the date before analyzing (or analyze conversation-level if threads not created)
- Generate 9-dimensional analyses per date:

  1. Meaning - Core themes, values, beliefs
  2. Signals - Behavioral patterns, communication styles
  3. Shadows - Unconscious patterns, defense mechanisms
  4. Lies - Self-deceptions, rationalizations
  5. Truths - Authentic expressions, validated experiences
  6. Questionable Truths - Beliefs requiring examination
  7. Actionable Items - Concrete growth steps
  8. Doubts - Uncertainties, unresolved questions
  9. Topics of Interest - Recurring themes, passions

- Generate synthesis.md and summary.md per date
- Store in Analysis model with date and thread_id (if available) for consistency
- Validate that Analysis.date matches Thread.date when thread_id is set
- Save markdown files to `data/analysis/YYYY-MM-DD/`
- Async processing, batch generation (100 analyses per transaction)
- Idempotent: Check if analysis exists before generating (unless force=true)

### ai_enhancement.go

- Enhance template analyses with AI (Anthropic preferred, OpenAI fallback)
- Handle rate limits, errors gracefully
- Context cancellation support
- Update Analysis records with enhanced content
- Track AI provider, token counting

### noise_detection.go

- AI-assisted filtering of low-value conversations
- Analyze signal-to-noise ratio
- Flag based on configurable threshold (default: 0.3)
- Create NoiseFlag records
- Manual override support

### actionable_extraction.go

- Extract from user messages (patterns: "I should/want/need to...")
- Extract from assistant messages (recommendations)
- Extract from analysis results (link via analysis_id FK)
- Categorize: business, artistic, other
- Create ActionableItem records with proper FK relationships
- Validate: If source is "analysis", set analysis_id; if source is message, set message_id
- Generate: BUSINESS_ACTIONABLES.md, ARTISTIC_ACTIONABLES.md, OTHER_ACTIONABLES.md

### question_extraction.go

- Extract all questions from conversations
- Separate user vs assistant questions
- Create Question records
- Generate: ALL_QUESTIONS.md, MY_QUESTIONS.md, CHATGPT_QUESTIONS.md, QUESTIONS_SUMMARY.md

### cross_file_analysis.go

- Analyze patterns across all dates
- Generate reports: topics_over_time.md, relationship_patterns.md, work_patterns.md, parenting_patterns.md, personal_development_patterns.md, keywords.md
- Store in `data/analysis/cross_file_analysis/`

### synthesis.go

- Generate comprehensive synthesis across all dates
- Create COMPREHENSIVE_SYNTHESIS.md
- Aggregate insights from all 9 dimensions
- Temporal analysis (theme evolution)

## Configuration

**Environment Variables:**

- **With prefix `CHATGPT_AUTOPSY_`:** PORT (8080), HOST (0.0.0.0), READ_TIMEOUT (30s), WRITE_TIMEOUT (30s), IDLE_TIMEOUT (120s), DB_PATH (data/chatgpt_autopsy.db), MAX_OPEN_CONNS (1), MAX_IDLE_CONNS (1), CONN_MAX_LIFETIME (1h), MAX_FILE_SIZE (500MB), MAX_EXTRACTION_SIZE (2GB), MAX_EXTRACTED_FILES (10000), UPLOADS_DIR (data/uploads), EXTRACTED_DIR (data/extracted), ANALYSIS_DIR (data/analysis), MESSAGES_DIR (data/messages), PREFERRED_AI_PROVIDER (anthropic), AI_ENHANCEMENT_ENABLED (false), MAX_TOKENS_PER_REQUEST (4000), AI_TEMPERATURE (0.7), ENABLE_NOISE_DETECTION (true), NOISE_DETECTION_THRESHOLD (0.3), REQUESTS_PER_MINUTE (100), BURST_SIZE (10), LOG_LEVEL (info), LOG_FORMAT (json), LOG_OUTPUT (stdout)
- **Without prefix (standard names):** OPENAI_API_KEY, ANTHROPIC_API_KEY (these are standard API key names, no prefix needed)
- Note: MAX_EXTRACTION_SIZE (2GB) > MAX_FILE_SIZE (500MB) is intentional - ZIP extraction can expand significantly

**Validation:**

- Port: 1024-65535
- File sizes: positive integers
- Directories: valid, writable
- Database path: valid, parent exists

**Optional:** YAML/JSON config file (env vars override file)

## Data Storage

### Directory Structure

- `data/uploads/` - Original ZIP files (UUID-based naming: {uuid}.zip, no date subdirs to avoid confusion)
- `data/extracted/{upload_uuid}/` - Extracted files per upload (use upload UUID, not date, to avoid confusion with message dates)
- `data/messages/YYYY-MM-DD.md` - Extracted user messages by message date (YYYY-MM-DD from message timestamps)
- `data/analysis/YYYY-MM-DD/` - Analysis results per date (meaning.md, signals.md, etc.) - date matches Thread.date
- `data/analysis/cross_file_analysis/` - Cross-file patterns
- `data/analysis/questions/` - Extracted questions
- Note: Extracted directory uses upload UUID to distinguish from message dates; analysis directory uses message/thread dates

### File Operations

- Naming: UUID v4 format (`{uuid}.zip`)
- Paths: Store relative in DB, resolve absolute at runtime
- Atomic writes: temp file → rename
- File locking: OS-level locks
- Cleanup on failure: Remove partial files
- Disk space: Warn if < 1GB free, reject if < 100MB free

### Permissions

- Windows: 0666 (files), 0777 (dirs) - Windows ignores but needed for Mac
- Unix/Mac: 0644 (files), 0755 (dirs)

## Processing Pipeline

1. **Upload** - Validate ZIP, save with UUID to `data/uploads/{uuid}.zip`, calculate SHA256, create Upload/Import records, status: pending
2. **Extract** - Unzip to `data/extracted/{upload_uuid}/` with path traversal protection, enforce limits, create Extraction records, status: pending → extracting → parsing
3. **Parse** - Parse JSON, extract conversations/messages, organize by message date (YYYY-MM-DD), save to `data/messages/YYYY-MM-DD.md`, batch create Conversation/Message records (1000 per transaction), status: parsing → importing
4. **Thread** - Group messages by date (from timestamps), validate message relationships, create Thread records with proper FKs, UTC normalization, status: importing → completed
5. **Analyze** (async, can trigger separately, requires Thread records) - Verify Thread records exist for date, generate 9-dimensional analyses per date, optional AI enhancement, create Analysis records with date+thread_id, save markdown files to `data/analysis/YYYY-MM-DD/`
6. **Extract** (async, can trigger separately, requires Analysis records) - Extract actionables/questions from messages and analyses, create ActionableItem/Question records with proper FKs, generate global files
7. **Cross-Analyze** (async, can trigger separately, requires all dates analyzed) - Verify all dates have analyses, analyze patterns across dates, generate reports to `data/analysis/cross_file_analysis/`
8. **Synthesize** (async, can trigger separately, requires all dates analyzed) - Verify all dates have analyses, generate comprehensive synthesis, create COMPREHENSIVE_SYNTHESIS.md

**Progress Tracking:** Import.progress_percent (0-100), Import.status, Import.stats (JSON with counts)

**Error Handling:** Partial failures skip invalid entries, failed imports can retry, transactions ensure consistency

**Dependencies:**

- Analysis requires Thread records (or can work at conversation level)
- Cross-file analysis and synthesis require all dates to have analyses (validation required)
- Actionable/question extraction can work from messages alone, but analysis extraction requires Analysis records

**Validation Points:**

- Thread: Verify start_message_id and end_message_id belong to same conversation
- Thread: Verify Thread.date matches message dates
- Analysis: Verify Thread.date matches Analysis.date when thread_id is set
- ActionableItem: Verify analysis_id is set when source is "analysis"
- Question: Verify asker matches message.role when message_id is set

## Main Application (cmd/server/main.go)

- Initialize structured logger (zap/logrus)
- Gin router (release mode)
- Middleware: CORS (configurable origins), request timeout (30s uploads, 10s others), rate limiting (100 req/min), request size limit (100MB), request logging, recovery, request ID
- Connect SQLite (WAL mode, retry 3x with backoff)
- Register routes (`/api/v1/*`)
- Health/ready endpoints
- Serve static files
- Graceful shutdown (SIGINT/SIGTERM, 30s timeout)

## Database Layer (internal/database/db.go)

- GORM + SQLite (WAL mode)
- Connection pool: MaxOpenConns=1, MaxIdleConns=1, ConnMaxLifetime=1h
- Foreign keys: `PRAGMA foreign_keys = ON`
- Auto-migration with version tracking (gormigrate or custom)
- Migration rollback support
- Retry logic (3 retries, exponential backoff)
- Initialize directories: data/uploads, data/extracted, data/analysis, data/messages
- Verify write permissions
- Database file: absolute path resolution

## API Layer (internal/api/)

### handlers.go

- File upload validation: Content-Type, extension, size, ZIP structure, path traversal prevention, ZIP bomb protection
- Request validation middleware (JSON schemas)
- Pagination helpers (page, limit, offset)
- Sorting/filtering helpers
- JSON response helpers (consistent error format: `{error: {code, message, details}, request_id}`)
- Error handling middleware (proper HTTP status codes)

### middleware.go

- Request logging (method, path, status, duration, IP)
- Recovery (panic recovery with logging)
- Authentication (placeholder)
- Request validation

## Error Handling

**Error Types:** ErrInvalidFile, ErrExtractionFailed, ErrDatabaseError, ErrNotFound, ErrDuplicateUpload

**Wrapping:** `fmt.Errorf` with `%w`

**Logging:** Context (request ID, operation, stack trace for panics)

**Responses:** User-friendly messages (no internal details)

**Retry:** Transient failures (DB locks, filesystem errors) with exponential backoff

## Logging

- Structured logging (zap/logrus, JSON format)
- Levels: Debug, Info, Warn, Error
- Fields: request_id, operation, duration, error details
- Never log: file contents, passwords, full paths (sanitize)
- Optional: Log rotation, async logging

## Security

- **File Upload:** Validate by content (not extension), scan for path traversal/symlinks, limit extraction depth
- **Path Traversal:** Sanitize paths, use `filepath.Clean()`, validate against base dir, reject `..` and absolute paths
- **Input Validation:** JSON schemas, query params, sanitize strings, validate UUIDs/IDs
- **SQL Injection:** GORM parameterized queries (validate inputs)
- **Rate Limiting:** Per-IP limiting
- **CORS:** Restrict to specific origins (not `*` in production)

## Performance

- **Indexing:** All FKs and frequently queried fields indexed
- **Batch Operations:** 1000 messages per transaction, 1000 conversations per transaction, 1000 threads per transaction, 100 analyses per transaction
- **Pagination:** All list endpoints (default: 50, max: 500)
- **Async Processing:** Long operations in goroutines
- **Connection Pooling:** SQLite configured (limited to 1 connection due to single-writer limitation)
- **Retry Strategy:** Exponential backoff for database locked errors (max 3 retries, 100ms initial delay)
- **Full-Text Search:** Consider FTS5 for message content

## SQLite Considerations

- **Concurrency:** WAL mode, retry on database locked errors, connection pool size 1
- **File Locking:** Handle gracefully
- **Transactions:** Keep small to avoid long locks
- **Future:** Periodic VACUUM, backup API

## Cross-Platform

- **Paths:** Use `path/filepath`, normalize (forward slashes in DB, OS-specific in FS), handle Windows 260 char limit
- **Permissions:** Windows (0666/0777), Unix/Mac (0644/0755)
- **Line Endings:** Handle CRLF/LF, normalize to LF
- **Environment Variables:** Support Windows (`%VAR%`) and Unix (`$VAR`) syntax in docs

## Analysis Framework

### 9 Dimensions

1. Meaning - Core themes, values, beliefs
2. Signals - Behavioral patterns, communication styles
3. Shadows - Unconscious patterns, defense mechanisms
4. Lies - Self-deceptions, rationalizations
5. Truths - Authentic expressions, validated experiences
6. Questionable Truths - Beliefs requiring examination
7. Actionable Items - Concrete growth steps
8. Doubts - Uncertainties, unresolved questions
9. Topics of Interest - Recurring themes, passions

**Additional:** Synthesis (integrated view), Summary (quick overview)

### Output Structure

```
data/analysis/YYYY-MM-DD/
├── meaning.md
├── signals.md
├── shadows.md
├── lies.md
├── truths.md
├── questionable_truths.md
├── actionable_items.md
├── doubts.md
├── topics_of_interest.md
├── synthesis.md
└── summary.md

data/analysis/
├── BUSINESS_ACTIONABLES.md
├── ARTISTIC_ACTIONABLES.md
├── OTHER_ACTIONABLES.md
├── COMPREHENSIVE_SYNTHESIS.md
├── cross_file_analysis/
│   ├── topics_over_time.md
│   ├── relationship_patterns.md
│   ├── work_patterns.md
│   ├── parenting_patterns.md
│   ├── personal_development_patterns.md
│   └── keywords.md
└── questions/
    ├── ALL_QUESTIONS.md
    ├── MY_QUESTIONS.md
    ├── CHATGPT_QUESTIONS.md
    └── QUESTIONS_SUMMARY.md
```

## Web Interface (Optional)

**React Frontend:**

- Date navigation with unseen count badges
- Analysis type selector with seen status
- Analysis viewer with markdown rendering
- Aggregated analysis view (actionables, synthesis, cross-file, questions)
- Seen status tracking (red borders unseen, green seen)
- Real-time updates
- localStorage fallback

## Seen Status System (UI Metadata)

**Purpose:** Track viewed analysis pages (UI state, not core data)

**Storage:** SQLite (primary) + localStorage (fallback/redundancy)

**Features:** Auto-tracking, persistent storage, per-date unseen counts, per-type indicators, aggregated analysis tracking

**API:** Exposed as query parameters or sub-resources of analysis endpoints, not separate top-level endpoints

## AI Enhancement

**Providers:** Anthropic Claude (preferred), OpenAI GPT-4o (fallback)

**Features:** Optional enhancement, configurable via env vars, graceful degradation, token counting, rate limit handling

**Config:** Set API keys, enable via `AI_ENHANCEMENT_ENABLED=true`, prefer Anthropic

## Export Processing

**Auto-Detection:** Check `exports/conversations.json` (primary), fallback to root `conversations.json`, support ZIP uploads via API

## Disclaimers & Safety

**AI Analysis Disclaimers (CRITICAL):**

- NOT a substitute for professional mental health care
- AI limitations apply (errors, biases possible)
- No accuracy guarantees (experimental)
- Use at your own risk

**Privacy & Security:**

- Sensitive data processing (personal conversations)
- No encryption by default
- Third-party APIs (OpenAI/Anthropic) - review privacy policies
- Local storage (data on user's machine)
- No access controls (file system permissions only)
- Secure backups required

**Implementation:** Display disclaimers in UI (header/footer), include in API responses, create AI_DISCLAIMERS.md

## Limitations & Trade-offs

1. **SQLite Concurrency:** Single writer limit → WAL mode, retry logic, consider PostgreSQL for scale
2. **File Storage:** Local only → Future: S3/cloud adapter
3. **No Authentication:** Initial version → Future: JWT auth
4. **Synchronous Processing:** Blocks request (goroutine) → Future: Job queue (Redis)
5. **Single Instance:** No distributed deployment → Future: Multi-instance support
6. **No Real-time Updates:** Polling required → Future: WebSocket support
7. **AI Dependency:** Requires API keys/internet → Works without AI (template-only), graceful degradation

## Migration Strategy

- **Initial:** GORM AutoMigrate
- **Future:** Migration system (gormigrate or custom)
- **Data Migration:** Support for schema changes
- **Backward Compatibility:** Version analysis_data JSON structure

## Project Files

- **.gitignore:** Standard Go ignores, IDE files, database files (*.db, *.db-shm, *.db-wal), data/ directory, logs, .env files, build artifacts
- **README.md:** Project description, prerequisites, installation, configuration, API docs, development setup, troubleshooting
- **.env.example:** Environment variable template
- **AI_DISCLAIMERS.md:** Comprehensive AI disclaimers
- **GETTING_STARTED.md:** Onboarding guide
- **ARCHITECTURE.md:** System architecture and design decisions
- **Makefile** (optional): Common commands
- **docker-compose.yml** (optional): Containerized development

## Implementation Phases

**Phase 1: Core Infrastructure**

- ZIP parsing/extraction
- ChatGPT JSON parser
- Database models/migrations
- Basic API endpoints

**Phase 2: Analysis Framework**

- 9-dimensional analysis templates
- Date-based thread division
- Message extraction/organization
- Analysis result storage

**Phase 3: AI Integration**

- AI enhancement service
- Noise detection
- API client integration

**Phase 4: Extraction Services**

- Actionable items extraction
- Questions extraction
- Cross-file analysis
- Comprehensive synthesis

**Phase 5: Web Interface**

- React frontend setup
- Seen status tracking
- API integration
- Markdown rendering

**Phase 6: Polish**

- Error handling improvements
- Performance optimization
- Documentation
- Testing