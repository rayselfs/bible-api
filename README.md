# Bible API

A modern, RESTful Bible API built with Go. This API provides access to Bible content across multiple versions with a clean and well-documented interface.

## 🚀 Features

- **RESTful API**: Clean, well-documented REST endpoints
- **Multiple Bible Versions**: Support for different Bible translations
- **AI-Powered Search**: Semantic search using Azure AI Search and OpenAI embeddings
- **Hybrid Search**: Combines keyword and vector search for better results
- **Swagger Documentation**: Interactive API documentation
- **Database Migrations**: Automated database schema management
- **Docker Support**: Containerized deployment
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
- **Database**: MySQL with GORM (ORM)
- **AI Search**: Azure AI Search for semantic search
- **AI Embeddings**: Azure OpenAI text-embedding-3-small
- **Documentation**: Swagger/OpenAPI 3.0
- **Migration**: Gormigrate
- **Containerization**: Docker & Docker Compose

## 📦 Project Structure

```
bible-api/
├── cmd/
│   ├── main.go              # Application entry point
│   └── import/              # Data import utilities
├── configs/
│   └── configs.go           # Configuration management
├── docs/                    # Swagger documentation
├── internal/
│   ├── models/              # Database models and stores
│   ├── pkg/
│   │   └── ai-search/       # AI Search services
│   │       ├── service.go   # Azure AI Search service
│   │       └── openai.go    # OpenAI embedding service
│   └── server/              # HTTP handlers and routes
├── migrations/              # Database migrations
├── Dockerfile
├── docker-compose.yml
└── README.md
```

## 🚀 Quick Start

### Prerequisites

- Go 1.24 or higher
- MySQL 8.0+
- Docker (optional)

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
   # Using Docker Compose
   docker-compose up -d mysql
   
   # Or manually create MySQL database
   mysql -u root -p -e "CREATE DATABASE bible;"
   ```

4. **Configure environment variables**
   ```bash
   # Database configuration
   export MYSQL_HOST=localhost
   export MYSQL_PORT=3306
   export MYSQL_USER=bible
   export MYSQL_PASS=bible
   export MYSQL_DB=bible
   export SERVER_PORT=8080
   
   # AI Search configuration (required for search functionality)
   export AZURE_AI_SEARCH_ENDPOINT="https://your-search-service.search.windows.net/indexes/bible-verses/docs"
   export AZURE_AI_SEARCH_QUERY_KEY="your-admin-key"
   export AZURE_OPENAI_BASE_URL="https://your-service.openai.azure.com/openai/v1/"
   export AZURE_OPENAI_KEY="your-api-key"
   export AZURE_OPENAI_MODEL_NAME="text-embedding-3-small"
   ```

5. **Run the application**
   ```bash
   go run cmd/main.go
   ```

6. **Access the API**
   - API: http://localhost:8080
   - Swagger UI: http://localhost:8080/swagger/index.html
   - Health Check: http://localhost:8080/health

## 🐳 Docker Deployment

### Using Docker Compose (Recommended)

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

# Run the container
docker run -p 8080:8080 \
  -e MYSQL_HOST=host.docker.internal \
  -e MYSQL_USER=bible \
  -e MYSQL_PASS=bible \
  -e MYSQL_DB=bible \
  bible-api
```

## ⚙️ Configuration

The application uses environment variables for configuration:

| Variable | Description | Default |
|----------|-------------|---------|
| `MYSQL_HOST` | MySQL server host | `localhost` |
| `MYSQL_PORT` | MySQL server port | `3306` |
| `MYSQL_USER` | MySQL username | `bible` |
| `MYSQL_PASS` | MySQL password | `bible` |
| `MYSQL_DB` | MySQL database name | `bible` |
| `SERVER_PORT` | Server port | `8080` |
| `AZURE_AI_SEARCH_ENDPOINT` | Azure AI Search endpoint | Required for search |
| `AZURE_AI_SEARCH_QUERY_KEY` | Azure AI Search query key | Required for search |
| `AZURE_OPENAI_BASE_URL` | Azure OpenAI base URL | Required for embedding |
| `AZURE_OPENAI_KEY` | Azure OpenAI API key | Required for embedding |
| `AZURE_OPENAI_MODEL_NAME` | Azure OpenAI model name | Required for embedding |

## 📖 API Usage Examples

### Get All Bible Versions
```bash
curl -X GET "http://localhost:8080/api/bible/v1/versions"
```

### Get Bible Content by Version
```bash
curl -X GET "http://localhost:8080/api/bible/v1/version/1"
```

### Search Bible Verses
```bash
curl -X GET "http://localhost:8080/api/bible/v1/search?q=愛&version=CUV&top=10"
```

#### Search API Request Format
```
GET /api/bible/v1/search?q=搜尋關鍵字&version=版本代碼&top=結果數量
```

**參數說明：**
- `q` (required): 搜尋查詢字串
- `version` (required): 聖經版本代碼 (如: CUV, ESV, NIV)
- `top` (optional): 回傳結果數量限制，預設為 10

#### Search API Response Format
```json
{
  "query": "愛",
  "results": [
    {
      "verse_id": "123",
      "version_code": "CUV",
      "book_number": 1,
      "chapter_number": 1,
      "verse_number": 1,
      "text": "起初，神創造天地。",
      "score": 0.95
    }
  ],
  "total": 1
}
```

#### Search Parameters
- `q` (required): Search query string
- `version` (required): Bible version code (e.g., CUV, ESV, NIV)
- `top` (optional): Maximum number of results (default: 10)

## 🔍 AI Search Integration

The search API integrates with Azure AI Search for semantic search capabilities:

### Features
- **Text Search**: Full-text search using Azure AI Search
- **Vector Search**: Semantic similarity search (requires embedding implementation)
- **Version Filtering**: Filter results by Bible version
- **Relevance Scoring**: Results include similarity scores

### Setup
1. Configure Azure AI Search environment variables
2. Ensure the search index contains Bible verse data
3. See [AI_SEARCH_SETUP.md](AI_SEARCH_SETUP.md) for detailed setup instructions

### Current Implementation
- ✅ Hybrid search (text + vector) using Azure AI Search
- ✅ OpenAI embedding integration for semantic search
- ✅ Version filtering (required parameter)
- ✅ Result scoring and ranking
- ✅ RESTful GET API with query parameters

## 💰 Cost Estimation

### Search API Costs
- **OpenAI text-embedding-3-small**: $0.00002 per 1,000 tokens
- **Azure AI Search Basic**: $73/month (fixed cost)
- **Estimated cost per search**: ~$0.0073 (based on 10,000 searches/month)

### Cost Breakdown
| Usage Level | Monthly Searches | OpenAI Cost | Azure AI Search | Total/Month |
|-------------|------------------|-------------|-----------------|-------------|
| Light | 1,000 | $0.00004 | $73 | $73.00 |
| Medium | 10,000 | $0.0004 | $73 | $73.00 |
| Heavy | 100,000 | $0.004 | $73 | $73.00 |

*Note: OpenAI costs are negligible compared to Azure AI Search fixed monthly fee*

## 🔄 Database Migrations

The application automatically runs database migrations on startup. To create new migrations:

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
# Test basic search
curl -X GET "http://localhost:8080/api/bible/v1/search?q=愛&version=CUV&top=5"

# Test with different version
curl -X GET "http://localhost:8080/api/bible/v1/search?q=love&version=ESV&top=3"

# Test error cases
curl -X GET "http://localhost:8080/api/bible/v1/search?q=愛"  # Missing version
curl -X GET "http://localhost:8080/api/bible/v1/search?version=CUV"  # Missing query
```

### Automated Testing
```bash
# Run the test script
chmod +x test_hybrid_search.sh
./test_hybrid_search.sh
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
   - Verify MySQL is running
   - Check connection parameters
   - Ensure database exists

2. **Port Already in Use**
   - Change SERVER_PORT environment variable
   - Kill existing processes on the port

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
- [ ] Multi-language search support
- [ ] Search analytics and insights
- [ ] Performance Optimization

## 📞 Support

For support and questions, please open an issue in the repository or contact the development team.

---

**Built with ❤️ using Go**
