# Bible API

A modern, RESTful Bible API built with Go, featuring semantic search capabilities powered by AI. This API provides access to Bible content with intelligent search functionality that understands the meaning behind queries.

## 🚀 Features

- **RESTful API**: Clean, well-documented REST endpoints
- **Semantic Search**: AI-powered search that understands query intent
- **Multiple Bible Versions**: Support for different Bible translations
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

### Semantic Search
- `POST /api/bible/v1/search` - Search Bible verses using semantic understanding

### Documentation
- `GET /swagger/*` - Interactive Swagger documentation

## 🛠️ Technology Stack

- **Language**: Go 1.24+
- **Framework**: Gin (HTTP web framework)
- **Database**: MySQL with GORM (ORM)
- **AI Search**: External AI search service integration
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
│   │   └── search/          # Search functionality
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
   # Copy and modify environment variables
   export MYSQL_HOST=localhost
   export MYSQL_PORT=3306
   export MYSQL_USER=bible
   export MYSQL_PASS=bible
   export MYSQL_DB=bible
   export AI_SEARCH_URL=http://localhost:9999
   export SERVER_PORT=8080
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
| `AI_SEARCH_URL` | AI search service URL | `http://localhost:9999` |
| `SERVER_PORT` | Server port | `8080` |

## 📖 API Usage Examples

### Get All Bible Versions
```bash
curl -X GET "http://localhost:8080/api/bible/v1/versions"
```

### Get Bible Content by Version
```bash
curl -X GET "http://localhost:8080/api/bible/v1/version/1"
```

### Semantic Search
```bash
curl -X POST "http://localhost:8080/api/bible/v1/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "love and forgiveness",
    "top_k": 10
  }'
```

### Response Example
```json
{
  "query": "love and forgiveness",
  "results": [
    {
      "rank": 1,
      "score": 0.95,
      "book_name": "John",
      "chapter": 3,
      "verse": 16,
      "text": "For God so loved the world...",
      "version_id": 1,
      "version_name": "King James Version"
    }
  ],
  "total": 1
}
```

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

### Code Formatting
```bash
go fmt ./...
go vet ./...
```

## 📊 Monitoring & Health Checks

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

2. **AI Search Service Unavailable**
   - Verify AI_SEARCH_URL is correct
   - Check if the AI search service is running

3. **Port Already in Use**
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
- [ ] Caching Layer (Redis)
- [ ] Metrics & Monitoring (Prometheus)
- [ ] Load Balancing
- [ ] API Versioning Strategy
- [ ] Comprehensive Test Suite
- [ ] Performance Optimization

## 📞 Support

For support and questions, please open an issue in the repository or contact the development team.

---

**Built with ❤️ using Go**
