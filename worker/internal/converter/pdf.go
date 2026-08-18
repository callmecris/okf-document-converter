package converter

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// PdfConverter extrae y segmenta PDFs con 3 niveles de fallback:
//
//	Nivel 1: marcadores/tabla de contenido del PDF (pdfcpu bookmarks export,
//	incluye página → contenido real por sección).
//	Nivel 2: heurística de encabezados por tamaño de fuente (pdftohtml -xml,
//	fontspec) y patrón numérico de sección.
//	Nivel 3: fallback por rangos de N páginas (pdftotext) - nunca falla.
type PdfConverter struct {
	ChunkPages int
}

func (p *PdfConverter) Convert(ctx context.Context, opts Options) ([]Segment, error) {
	if segments, err := p.level1Bookmarks(ctx, opts); err == nil && len(segments) > 0 {
		return segments, nil
	}
	if segments, err := p.level2Heuristics(ctx, opts); err == nil && len(segments) > 0 {
		return segments, nil
	}
	return p.level3PageChunks(ctx, opts)
}

// --- Nivel 1: marcadores/tabla de contenido del PDF ---
//
// Usa la salida JSON de `pdfcpu bookmarks export` (v0.15+) que incluye la
// página de cada marcador; con eso se extrae el contenido real de cada
// sección mediante las líneas reconstruidas del XML (buildPdfLines).

type pdfBookmark struct {
	Title string        `json:"title"`
	Page  int           `json:"page"`
	Kids  []pdfBookmark `json:"kids"`
}

type pdfBookmarksFile struct {
	Bookmarks []pdfBookmark `json:"bookmarks"`
}

func (p *PdfConverter) level1Bookmarks(ctx context.Context, opts Options) ([]Segment, error) {
	raw, err := runBookmarks(ctx, opts.SourcePath)
	if err != nil {
		return nil, err
	}

	var data pdfBookmarksFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse bookmarks json: %w", err)
	}

	var flat []pdfBookmark
	flattenBookmarks(data.Bookmarks, 0, &flat)
	if len(flat) == 0 {
		return nil, fmt.Errorf("pdf sin marcadores")
	}

	// Elimina marcadores duplicados consecutivos (Typst exporta "9." doble).
	var unique []pdfBookmark
	for _, bm := range flat {
		name := normalizeText(bm.Title)
		if bm.Page < 1 || (len(unique) > 0 && normalizeText(unique[len(unique)-1].Title) == name) {
			continue
		}
		unique = append(unique, bm)
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("pdf sin marcadores utilizables")
	}

	// Reconstruye las líneas del documento y localiza cada marcador.
	lines, err := p.pdfLinesFromXML(ctx, opts)
	if err != nil {
		return nil, err
	}

	starts := make([]int, len(unique))
	for i, bm := range unique {
		starts[i] = lineForBookmark(lines, bm)
	}
	// Mantener orden estricto (marcadores en posiciones raras): si un
	// comienzo queda antes que el anterior, se fuerza al siguiente.
	for i := 1; i < len(starts); i++ {
		if starts[i] <= starts[i-1] {
			starts[i] = starts[i-1] + 1
		}
	}
	if starts[len(starts)-1] >= len(lines) {
		starts[len(starts)-1] = len(lines) - 1
	}

	segments := make([]Segment, 0, len(unique)+1)
	// Contenido previo al primer marcador (portada, títulos, tablas).
	if starts[0] > 0 {
		seg, err := writeSegment(opts.WorkDir, 1, "Introducción", renderPdfLines(lines[:starts[0]]))
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	for i, start := range starts {
		beg := start + 1 // el título del marcador es el encabezado de la sección
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		content := renderPdfLines(lines[beg:end])
		// Marcador "padre" seguido de sub-secciones sin contenido propio: se
		// omite para no generar conceptos vacíos.
		if content == "" && len(starts) > 1 {
			continue
		}
		seg, err := writeSegment(opts.WorkDir, len(segments)+1, cleanTitle(unique[i].Title), content)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

func runBookmarks(ctx context.Context, srcPath string) ([]byte, error) {
	// Sintaxis nueva (pdfcpu >= 0.15): bookmarks export <file> -
	if out, err := exec.CommandContext(ctx, "pdfcpu", "bookmarks", "export", srcPath, "-").CombinedOutput(); err == nil {
		return out, nil
	}
	// Sintaxis antigua (pdfcpu <= 0.14): outline -o <file> <pdf>
	outlinePath := filepath.Join(filepath.Dir(srcPath), "outline.txt")
	cmd := exec.CommandContext(ctx, "pdfcpu", "outline", "-o", outlinePath, srcPath)
	if _, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdfcpu bookmarks: %w", err)
	}
	raw, err := os.ReadFile(outlinePath)
	if err != nil {
		return nil, fmt.Errorf("read outline: %w", err)
	}
	// Convierte el texto indentado "- Título" al JSON mínimo esperado,
	// asignando páginas secuenciales (no disponibles en este formato).
	var bms []pdfBookmark
	page := 1
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		depth := 0
		for strings.HasPrefix(line, "  ") {
			depth++
			line = line[2:]
		}
		line = strings.TrimPrefix(line, "- ")
		if line == "" || depth > 1 {
			continue
		}
		if title := strings.TrimSpace(line); title != "" {
			bms = append(bms, pdfBookmark{Title: title, Page: page})
			page++
		}
	}
	js, err := json.Marshal(pdfBookmarksFile{Bookmarks: bms})
	if err != nil {
		return nil, err
	}
	return js, nil
}

func flattenBookmarks(bms []pdfBookmark, depth int, out *[]pdfBookmark) {
	for _, bm := range bms {
		if bm.Page >= 0 && bm.Title != "" {
			*out = append(*out, bm)
		}
		if depth < 2 {
			flattenBookmarks(bm.Kids, depth+1, out)
		}
	}
}

// lineForBookmarks localiza la línea que contiene el título del marcador en
// su página. Si no la encuentra, usa el inicio de la página como aproximación.
func lineForBookmark(lines []pdfLine, bm pdfBookmark) int {
	target := normalizeText(bm.Title)
	for i, ln := range lines {
		if ln.Page != bm.Page {
			continue
		}
		text := normalizeText(ln.Text)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, target) || strings.HasPrefix(target, text) && len(target)-len(text) < 30 {
			return i
		}
	}
	// No encontrado: primera línea de la página del marcador.
	for i, ln := range lines {
		if ln.Page == bm.Page {
			return i
		}
	}
	return 0
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.TrimSpace(s)
}

// --- Nivel 2: heurística de encabezados por tamaño de fuente ---
//
// pdftohtml -xml informa los tamaños en <fontspec>, no en cada <text>, por lo
// que primero se construye el mapa fuente->tamaño y luego se agrupan los runs
// en líneas (misma página y coordenada top). Un encabezado es una línea cuyo
// tamaño de fuente supera el percentil del cuerpo del documento. Cada sección
// conserva su texto real (líneas entre encabezados) en lugar de un lorem.

type pdfFontSpec struct {
	ID     string  `xml:"id,attr"`
	Size   float64 `xml:"size,attr"`
	Family string  `xml:"family,attr"`
}

type pdfText struct {
	Top   string  `xml:"top,attr"`
	Left  string  `xml:"left,attr"`
	Width string  `xml:"width,attr"`
	Font  string  `xml:"font,attr"`
	Text  string  `xml:",chardata"`
}

type pdfPage struct {
	Number string    `xml:"number,attr"`
	Texts  []pdfText `xml:"text"`
}

type pdfDocument struct {
	Fonts []pdfFontSpec `xml:"fontspec"`
	Pages []pdfPage     `xml:"page"`
}

// pdfLine es una línea visual reconstruida en una página de un PDF.
// Size es el mayor tamaño de fuente presente en la línea.
// Cols son las posiciones izquierdas de cada columna (runs) y Cells sus textos.
type pdfLine struct {
	Page  int
	Top   float64
	Left  float64
	Size  float64
	Text  string
	Cols  []float64
	Cells []string
}

// pdfHeading es un encabezado detectado con su posición para ordenar secciones.
type pdfHeading struct {
	Index int
	Page  int
	Top   float64
	Title string
}

var numberedSectionRe = regexp.MustCompile(`^\d+(\.\d+)*[.)]?\s+\S`)

func (p *PdfConverter) level2Heuristics(ctx context.Context, opts Options) ([]Segment, error) {
	lines, err := p.pdfLinesFromXML(ctx, opts)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("pdf sin texto extraíble")
	}

	// Tamaño del cuerpo: mediana de todos los tamaños de línea (robusto
	// frente a títulos muy grandes o tablas con fuentes monoespaciadas).
	var sizes []float64
	for _, ln := range lines {
		if ln.Size > 0 {
			sizes = append(sizes, ln.Size)
		}
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("pdf sin tamaños de fuente extraíbles")
	}
	sort.Float64s(sizes)
	bodySize := sizes[len(sizes)/2]

	// Un encabezado se distingue por tamaño (>= body*1.3 o +6pt mínimo) o por
	// patrón numérico de sección (p. ej. "2. La tarea central", "10.1 Detalle").
	threshold := math.Max(bodySize*1.3, bodySize+6)

	var headings []pdfHeading
	for i, ln := range lines {
		text := strings.TrimSpace(ln.Text)
		if text == "" {
			continue
		}
		isBig := ln.Size >= threshold
		isSizedSection := ln.Size >= bodySize+1 && numberedSectionRe.MatchString(text)
		if !isBig && !isSizedSection {
			continue
		}
		// Descarta líneas demasiado largas (párrafos) y números de página.
		if len(text) > 120 || !hasLetter(text) {
			continue
		}
		headings = append(headings, pdfHeading{
			Index: i,
			Page:  ln.Page,
			Top:   ln.Top,
			Title: cleanTitle(text),
		})
	}

	// Sin estructura detectada: el conversor cae al nivel 3.
	if len(headings) == 0 {
		return nil, fmt.Errorf("sin encabezados detectados en el pdf")
	}

	return segmentsFromHeadings(opts, lines, headings)
}

// segmentsFromHeadings construye los segmentos cortando en los índices de los
// encabezados. El contenido anterior al primer encabezado (portada, tabla de
// metadatos) se conserva como segmento "Introducción".
func segmentsFromHeadings(opts Options, lines []pdfLine, headings []pdfHeading) ([]Segment, error) {
	starts := make([]int, len(headings))
	for i, h := range headings {
		starts[i] = h.Index
	}

	segments := make([]Segment, 0, len(headings)+1)
	// Contenido anterior al primer encabezado (portada, títulos, tablas).
	if starts[0] > 0 {
		if intro := renderPdfLines(lines[:starts[0]]); intro != "" {
			seg, err := writeSegment(opts.WorkDir, 1, "Introducción", intro)
			if err != nil {
				return nil, err
			}
			segments = append(segments, seg)
		}
	}
	for i, start := range starts {
		beg := start + 1 // el propio encabezado se convierte en el título
		end := len(lines)
		if i+1 < len(headings) {
			end = starts[i+1]
		}
		content := renderPdfLines(lines[beg:end])
		// Un encabezado "padre" seguido de sub-secciones no deja contenido:
		// se omite para no generar conceptos vacíos (salvo que sea el único).
		if content == "" && len(starts) > 1 {
			continue
		}
		seg, err := writeSegment(opts.WorkDir, len(segments)+1, headings[i].Title, content)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

// pdfLinesFromXML ejecuta pdftohtml -xml y reconstruye las líneas visuales
// del documento con su tamaño de fuente, página y coordenadas.
func (p *PdfConverter) pdfLinesFromXML(ctx context.Context, opts Options) ([]pdfLine, error) {
	xmlPath := filepath.Join(opts.WorkDir, "pdf.xml")
	cmd := exec.CommandContext(ctx, "pdftohtml", "-xml", "-i", "-q", "-noframes", opts.SourcePath, xmlPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftohtml: %v: %s", err, output)
	}

	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, fmt.Errorf("read pdf xml: %w", err)
	}

	var doc pdfDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse pdf xml: %w", err)
	}

	// Mapa de fuentes: pdftohtml declara el tamaño en <fontspec>, y cada
	// <text font="N"> referencia a esa fuente por id.
	fontSizes := make(map[string]float64, len(doc.Fonts))
	for _, f := range doc.Fonts {
		if f.Size > 0 {
			fontSizes[f.ID] = f.Size
		}
	}

	return buildPdfLines(doc, fontSizes), nil
}

// buildPdfLines agrupa los runs de texto en líneas visuales ordenadas por
// (página, top), uniendo runs de la misma línea con espacios simples.
func buildPdfLines(doc pdfDocument, fontSizes map[string]float64) []pdfLine {
	type run struct {
		top   float64
		left  float64
		width float64
		size  float64
		text  string
	}

	var pages []struct {
		run
		pageNum int
	}
	for _, page := range doc.Pages {
		pn, _ := strconv.Atoi(page.Number)
		if pn == 0 {
			pn = len(pages) + 1
		}
		for _, t := range page.Texts {
			top, _ := strconv.ParseFloat(t.Top, 64)
			left, _ := strconv.ParseFloat(t.Left, 64)
			width, _ := strconv.ParseFloat(t.Width, 64)
			text := strings.TrimSpace(t.Text)
			if text == "" {
				continue
			}
			pages = append(pages, struct {
				run
				pageNum int
			}{
				run:     run{top: top, left: left, width: width, size: fontSizes[t.Font], text: text},
				pageNum: pn,
			})
		}
	}

	sort.Slice(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]
		if a.pageNum != b.pageNum {
			return a.pageNum < b.pageNum
		}
		if a.top != b.top {
			return a.top < b.top
		}
		return a.left < b.left
	})

	var lines []pdfLine
	for i := 0; i < len(pages); {
		j := i
		maxLeft := pages[i].left
		maxTop := pages[i].top
		maxSize := pages[i].size
		for j < len(pages) && pages[j].pageNum == pages[i].pageNum && absF(pages[j].top-pages[i].top) < 1 {
			maxSize = math.Max(maxSize, pages[j].size)
			maxLeft = math.Min(maxLeft, pages[j].left)
			maxTop = pages[j].top
			j++
		}

		var parts []string
		var cols []float64
		var cells []string
		prevEnd := 0.0
		for k := i; k < j; k++ {
			// Si el run anterior no termina aquí, hubo salto de línea dentro
			// de la misma top (columnas): usa la distancia para espaciado.
			if k > i {
				gap := pages[k].left - prevEnd
				// Runs solapados (columnas muy juntas, superíndices) producen
				// gaps negativos: el conteo nunca puede ser menor que 1.
				spaceCount := 1 + int((gap-2)/3) // ~3 unidades por espacio
				if spaceCount < 1 {
					spaceCount = 1
				}
				if spaceCount > 4 {
					spaceCount = 1
				}
				parts = append(parts, strings.Repeat(" ", spaceCount))
			}
			parts = append(parts, pages[k].text)
			cols = append(cols, pages[k].left)
			cells = append(cells, pages[k].text)
			prevEnd = pages[k].left + pages[k].width
		}

		lines = append(lines, pdfLine{
			Page:  pages[i].pageNum,
			Top:   maxTop,
			Left:  maxLeft,
			Size:  maxSize,
			Text:  strings.Join(parts, ""),
			Cols:  cols,
			Cells: cells,
		})
		i = j
	}
	return lines
}

// renderPdfLines une las líneas de una sección y normaliza el texto para
// markdown: une palabras partidas por justificación, detecta bloques de
// columnas alineadas (tablas) y los convierte a tablas markdown.
func renderPdfLines(lines []pdfLine) string {
	var b strings.Builder
	for i := 0; i < len(lines); {
		if n, ok := tableBlock(lines, i); ok {
			b.WriteString(tableMarkdown(lines[i : i+n]))
			b.WriteString("\n\n")
			i += n
			continue
		}
		text := cleanPdfText(lines[i].Text)
		if text != "" {
			b.WriteString(text)
			b.WriteString("\n")
		}
		i++
	}
	out := b.String()
	out = collapseTrailingHyphens(out)
	return strings.TrimSpace(out)
}

// tableBlock devuelve la longitud de un bloque de líneas que forman una tabla:
// al menos dos líneas consecutivas, en la misma página, con dos o más columnas
// alineadas (mismo left inicial, tolerancia de 4 unidades).
func tableBlock(lines []pdfLine, start int) (int, bool) {
	if start+1 >= len(lines) {
		return 0, false
	}
	first := lines[start]
	if len(first.Cells) < 2 || lines[start+1].Page != first.Page {
		return 0, false
	}
	n := 1
	prevPage := first.Page
	for i := start + 1; i < len(lines); i++ {
		ln := lines[i]
		if ln.Page != prevPage || len(ln.Cells) < 2 {
			break
		}
		if absF(ln.Cols[0]-first.Cols[0]) > 4 {
			break
		}
		n++
	}
	return n, n >= 2
}

// tableMarkdown convierte líneas de columnas alineadas a una tabla markdown.
func tableMarkdown(rows []pdfLine) string {
	var b strings.Builder
	for i, row := range rows {
		cells := make([]string, len(row.Cells))
		for j, c := range row.Cells {
			cells[j] = strings.TrimSpace(cleanPdfText(c))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			sep := make([]string, len(cells))
			for j := range sep {
				sep[j] = "---"
			}
			b.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// cleanPdfText elimina caracteres de control y normaliza espacios.
// No elimina el guión suave (U+00AD): collapseTrailingHyphens lo usa para
// re-unir palabras partidas por justificación.
func cleanPdfText(s string) string {
	s = strings.ReplaceAll(s, "\u200b", "")
	// Reemplaza múltiples espacios internos por uno, salvo comienzo de línea
	// (indentación de listas).
	s = strings.TrimRight(s, " \t")
	return s
}

// collapseTrailingHyphens une palabras partidas por un guión al final de línea
// ("conoci-\nmiento" o "docu\u00ad\nmental" → "conocimiento"/"documental").
func collapseTrailingHyphens(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		trimmed := strings.TrimRight(ln, " \t")
		joinsWithNext := false
		if i+1 < len(lines) {
			next := strings.TrimLeft(lines[i+1], " \t")
			nextStartsWord := len(next) > 1 && isLowerRune(next)
			if strings.HasSuffix(trimmed, "\u00ad") && nextStartsWord {
				trimmed = strings.TrimSuffix(trimmed, "\u00ad")
				joinsWithNext = true
			} else if strings.HasSuffix(trimmed, "-") && !strings.HasSuffix(trimmed, "\\-") && nextStartsWord {
				trimmed = strings.TrimSuffix(trimmed, "-")
				joinsWithNext = true
			}
			if joinsWithNext {
				lines[i+1] = trimmed + strings.TrimLeft(lines[i+1], " \t")
				continue
			}
		}
		_ = joinsWithNext
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

func isLowerRune(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

func cleanTitle(s string) string {
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 100 {
		s = strings.TrimSpace(s[:100])
	}
	return s
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// --- Nivel 3: fallback por rango de páginas ---

func (p *PdfConverter) level3PageChunks(ctx context.Context, opts Options) ([]Segment, error) {
	total, err := pdfPageCount(ctx, opts.SourcePath)
	if err != nil {
		return nil, err
	}

	chunkSize := p.ChunkPages
	if chunkSize < 1 {
		chunkSize = 10
	}

	segments := make([]Segment, 0)
	for start := 1; start <= total; start += chunkSize {
		end := start + chunkSize - 1
		if end > total {
			end = total
		}

		cmd := exec.CommandContext(ctx, "pdftotext", "-f", strconv.Itoa(start), "-l", strconv.Itoa(end), "-layout", opts.SourcePath, "-")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("pdftotext pages %d-%d: %w", start, end, err)
		}

		title := fmt.Sprintf("Páginas %d–%d", start, end)
		seg, err := writeSegment(opts.WorkDir, len(segments)+1, title, string(output))
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("pdf sin páginas")
	}
	return segments, nil
}

func pdfPageCount(ctx context.Context, path string) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", path)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				n, err := strconv.Atoi(fields[1])
				if err == nil && n > 0 {
					return n, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("pdfinfo sin campo Pages")
}