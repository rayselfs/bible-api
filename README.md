# Bible API

A modern, RESTful Bible API built with Go. This API provides access to Bible content across multiple versions with semantic search capabilities powered by pgvector and OpenAI embeddings.

## 🚀 Features

- **RESTful API**: Clean, well-documented REST endpoints
- **Multiple Bible Versions**: Support for CUV-TW, NIV, NKJV, KJV and more
- **AI-Powered Search**: Semantic search using pgvector and OpenAI embeddings
- **Hybrid Search**: Combines keyword (full-text) and vector (semantic) search using RRF (Reciprocal Rank Fusion)
- **Vector Embeddings**: Automatic embedding generation during import using OpenAI/Azure OpenAI
- **Data Import**: Import Bible data from JSON format with automatic embedding generation
- **Swagger Documentation**: Interactive API documentation
- **Database Migrations**: Automated database schema management with pgvector support
- **Docker Support**: Containerized deployment with PostgreSQL + pgvector
- **Health Checks**: Built-in health monitoring
- **Environment Configuration**: Flexible configuration management

## 📋 API Endpoints

### Health Check

- `GET /health` - Service health status

### Bible Versions

- `GET /api/bible/v1/versions` - Get all available Bible versions
- `GET /api/bible/v1/version/{version_id}` - Get complete Bible content by version ID

### Bible Search

- `GET /api/bible/v1/search` - Search Bible verses using semantic similarity

### Documentation

- `GET /swagger/*` - Interactive Swagger documentation

## 🛠️ Technology Stack

- **Language**: Go 1.24+
- **Framework**: Gin (HTTP web framework)
- **Database**: PostgreSQL 17 with pgvector extension
- **ORM**: GORM with custom logger
- **Vector Search**: pgvector for semantic similarity search
- **AI Embeddings**: Azure OpenAI / OpenAI API (text-embedding-3-small)
- **Hybrid Search**: RRF (Reciprocal Rank Fusion) combining vector and keyword search
- **Documentation**: Swagger/OpenAPI 3.0
- **Migration**: Gormigrate
- **Containerization**: Docker & Docker Compose

## 📦 Project Structure

```
bible-api/
├── cmd/
│   └── main.go              # Application entry point (server + import)
├── configs/
│   └── configs.go           # Configuration management
├── data/                     # Bible data files (JSON format)
│   ├── bible.json           # CUV-TW (Traditional Chinese)
│   ├── bible_simplified.json # CUV-SC (Simplified Chinese)
│   ├── bible_niv.json       # NIV (New International Version)
│   ├── bible_nkjv.json      # NKJV (New King James Version)
│   └── bible_kjv.json       # KJV (King James Version)
├── scripts/                  # Data fetching scripts
│   ├── fetch_bst_bible.py   # Scrape BibleStudyTools.com
│   ├── fetch_kjv_txt.py     # Fetch KJV from OpenBible.com
│   └── convert_to_simplified.py # Convert traditional to simplified
├── docs/                     # Swagger documentation
├── internal/
│   ├── import/              # Bible data import logic
│   │   └── import.go        # Import with embedding generation
│   ├── models/               # Database models and stores
│   │   ├── models.go        # GORM models
│   │   └── stores.go        # Database operations (hybrid search)
│   ├── pkg/
│   │   └── openai/          # OpenAI embedding service
│   │       └── openai.go    # Embedding generation
│   ├── logger/              # Structured logging
│   └── server/               # HTTP handlers and routes
├── migrations/               # Database migrations (with pgvector)
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 🚀 Quick Start

### Prerequisites

- Go 1.24 or higher
- PostgreSQL 17+ with pgvector extension (or use Docker Compose)
- Azure OpenAI / OpenAI API key (for embeddings)
- Docker (optional, recommended)

### Installation

1. **Clone the repository**

   ```bash
   git clone <repository-url>
   cd bible-api
   ```

2. **Install dependencies**

   ```bash
   go mod download
   ```

3. **Set up the database**

   ```bash
   # Using Docker Compose (recommended - includes pgvector)
   docker-compose up -d postgres

   # Or manually setup PostgreSQL with pgvector
   # Install pgvector extension: https://github.com/pgvector/pgvector
   psql -U postgres -c "CREATE DATABASE bible;"
   psql -U postgres -d bible -c "CREATE EXTENSION IF NOT EXISTS vector;"
   ```

4. **Configure environment variables**

   ```bash
   # Database configuration
   export POSTGRES_HOST=localhost
   export POSTGRES_PORT=5432
   export POSTGRES_USER=bible
   export POSTGRES_PASS=bible
   export POSTGRES_DB=bible
   export POSTGRES_SSLMODE=disable  # disable, require, verify-full, etc.
   export SERVER_PORT=9999

   # Azure OpenAI configuration (required for embeddings)
   export AZURE_OPENAI_BASE_URL="https://your-service.openai.azure.com/openai/v1/"
   export AZURE_OPENAI_KEY="your-api-key"
   export AZURE_OPENAI_MODEL_NAME="text-embedding-3-small"
   ```

   Or use the example env file:
   ```bash
   cp env.sh.example env.sh
   # Edit env.sh with your credentials
   source env.sh
   ```

5. **Import Bible data**

   ```bash
   # Import a Bible version (generates embeddings automatically)
   go run cmd/main.go import ./data/bible.json
   go run cmd/main.go import ./data/bible_niv.json
   go run cmd/main.go import ./data/bible_kjv.json
   ```

6. **Run the application**

   ```bash
   go run cmd/main.go
   ```

6. **Access the API**
   - API: http://localhost:9999
   - Swagger UI: http://localhost:9999/swagger/index.html
   - Health Check: http://localhost:9999/health

## 🐳 Docker Deployment

### Using Docker Compose (Recommended)

The `docker-compose.yml` includes PostgreSQL with pgvector extension pre-installed:

```bash
# Start PostgreSQL with pgvector
docker-compose up -d postgres

# Wait for database to be ready, then import data
go run cmd/main.go import ./data/bible.json

# Run the API server
go run cmd/main.go
```

Or run everything in Docker:

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

### Manual Docker Build

```bash
# Build the image
docker build -t bible-api .

# Run the container (ensure PostgreSQL with pgvector is accessible)
docker run -p 9999:9999 \
  -e POSTGRES_HOST=host.docker.internal \
  -e POSTGRES_USER=bible \
  -e POSTGRES_PASS=bible \
  -e POSTGRES_DB=bible \
  -e POSTGRES_SSLMODE=disable \
  -e AZURE_OPENAI_BASE_URL="https://your-service.openai.azure.com/openai/v1/" \
  -e AZURE_OPENAI_KEY="your-api-key" \
  -e AZURE_OPENAI_MODEL_NAME="text-embedding-3-small" \
  bible-api
```

## ⚙️ Configuration

The application uses environment variables for configuration:

| Variable                    | Description                        | Default                |
| --------------------------- | ---------------------------------- | ---------------------- |
| `POSTGRES_HOST`             | PostgreSQL server host             | `localhost`            |
| `POSTGRES_PORT`             | PostgreSQL server port             | `5432`                 |
| `POSTGRES_USER`             | PostgreSQL username                | `bible`                |
| `POSTGRES_PASS`             | PostgreSQL password                | `bible`                |
| `POSTGRES_DB`               | PostgreSQL database name           | `bible`                |
| `POSTGRES_SSLMODE`          | PostgreSQL SSL mode                | `disable`              |
| `SERVER_PORT`               | Server port                        | `9999`                 |
| `AZURE_OPENAI_BASE_URL`     | Azure OpenAI base URL              | Required for embedding |
| `AZURE_OPENAI_KEY`          | Azure OpenAI API key                | Required for embedding |
| `AZURE_OPENAI_MODEL_NAME`   | Azure OpenAI model name            | Required for embedding |

**PostgreSQL SSL Mode Options:**
- `disable`: No SSL (default, for local development)
- `require`: Require SSL connection
- `verify-ca`: Require SSL and verify CA
- `verify-full`: Require SSL, verify CA and hostname

**Note**: The database must have the `pgvector` extension installed. This is automatically handled when using Docker Compose.

## 📖 API Usage Examples

### Get All Bible Versions

```bash
curl -X GET "http://localhost:9999/api/bible/v1/versions"
```

### Get Bible Content by Version

```bash
curl -X GET "http://localhost:9999/api/bible/v1/version/1"
```

### Search Bible Verses

```bash
# Search in Chinese (CUV-TW)
curl -X GET "http://localhost:9999/api/bible/v1/search?q=愛&version=CUV-TW&top=10"

# Search in English (NIV)
curl -X GET "http://localhost:9999/api/bible/v1/search?q=love&version=NIV&top=10"

# Search in KJV
curl -X GET "http://localhost:9999/api/bible/v1/search?q=faith&version=KJV&top=5"
```

#### Search API Request Format

```
GET /api/bible/v1/search?q=搜尋關鍵字&version=版本代碼&top=結果數量
```

**Parameters:**

- `q` (required): Search query string
- `version` (required): Bible version code (e.g., `CUV-TW`, `NIV`, `NKJV`, `KJV`)
- `top` (optional): Maximum number of results (default: 10)

#### Search API Response Format

```json
{
  "query": "love",
  "results": [
    {
      "verse_id": 123,
      "version_code": "NIV",
      "book_name": "John",
      "book_number": 43,
      "chapter_number": 3,
      "verse_number": 16,
      "text": "For God so loved the world that he gave his one and only Son...",
      "score": 0.95
    }
  ],
  "total": 1
}
```

**Note**: The search uses hybrid search combining:
- **Vector Search**: Semantic similarity using pgvector cosine distance
- **Keyword Search**: Full-text search using PostgreSQL
- **RRF (Reciprocal Rank Fusion)**: Combines both results for optimal ranking

## 🔍 Hybrid Search Implementation

The search API uses PostgreSQL with pgvector for hybrid search capabilities:

### Features

- **Vector Search**: Semantic similarity search using pgvector (cosine distance)
- **Keyword Search**: Full-text search using PostgreSQL text search
- **Hybrid Search**: Combines both using RRF (Reciprocal Rank Fusion) for optimal results
- **Version Filtering**: Filter results by Bible version
- **Relevance Scoring**: Results include combined similarity scores

### How It Works

1. **Query Embedding**: The search query is converted to a vector using OpenAI/Azure OpenAI
2. **Vector Search**: Find similar verses using cosine distance in pgvector
3. **Keyword Search**: Find verses matching keywords using PostgreSQL full-text search
4. **RRF Fusion**: Combine both result sets using Reciprocal Rank Fusion algorithm
5. **Ranking**: Return top results sorted by combined score

### Database Schema

The `bible_vectors` table stores embeddings:
- `verse_id`: Foreign key to verses table
- `embedding`: Vector of 1536 dimensions (text-embedding-3-small)

### Current Implementation

- ✅ Hybrid search (vector + keyword) using pgvector
- ✅ OpenAI/Azure OpenAI embedding integration
- ✅ Automatic embedding generation during import
- ✅ Version filtering (required parameter)
- ✅ RRF-based result fusion and ranking
- ✅ RESTful GET API with query parameters

## 💰 Cost Estimation

### Search API Costs

- **OpenAI text-embedding-3-small**: $0.00002 per 1,000 tokens (~$0.00003 per search query)
- **PostgreSQL + pgvector**: Self-hosted (no additional cost) or managed database costs
- **Estimated cost per search**: ~$0.00003 (embedding generation only)

### Cost Breakdown

| Usage Level | Monthly Searches | OpenAI Cost | Database Hosting | Total/Month |
| ----------- | ---------------- | ----------- | ---------------- | ----------- |
| Light       | 1,000            | $0.03       | $0-20 (self-hosted/cloud) | $0.03-20.03 |
| Medium      | 10,000           | $0.30       | $0-20 (self-hosted/cloud) | $0.30-20.30 |
| Heavy       | 100,000          | $3.00       | $0-50 (self-hosted/cloud) | $3.00-53.00 |

### Import Costs

- **Embedding generation during import**: ~$0.00002 per verse
- **Full Bible (31,000 verses)**: ~$0.62 per version
- **Multiple versions**: Linear scaling

_Note: Using self-hosted PostgreSQL eliminates monthly service fees. Only OpenAI embedding costs apply._

## 📚 Data Collection Scripts

The `scripts/` directory contains Python scripts for fetching Bible data from various sources:

### Available Scripts

- **`fetch_bst_bible.py`**: Scrape Bible versions from BibleStudyTools.com
  - Supports NIV, NKJV and other versions
  - Includes retry mechanism and chapter-level retry support
  - Usage: `python3 scripts/fetch_bst_bible.py -o bible_niv.json -t niv`

- **`fetch_kjv_txt.py`**: Fetch KJV from OpenBible.com text file
  - Direct text file parsing (no web scraping)
  - Automatically removes `[ ]` brackets from KJV text
  - Usage: `python3 scripts/fetch_kjv_txt.py -o bible_kjv.json`

- **`convert_to_simplified.py`**: Convert Traditional Chinese to Simplified
  - Converts CUV-TW to CUV-SC
  - Usage: `python3 scripts/convert_to_simplified.py input.json output.json`

### Script Features

- Automatic retry on failures
- Progress logging to files
- Chapter-level retry and merge support
- JSON format validation

## 📥 Data Import

### Import Bible Data

The application supports importing Bible data from JSON format with automatic embedding generation:

```bash
# Import a Bible version
go run cmd/main.go import ./data/bible.json

# Import multiple versions
go run cmd/main.go import ./data/bible_niv.json
go run cmd/main.go import ./data/bible_kjv.json
go run cmd/main.go import ./data/bible_nkjv.json
```

### JSON Format

The import expects JSON files in the following format:

```json
{
  "version": {
    "code": "CUV-TW",
    "name": "和合本（繁體）"
  },
  "books": [
    {
      "number": 1,
      "name": "創世記",
      "abbreviation": "創",
      "chapters": [
        {
          "number": 1,
          "verses": [
            {
              "number": 1,
              "text": "起初，神創造天地。"
            }
          ]
        }
      ]
    }
  ]
}
```

### Import Process

1. Creates or finds the Bible version
2. Imports books, chapters, and verses
3. Generates embeddings for each verse using OpenAI/Azure OpenAI
4. Stores embeddings in `bible_vectors` table
5. Shows progress and statistics

**Note**: Import can take time due to embedding generation. Progress is shown during import.

## 🔄 Database Migrations

The application automatically runs database migrations on startup. Migrations include:
- Initial schema creation
- pgvector extension setup
- Hybrid search indexes
- Synonyms table for search optimization

To create new migrations:

```bash
# Create a new migration file
touch migrations/$(date +%Y%m%d%H%M%S)_migration_name.go
```

## 🧪 Development

### Code Generation

Generate Swagger documentation:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init --dir cmd
```

### Running Tests

```bash
go test ./...
```

### Testing the Search API

```bash
# Test basic search (Chinese)
curl -X GET "http://localhost:9999/api/bible/v1/search?q=愛&version=CUV-TW&top=5"

# Test with different version (English)
curl -X GET "http://localhost:9999/api/bible/v1/search?q=love&version=NIV&top=3"

# Test semantic search
curl -X GET "http://localhost:9999/api/bible/v1/search?q=faith%20and%20hope&version=KJV&top=10"

# Test error cases
curl -X GET "http://localhost:9999/api/bible/v1/search?q=愛"  # Missing version
curl -X GET "http://localhost:9999/api/bible/v1/search?version=CUV-TW"  # Missing query
```

### Code Formatting

```bash
go fmt ./...
go vet ./...
```

## 📊 Monitoring & Logging

### Structured Logging

The API uses structured JSON logging format for better monitoring and debugging:

```json
{
  "timestamp": "2024-01-15 10:30:45",
  "level": "INFO",
  "message": "Starting Bible API Service..."
}
```

### Log Levels

- **INFO**: General application flow and successful operations
- **WARN**: Warning messages for non-critical issues
- **ERROR**: Error messages for failed operations
- **DEBUG**: Detailed debugging information (when enabled)

### Health Checks

The API provides health check endpoints for monitoring:

- **Health Status**: `GET /health`
- **Response**: `{"status": "UP"}`

## 🔒 Security Considerations

- Environment variables for sensitive configuration
- Input validation on all endpoints
- SQL injection protection via GORM
- Rate limiting (recommended for production)

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Troubleshooting

### Common Issues

1. **Database Connection Failed**

   - Verify PostgreSQL is running
   - Check connection parameters
   - Ensure database exists
   - For Docker: `docker-compose ps` to check container status

2. **pgvector Extension Error**

   - Ensure PostgreSQL has pgvector extension installed
   - Run: `CREATE EXTENSION IF NOT EXISTS vector;`
   - Docker Compose includes pgvector automatically

3. **Import Fails with Embedding Error**

   - Check Azure OpenAI credentials are correct
   - Verify `AZURE_OPENAI_BASE_URL` and `AZURE_OPENAI_KEY` are set
   - Check API quota and rate limits

4. **Search Returns Empty Results**

   - Ensure Bible data has been imported
   - Verify embeddings were generated during import
   - Check version code matches exactly (case-sensitive)

5. **Port Already in Use**
   - Change SERVER_PORT environment variable
   - Kill existing processes on the port: `lsof -ti:9999 | xargs kill`

### Logs

Check application logs for detailed error information:

```bash
# Docker logs
docker-compose logs -f bible-api

# Direct execution logs
go run cmd/main.go
```

## 🔮 Future Enhancements

- [ ] Authentication & Authorization
- [ ] Rate Limiting
- [ ] Caching Layer (Redis) for search results
- [ ] Metrics & Monitoring (Prometheus)
- [ ] Load Balancing
- [ ] API Versioning Strategy
- [ ] Search result ranking improvements
- [ ] Multi-language search support (cross-version search)
- [ ] Search analytics and insights
- [ ] Performance Optimization (connection pooling, query optimization)
- [ ] Batch import support
- [ ] Export functionality
- [ ] Verse comparison across versions

## 📞 Support

For support and questions, please open an issue in the repository or contact the development team.

---

**Built with ❤️ using Go**
