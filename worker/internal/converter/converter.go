// Package converter implementa el pipeline unificado de extracción y
// segmentación: Documento Entrada → Markdown → Unidades Lógicas.
package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"okf/pkg/domain"
)

// Options controla la conversión de un trabajo.
type Options struct {
	Format       domain.DocFormat
	SourcePath   string // archivo original descargado
	WorkDir      string // directorio de trabajo temporal
	PDFChunkSize int    // nivel 3 de PDF: páginas por concepto
	// Assets recolecta los recursos referenciados (imágenes) en assets/.
	// Si es nil, la extracción de recursos queda desactivada.
	Assets *assetCollector
}

// Segment es una unidad lógica del documento (capítulo/páginas).
type Segment struct {
	Title string
	File  string // ruta absoluta al archivo .md
	Order int
}

// Converter extrae y segmenta un formato concreto.
type Converter interface {
	Convert(ctx context.Context, opts Options) ([]Segment, error)
}

// Pipeline despacha al conversor según el formato del documento.
type Pipeline struct {
	converters map[domain.DocFormat]Converter
}

func NewPipeline(pdfChunkSize int) *Pipeline {
	return &Pipeline{
		converters: map[domain.DocFormat]Converter{
			domain.FormatEPUB: &EpubConverter{},
			domain.FormatDOCX: &DocxConverter{},
			domain.FormatPDF:  &PdfConverter{ChunkPages: pdfChunkSize},
			domain.FormatMD:   &TextConverter{},
			domain.FormatTXT:  &TextConverter{PlainText: true},
			domain.FormatHTML: &TextConverter{HTML: true},
		},
	}
}

// Result agrupa la salida de una conversión.
type Result struct {
	Segments []Segment
	// Assets es el número de recursos extraídos a assets/.
	Assets int
}

// Convert ejecuta el conversor del formato, habilitando la extracción de
// recursos a assets/ dentro del directorio de trabajo.
func (p *Pipeline) Convert(ctx context.Context, opts Options) (Result, error) {
	c, ok := p.converters[opts.Format]
	if !ok {
		return Result{}, fmt.Errorf("unsupported format %q", opts.Format)
	}
	if opts.Assets == nil {
		opts.Assets = newAssetCollector(opts.WorkDir)
	}

	segments, err := c.Convert(ctx, opts)
	if err != nil {
		return Result{}, err
	}
	return Result{Segments: segments, Assets: opts.Assets.Count()}, nil
}

// writeSegment escribe un segmento markdown en conceptos/, garantizando que
// el archivo empiece por un encabezado. Si el contenido ya trae el suyo
// (caso de la segmentación por headings) no se duplica.
func writeSegment(workDir string, order int, title, content string) (Segment, error) {
	dir := filepath.Join(workDir, "conceptos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Segment{}, fmt.Errorf("create conceptos dir: %w", err)
	}
	file := filepath.Join(dir, fmt.Sprintf("fragmento-%02d.md", order))
	body := fmt.Sprintf("# %s\n\n%s\n", title, content)
	if strings.HasPrefix(strings.TrimSpace(content), "#") {
		body = strings.TrimSpace(content) + "\n"
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return Segment{}, fmt.Errorf("write segment %s: %w", file, err)
	}
	return Segment{Title: title, File: file, Order: order}, nil
}