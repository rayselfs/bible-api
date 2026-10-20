package configs

import (
	"fmt"
	"github.com/caarlos0/env/v11"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env struct {
	DaprHTTPPort      int
	DaprAPIToken      string        `env:"DAPR_API_TOKEN"`
	DBMaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"8"`
	DBMaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"2"`
	DBConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"30m"`
	DBConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"5m"`
	PostgresHost      string        `env:"POSTGRES_HOST" envDefault:"localhost"`
	PostgresPort      string        `env:"POSTGRES_PORT" envDefault:"5432"`
	PostgresUser      string        `env:"POSTGRES_USER" envDefault:"bible"`
	PostgresPass      string        `env:"POSTGRES_PASS" envDefault:"bible"`
	PostgresDB        string        `env:"POSTGRES_DB" envDefault:"bible"`
	PostgresSSLMode   string        `env:"POSTGRES_SSLMODE" envDefault:"disable"` // disable, require
	ServerPort        string        `env:"SERVER_PORT" envDefault:"9999"`
}

func InitConfig() (*Env, error) {
	var cfg Env
	err := env.Parse(&cfg)
	if err != nil {
		return nil, err
	}
	if cfg.DBMaxOpenConns <= 0 || cfg.DBMaxIdleConns < 0 || cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		return nil, fmt.Errorf("invalid database pool limits")
	}
	if cfg.DBConnMaxLifetime <= 0 || cfg.DBConnMaxIdleTime <= 0 {
		return nil, fmt.Errorf("invalid database pool connection durations")
	}
	port := os.Getenv("DAPR_HTTP_PORT")
	if port == "" {
		port = "3500"
	}
	if strings.Trim(port, "0123456789") != "" {
		return nil, fmt.Errorf("invalid Dapr HTTP port")
	}
	cfg.DaprHTTPPort, err = strconv.Atoi(port)
	if err != nil || cfg.DaprHTTPPort < 1 || cfg.DaprHTTPPort > 65535 {
		return nil, fmt.Errorf("invalid Dapr HTTP port")
	}
	return &cfg, nil
}
