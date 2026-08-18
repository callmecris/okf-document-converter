package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL     string
	RabbitMQURL     string
	RabbitMQQueue   string
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOUseSSL     bool
	BucketOriginals string
	BucketBundles   string
	LogLevel        string
	WorkDir         string
	PDFChunkPages   int
}

// Load lee las variables de entorno con defaults para desarrollo local.
func Load() (*Config, error) {
	_ = godotenv.Load() // opcional: solo aplica fuera de Docker

	chunkPages, err := strconv.Atoi(getEnv("PDF_CHUNK_PAGES", "10"))
	if err != nil || chunkPages < 1 {
		return nil, fmt.Errorf("invalid PDF_CHUNK_PAGES: %q", getEnv("PDF_CHUNK_PAGES", "10"))
	}

	cfg := &Config{
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://okf_user:okf_password@localhost:5432/okf_db?sslmode=disable"),
		RabbitMQURL:     getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		RabbitMQQueue:   getEnv("RABBITMQ_QUEUE", "jobs"),
		MinIOEndpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:  getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:  getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseSSL:     getEnv("MINIO_USE_SSL", "false") == "true",
		BucketOriginals: getEnv("BUCKET_ORIGINALS", "originals"),
		BucketBundles:   getEnv("BUCKET_BUNDLES", "bundles"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		WorkDir:         getEnv("WORK_DIR", "/tmp/okf"),
		PDFChunkPages:   chunkPages,
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}