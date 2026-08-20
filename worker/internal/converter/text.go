package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

// TextConverter maneja los formatos de texto con estructura detectable por
// encabezados: Markdown, texto plano y HTML. Es el formato base exigido por el
// alcance mínimo; no requiere binarios externos.
//
//   - .md  se segmenta directamente.
//   - .txt se normaliza a markdown (líneas MAYÚSCULAS o "1. Título" pasan a #).
//   - .html se convierte a markdown y luego se segmenta.
type TextConverter struct {
	// HTML indica que la entrada debe convertirse de HTML a Markdown.
	HTML bool
	// PlainText indica que deben promoverse encabezados heurísticos.
	PlainText bool
}

func (t *TextConverter) Convert(ctx context.Context, opts Options) ([]Segment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}
	content := string(data)

	switch {
	case t.HTML:
		content, err = md.NewConverter("", true, nil).ConvertString(content)
		if err != nil {
			return nil, fmt.Errorf("html to markdown: %w", err)
		}
	case t.PlainText:
		content = promoteHeadings(content)
	}

	mdPath := filepath.Join(opts.WorkDir, "document.md")
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write markdown: %w", err)
	}

	// Imágenes locales referenciadas junto al documento de entrada.
	if err := collectMarkdownAssets(mdPath, opts); err != nil {
		return nil, fmt.Errorf("extraer recursos: %w", err)
	}

	segments, err := segmentMarkdown(opts.WorkDir, mdPath)
	if err != nil {
		return nil, fmt.Errorf("segment text markdown: %w", err)
	}
	return segments, nil
}

// promoteHeadings convierte a encabezados markdown las líneas de un .txt que
// se comportan como títulos: cortas, aisladas y en mayúsculas o numeradas
// ("1. Introducción", "CAPITULO I"). Si no detecta ninguna, el documento
// queda como un único concepto (comportamiento válido para documento breve).
func promoteHeadings(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			out = append(out, raw)
			continue
		}
		// Un título va precedido y seguido de línea en blanco (o extremo).
		blankBefore := i == 0 || strings.TrimSpace(lines[i-1]) == ""
		blankAfter := i == len(lines)-1 || strings.TrimSpace(lines[i+1]) == ""

		if blankBefore && blankAfter && len(line) <= 80 && isHeadingLike(line) {
			out = append(out, "# "+line)
			continue
		}
		out = append(out, raw)
	}
	return strings.Join(out, "\n")
}

// isHeadingLike reconoce títulos en MAYÚSCULAS o con prefijo numérico
// ("1.", "1.2", "IV."), siempre que no terminen en punto de oración.
func isHeadingLike(line string) bool {
	if strings.HasSuffix(line, ".") && strings.Count(line, " ") > 8 {
		return false
	}
	letters := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, line)
	if letters != "" && letters == strings.ToUpper(letters) {
		return true
	}

	head := strings.SplitN(line, " ", 2)[0]
	head = strings.TrimSuffix(head, ".")
	if head == "" {
		return false
	}
	for _, r := range head {
		if !strings.ContainsRune("0123456789.IVXLC", r) {
			return false
		}
	}
	return strings.Contains(line, " ")
}
