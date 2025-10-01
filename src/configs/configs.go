package configs

import (
	"github.com/caarlos0/env/v11"
)

type Env struct {
	MysqlHost string `env:"MYSQL_HOST" envDefault:"localhost"`
	MysqlPort string `env:"MYSQL_PORT" envDefault:"3306"`
	MysqlUser string `env:"MYSQL_USER" envDefault:"bible"`
	MysqlPass string `env:"MYSQL_PASS" envDefault:"bible"`
	MysqlDB   string `env:"MYSQL_DB" envDefault:"bible"`
}

func InitConfig() (*Env, error) {
	var cfg Env
	err := env.Parse(&cfg)
	return &cfg, err
}
