package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL  string
	HTTPAddr     string
	DBMaxConns   int32
	DBMinConns   int32
	MaxBatchSize int
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:  getenv("DATABASE_URL", "postgres://app:app@localhost:5432/records?sslmode=disable"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		DBMaxConns:   int32(getenvInt("DB_MAX_CONNS", 24)),
		DBMinConns:   int32(getenvInt("DB_MIN_CONNS", 4)),
		MaxBatchSize: getenvInt("MAX_BATCH_SIZE", 50000),
	}
	if c.DBMinConns < 1 || c.DBMaxConns < c.DBMinConns || c.MaxBatchSize < 1 {
		return Config{}, fmt.Errorf("invalid pool or batch configuration")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value, err := strconv.Atoi(getenv(key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
