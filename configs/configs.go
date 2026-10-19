package configs

import (
	"fmt"
	"github.com/caarlos0/env/v11"
	"time"
)

type Env struct {
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
	return &cfg, nil
}
