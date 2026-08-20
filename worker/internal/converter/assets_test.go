package converter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteMarkdownImagesPointsToAssets(t *testing.T) {
	in := "Texto\n\n![Un diagrama](media/img.png)\n\ny ![otra](sub/dir/foto.jpg \"titulo\")\n"
	out := rewriteMarkdownImages(in, func(src string) string {
		return filepath.Base(src)
	})

	if !strings.Contains(out, "![Un diagrama](../assets/img.png)") {
		t.Errorf("no reescribió la primera imagen: %q", out)
	}
	if !strings.Contains(out, `![otra](../assets/foto.jpg "titulo")`) {
		t.Errorf("no preservó el título de la segunda imagen: %q", out)
	}
}

// Las referencias remotas se dejan intactas: no son recursos del documento.
func TestRewriteMarkdownImagesKeepsRemoteRefs(t *testing.T) {
	for _, src := range []string{
		"https://ejemplo.com/a.png",
		"http://ejemplo.com/b.png",
		"data:image/png;base64,AAAA",
	} {
		in := "![x](" + src + ")"
		out := rewriteMarkdownImages(in, func(s string) string {
			if isRemoteRef(s) {
				return ""
			}
			return "local.png"
		})
		if out != in {
			t.Errorf("se reescribió una referencia remota %q -> %q", in, out)
		}
	}
}

func TestAssetCollectorDeduplicatesAndAvoidsCollisions(t *testing.T) {
	a := newAssetCollector(t.TempDir())

	n1, err := a.add("a/img.png", []byte("uno"))
	if err != nil {
		t.Fatal(err)
	}
	// Mismo origen: no debe duplicarse.
	n1b, err := a.add("a/img.png", []byte("uno"))
	if err != nil {
		t.Fatal(err)
	}
	if n1 != n1b || a.Count() != 1 {
		t.Errorf("no deduplicó: %q vs %q, count=%d", n1, n1b, a.Count())
	}

	// Distinto origen con el mismo nombre base: debe desambiguarse.
	n2, err := a.add("b/img.png", []byte("dos"))
	if err != nil {
		t.Fatal(err)
	}
	if n2 == n1 {
		t.Errorf("colisión de nombres no resuelta: ambos %q", n1)
	}
	if a.Count() != 2 {
		t.Errorf("count = %d, se esperaban 2", a.Count())
	}
	if filepath.Ext(n2) != ".png" {
		t.Errorf("se perdió la extensión: %q", n2)
	}
}

func TestSanitizeAssetName(t *testing.T) {
	for in, want := range map[string]string{
		"Foto Final.PNG":  "foto-final.png",
		"a/b/c.png":       "a-b-c.png",
		"imagen_1.jpeg":   "imagen_1.jpeg",
		"árbol.png":       "rbol.png",
	} {
		if got := sanitizeAssetName(in); got != want {
			t.Errorf("sanitizeAssetName(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// Un markdown con una imagen local acaba con el recurso en assets/ y el
// enlace apuntando allí.
func TestTextConverterExtractsLocalImage(t *testing.T) {
	workDir := t.TempDir()
	src := filepath.Join(workDir, "doc.md")
	img := filepath.Join(workDir, "diagrama.png")

	if err := os.WriteFile(img, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# Uno\n\n![Diagrama](diagrama.png)\n\nContenido del concepto.\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	assets := newAssetCollector(workDir)
	segments, err := (&TextConverter{}).Convert(context.Background(), Options{
		SourcePath: src,
		WorkDir:    workDir,
		Assets:     assets,
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if assets.Count() != 1 {
		t.Fatalf("recursos extraídos = %d, se esperaba 1", assets.Count())
	}

	copied := filepath.Join(workDir, "assets", "diagrama.png")
	if data, err := os.ReadFile(copied); err != nil || string(data) != "PNGDATA" {
		t.Errorf("el recurso no se copió a assets/: %v", err)
	}

	seg, err := os.ReadFile(segments[0].File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seg), "../assets/diagrama.png") {
		t.Errorf("el concepto no referencia el asset: %s", seg)
	}
}

// Una imagen inexistente no rompe la conversión: se deja la referencia tal cual.
func TestAssetsMissingImageDoesNotFail(t *testing.T) {
	workDir := t.TempDir()
	src := filepath.Join(workDir, "doc.md")
	if err := os.WriteFile(src, []byte("# Uno\n\n![x](no-existe.png)\n\nTexto.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	assets := newAssetCollector(workDir)
	if _, err := (&TextConverter{}).Convert(context.Background(), Options{
		SourcePath: src, WorkDir: workDir, Assets: assets,
	}); err != nil {
		t.Fatalf("una imagen rota no debe fallar la conversión: %v", err)
	}
	if assets.Count() != 0 {
		t.Errorf("no debería haber extraído recursos, count=%d", assets.Count())
	}
}
