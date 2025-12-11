# ChatGPT Autopsy

A Go-based system for importing, organizing, and analyzing ChatGPT conversation exports. Processes ZIP exports, extracts conversations by date, and generates 9-dimensional psychological/behavioral analyses with optional AI enhancement.

## Features

- **9-Dimensional Analysis Framework** - Comprehensive analysis across meaning, signals, shadows, lies, truths, questionable truths, actionable items, doubts, and topics of interest
- **Date-based Organization** - Messages and analyses organized by date (YYYY-MM-DD)
- **Seen Status Tracking** - Database-backed tracking of viewed analysis pages
- **Optional AI Enhancement** - OpenAI/Anthropic integration for enhanced analyses
- **Aggregated Analysis** - Actionables, questions, cross-file patterns, and synthesis
- **RESTful API** - Full REST API for all operations

## Prerequisites

- Go 1.21 or higher
- SQLite (included with Go SQLite driver)
- Disk space for data storage (configurable)

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd chatgpt-autopsy-go
```

2. Install dependencies:
```bash
go mod download
```

3. Configure environment variables (optional):
```bash
cp .env.example .env
# Edit .env with your configuration
```

## Configuration

Configuration is done via environment variables with the prefix `CHATGPT_AUTOPSY_`:

### Server Configuration
- `CHATGPT_AUTOPSY_PORT` - Server port (default: 8080)
- `CHATGPT_AUTOPSY_HOST` - Server host (default: 0.0.0.0)

### Database Configuration
- `CHATGPT_AUTOPSY_DB_PATH` - Database file path (default: data/chatgpt_autopsy.db)

### Upload Configuration
- `CHATGPT_AUTOPSY_MAX_FILE_SIZE` - Maximum upload file size in bytes (default: 500MB)
- `CHATGPT_AUTOPSY_MAX_EXTRACTION_SIZE` - Maximum extraction size (default: 2GB)

### AI Enhancement (Optional)
- `OPENAI_API_KEY` - OpenAI API key
- `ANTHROPIC_API_KEY` - Anthropic API key
- `CHATGPT_AUTOPSY_AI_ENHANCEMENT_ENABLED` - Enable AI enhancement (default: false)

See `.env.example` for all available configuration options.

## Usage

### Starting the Server

```bash
go run cmd/server/main.go
```

The server will start on `http://localhost:8080` by default.

### API Endpoints

#### Upload
- `POST /api/v1/upload` - Upload ChatGPT export ZIP file
- `GET /api/v1/uploads` - List all uploads
- `GET /api/v1/uploads/:id` - Get upload details
- `DELETE /api/v1/uploads/:id` - Delete upload

#### Conversations
- `GET /api/v1/conversations` - List conversations
- `GET /api/v1/conversations/:id` - Get conversation with messages

#### Analysis
- `GET /api/v1/dates` - List all analysis dates
- `GET /api/v1/analysis/:date/:type` - Get analysis for a date and type

#### System
- `GET /api/v1/health` - Health check
- `GET /api/v1/ready` - Readiness check

### Example: Upload and Process

```bash
# Upload a ChatGPT export ZIP file
curl -X POST http://localhost:8080/api/v1/upload \
  -F "file=@chatgpt_export.zip"

# List uploads
curl http://localhost:8080/api/v1/uploads

# Get analysis dates
curl http://localhost:8080/api/v1/dates

# Get analysis for a specific date
curl http://localhost:8080/api/v1/analysis/2024-01-15/meaning
```

## Project Structure

```
chatgpt-autopsy-go/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/             # HTTP handlers, routes, middleware
│   ├── models/           # Database models
│   ├── database/         # Database connection and migrations
│   ├── services/         # Business logic
│   └── config/           # Configuration management
├── data/                 # Data storage (uploads, extracted files, analysis)
└── web/                  # Web frontend (optional)
```

## Processing Pipeline

1. **Upload** - User uploads ChatGPT export ZIP file
2. **Extract** - ZIP file is extracted with security validation
3. **Parse** - ChatGPT JSON is parsed, conversations and messages extracted
4. **Thread** - Messages are grouped by date into threads
5. **Analyze** - 9-dimensional analyses are generated per date
6. **Extract** - Actionables and questions are extracted
7. **Cross-Analyze** - Patterns across dates are analyzed
8. **Synthesize** - Comprehensive synthesis is generated

## Development

### Running Tests
```bash
go test ./...
```

### Building
```bash
go build -o bin/server cmd/server/main.go
```

## Limitations

- SQLite database (single writer limitation)
- Local file storage only
- No authentication in initial version
- Polling required for status updates (no WebSocket)

## License

[Your License Here]

## Support

For issues and questions, please open an issue on GitHub.

