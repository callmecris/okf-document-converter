package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// segmentMarkdown divide un documento markdown en segmentos, usando el AST de
// goldmark y cortando en cada encabezado de nivel 1 (#) o 2 (##).
// Si no hay encabezados, genera un único segmento con el documento completo
// (garantiza que el flujo siempre produzca conceptos validables).
func segmentMarkdown(workDir, srcPath string) ([]Segment, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read markdown %s: %w", srcPath, err)
	}

	doc := goldmark.New().Parser().Parse(text.NewReader(data))

	var headings []*ast.Heading
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok && h.Level <= 2 {
			headings = append(headings, h)
		}
		return ast.WalkContinue, nil
	})

	// Sin estructura de encabezados -> documento completo como único segmento.
	if len(headings) == 0 {
		title := "Documento completo"
		body := fmt.Sprintf("# %s\n\n%s\n", title, strings.TrimSpace(string(data)))
		dir := filepath.Join(workDir, "conceptos")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create conceptos dir: %w", err)
		}
		file := filepath.Join(dir, "fragmento-01.md")
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			return nil, fmt.Errorf("write segment %s: %w", file, err)
		}
		return []Segment{{Title: title, File: file, Order: 1}}, nil
	}

	// Offset de inicio de cada sección: la línea del encabezado. goldmark
	// reporta en Lines() solo el texto del heading (sin los '#'), por lo que se
	// retrocede hasta el inicio de esa línea para no perder el marcador.
	starts := make([]int, len(headings))
	for i, h := range headings {
		start := 0
		if lines := h.Lines(); lines.Len() > 0 {
			start = lineStart(data, lines.At(0).Start)
		}
		starts[i] = start
	}

	segments := make([]Segment, 0, len(headings))
	for i, h := range headings {
		title := strings.TrimSpace(string(h.Text(data)))
		if title == "" {
			title = fmt.Sprintf("Sección %d", i+1)
		}

		// La sección va desde su propio encabezado hasta el inicio del
		// siguiente (o el fin del documento para la última).
		sectionEnd := len(data)
		if i+1 < len(headings) {
			sectionEnd = starts[i+1]
		}

		// Preludio: contenido anterior al primer encabezado. Se antepone al
		// primer segmento para no perderlo.
		sectionStart := starts[i]
		if i == 0 {
			sectionStart = 0
		}

		content := strings.TrimSpace(string(data[sectionStart:sectionEnd]))
		seg, err := writeSegment(workDir, i+1, title, content)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// lineStart retrocede desde off hasta el inicio de su línea.
func lineStart(data []byte, off int) int {
	if off > len(data) {
		off = len(data)
	}
	for off > 0 && data[off-1] != '\n' {
		off--
	}
	return off
}
