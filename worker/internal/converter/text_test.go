package converter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okf/pkg/domain"
)

func convertSource(t *testing.T, c Converter, name, content string) []Segment {
	t.Helper()
	workDir := t.TempDir()
	src := filepath.Join(workDir, name)
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	segments, err := c.Convert(context.Background(), Options{
		SourcePath: src,
		WorkDir:    workDir,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return segments
}

func TestTextConverterMarkdownSegmentsByHeadings(t *testing.T) {
	segments := convertSource(t, &TextConverter{}, "doc.md",
		"# Uno\n\nCUERPO_UNO\n\n# Dos\n\nCUERPO_DOS\n")

	if len(segments) != 2 {
		t.Fatalf("esperados 2 segmentos, got %d", len(segments))
	}
	if segments[0].Title != "Uno" || segments[1].Title != "Dos" {
		t.Errorf("títulos = %q, %q", segments[0].Title, segments[1].Title)
	}
	data, err := os.ReadFile(segments[1].File)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "CUERPO_UNO") {
		t.Errorf("segmento 2 contaminado con el cuerpo del 1: %s", data)
	}
}

// Documento breve sin divisiones: un único concepto, sin error (spec sec.6).
func TestTextConverterShortDocumentProducesOneSegment(t *testing.T) {
	segments := convertSource(t, &TextConverter{PlainText: true}, "doc.txt",
		"Un documento breve sin divisiones de ningun tipo.\n")

	if len(segments) != 1 {
		t.Fatalf("esperado 1 segmento, got %d", len(segments))
	}
}

func TestTextConverterPlainTextPromotesHeadings(t *testing.T) {
	segments := convertSource(t, &TextConverter{PlainText: true}, "doc.txt",
		"INTRODUCCION\n\nTexto de la introduccion.\n\n2. Desarrollo\n\nTexto del desarrollo.\n")

	if len(segments) != 2 {
		t.Fatalf("esperados 2 segmentos, got %d: %+v", len(segments), segments)
	}
	if segments[0].Title != "INTRODUCCION" {
		t.Errorf("título 1 = %q", segments[0].Title)
	}
}

func TestTextConverterHTML(t *testing.T) {
	segments := convertSource(t, &TextConverter{HTML: true}, "doc.html",
		"<h1>Alfa</h1><p>Cuerpo alfa.</p><h1>Beta</h1><p>Cuerpo beta.</p>")

	if len(segments) != 2 {
		t.Fatalf("esperados 2 segmentos, got %d", len(segments))
	}
}

func TestPipelineSupportsTextFormats(t *testing.T) {
	p := NewPipeline(10)
	for _, f := range []domain.DocFormat{domain.FormatMD, domain.FormatTXT, domain.FormatHTML} {
		if _, ok := p.converters[f]; !ok {
			t.Errorf("pipeline sin converter para %q", f)
		}
	}
}

func TestParseFormatAcceptsTextFormats(t *testing.T) {
	for ext, want := range map[string]domain.DocFormat{
		".md": domain.FormatMD, ".MD": domain.FormatMD,
		".txt": domain.FormatTXT, ".html": domain.FormatHTML, ".htm": domain.FormatHTML,
		".pdf": domain.FormatPDF,
	} {
		got, ok := domain.ParseFormat(ext)
		if !ok || got != want {
			t.Errorf("ParseFormat(%q) = %q,%v; quiero %q", ext, got, ok, want)
		}
	}
	if _, ok := domain.ParseFormat(".exe"); ok {
		t.Error("ParseFormat(.exe) debería rechazarse")
	}
}
