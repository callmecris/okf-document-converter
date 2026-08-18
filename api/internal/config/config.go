package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DatabaseURL     string
	RabbitMQURL     string
	RabbitMQQueue   string
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOUseSSL     bool
	BucketOriginals string
	BucketBundles   string
	JWTSecret       string
	LogLevel        string
}

// Load lee las variables de entorno con defaults para desarrollo local.
func Load() (*Config, error) {
	_ = godotenv.Load() // opcional: solo aplica fuera de Docker

	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://okf_user:okf_password@localhost:5432/okf_db?sslmode=disable"),
		RabbitMQURL:     getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQQueue:   getEnv("RABBITMQ_QUEUE", "jobs"),
		MinIOEndpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:  getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:  getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseSSL:     getEnv("MINIO_USE_SSL", "false") == "true",
		BucketOriginals: getEnv("BUCKET_ORIGINALS", "originals"),
		BucketBundles:   getEnv("BUCKET_BUNDLES", "bundles"),
		JWTSecret:       getEnv("JWT_SECRET", "dev-insecure-secret-change-me"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}