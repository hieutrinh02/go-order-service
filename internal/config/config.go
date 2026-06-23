package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DatabaseURL     string
	NATSURL         string
	JWTSecret       string
	CookieSecure    bool
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

func Load() Config {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://orderservice:orderservice@localhost:5434/order_service?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4223"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-change-me"
	}

	cookieSecure := getEnvBool("COOKIE_SECURE", false)

	accessTokenTTL := getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute)
	refreshTokenTTL := getEnvDuration("REFRESH_TOKEN_TTL", 168*time.Hour)

	return Config{
		Port:            port,
		DatabaseURL:     databaseURL,
		NATSURL:         natsURL,
		JWTSecret:       jwtSecret,
		CookieSecure:    cookieSecure,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTokenTTL,
	}
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

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}
