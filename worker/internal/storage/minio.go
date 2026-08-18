// Package storage contiene el cliente MinIO del worker:
// descarga de originales y subida de bundles generados.
package storage

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Storage struct {
	client          *minio.Client
	originalsBucket string
	bundlesBucket   string
}

func New(endpoint, accessKey, secretKey string, useSSL bool, originalsBucket, bundlesBucket string) (*Storage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Storage{
		client:          client,
		originalsBucket: originalsBucket,
		bundlesBucket:   bundlesBucket,
	}, nil
}

func (s *Storage) OriginalsBucket() string { return s.originalsBucket }
func (s *Storage) BundlesBucket() string   { return s.bundlesBucket }

// EnsureBuckets crea los buckets si no existen (modo standalone).
func (s *Storage) EnsureBuckets(ctx context.Context) error {
	for _, b := range []string{s.originalsBucket, s.bundlesBucket} {
		exists, err := s.client.BucketExists(ctx, b)
		if err != nil {
			return fmt.Errorf("check bucket %s: %w", b, err)
		}
		if !exists {
			if err := s.client.MakeBucket(ctx, b, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("create bucket %s: %w", b, err)
			}
		}
	}
	return nil
}

// Download descarga un objeto a un archivo local.
func (s *Storage) Download(ctx context.Context, bucket, objectKey, dstPath string) error {
	if err := s.client.FGetObject(ctx, bucket, objectKey, dstPath, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("download %s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

// UploadDir sube recursivamente un directorio local preservando rutas
// relativas bajo el prefijo indicado (ej: bundles/<userId>/<jobId>/...).
func (s *Storage) UploadDir(ctx context.Context, bucket, localDir, prefix string) error {
	return filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(prefix, rel))
		if _, err := s.client.FPutObject(ctx, bucket, key, path, minio.PutObjectOptions{
			ContentType: contentTypeFor(rel),
		}); err != nil {
			return fmt.Errorf("upload %s: %w", key, err)
		}
		return nil
	})
}

func contentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}