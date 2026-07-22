package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Host         string
	Port         string
	DatabasePath string
}

func Load() Config {
	return Config{
		Host:         compatibleEnv("MVFORGE_API_HOST", "MEDIAFORGE_API_HOST", "127.0.0.1"),
		Port:         compatibleEnv("MVFORGE_API_PORT", "MEDIAFORGE_API_PORT", "8080"),
		DatabasePath: compatibleEnv("MVFORGE_DB_PATH", "MEDIAFORGE_DB_PATH", defaultDatabasePath()),
	}
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func compatibleEnv(key string, legacyKey string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return env(legacyKey, fallback)
}

func defaultDatabasePath() string {
	legacy := filepath.Join("data", "mediaforge.db")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return filepath.Join("data", "mvforge.db")
}
