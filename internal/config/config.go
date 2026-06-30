package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                            string
	DatabaseURL                     string
	NATSURL                         string
	JWTSecret                       string
	CookieSecure                    bool
	AccessTokenTTL                  time.Duration
	RefreshTokenTTL                 time.Duration
	PublisherBatchSize              int32
	PublisherPollInterval           time.Duration
	PublisherMetricsPort            string
	ConsumerMetricsPort             string
	RedisURL                        string
	OrderLockTTL                    time.Duration
	RateLimitEnabled                bool
	RateLimitRequestsPerMinute      int
	AuthRateLimitRequestsPerMinute  int
	LoginRateLimitRequestsPerMinute int
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

	publisherBatchSize := getEnvInt("PUBLISHER_BATCH_SIZE", 10)
	publisherPollInterval := getEnvDuration("PUBLISHER_POLL_INTERVAL", 2*time.Second)

	publisherMetricsPort := os.Getenv("PUBLISHER_METRICS_PORT")
	if publisherMetricsPort == "" {
		publisherMetricsPort = "8081"
	}

	consumerMetricsPort := os.Getenv("CONSUMER_METRICS_PORT")
	if consumerMetricsPort == "" {
		consumerMetricsPort = "8082"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	orderLockTTL := getEnvDuration("ORDER_LOCK_TTL", 10*time.Second)

	rateLimitEnabled := getEnvBool("RATE_LIMIT_ENABLED", true)
	rateLimitRequestsPerMinute := getEnvInt("RATE_LIMIT_REQUESTS_PER_MINUTE", 60)
	authRateLimitRequestsPerMinute := getEnvInt("AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE", 10)
	loginRateLimitRequestsPerMinute := getEnvInt("LOGIN_RATE_LIMIT_REQUESTS_PER_MINUTE", 5)

	return Config{
		Port:                            port,
		DatabaseURL:                     databaseURL,
		NATSURL:                         natsURL,
		JWTSecret:                       jwtSecret,
		CookieSecure:                    cookieSecure,
		AccessTokenTTL:                  accessTokenTTL,
		RefreshTokenTTL:                 refreshTokenTTL,
		PublisherBatchSize:              int32(publisherBatchSize),
		PublisherPollInterval:           publisherPollInterval,
		PublisherMetricsPort:            publisherMetricsPort,
		ConsumerMetricsPort:             consumerMetricsPort,
		RedisURL:                        redisURL,
		OrderLockTTL:                    orderLockTTL,
		RateLimitEnabled:                rateLimitEnabled,
		RateLimitRequestsPerMinute:      rateLimitRequestsPerMinute,
		AuthRateLimitRequestsPerMinute:  authRateLimitRequestsPerMinute,
		LoginRateLimitRequestsPerMinute: loginRateLimitRequestsPerMinute,
	}
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
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
