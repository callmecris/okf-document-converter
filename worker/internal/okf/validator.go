package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var linkRe = regexp.MustCompile(`\]\(([^)]+)\)`)

// Validate verifica la integridad estructural de un bundle OKF:
//   - index.md y log.md existen
//   - la carpeta conceptos/ existe y no está vacía
//   - todos los links relativos de index.md resuelven a archivos reales
func Validate(root string) error {
	for _, required := range []string{"index.md", "log.md"} {
		if info, err := os.Stat(filepath.Join(root, required)); err != nil || info.IsDir() {
			return fmt.Errorf("bundle incompleto: falta %s", required)
		}
	}

	conceptsDir := filepath.Join(root, "conceptos")
	entries, err := os.ReadDir(conceptsDir)
	if err != nil {
		return fmt.Errorf("bundle incompleto: falta conceptos/: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("bundle inválido: conceptos/ vacío")
	}

	index, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		return fmt.Errorf("read index.md: %w", err)
	}

	for _, link := range linkRe.FindAllStringSubmatch(string(index), -1) {
		target := link[1]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		resolved := filepath.Clean(filepath.Join(root, target))
		if info, err := os.Stat(resolved); err != nil || info.IsDir() {
			return fmt.Errorf("link roto en index.md: %q", target)
		}
	}

	return nil
}