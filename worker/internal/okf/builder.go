// Package okf genera y valida el bundle OKF:
//
//	bundle/
//	├── index.md           -> tabla de contenidos con links a conceptos
//	├── log.md             -> metadatos de conversión
//	└── conceptos/
//	    ├── fragmento-01.md
//	    └── ...
package okf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"okf/worker/internal/converter"
)

type Meta struct {
	JobID        string
	UserID       string
	OriginalName string
	Format       string
	SourcePath   string
	ConvertedAt  time.Time
	// Assets es el número de recursos extraídos a assets/.
	Assets int
}

type BuildResult struct {
	Dir        string                       // ruta local del bundle
	BundlePath string                       // prefijo en MinIO: bundles/<user>/<job>
	Segments   []converter.Segment
}

// Build arma el bundle OKF en disco a partir de los segmentos generados.
func Build(ctx context.Context, workDir string, segments []converter.Segment, meta Meta) (*BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	root := filepath.Join(workDir, "bundle")
	if err := os.MkdirAll(filepath.Join(root, "conceptos"), 0o755); err != nil {
		return nil, fmt.Errorf("create bundle dirs: %w", err)
	}

	sorted := make([]converter.Segment, len(segments))
	copy(sorted, segments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })

	// Mover segmentos generados por el conversor dentro del bundle.
	for _, seg := range sorted {
		dst := filepath.Join(root, "conceptos", filepath.Base(seg.File))
		if err := os.Rename(seg.File, dst); err != nil {
			return nil, fmt.Errorf("move segment to bundle: %w", err)
		}
	}

	// Los recursos extraídos (imágenes) acompañan al bundle en assets/.
	if src := filepath.Join(workDir, "assets"); dirHasFiles(src) {
		if err := os.Rename(src, filepath.Join(root, "assets")); err != nil {
			return nil, fmt.Errorf("move assets to bundle: %w", err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte(indexMarkdown(sorted)), 0o644); err != nil {
		return nil, fmt.Errorf("write index.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "log.md"), []byte(logMarkdown(meta, len(sorted))), 0o644); err != nil {
		return nil, fmt.Errorf("write log.md: %w", err)
	}

	return &BuildResult{
		Dir:        root,
		BundlePath: filepath.Join("bundles", meta.UserID, meta.JobID),
		Segments:   sorted,
	}, nil
}

func indexMarkdown(segments []converter.Segment) string {
	var b strings.Builder
	b.WriteString("# Índice del Bundle OKF\n\n")
	b.WriteString("| # | Título | Archivo |\n")
	b.WriteString("|---|--------|---------|\n")
	for _, seg := range segments {
		name := filepath.Base(seg.File)
		title := strings.ReplaceAll(seg.Title, "|", "\\|")
		b.WriteString(fmt.Sprintf("| %d | %s | [%s](conceptos/%s) |\n", seg.Order, title, name, name))
	}
	return b.String()
}

// logMarkdown registra metadatos de conversión, incluyendo el SHA-256 del
// documento original para trazabilidad e idempotencia.
func logMarkdown(meta Meta, segments int) string {
	hash := sourceSHA256(meta.SourcePath)
	size := sourceSize(meta.SourcePath)

	var b strings.Builder
	b.WriteString("# Log de Conversión\n\n")
	for _, kv := range [][2]string{
		{"Job ID", meta.JobID},
		{"Archivo original", meta.OriginalName},
		{"Formato", strings.ToUpper(meta.Format)},
		{"SHA-256 (original)", hash},
		{"Tamaño (bytes)", fmt.Sprintf("%d", size)},
		{"Conceptos generados", fmt.Sprintf("%d", segments)},
		{"Recursos extraídos", fmt.Sprintf("%d", meta.Assets)},
		{"Fecha de conversión", meta.ConvertedAt.UTC().Format(time.RFC3339)},
		{"Motor", "okf-worker"},
	} {
		b.WriteString(fmt.Sprintf("- **%s:** %s\n", kv[0], kv[1]))
	}
	return b.String()
}

func sourceSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sourceSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// dirHasFiles indica si un directorio existe y contiene al menos una entrada.
func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
