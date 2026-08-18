package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMarkdown(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSegmentMarkdownSplitsAtHeadings(t *testing.T) {
	src := writeMarkdown(t, `# Introducción

Texto introductorio.

## Capítulo Uno

Contenido del capítulo uno.

# Capítulo Dos

Contenido del capítulo dos con más detalle y párrafos.

- item
- otro item
`)
	workDir := t.TempDir()
	segments, err := segmentMarkdown(workDir, src)
	if err != nil {
		t.Fatalf("segmentMarkdown: %v", err)
	}

	if len(segments) != 3 {
		t.Fatalf("esperados 3 segmentos, got %d", len(segments))
	}

	// Cada segmento debe incluir su propio encabezado.
	for i, seg := range segments {
		data, err := os.ReadFile(seg.File)
		if err != nil {
			t.Fatalf("read segment %d: %v", i, err)
		}
		if !strings.HasPrefix(string(data), "# ") {
			t.Errorf("segmento %d no empieza con heading: %s", i, data[:20])
		}
	}

	if got := segments[0].Title; got != "Introducción" {
		t.Errorf("título 1 = %q", got)
	}
	if got := segments[2].Title; got != "Capítulo Dos" {
		t.Errorf("título 3 = %q", got)
	}

	// El tercer segmento debe incluir la lista final.
	data, _ := os.ReadFile(segments[2].File)
	if !strings.Contains(string(data), "- item") {
		t.Error("el tercer segmento perdió contenido al final del documento")
	}
}

func TestSegmentMarkdownSingleSegmentWithoutHeadings(t *testing.T) {
	src := writeMarkdown(t, "Texto plano sin encabezados.\nContinúa igual.\n")
	workDir := t.TempDir()
	segments, err := segmentMarkdown(workDir, src)
	if err != nil {
		t.Fatalf("segmentMarkdown: %v", err)
	}

	if len(segments) != 1 {
		t.Fatalf("esperado 1 segmento, got %d", len(segments))
	}
	data, err := os.ReadFile(segments[0].File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Texto plano sin encabezados") {
		t.Errorf("segmento sin heading no conserva el contenido: %s", data)
	}
}