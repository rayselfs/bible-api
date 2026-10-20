package configs

import (
	"testing"
	"time"
)

func TestPoolConfigDefaultsAndValidation(t *testing.T) {
	for _, key := range []string{"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME", "DB_CONN_MAX_IDLE_TIME"} {
		t.Setenv(key, "")
	}
	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpenConns != 8 || cfg.DBMaxIdleConns != 2 || cfg.DBConnMaxLifetime != 30*time.Minute || cfg.DBConnMaxIdleTime != 5*time.Minute {
		t.Fatal("unexpected pool defaults")
	}
	for _, tc := range []struct{ key, value string }{
		{"DB_MAX_OPEN_CONNS", "0"}, {"DB_MAX_OPEN_CONNS", "-1"}, {"DB_MAX_OPEN_CONNS", "bad"},
		{"DB_MAX_IDLE_CONNS", "-1"}, {"DB_MAX_IDLE_CONNS", "9"}, {"DB_MAX_IDLE_CONNS", "bad"},
		{"DB_CONN_MAX_LIFETIME", "0s"}, {"DB_CONN_MAX_LIFETIME", "bad"}, {"DB_CONN_MAX_IDLE_TIME", "-1s"}, {"DB_CONN_MAX_IDLE_TIME", "0s"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := InitConfig(); err == nil {
				t.Fatal("invalid pool configuration accepted")
			}
		})
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "0")
	t.Setenv("DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "1m")
	cfg, err = InitConfig()
	if err != nil || cfg.DBMaxOpenConns != 12 || cfg.DBMaxIdleConns != 0 || cfg.DBConnMaxLifetime != 10*time.Minute || cfg.DBConnMaxIdleTime != time.Minute {
		t.Fatal("pool overrides not applied")
	}
}

func TestDaprPortValidation(t *testing.T) {
	for _, value := range []string{"0", "65536", "-1", "+3500", " 3500", "0xDAC", "bad"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DAPR_HTTP_PORT", value)
			if _, err := InitConfig(); err == nil {
				t.Fatal("invalid Dapr port accepted")
			}
		})
	}
}

func TestDaprConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DAPR_HTTP_PORT", "")
	t.Setenv("DAPR_API_TOKEN", "")
	cfg, err := InitConfig()
	if err != nil || cfg.DaprHTTPPort != 3500 || cfg.DaprAPIToken != "" {
		t.Fatal("unexpected Dapr defaults")
	}
	t.Setenv("DAPR_HTTP_PORT", "12345")
	t.Setenv("DAPR_API_TOKEN", "fixture-token")
	cfg, err = InitConfig()
	if err != nil || cfg.DaprHTTPPort != 12345 || cfg.DaprAPIToken != "fixture-token" {
		t.Fatal("Dapr overrides not applied")
	}
}
