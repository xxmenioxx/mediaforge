package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host         string
	Port         string
	DatabasePath string
}

func Load() Config {
	return Config{
		Host:         env("MEDIAFORGE_API_HOST", "127.0.0.1"),
		Port:         env("MEDIAFORGE_API_PORT", "8080"),
		DatabasePath: env("MEDIAFORGE_DB_PATH", "data/mediaforge.db"),
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
