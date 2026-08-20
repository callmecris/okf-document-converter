package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	linkRe = regexp.MustCompile(`\]\(([^)]+)\)`)
	// imgRe captura las imágenes de los conceptos: ![alt](ruta).
	imgRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)`)
)

// Level clasifica el resultado de la validación, separando la validez de
// plataforma (¿se puede publicar?) de la conformidad OKF (¿es un buen bundle?).
type Level string

const (
	// LevelValid: estructura mínima correcta y sin observaciones.
	LevelValid Level = "valid"
	// LevelValidWithWarnings: publicable, pero con observaciones de conformidad.
	LevelValidWithWarnings Level = "valid_with_warnings"
	// LevelInvalid: no cumple la estructura mínima; NO se publica.
	LevelInvalid Level = "invalid"
)

// Report es el resultado de validar un bundle.
type Report struct {
	Level    Level
	Warnings []string
	// Err explica por qué el bundle es inválido (nil si es publicable).
	Err error
}

// Publishable indica si el bundle puede subirse y ofrecerse en descarga.
func (r Report) Publishable() bool { return r.Level != LevelInvalid }

// Validate verifica un bundle OKF y clasifica el resultado.
//
// Invalidan la publicación (estructura mínima, spec §5.1.6):
//   - falta index.md o log.md
//   - falta conceptos/ o está vacío
//   - un link relativo de index.md no resuelve
//
// Solo generan advertencia (conformidad OKF):
//   - un concepto no aparece enlazado en index.md
//   - un concepto está vacío o no empieza por encabezado
//   - log.md sin los metadatos de trazabilidad esperados
//   - un concepto referencia un recurso de assets/ que no existe
func Validate(root string) Report {
	for _, required := range []string{"index.md", "log.md"} {
		if info, err := os.Stat(filepath.Join(root, required)); err != nil || info.IsDir() {
			return Report{Level: LevelInvalid, Err: fmt.Errorf("bundle incompleto: falta %s", required)}
		}
	}

	conceptsDir := filepath.Join(root, "conceptos")
	entries, err := os.ReadDir(conceptsDir)
	if err != nil {
		return Report{Level: LevelInvalid, Err: fmt.Errorf("bundle incompleto: falta conceptos/: %w", err)}
	}

	concepts := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			concepts = append(concepts, e.Name())
		}
	}
	if len(concepts) == 0 {
		return Report{Level: LevelInvalid, Err: fmt.Errorf("bundle inválido: conceptos/ vacío")}
	}
	sort.Strings(concepts)

	index, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		return Report{Level: LevelInvalid, Err: fmt.Errorf("read index.md: %w", err)}
	}

	// Los links del índice deben resolver: un índice roto invalida el bundle.
	linked := map[string]bool{}
	for _, link := range linkRe.FindAllStringSubmatch(string(index), -1) {
		target := link[1]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		resolved := filepath.Clean(filepath.Join(root, target))
		if info, err := os.Stat(resolved); err != nil || info.IsDir() {
			return Report{Level: LevelInvalid, Err: fmt.Errorf("link roto en index.md: %q", target)}
		}
		linked[filepath.Base(resolved)] = true
	}

	var warnings []string

	// Conceptos huérfanos: existen pero no se navegan desde el índice.
	for _, name := range concepts {
		if !linked[name] {
			warnings = append(warnings, fmt.Sprintf("el concepto %q no está enlazado desde index.md", name))
		}
	}

	// Conceptos vacíos o sin encabezado.
	for _, name := range concepts {
		data, err := os.ReadFile(filepath.Join(conceptsDir, name))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("no se pudo leer el concepto %q", name))
			continue
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			warnings = append(warnings, fmt.Sprintf("el concepto %q está vacío", name))
			continue
		}
		if !strings.HasPrefix(body, "#") {
			warnings = append(warnings, fmt.Sprintf("el concepto %q no empieza con un encabezado", name))
		}
		if len(strings.Fields(body)) < 3 {
			warnings = append(warnings, fmt.Sprintf("el concepto %q casi no tiene contenido", name))
		}
	}

	// Recursos referenciados por los conceptos: una referencia rota a assets/
	// no invalida el bundle (el texto sigue siendo utilizable) pero se advierte.
	for _, name := range concepts {
		data, err := os.ReadFile(filepath.Join(conceptsDir, name))
		if err != nil {
			continue // ya se advirtió arriba
		}
		for _, m := range imgRe.FindAllStringSubmatch(string(data), -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") ||
				strings.HasPrefix(ref, "data:") {
				continue
			}
			resolved := filepath.Clean(filepath.Join(conceptsDir, filepath.FromSlash(ref)))
			if info, err := os.Stat(resolved); err != nil || info.IsDir() {
				warnings = append(warnings,
					fmt.Sprintf("el concepto %q referencia un recurso inexistente: %q", name, ref))
			}
		}
	}

	// Trazabilidad mínima en log.md.
	logData, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil {
		warnings = append(warnings, "no se pudo leer log.md")
	} else {
		for _, key := range []string{"SHA-256", "Formato", "Conceptos generados"} {
			if !strings.Contains(string(logData), key) {
				warnings = append(warnings, fmt.Sprintf("log.md sin el campo de trazabilidad %q", key))
			}
		}
	}

	if len(warnings) > 0 {
		return Report{Level: LevelValidWithWarnings, Warnings: warnings}
	}
	return Report{Level: LevelValid}
}
