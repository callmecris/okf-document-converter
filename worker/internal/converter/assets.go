package converter

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// assetsDirName es el subdirectorio del bundle donde viven los recursos.
const assetsDirName = "assets"

// mdImageRe captura las imágenes markdown: ![alt](ruta "titulo opcional").
// La ruta no puede contener espacios sin escapar ni paréntesis.
var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)([^)]*)\)`)

// assetCollector copia recursos referenciados a assets/ y reescribe los
// enlaces del markdown para que apunten allí con una ruta relativa estable.
//
// Los conceptos viven en conceptos/, así que la referencia desde un concepto
// es "../assets/<archivo>".
type assetCollector struct {
	workDir string
	// seen evita copiar dos veces el mismo recurso: hash del contenido -> nombre
	// final. Así un logo repetido en muchas páginas se guarda una sola vez,
	// venga de la ruta que venga.
	seen map[string]string
	// used registra los nombres ya asignados para no colisionar.
	used map[string]bool
}

func newAssetCollector(workDir string) *assetCollector {
	return &assetCollector{
		workDir: workDir,
		seen:    map[string]string{},
		used:    map[string]bool{},
	}
}

// dir devuelve el directorio de assets dentro del workDir.
func (a *assetCollector) dir() string { return filepath.Join(a.workDir, assetsDirName) }

// Count devuelve cuántos recursos distintos se extrajeron.
func (a *assetCollector) Count() int { return len(a.seen) }

// add copia el recurso indicado a assets/ y devuelve su nombre final dentro
// del directorio. Recursos con contenido idéntico se copian una sola vez.
func (a *assetCollector) add(srcPath string, data []byte) (string, error) {
	sum := sha1.Sum(data)
	key := hex.EncodeToString(sum[:])
	if name, ok := a.seen[key]; ok {
		return name, nil
	}
	if err := os.MkdirAll(a.dir(), 0o755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}

	name := a.uniqueName(srcPath)
	if err := os.WriteFile(filepath.Join(a.dir(), name), data, 0o644); err != nil {
		return "", fmt.Errorf("write asset %s: %w", name, err)
	}
	a.seen[key] = name
	return name, nil
}

// addFile copia un recurso ya existente en disco.
func (a *assetCollector) addFile(srcPath string) (string, error) {
	if name, ok := a.seen[srcPath]; ok {
		return name, nil
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("read asset %s: %w", srcPath, err)
	}
	return a.add(srcPath, data)
}

// uniqueName construye un nombre de archivo seguro y sin colisiones,
// preservando la extensión original para que el visor elija bien el tipo.
func (a *assetCollector) uniqueName(srcPath string) string {
	base := sanitizeAssetName(filepath.Base(srcPath))
	if base == "" {
		base = "asset"
	}
	if !a.used[base] {
		a.used[base] = true
		return base
	}
	// Colisión de nombres entre recursos de distintos directorios: se
	// desambigua con un hash corto de la ruta de origen.
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	sum := sha1.Sum([]byte(srcPath))
	name := fmt.Sprintf("%s-%s%s", stem, hex.EncodeToString(sum[:])[:8], ext)
	a.used[name] = true
	return name
}

// sanitizeAssetName deja solo caracteres seguros para una ruta de bundle.
func sanitizeAssetName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// rewriteMarkdownImages reescribe las rutas de imagen de un markdown usando
// resolve, que recibe la ruta original y devuelve el nombre del asset (o ""
// si no debe reescribirse, por ejemplo una URL remota).
func rewriteMarkdownImages(markdown string, resolve func(src string) string) string {
	return mdImageRe.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := mdImageRe.FindStringSubmatch(match)
		alt, src, rest := parts[1], parts[2], parts[3]

		name := resolve(src)
		if name == "" {
			return match
		}
		// Desde conceptos/<fragmento>.md hay que subir un nivel.
		return fmt.Sprintf("![%s](../%s/%s%s)", alt, assetsDirName, name, rest)
	})
}

// isRemoteRef indica si una referencia apunta fuera del documento y por tanto
// no debe extraerse como asset.
func isRemoteRef(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "//")
}
