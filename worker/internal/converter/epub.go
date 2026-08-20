package converter

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

// EpubConverter extrae capítulos de un EPUB:
//  1. Leyendo toc.ncx (índice con títulos y orden exacto).
//  2. Fallback a nav.xhtml (navigation document).
//  3. Fallback: todos los XHTML ordenados por nombre.
type EpubConverter struct{}

// --- toc.ncx ---

type ncxXML struct {
	XMLName xml.Name   `xml:"ncx"`
	NavMap  ncxNavMap  `xml:"navMap"`
}

type ncxNavMap struct {
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

type ncxNavPoint struct {
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

// --- nav.xhtml ---

type epubEntry struct {
	title string
	src   string
}

func (e *EpubConverter) Convert(ctx context.Context, opts Options) ([]Segment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	zr, err := zip.OpenReader(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer zr.Close()

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			files[f.Name] = f
		}
	}

	entries, err := e.navFromNCX(zr)
	if err != nil || len(entries) == 0 {
		entries, err = e.navFromNavXHTML(files)
	}
	if err != nil || len(entries) == 0 {
		entries = e.fallbackEntries(files)
	}
	if len(entries) == 0 {
		return nil, errors.New("epub sin índice ni contenido HTML")
	}

	segments := make([]Segment, 0, len(entries))
	// order numera los capítulos efectivamente escritos: si un capítulo
	// referenciado en el índice no existe en el zip se omite, y usar el
	// índice del entry dejaría huecos (capitulo-01, capitulo-02, capitulo-04).
	order := 0
	for _, entry := range entries {
		srcFile := e.resolveChapter(files, entry.src)
		if srcFile == nil {
			continue // capítulo referenciado pero ausente: se omite
		}
		order++
		rc, err := srcFile.Open()
		if err != nil {
			return nil, fmt.Errorf("open chapter %s: %w", entry.src, err)
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			rc.Close()
			return nil, fmt.Errorf("read chapter %s: %w", entry.src, err)
		}
		rc.Close()

		markdown, err := md.NewConverter("", true, nil).ConvertString(buf.String())
		if err != nil {
			return nil, fmt.Errorf("html to markdown %s: %w", entry.src, err)
		}

		title := entry.title
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("Capítulo %d", order)
		}
		seg, err := writeSegment(opts.WorkDir, order, title, markdown)
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

func (e *EpubConverter) navFromNCX(zr *zip.ReadCloser) ([]epubEntry, error) {
	// El toc.ncx no está siempre en la raíz: depende del OPF (OEBPS/toc.ncx,
	// content/toc.ncx, etc.). Se busca por nombre base en todo el archivo.
	var toc *zip.File
	for _, f := range zr.File {
		if strings.EqualFold(path.Base(f.Name), "toc.ncx") {
			toc = f
			break
		}
	}
	if toc == nil {
		return nil, errors.New("toc.ncx no encontrado")
	}

	f, err := toc.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var ncx ncxXML
	if err := xml.NewDecoder(f).Decode(&ncx); err != nil {
		return nil, fmt.Errorf("decode toc.ncx: %w", err)
	}

	flat := make([]ncxNavPoint, 0)
	var flatten func(points []ncxNavPoint)
	flatten = func(points []ncxNavPoint) {
		for _, p := range points {
			flat = append(flat, p)
			flatten(p.NavPoints)
		}
	}
	flatten(ncx.NavMap.NavPoints)

	entries := make([]epubEntry, 0, len(flat))
	for _, p := range flat {
		entries = append(entries, epubEntry{
			title: strings.TrimSpace(p.NavLabel.Text),
			src:   strings.TrimSpace(p.Content.Src),
		})
	}
	return entries, nil
}

func (e *EpubConverter) navFromNavXHTML(files map[string]*zip.File) ([]epubEntry, error) {
	var navFile *zip.File
	for _, name := range []string{"nav.xhtml", "nav.html", "toc.xhtml", "toc.html"} {
		if f, ok := files[name]; ok {
			navFile = f
			break
		}
	}
	if navFile == nil {
		for name, f := range files {
			if !strings.HasSuffix(strings.ToLower(name), ".html") && !strings.HasSuffix(strings.ToLower(name), ".xhtml") {
				continue
			}
			if rc, err := f.Open(); err == nil {
				doc, _ := html.Parse(rc)
				rc.Close()
				if findNav(doc) != nil {
					navFile = f
					break
				}
			}
		}
	}
	if navFile == nil {
		return nil, errors.New("no se encontró toc.ncx ni nav document")
	}

	rc, err := navFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	doc, err := html.Parse(rc)
	if err != nil {
		return nil, fmt.Errorf("parse nav %s: %w", navFile.Name, err)
	}
	nav := findNav(doc)
	if nav == nil {
		return nil, errors.New("nav document sin elemento <nav>")
	}

	baseDir := path.Dir(navFile.Name)
	entries := make([]epubEntry, 0)
	walkNodes(nav, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "a" {
			return
		}
		href := attr(n, "href")
		if href == "" {
			return
		}
		src := href
		if baseDir != "." && !strings.HasPrefix(href, "/") {
			src = path.Join(baseDir, href)
		}
		entries = append(entries, epubEntry{title: strings.TrimSpace(textOf(n)), src: src})
	})
	return entries, nil
}

// fallbackEntries ordena todos los XHTML/HTML por nombre cuando no hay índice.
func (e *EpubConverter) fallbackEntries(files map[string]*zip.File) []epubEntry {
	var names []string
	for name := range files {
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".xhtml") || strings.HasSuffix(lower, ".html") {
			if strings.Contains(lower, "nav") || strings.Contains(lower, "toc") || strings.Contains(lower, "cover") {
				continue
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)

	chapters := make([]epubEntry, 0, len(names))
	for i, name := range names {
		chapters = append(chapters, epubEntry{
			title: fmt.Sprintf("Capítulo %d", i+1),
			src:   name,
		})
	}
	return chapters
}

// resolveChapter localiza el archivo destino (las rutas del índice suelen ser
// relativas a la raíz del contenido; si no existe, se busca por nombre base).
func (e *EpubConverter) resolveChapter(files map[string]*zip.File, src string) *zip.File {
	name := path.Clean(strings.TrimPrefix(src, "/"))
	if f, ok := files[name]; ok {
		return f
	}
	base := path.Base(name)
	for k, f := range files {
		if path.Base(k) == base {
			return f
		}
	}
	return nil
}

func findNav(n *html.Node) *html.Node {
	var result *html.Node
	walkNodes(n, func(node *html.Node) {
		if result != nil || node.Type != html.ElementNode || node.Data != "nav" {
			return
		}
		navType := attr(node, "epub:type")
		if navType == "" || strings.Contains(navType, "toc") {
			result = node
		}
	})
	return result
}

func walkNodes(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, fn)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	walkNodes(n, func(node *html.Node) {
		if node.Type == html.TextNode {
			if parent := node.Parent; parent != nil && (parent.Data == "script" || parent.Data == "style") {
				return
			}
			b.WriteString(node.Data)
			b.WriteRune(' ')
		}
	})
	return strings.TrimSpace(b.String())
}