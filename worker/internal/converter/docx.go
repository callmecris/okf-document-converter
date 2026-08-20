package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DocxConverter convierte documentos Word a Markdown usando pandoc
// (instalado en la imagen del worker) y luego segmenta por encabezados.
//
// Las imágenes incrustadas se extraen con --extract-media y se recolocan en
// assets/, reescribiendo las referencias del markdown.
type DocxConverter struct{}

func (d *DocxConverter) Convert(ctx context.Context, opts Options) ([]Segment, error) {
	outPath := filepath.Join(opts.WorkDir, "document.md")
	mediaDir := filepath.Join(opts.WorkDir, "media")

	cmd := exec.CommandContext(ctx, "pandoc", opts.SourcePath,
		"-t", "markdown", "--extract-media", mediaDir, "-o", outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pandoc failed: %v: %s", err, output)
	}

	if err := collectMarkdownAssets(outPath, opts); err != nil {
		return nil, fmt.Errorf("extraer recursos del docx: %w", err)
	}

	segments, err := segmentMarkdown(opts.WorkDir, outPath)
	if err != nil {
		return nil, fmt.Errorf("segment docx markdown: %w", err)
	}
	return segments, nil
}

// collectMarkdownAssets mueve a assets/ las imágenes locales referenciadas por
// un markdown y reescribe sus enlaces. Reescribe el archivo en el sitio.
//
// Se aplica a cualquier markdown ya materializado en disco (pandoc, .md de
// entrada), por lo que lo comparten DocxConverter y TextConverter.
func collectMarkdownAssets(mdPath string, opts Options) error {
	if opts.Assets == nil {
		return nil
	}
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("read markdown: %w", err)
	}

	baseDir := filepath.Dir(mdPath)
	var failed error
	rewritten := rewriteMarkdownImages(string(data), func(src string) string {
		if isRemoteRef(src) || failed != nil {
			return ""
		}
		candidate := src
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(baseDir, filepath.FromSlash(src))
		}
		// Si el original venía junto al documento de entrada, también se busca allí.
		if _, err := os.Stat(candidate); err != nil {
			alt := filepath.Join(filepath.Dir(opts.SourcePath), filepath.FromSlash(src))
			if _, err2 := os.Stat(alt); err2 != nil {
				return "" // referencia rota: se deja tal cual, ya lo advierte la validación
			}
			candidate = alt
		}

		name, err := opts.Assets.addFile(candidate)
		if err != nil {
			failed = err
			return ""
		}
		return name
	})
	if failed != nil {
		return failed
	}

	if rewritten == string(data) {
		return nil
	}
	return os.WriteFile(mdPath, []byte(rewritten), 0o644)
}

// stripMediaPrefix normaliza rutas de pandoc del tipo "media/media/img.png".
func stripMediaPrefix(src string) string {
	return strings.TrimPrefix(filepath.ToSlash(src), "./")
}
