package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	HTTPAddress string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddress: os.Getenv("HTTP_ADDR"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required; see .env.example")
	}
	if cfg.HTTPAddress == "" {
		cfg.HTTPAddress = "127.0.0.1:18080"
	}
	return cfg, nil
}
