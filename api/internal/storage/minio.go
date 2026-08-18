// Package storage contiene el cliente MinIO de la API.
// La API solo sube originales y genera URLs firmadas; nunca lee archivos locales.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

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

// EnsureBuckets crea los buckets si no existen.
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

// PutFile sube un archivo local a un bucket/object key.
func (s *Storage) PutFile(ctx context.Context, bucket, objectKey, filePath, contentType string) error {
	if _, err := s.client.FPutObject(ctx, bucket, objectKey, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("upload %s: %w", objectKey, err)
	}
	return nil
}

// PresignedGetURL devuelve una URL temporal para descargar un objeto.
func (s *Storage) PresignedGetURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (*url.URL, error) {
	return s.client.PresignedGetObject(ctx, bucket, objectKey, expiry, nil)
}

// ListObjects devuelve las claves de objetos bajo un prefijo.
func (s *Storage) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	for obj := range s.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects %s/%s: %w", bucket, prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// GetObject abre un stream de lectura de un objeto (para servirlo en la API).
func (s *Storage) GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if _, err := s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{}); err != nil {
		return nil, fmt.Errorf("stat %s/%s: %w", bucket, objectKey, err)
	}
	return s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
}