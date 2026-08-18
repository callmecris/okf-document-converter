package converter

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// DocxConverter convierte documentos Word a Markdown usando pandoc
// (instalado en la imagen del worker) y luego segmenta por encabezados.
type DocxConverter struct{}

func (d *DocxConverter) Convert(ctx context.Context, opts Options) ([]Segment, error) {
	outPath := filepath.Join(opts.WorkDir, "document.md")

	cmd := exec.CommandContext(ctx, "pandoc", opts.SourcePath, "-t", "markdown", "-o", outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pandoc failed: %v: %s", err, output)
	}

	segments, err := segmentMarkdown(opts.WorkDir, outPath)
	if err != nil {
		return nil, fmt.Errorf("segment docx markdown: %w", err)
	}
	return segments, nil
}