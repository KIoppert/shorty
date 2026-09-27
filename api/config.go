package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	DatabaseURL  string
	JWTSecret    []byte
	TokenTTL     time.Duration
	BaseURL      string
	CookieSecure bool
}

func loadConfig() (Config, error) {
	cfg := Config{
		Port:        env("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
		BaseURL:     env("BASE_URL", "http://localhost:8080"),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return cfg, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}

	ttl, err := time.ParseDuration(env("TOKEN_TTL", "168h"))
	if err != nil {
		return cfg, fmt.Errorf("TOKEN_TTL: %w", err)
	}
	cfg.TokenTTL = ttl

	secure, err := strconv.ParseBool(env("COOKIE_SECURE", "false"))
	if err != nil {
		return cfg, fmt.Errorf("COOKIE_SECURE: %w", err)
	}
	cfg.CookieSecure = secure

	return cfg, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
