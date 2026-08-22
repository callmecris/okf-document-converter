package converter

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// El XML de pdftohtml con imágenes debe producir pseudo-líneas ImgSrc en su
// posición, aplicando los filtros de tamaño y cobertura.
func TestBuildPdfLinesWithImages(t *testing.T) {
	doc := pdfDocument{
		Pages: []pdfPage{
			{
				Number: "1", Width: "800", Height: "600",
				Texts: []pdfText{
					{Top: "10", Left: "10", Width: "200", Font: "0", Text: "Título"},
					{Top: "50", Left: "10", Width: "400", Font: "1", Text: "Párrafo"},
				},
				Images: []pdfImage{
					// Válida: se conserva.
					{Top: "30", Left: "100", Width: "300", Height: "150", Src: "pdf-000.png"},
					// Demasiado pequeña: descartada.
					{Top: "70", Left: "10", Width: "10", Height: "10", Src: "pdf-001.png"},
					// Cubre toda la página (fondo/watermark): descartada.
					{Top: "0", Left: "0", Width: "799", Height: "599", Src: "pdf-002.png"},
					// Sin archivo asociado: descartada.
					{Top: "80", Left: "10", Width: "100", Height: "100", Src: ""},
				},
			},
		},
		Fonts: []pdfFontSpec{{ID: "0", Size: 20}, {ID: "1", Size: 11}},
	}

	lines := buildPdfLines(doc, map[string]float64{"0": 20, "1": 11})
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, se esperaban 3 (título, imagen, párrafo)", len(lines))
	}

	var imgAt int
	for i, ln := range lines {
		if ln.ImgSrc != "" {
			imgAt = i
			if ln.Text != "" || ln.Page != 1 {
				t.Errorf("pseudo-línea de imagen mal formada: %+v", ln)
			}
		}
	}
	// La imagen va entre el título (top 10) y el párrafo (top 50).
	if imgAt != 1 {
		t.Errorf("la imagen quedó en la posición %d, se esperaba 1", imgAt)
	}
}

// El límite de imágenes por documento evita PDFs patológicos.
func TestBuildPdfLinesImageLimit(t *testing.T) {
	page := pdfPage{Number: "1", Width: "8000", Height: "60000"}
	for i := 0; i < pdfImageLimit+50; i++ {
		page.Images = append(page.Images, pdfImage{
			Top:    strconv.Itoa(i * 40),
			Left:   "10",
			Width:  "50",
			Height: "40",
			Src:    "img-" + strconv.Itoa(i) + ".png",
		})
	}
	doc := pdfDocument{Pages: []pdfPage{page}}
	lines := buildPdfLines(doc, nil)

	count := 0
	for _, ln := range lines {
		if ln.ImgSrc != "" {
			count++
		}
	}
	if count != pdfImageLimit {
		t.Errorf("imágenes aceptadas = %d, se esperaban %d (tope)", count, pdfImageLimit)
	}
}

// renderPdfLines emite las referencias markdown para las pseudo-líneas.
func TestRenderPdfLinesEmitsImages(t *testing.T) {
	lines := []pdfLine{
		{Page: 1, Top: 10, Text: "Intro", Cols: []float64{10}, Cells: []string{"Intro"}},
		{Page: 1, Top: 20, ImgSrc: "pdf-000.png"},
		{Page: 1, Top: 30, Text: "Cuerpo", Cols: []float64{10}, Cells: []string{"Cuerpo"}},
	}
	out := renderPdfLines(lines)
	if !strings.Contains(out, "![imagen](pdf-000.png)") {
		t.Errorf("no se emitió la referencia de imagen: %q", out)
	}
	if !strings.Contains(out, "Intro") || !strings.Contains(out, "Cuerpo") {
		t.Errorf("se perdió texto alrededor de la imagen: %q", out)
	}
}

// resolvePdfImages copia los archivos extraídos a assets/ y reescribe las
// rutas; las referencias remotas y rotas quedan como están.
func TestResolvePdfImages(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "pdf-000.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{WorkDir: workDir, Assets: newAssetCollector(workDir)}

	in := "# Sec\n\n![imagen](pdf-000.png)\n\n![rota](falta.png)\n\n![web](https://ejemplo.com/a.png)\n"
	out := resolvePdfImages(opts, in)

	if !strings.Contains(out, "../assets/pdf-000.png") {
		t.Errorf("no se reescribió la imagen existente: %q", out)
	}
	if !strings.Contains(out, "![rota](falta.png)") {
		t.Errorf("la referencia rota debía conservarse: %q", out)
	}
	if !strings.Contains(out, "![web](https://ejemplo.com/a.png)") {
		t.Errorf("la referencia remota debía conservarse: %q", out)
	}
	if opts.Assets.Count() != 1 {
		t.Errorf("assets = %d, se esperaba 1", opts.Assets.Count())
	}
	copied := filepath.Join(workDir, "assets", "pdf-000.png")
	if data, err := os.ReadFile(copied); err != nil || string(data) != "PNGDATA" {
		t.Errorf("el recurso no llegó a assets/: %v", err)
	}
}

// Contenido idéntico desde rutas distintas ocupa un solo asset.
func TestAssetCollectorDeduplicatesByContent(t *testing.T) {
	a := newAssetCollector(t.TempDir())

	n1, err := a.add("p1/logo.png", []byte("MISMO-CONTENIDO"))
	if err != nil {
		t.Fatal(err)
	}
	n2, err := a.add("p45/logo.png", []byte("MISMO-CONTENIDO"))
	if err != nil {
		t.Fatal(err)
	}
	if n1 != n2 || a.Count() != 1 {
		t.Errorf("contenido duplicado no colapsó: %q vs %q, count=%d", n1, n2, a.Count())
	}

	if _, err := a.add("otro.png", []byte("OTRO")); err != nil {
		t.Fatal(err)
	}
	if a.Count() != 2 {
		t.Errorf("count = %d, se esperaban 2", a.Count())
	}
}
