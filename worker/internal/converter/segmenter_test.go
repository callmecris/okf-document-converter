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
		if !strings.HasPrefix(string(data), "# ") && !strings.HasPrefix(string(data), "## ") {
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
// Regresión: cada segmento debe contener SOLO su propia sección. El corte
// anterior arrancaba al final del encabezado previo, por lo que cada concepto
// arrastraba el cuerpo del anterior.
func TestSegmentMarkdownNoContentBleed(t *testing.T) {
	src := writeMarkdown(t, `# Uno

CUERPO_UNO

# Dos

CUERPO_DOS

# Tres

CUERPO_TRES
`)
	segments, err := segmentMarkdown(t.TempDir(), src)
	if err != nil {
		t.Fatalf("segmentMarkdown: %v", err)
	}
	if len(segments) != 3 {
		t.Fatalf("esperados 3 segmentos, got %d", len(segments))
	}

	bodies := []string{"CUERPO_UNO", "CUERPO_DOS", "CUERPO_TRES"}
	for i, seg := range segments {
		data, err := os.ReadFile(seg.File)
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		if !strings.Contains(got, bodies[i]) {
			t.Errorf("segmento %d no contiene su propio cuerpo %s: %q", i+1, bodies[i], got)
		}
		for j, other := range bodies {
			if j != i && strings.Contains(got, other) {
				t.Errorf("segmento %d contaminado con %s: %q", i+1, other, got)
			}
		}
	}
}
