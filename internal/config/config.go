package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPPort           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	OpenSearchEnabled  bool
	OpenSearchURL      string
	OpenSearchUser     string
	OpenSearchPassword string
}

func Load() Config {
	return Config{
		HTTPPort:           getEnv("HTTP_PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://root:root@localhost:5432/lead_scoring?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		RedisDB:            getEnvInt("REDIS_DB", 0),
		OpenSearchEnabled:  getEnvBool("OPENSEARCH_DIRECT_LOGS", false),
		OpenSearchURL:      getEnv("OPENSEARCH_URL", "https://opensearch:9200"),
		OpenSearchUser:     getEnv("OPENSEARCH_USER", "admin"),
		OpenSearchPassword: getEnv("OPENSEARCH_PASSWORD", "SecureLeadScore_2024!"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}
