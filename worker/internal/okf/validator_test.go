package okf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildBundle arma un bundle en disco a partir de un mapa ruta -> contenido.
func buildBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const okLog = "# Log\n- **SHA-256 (original):** abc\n- **Formato:** MD\n- **Conceptos generados:** 1\n"

func validBundle() map[string]string {
	return map[string]string{
		"index.md":                "# Índice\n\n| 1 | Uno | [capitulo-01.md](conceptos/capitulo-01.md) |\n",
		"log.md":                  okLog,
		"conceptos/capitulo-01.md": "# Uno\n\nContenido suficiente del concepto.\n",
	}
}

func TestValidateValidBundle(t *testing.T) {
	report := Validate(buildBundle(t, validBundle()))
	if report.Level != LevelValid {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
	if !report.Publishable() {
		t.Error("un bundle válido debe ser publicable")
	}
}

// Spec §6: ante la ausencia de index.md o log.md el bundle no se publica.
func TestValidateMissingRequiredFilesIsInvalid(t *testing.T) {
	for _, missing := range []string{"index.md", "log.md"} {
		files := validBundle()
		delete(files, missing)

		report := Validate(buildBundle(t, files))
		if report.Level != LevelInvalid {
			t.Errorf("sin %s: nivel = %q; se esperaba invalid", missing, report.Level)
		}
		if report.Publishable() {
			t.Errorf("sin %s: el bundle NO debe ser publicable", missing)
		}
		if report.Err == nil || !strings.Contains(report.Err.Error(), missing) {
			t.Errorf("sin %s: error poco claro: %v", missing, report.Err)
		}
	}
}

func TestValidateEmptyConceptsIsInvalid(t *testing.T) {
	files := validBundle()
	delete(files, "conceptos/capitulo-01.md")
	files["index.md"] = "# Índice\n\nsin links\n"

	root := buildBundle(t, files)
	if err := os.MkdirAll(filepath.Join(root, "conceptos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if report := Validate(root); report.Level != LevelInvalid {
		t.Errorf("nivel = %q; se esperaba invalid", report.Level)
	}
}

func TestValidateBrokenLinkIsInvalid(t *testing.T) {
	files := validBundle()
	files["index.md"] = "# Índice\n\n[roto](conceptos/no-existe.md)\n"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelInvalid {
		t.Fatalf("nivel = %q; se esperaba invalid", report.Level)
	}
	if report.Publishable() {
		t.Error("un índice con links rotos no debe publicarse")
	}
}

// Un concepto huérfano no invalida el bundle: lo degrada a "con advertencias".
func TestValidateOrphanConceptWarns(t *testing.T) {
	files := validBundle()
	files["conceptos/capitulo-02.md"] = "# Dos\n\nContenido del segundo concepto.\n"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelValidWithWarnings {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
	if !report.Publishable() {
		t.Error("un bundle con advertencias sí es publicable")
	}
	if len(report.Warnings) == 0 {
		t.Error("se esperaba al menos una advertencia")
	}
}

func TestValidateConceptWithoutHeadingWarns(t *testing.T) {
	files := validBundle()
	files["conceptos/capitulo-01.md"] = "Texto sin encabezado inicial del concepto.\n"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelValidWithWarnings {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
}

func TestValidateLogWithoutTraceabilityWarns(t *testing.T) {
	files := validBundle()
	files["log.md"] = "# Log\n\nsin metadatos\n"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelValidWithWarnings {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
	if report.Publishable() != true {
		t.Error("debe seguir siendo publicable")
	}
}

// Un recurso de assets/ referenciado pero ausente genera advertencia,
// no invalida el bundle.
func TestValidateMissingAssetWarns(t *testing.T) {
	files := validBundle()
	files["conceptos/capitulo-01.md"] = "# Uno\n\n![falta](../assets/no-existe.png)\n\nContenido del concepto.\n"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelValidWithWarnings {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
	if !report.Publishable() {
		t.Error("una imagen rota no debe impedir la publicacion")
	}
	found := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "recurso inexistente") {
			found = true
		}
	}
	if !found {
		t.Errorf("falta la advertencia de recurso: %v", report.Warnings)
	}
}

// Un recurso presente en assets/ no genera advertencia.
func TestValidateExistingAssetIsClean(t *testing.T) {
	files := validBundle()
	files["conceptos/capitulo-01.md"] = "# Uno\n\n![ok](../assets/img.png)\n\nContenido del concepto.\n"
	files["assets/img.png"] = "PNGDATA"

	report := Validate(buildBundle(t, files))
	if report.Level != LevelValid {
		t.Fatalf("nivel = %q, advertencias = %v", report.Level, report.Warnings)
	}
}
