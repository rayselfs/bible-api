package configs

import (
	"github.com/caarlos0/env/v11"
)

type Env struct {
	MysqlHost  string `env:"MYSQL_HOST" envDefault:"localhost"`
	MysqlPort  string `env:"MYSQL_PORT" envDefault:"3306"`
	MysqlUser  string `env:"MYSQL_USER" envDefault:"bible"`
	MysqlPass  string `env:"MYSQL_PASS" envDefault:"bible"`
	MysqlDB    string `env:"MYSQL_DB" envDefault:"bible"`
	MysqlCert  string `env:"MYSQL_CERT" envDefault:"/app/DigiCertGlobalRootG2.crt.pem"`
	ServerPort string `env:"SERVER_PORT" envDefault:"8080"`

	// Azure AI Search configuration
	AzureAISearchBaseURL    string `env:"AZURE_AI_SEARCH_BASE_URL"`
	AzureAISearchQueryKey   string `env:"AZURE_AI_SEARCH_QUERY_KEY"`
	AzureAISearchIndexName  string `env:"AZURE_AI_SEARCH_INDEX_NAME"`
	AzureAISearchAPIVersion string `env:"AZURE_AI_SEARCH_API_VERSION" envDefault:"2023-11-01"`

	// Azure OpenAI configuration for search query embedding
	AzureOpenAIBaseURL   string `env:"AZURE_OPENAI_BASE_URL"`
	AzureOpenAIKey       string `env:"AZURE_OPENAI_KEY"`
	AzureOpenAIModelName string `env:"AZURE_OPENAI_MODEL_NAME"`
}

func InitConfig() (*Env, error) {
	var cfg Env
	err := env.Parse(&cfg)
	return &cfg, err
}
