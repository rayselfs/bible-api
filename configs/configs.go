package configs

import (
	"github.com/caarlos0/env/v11"
)

type Env struct {
	PostgresHost    string `env:"POSTGRES_HOST" envDefault:"localhost"`
	PostgresPort    string `env:"POSTGRES_PORT" envDefault:"5432"`
	PostgresUser    string `env:"POSTGRES_USER" envDefault:"bible"`
	PostgresPass    string `env:"POSTGRES_PASS" envDefault:"bible"`
	PostgresDB      string `env:"POSTGRES_DB" envDefault:"bible"`
	PostgresSSLMode string `env:"POSTGRES_SSLMODE" envDefault:"disable"` // disable, require
	ServerPort      string `env:"SERVER_PORT" envDefault:"9999"`
}

func InitConfig() (*Env, error) {
	var cfg Env
	err := env.Parse(&cfg)
	return &cfg, err
}
